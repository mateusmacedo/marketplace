// Package api is the public REST edge: each route is a kernel http.Route that
// references a published OpenAPI operation (RST-04) and runs only behind the
// identity the authn verified and the tenant it resolved.
package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	kernelhttp "github.com/mateusmacedo/dmpf/libs/backend/go/http"
	obsclock "github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/metrics"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/admission"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/deadline"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	IdempotencyHeader = "Idempotency-Key"
	CorrelationHeader = "X-Correlation-ID"

	// ProfilePermission is the standard OIDC scope a client asks for at sign-in:
	// reading one's own identity needs no business role, which the RBAC owns.
	ProfilePermission ports.Permission = "profile"

	contract = "apps/backend/bff/contract/openapi/v1/openapi.yaml#/paths/"
)

// Options is what the composition root decides beyond the routes: the route
// budget the request deadline derives from, the browser origins allowed to
// call the edge (none by default) and the readiness of the process (nil serves
// no readiness route).
type Options struct {
	Budget         deadline.Budget
	Authenticator  ports.Authenticator
	CORSOrigins    []string
	Logger         *slog.Logger
	Ready          func(context.Context) error
	Draining       func() bool
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	Clock          obsclock.Clock
}

type binding struct {
	route   kernelhttp.Route
	handler http.HandlerFunc
}

func bindings(budget deadline.Budget) []binding {
	return []binding{
		{route: route("me", http.MethodGet, "/v1/me", budget, ProfilePermission), handler: me},
	}
}

func route(name, method, path string, budget deadline.Budget, permission ports.Permission) kernelhttp.Route {
	return kernelhttp.Route{
		Name:        name,
		Method:      method,
		Path:        path,
		ContractRef: contract + strings.ReplaceAll(path, "/", "~1") + "/" + strings.ToLower(method),
		Budget:      budget,
		Requires:    kernelhttp.RequireSubjectAndTenant,
		Permission:  permission,
	}
}

func Routes(budget deadline.Budget) []kernelhttp.Route {
	bound := bindings(budget)
	routes := make([]kernelhttp.Route, len(bound))
	for i, b := range bound {
		routes[i] = b.route
	}
	return routes
}

// ValidateRoutes refuses the first route an inbound edge cannot serve, such as
// one that demands a subject without declaring the permission it needs: the
// process refuses to start instead of denying every call at the first request.
func ValidateRoutes(routes []kernelhttp.Route) error {
	for _, route := range routes {
		if err := route.ValidateEdge(); err != nil {
			return err
		}
	}
	return nil
}

func Limits(limit admission.Limit) map[string]admission.Limit {
	routes := Routes(deadline.Budget{})
	limits := make(map[string]admission.Limit, len(routes))
	for _, route := range routes {
		limits[pattern(route)] = limit
	}
	return limits
}

func pattern(route kernelhttp.Route) string { return route.Method + " " + route.Path }

// ErrAuthenticatorRequired refuses a handler that could not authenticate: every
// route here demands a subject, and a nil verifier would deny all of them at
// the first request instead of at the start.
var ErrAuthenticatorRequired = errors.New("api: no authenticator provided")

// WHY: the deadline wraps the execution context, not the reverse, because
// CTX-01 makes deadline mandatory and the context cannot resolve what the
// timeout has not yet set. Admission stays inside, still ahead of the body.
func NewHandler(ctrl *admission.Controller, instruments *metrics.Instruments, opts Options) (http.Handler, error) {
	if opts.Authenticator == nil {
		return nil, ErrAuthenticatorRequired
	}
	bound := bindings(opts.Budget)
	if err := ValidateRoutes(Routes(opts.Budget)); err != nil {
		return nil, err
	}

	logger := loggerOf(opts)
	admit := kernelhttp.Admission(ctrl, routeOf, tenantOf, instruments, refuseAsRejection)
	mux := http.NewServeMux()
	for _, b := range bound {
		handler := requireIdempotencyKey(b.route, b.handler)
		mounted := withExecutionContext(logger, opts.Authenticator, b.route, admit(handler))
		mux.Handle(pattern(b.route), withRecover(withRouteDeadline(b.route.Budget, mounted)))
	}
	return otelhttp.NewHandler(withRoute(withAccessLog(logger, withCORS(opts.CORSOrigins, mux))), "bff", instrumentation(opts)...), nil
}

func instrumentation(opts Options) []otelhttp.Option {
	return []otelhttp.Option{
		otelhttp.WithTracerProvider(opts.TracerProvider),
		otelhttp.WithMeterProvider(opts.MeterProvider),
		otelhttp.WithPropagators(traceparentOnly{}),
	}
}

func loggerOf(opts Options) *slog.Logger {
	if opts.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return opts.Logger
}

func clockOf(opts Options) obsclock.Clock {
	if opts.Clock == nil {
		return obsclock.System()
	}
	return opts.Clock
}

func withRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if _, route, routed := strings.Cut(r.Pattern, " "); routed {
			trace.SpanFromContext(r.Context()).SetAttributes(semconv.HTTPRoute(route))
		}
	})
}

func routeOf(r *http.Request) string { return r.Pattern }

// tenantOf keys the admission bucket by the tenant the edge authenticated
// (RES-16); admission runs inside withExecutionContext, so the context is there.
func tenantOf(r *http.Request) string {
	execution, ok := ports.ExecutionContextFrom(r.Context())
	if !ok {
		return ""
	}
	tenant, _ := execution.Tenant()
	return string(tenant)
}
