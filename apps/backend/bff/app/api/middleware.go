package api

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/mateusmacedo/dmpf/libs/backend/go/authn"
	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	kernelhttp "github.com/mateusmacedo/dmpf/libs/backend/go/http"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/logging"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/retry"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/tracing"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/deadline"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

// withExecutionContext authenticates and mounts the nine-field context. It runs
// inside withRouteDeadline, never outside: deadline is mandatory in CTX-01, and
// mounting before the timeout existed would leave the field unresolvable.
func withExecutionContext(logger *slog.Logger, authenticator ports.Authenticator, route kernelhttp.Route, next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		markSelfLogged(rw)
		ctx := r.Context()
		span := trace.SpanFromContext(ctx)
		span.SetAttributes(allowedHeaders(r)...)

		correlation := r.Header.Get(CorrelationHeader)
		if !kernelgrpc.ValidCorrelation(correlation) {
			correlation = kernelgrpc.NewID("api")
		}
		requestID := kernelgrpc.NewID("api")

		w := newRecorder(rw)
		clientKey := r.Header.Get(idempotencyHeaderOf(route))
		defer func() { finishRequest(ctx, span, logger, route, r, w, clientKey) }()
		defer recordPanicStatus(w)

		w.Header().Set(CorrelationHeader, correlation)

		deadline, governed := r.Context().Deadline()
		if !governed {
			tracing.RecordError(span, categoryUnexpected)
			writeInternal(w, r)
			return
		}

		identity, status, code := kernelhttp.ResolveIdentity(ctx, authenticator, route, authn.CredentialFrom(r))
		if status == 0 {
			status, code = kernelhttp.RefuseAssertedIdentity(r, identity)
		}
		if status != 0 {
			writeRejection(r, w, status, code, rejectionMessage(status))
			return
		}

		execution, err := ports.NewExecutionContext(ports.ExecutionContextSpec{
			RequestID:     requestID,
			CorrelationID: correlation,
			TraceContext:  span.SpanContext().TraceID().String(),
			Subject:       identity.Subject,
			Tenant:        identity.Tenant,
			Permissions:   identity.Permissions,
			Deadline:      ports.Instant(deadline.UnixNano()),
			Locale:        localeOf(r),
		})
		if err != nil {
			tracing.RecordError(span, categoryUnexpected)
			writeInternal(w, r)
			return
		}
		span.SetAttributes(tracing.ExecutionAttributes(execution).KeyValues()...)

		ctx = tracing.WithExecutionBaggage(kernelhttp.WithExecutionContext(ctx, execution), execution)
		next.ServeHTTP(w, r.WithContext(kernelhttp.WithVerifiedPlatformReach(ctx, route)))
	})
}

func allowedHeaders(r *http.Request) []attribute.KeyValue {
	var kept []attribute.KeyValue
	for _, name := range []string{"content-type", "accept"} {
		if values := r.Header.Values(name); len(values) > 0 {
			kept = append(kept, semconv.HTTPRequestHeader(name, values...))
		}
	}
	return kept
}

type statusRecorder struct {
	http.ResponseWriter
	status     int
	selfLogged bool
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func markSelfLogged(w http.ResponseWriter) {
	if recorder, ok := w.(*statusRecorder); ok {
		recorder.selfLogged = true
	}
}

func recordPanicStatus(w *statusRecorder) {
	if recovered := recover(); recovered != nil {
		w.status = http.StatusInternalServerError
		panic(recovered)
	}
}

func finishRequest(ctx context.Context, span trace.Span, logger *slog.Logger, route kernelhttp.Route, r *http.Request, w *statusRecorder, clientKey string) {
	span.SetAttributes(tracing.Attributes{}.OutcomeCategory(string(outcomeOf(w.status))).KeyValues()...)

	level, enabled := accessLevel(ctx, logger, w.status)
	if !enabled {
		return
	}
	logRequest(ctx, r, logger, level, route.Path, w.status, idempotencyAttrs(clientKey)...)
}

func idempotencyAttrs(clientKey string) []slog.Attr {
	switch {
	case clientKey == "":
		return nil
	case ports.ValidIdempotencyKey(clientKey):
		return []slog.Attr{slog.String(tracing.KeyIdempotencyKey, clientKey)}
	default:
		return []slog.Attr{slog.Bool(tracing.KeyIdempotencyKeyInvalid, true)}
	}
}

func accessLevel(ctx context.Context, logger *slog.Logger, status int) (slog.Level, bool) {
	level := logging.Severity(logging.Server, outcomeOf(status))
	return level, logger.Enabled(ctx, level)
}

func logRequest(ctx context.Context, r *http.Request, logger *slog.Logger, level slog.Level, route string, status int, extra ...slog.Attr) {
	logger.LogAttrs(ctx, level, "http request", append(accessAttrs(r.Method, route, status, outcomeOf(status)), extra...)...)
}

func newRecorder(rw http.ResponseWriter) *statusRecorder {
	return &statusRecorder{ResponseWriter: rw, status: http.StatusOK}
}

func accessAttrs(method, route string, status int, outcome ports.OutcomeCategory) []slog.Attr {
	attrs := []slog.Attr{slog.String(string(semconv.HTTPRequestMethodKey), semconvMethod(method))}
	if route != "" {
		attrs = append(attrs, slog.String(string(semconv.HTTPRouteKey), route))
	}
	return append(attrs,
		slog.Int(string(semconv.HTTPResponseStatusCodeKey), status),
		slog.String(tracing.KeyOutcomeCategory, string(outcome)),
	)
}

func semconvMethod(method string) string {
	switch upper := strings.ToUpper(method); upper {
	case http.MethodConnect, http.MethodDelete, http.MethodGet, http.MethodHead, http.MethodOptions,
		http.MethodPatch, http.MethodPost, http.MethodPut, http.MethodTrace:
		return upper
	}
	return semconv.HTTPRequestMethodOther.Value.AsString()
}

func withAccessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w := newRecorder(rw)
		next.ServeHTTP(w, r)
		if w.selfLogged {
			return
		}
		level, enabled := accessLevel(r.Context(), logger, w.status)
		if !enabled {
			return
		}
		_, route, _ := strings.Cut(r.Pattern, " ")
		logRequest(r.Context(), r, logger, level, route, w.status)
	})
}

