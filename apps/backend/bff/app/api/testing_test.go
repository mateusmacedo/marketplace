package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mateusmacedo/dmpf/libs/backend/go/authn"
	obsclock "github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/logging"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/metrics"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/otelboot"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/admission"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/deadline"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app/api"
)

var routeBudget = deadline.Budget{Dependency: "edge", Method: "route", Limit: 2 * time.Second, Slack: 200 * time.Millisecond, EstimatedDuration: 100 * time.Millisecond}

type fixture struct {
	handler     http.Handler
	admin       http.Handler
	spans       *tracetest.InMemoryExporter
	logs        *bytes.Buffer
	records     *logRecorder
	logProvider *sdklog.LoggerProvider
	metrics     *sdkmetric.ManualReader
}

type logRecorder struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (r *logRecorder) Export(_ context.Context, records []sdklog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, record := range records {
		r.records = append(r.records, record.Clone())
	}
	return nil
}

func (*logRecorder) Shutdown(context.Context) error { return nil }

func (*logRecorder) ForceFlush(context.Context) error { return nil }

func (r *logRecorder) all() []sdklog.Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.records)
}

type setup struct {
	budget        deadline.Budget
	limit         admission.Limit
	cors          []string
	ready         func(context.Context) error
	authenticator ports.Authenticator
	draining      func() bool
	clock         obsclock.Clock
}

type option func(*setup)

func withLimit(l admission.Limit) option { return func(s *setup) { s.limit = l } }

func withCORS(origins ...string) option { return func(s *setup) { s.cors = origins } }

func withReady(ready func(context.Context) error) option { return func(s *setup) { s.ready = ready } }

func withAuthenticator(a ports.Authenticator) option { return func(s *setup) { s.authenticator = a } }

func withDraining(draining func() bool) option { return func(s *setup) { s.draining = draining } }

func withClock(c obsclock.Clock) option { return func(s *setup) { s.clock = c } }

func newFixture(t *testing.T, options ...option) fixture {
	t.Helper()
	cfg := &setup{budget: routeBudget, limit: admission.Limit{PerSecond: 1000, Burst: 1000, Concurrency: 100}, authenticator: authn.DevAuthenticator{}}
	for _, apply := range options {
		apply(cfg)
	}

	logs := &bytes.Buffer{}
	spans := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(otelboot.NewPrivacyExporter(spans)))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	records := &logRecorder{}
	logProvider := otelboot.NewLoggerProvider(otelboot.Config{}, records)
	t.Cleanup(func() { _ = logProvider.Shutdown(context.Background()) })
	reader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = meterProvider.Shutdown(context.Background()) })
	logger := slog.New(slog.NewMultiHandler(
		slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}),
		logging.NewLogger(logProvider, "github.com/mateusmacedo/marketplace/apps/backend/bff/app/api").Handler(),
	))

	tenants, err := metrics.DeclareTenants(testTenant)
	if err != nil {
		t.Fatalf("DeclareTenants() = %v", err)
	}
	ctrl, err := admission.New(admission.Config{Limits: api.Limits(cfg.limit), Tenants: tenants, MaxKeys: 32, Clock: obsclock.System()})
	if err != nil {
		t.Fatalf("admission.New() = %v", err)
	}

	handlerOptions := api.Options{
		Budget:         cfg.budget,
		Authenticator:  cfg.authenticator,
		CORSOrigins:    cfg.cors,
		Logger:         logger,
		TracerProvider: provider,
		MeterProvider:  meterProvider,
		Ready:          cfg.ready,
		Draining:       cfg.draining,
		Clock:          cfg.clock,
	}
	handler, err := api.NewHandler(ctrl, nil, handlerOptions)
	if err != nil {
		t.Fatalf("NewHandler() = %v", err)
	}
	return fixture{handler: handler, admin: api.NewAdminHandler(handlerOptions), spans: spans, logs: logs, records: records, logProvider: logProvider, metrics: reader}
}

// testCredential is what the development authenticator reads back as identity.
// The edge demands a subject on every route, so a request without it is denied
// before reaching a handler — which is what the 401 cases assert by passing an
// empty Authorization explicitly.
const testCredential = `Bearer {"sub":"tester","tenant":"acme","permissions":["profile"]}`

// testTenant is the tenant testCredential resolves, declared so its bucket and
// label are its own (MET-07).
const testTenant = "acme"

func (f fixture) do(t *testing.T, method, path string, body io.Reader, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	return serve(f.handler, method, path, body, headers...)
}

func (f fixture) doAdmin(t *testing.T, method, path string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	return serve(f.admin, method, path, nil, headers...)
}

func (f fixture) doOn(t *testing.T, admin bool, method, path string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	if admin {
		return f.doAdmin(t, method, path, headers...)
	}
	return f.do(t, method, path, nil, headers...)
}

func serve(handler http.Handler, method, path string, body io.Reader, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	if _, declared := req.Header["Authorization"]; !declared {
		req.Header.Set("Authorization", testCredential)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, into any) {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json (body %s)", ct, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
		t.Fatalf("body %s is not the expected JSON: %v", rec.Body.String(), err)
	}
}

type rejection struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func requireRejection(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, status, rec.Body.String())
	}
	var body rejection
	decode(t, rec, &body)
	if body.Code != code || body.Message == "" {
		t.Fatalf("rejection = %+v, want code %q with a message", body, code)
	}
}