func outcomeOf(status int) ports.OutcomeCategory {
	switch {
	case status >= http.StatusInternalServerError:
		return ports.OutcomeFailed
	case status == http.StatusUnauthorized, status == http.StatusForbidden:
		return ports.OutcomeDenied
	case status >= http.StatusBadRequest:
		return ports.OutcomeRejected
	default:
		return ports.OutcomeAccepted
	}
}

func rejectionMessage(status int) string {
	if status == http.StatusForbidden {
		return "the resolved identity does not carry what this operation requires"
	}
	return "the request carries no verifiable credential"
}

// localeOf preserves the first language the caller declared and falls back to
// the edge default when none is declared or it is not a tag (CTX-11).
func localeOf(r *http.Request) string {
	first, _, _ := strings.Cut(r.Header.Get("Accept-Language"), ",")
	tag, _, _ := strings.Cut(first, ";")
	if tag = strings.TrimSpace(tag); kernelgrpc.ValidLocale(tag) {
		return tag
	}
	return kernelgrpc.DefaultLocale
}

// withRouteDeadline governs the time of the whole request at the edge (GRP-05)
// and grants the retry budget only the edge may mint (RES-24, RES-31).
func withRouteDeadline(budget deadline.Budget, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), budget.Limit)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(retry.WithBudget(ctx)))
	})
}

func idempotencyHeaderOf(route kernelhttp.Route) string {
	if route.IdempotencyKey != "" {
		return route.IdempotencyKey
	}
	return IdempotencyHeader
}

func requireIdempotencyKey(route kernelhttp.Route, next http.Handler) http.Handler {
	header := idempotencyHeaderOf(route)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get(header)
		if route.IdempotencyKey != "" && key == "" {
			writeRejection(r, w, http.StatusBadRequest, "missing-idempotency-key", route.Method+" requires the "+header+" header")
			return
		}
		if key != "" && !ports.ValidIdempotencyKey(key) {
			writeRejection(r, w, http.StatusBadRequest, "invalid-idempotency-key", header+" accepts up to 128 characters of [A-Za-z0-9._-]")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withCORS(origins []string, next http.Handler) http.Handler {
	if len(origins) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		// WHY: a shared cache that stores the answer given to an allowed origin
		// would serve it to a denied one; Vary has to be declared even when the
		// origin does not match, because that is the answer being cached.
		header.Add("Vary", "Origin")

		origin := r.Header.Get("Origin")
		if origin == "" || !slices.Contains(origins, origin) {
			next.ServeHTTP(w, r)
			return
		}
		header.Set("Access-Control-Allow-Origin", origin)
		header.Set("Access-Control-Expose-Headers", CorrelationHeader)
		if r.Method == http.MethodOptions {
			header.Set("Access-Control-Allow-Methods", strings.Join([]string{http.MethodGet, http.MethodPost, http.MethodOptions}, ", "))
			header.Set("Access-Control-Allow-Headers", strings.Join([]string{"Authorization", "Content-Type", IdempotencyHeader, CorrelationHeader}, ", "))
			header.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// WHY: without this a panic reaches net/http, which closes the connection with
// no HTTP answer at all and writes the stack to the default logger, bypassing
// the redacting handler this process installs.
func withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				tracing.RecordError(trace.SpanFromContext(r.Context()), categoryUnexpected)
				writeInternal(w, r)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
