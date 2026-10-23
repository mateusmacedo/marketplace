package app_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/otelboot"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/tracing"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/obstest"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/semconv/v1.43.0/httpconv"
	"go.opentelemetry.io/otel/trace"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app"
)

// unreachableDenyList is a deny-list nothing listens on: the cases that need
// none still start, and a token they would accept is refused (fail closed).
const unreachableDenyList = "redis://revocation:secret@127.0.0.1:1/0"

type edge struct {
	addr     string
	admin    string
	logs     *obstest.MemoryLogs
	provider *sdklog.LoggerProvider
	spans    *tracetest.InMemoryExporter
	metrics  *sdkmetric.ManualReader
	runtime  *otelboot.Runtime
	cancel   context.CancelFunc
	done     chan error
}

func startEdge(t *testing.T, env ...string) edge {
	t.Helper()
	base := []string{"REDIS_URL", unreachableDenyList, "HTTP_ADDR", "127.0.0.1:0", "ADMIN_ADDR", "127.0.0.1:0",
		"TENANCY_GRPC_TARGET", serveTenancy(t), "GRPC_INSECURE", "true"}
	cfg, err := app.FromEnv(lookup(append(base, env...)...))
	if err != nil {
		t.Fatalf("FromEnv() = %v", err)
	}
	telemetry := app.TelemetryOf(cfg)
	e := edge{logs: &obstest.MemoryLogs{}, spans: tracetest.NewInMemoryExporter(), metrics: sdkmetric.NewManualReader(), done: make(chan error, 1)}
	config := otelboot.Config{
		Propagator: propagation.TraceContext{},
		Resource: otelboot.Resource{ServiceName: telemetry.Service, ServiceVersion: "test",
			ServiceInstanceID: "bff-1", Role: telemetry.Role},
		TraceExporter: e.spans,
		MetricReader:  e.metrics,
	}
	e.provider = otelboot.NewLoggerProvider(config, e.logs)
	config.LoggerProvider = e.provider
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	if e.runtime, err = otelboot.Start(ctx, config); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	t.Cleanup(func() {
		cancel()
		_ = e.runtime.Shutdown(context.Background())
	})

	go func() { e.done <- app.RunWith(ctx, cfg, e.runtime) }()
	e.addr = listeningAddr(t, e, "http listening")
	e.admin = listeningAddr(t, e, "admin listening")
	return e
}

func (e edge) stop(t *testing.T) {
	t.Helper()
	e.cancel()
	if err := <-e.done; err != nil {
		t.Fatalf("RunWith() = %v, want nil on shutdown", err)
	}
	if err := e.provider.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush() = %v", err)
	}
}

func (e edge) records(t *testing.T) []sdklog.Record {
	t.Helper()
	if err := e.provider.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush() = %v", err)
	}
	return e.logs.Records()
}

func (e edge) request(t *testing.T, method, path string, headers ...string) int {
	t.Helper()
	status, _ := e.call(t, method, path, headers...)
	return status
}

// call answers the status and the body of a request to the public port.
func (e edge) call(t *testing.T, method, path string, headers ...string) (int, []byte) {
	t.Helper()
	return callAt(t, e.addr, method, path, headers...)
}

func (e edge) probe(t *testing.T, method, path string) int {
	t.Helper()
	return requestAt(t, e.admin, method, path)
}

func requestAt(t *testing.T, addr, method, path string, headers ...string) int {
	t.Helper()
	status, _ := callAt(t, addr, method, path, headers...)
	return status
}

func callAt(t *testing.T, addr, method, path string, headers ...string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, "http://"+addr+path, nil)
	if err != nil {
		t.Fatalf("NewRequest() = %v", err)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		if http.CanonicalHeaderKey(headers[i]) == "Host" {
			req.Host = headers[i+1]
			continue
		}
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s = %v", method, path, err)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("%s %s body = %v", method, path, err)
	}
	return res.StatusCode, body
}

func attributesOf(record sdklog.Record) map[string]attribute.Value {
	attributes := map[string]attribute.Value{}
	record.WalkAttributes(func(kv attribute.KeyValue) bool {
		attributes[string(kv.Key)] = kv.Value
		return true
	})
	return attributes
}

func recordNamed(t *testing.T, records []sdklog.Record, body string) sdklog.Record {
	t.Helper()
	var bodies []string
	for _, record := range records {
		if record.Body().AsString() == body {
			return record
		}
		bodies = append(bodies, record.Body().AsString())
	}
	t.Fatalf("no %q record in %v", body, bodies)
	return sdklog.Record{}
}

func requireNoExecution(t *testing.T, body string, attributes map[string]attribute.Value) {
	t.Helper()
	for _, key := range []string{tracing.KeyCorrelationID, tracing.KeyRequestID, tracing.KeyTenantID} {
		if value, present := attributes[key]; present {
			t.Errorf("%q carries %s = %s, want no key of an execution on a start or shutdown record (RF-A4)", body, key, value.String())
		}
	}
}

func listeningAddr(t *testing.T, e edge, body string) string {
	t.Helper()
	var bodies []string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		bodies = bodies[:0]
		for _, record := range e.records(t) {
			bodies = append(bodies, record.Body().AsString())
			if record.Body().AsString() != body {
				continue
			}
			if scope := record.InstrumentationScope().Name; scope != reflect.TypeFor[app.Config]().PkgPath() {
				t.Fatalf("readiness line scope = %q, want the import path of the app package that emits it (RF-A1)", scope)
			}
			attributes := attributesOf(record)
			host, port := attributes[string(semconv.ServerAddressKey)], attributes[string(semconv.ServerPortKey)]
			if host.AsString() == "" || port.Type() != attribute.INT64 {
				t.Fatalf("readiness line = %v, want server.address and server.port surviving the processor (RF-A3)", attributes)
			}
			requireNoExecution(t, body, attributes)
			return net.JoinHostPort(host.AsString(), strconv.FormatInt(port.AsInt64(), 10))
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no %q line in %v", body, bodies)
	return ""
}

func TestTheReadinessLineCarriesThePortTheKernelChose(t *testing.T) {
	e := startEdge(t, devMock, "true")

	if _, port, err := net.SplitHostPort(e.addr); err != nil || port == "0" || port == "" {
		t.Fatalf("addr = %q, want the address the listener resolved, not the configured one", e.addr)
	}
	conn, err := net.DialTimeout("tcp", e.addr, time.Second)
	if err != nil {
		t.Fatalf("dial %s = %v, want the edge accepting on the address it logged", e.addr, err)
	}
	_ = conn.Close()
	e.stop(t)
}

func TestTheShutdownLineCarriesTheDrainDelayUnderAPlatformKey(t *testing.T) {
	e := startEdge(t, devMock, "true", "DRAIN_DELAY", "10ms")

	e.stop(t)

	attributes := attributesOf(recordNamed(t, e.records(t), "http draining"))
	if got := attributes["dmpf.drain.delay"].AsString(); got != "10ms" {
		t.Fatalf("\"http draining\" = %v, want dmpf.drain.delay=10ms surviving the processor (RF-A3)", attributes)
	}
	requireNoExecution(t, "http draining", attributes)
}

func TestTheAdministrationPortServesLivenessAndReadinessWithoutCredential(t *testing.T) {
	e := startEdge(t, devMock, "true")

	if status := e.probe(t, http.MethodGet, "/livez"); status != http.StatusNoContent {
		t.Fatalf("GET /livez = %d, want 204 while the process serves", status)
	}
	if status := e.probe(t, http.MethodGet, "/readyz"); status != http.StatusNoContent {
		t.Fatalf("GET /readyz = %d, want 204 while the process and its context serve", status)
	}
	e.stop(t)
}

func TestTheAdministrationPortRefusesReadinessWhileThePublicOneDrains(t *testing.T) {
	e := startEdge(t, devMock, "true", "DRAIN_DELAY", "1s")
	readiness := func(want int) bool {
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			if e.probe(t, http.MethodGet, "/readyz") == want {
				return true
			}
		}
		return false
	}

	if !readiness(http.StatusNoContent) {
		t.Fatal("/readyz on the administration port never answered 204 while serving")
	}
	e.cancel()
	if !readiness(http.StatusServiceUnavailable) {
		t.Fatal("/readyz on the administration port never answered 503 during the drain")
	}
	if status := e.request(t, http.MethodGet, "/v1/me"); status != http.StatusUnauthorized {
		t.Fatalf("public GET /v1/me during the drain = %d, want it still served (401)", status)
	}
	e.stop(t)
}

// The request is held inside verification: the issuer's JWKS answers only on
// release, which is the one point a request of this edge waits on I/O before
// the deny-list.
func TestTheAdministrationPortClosesOnlyAfterThePublicOneHasDrained(t *testing.T) {
	arrived, release := make(chan struct{}, 1), make(chan struct{})
	issuer := newSigningIssuer(t, func() {
		select {
		case arrived <- struct{}{}:
		default:
		}
		<-release
	})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	e := startEdge(t, issuer.env()...)
	bearer := issuer.token(t, member("alice", "sid-1", "acme", nil))
	answered := make(chan error, 1)
	go func() {
		req, err := http.NewRequest(http.MethodGet, "http://"+e.addr+"/v1/me", nil)
		if err == nil {
			req.Header.Set("Authorization", bearer)
			var res *http.Response
			if res, err = http.DefaultClient.Do(req); err == nil {
				_ = res.Body.Close()
			}
		}
		answered <- err
	}()
	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("the public GET /v1/me never reached the verification of its token")
	}

	e.cancel()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		conn, err := net.DialTimeout("tcp", e.addr, time.Second)
		if err != nil {
			break
		}
		_ = conn.Close()
		if time.Now().After(deadline) {
			t.Fatal("the public port still accepts connections 5s after the shutdown began")
		}
	}
	res, err := http.Get("http://" + e.admin + "/readyz")
	if err != nil {
		t.Fatalf("/readyz on the administration port while the public one drains its request in flight = %v, want 503: the administration port closes only after the public one", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("/readyz on the administration port while the public one drains = %d, want 503", res.StatusCode)
	}
	close(release)
	if err := <-answered; err != nil {
		t.Fatalf("public GET /v1/me in flight when the shutdown began = %v, want an answer: the public port drains it before closing", err)
	}
	e.stop(t)
}

func TestARefusedTokenReachesTheLoggerProviderOfTheRuntime(t *testing.T) {
	issuer := newSigningIssuer(t, nil)
	e := startEdge(t, issuer.env()...)

	if status := e.request(t, http.MethodGet, "/v1/me", "Authorization", "Bearer not-a-jwt"); status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", status)
	}
	e.stop(t)

	record := recordNamed(t, e.records(t), "auth: token rejected")
	if scope := record.InstrumentationScope().Name; scope != "github.com/mateusmacedo/dmpf/libs/backend/go/authn" {
		t.Fatalf("refusal scope = %q, want the import path of authn (RF-A1)", scope)
	}
	attributes := attributesOf(record)
	if got := attributes[string(semconv.ErrorTypeKey)].AsString(); got != "Unauthenticated" {
		t.Fatalf("refusal error.type = %q, want the FND-07 category Unauthenticated (RF-B1)", got)
	}
	if got := attributes["dmpf.auth.refusal_reason"].AsString(); got != "malformed" {
		t.Fatalf("refusal dmpf.auth.refusal_reason = %q, want malformed (RF-A6)", got)
	}
}

// Fail closed: a valid token is never accepted without the deny-list answering.
func TestAValidTokenIsRefusedWhileTheDenyListIsUnreachable(t *testing.T) {
	issuer := newSigningIssuer(t, nil)
	e := startEdge(t, issuer.env()...)

	status, body := e.call(t, http.MethodGet, "/v1/me", "Authorization", issuer.token(t, member("alice", "sid-1", "acme", nil)))

	if status != http.StatusUnauthorized {
		t.Fatalf("GET /v1/me with the deny-list unreachable = %d %s, want 401", status, body)
	}
	e.stop(t)

	unavailable := recordNamed(t, e.records(t), "auth: revocation store unavailable")
	if scope := unavailable.InstrumentationScope().Name; scope != "github.com/mateusmacedo/dmpf/libs/backend/go/authn" {
		t.Fatalf("refusal scope = %q, want the 401 to come from the deny-list the authn could not consult", scope)
	}
}

func TestEveryRequestOfTheEdgeIsOneServerSpanAndOnePointOfItsDuration(t *testing.T) {
	e := startEdge(t, devMock, "true", "CORS_ORIGINS", "http://app.example")
	requests := []struct {
		method, path string
		headers      []string
		status       int
		span         string
		admin        bool
	}{
		{http.MethodGet, "/v1/me", nil, http.StatusUnauthorized, "GET /v1/me", false},
		{http.MethodGet, "/livez", nil, http.StatusNotFound, "GET", false},
		{http.MethodGet, "/readyz", nil, http.StatusNotFound, "GET", false},
		{http.MethodGet, "/livez", nil, http.StatusNoContent, "", true},
		{http.MethodGet, "/readyz", nil, http.StatusNoContent, "", true},
		{http.MethodGet, "/nowhere", nil, http.StatusNotFound, "GET", true},
		{http.MethodOptions, "/v1/me", []string{"Origin", "http://app.example", "Access-Control-Request-Method", "GET"}, http.StatusNoContent, "OPTIONS", false},
		{http.MethodGet, "/nowhere", nil, http.StatusNotFound, "GET", false},
		{http.MethodDelete, "/v1/me", nil, http.StatusMethodNotAllowed, "DELETE", false},
	}
	var want []string
	for _, r := range requests {
		addr := map[bool]string{false: e.addr, true: e.admin}[r.admin]
		if status := requestAt(t, addr, r.method, r.path, r.headers...); status != r.status {
			t.Fatalf("%s %s on %s = %d, want %d", r.method, r.path, addr, status, r.status)
		}
		if r.span != "" {
			want = append(want, r.span)
		}
	}
	if err := e.runtime.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush() = %v", err)
	}

	var got []string
	for _, span := range e.spans.GetSpans() {
		if span.SpanKind == trace.SpanKindServer {
			got = append(got, span.Name)
		}
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("SERVER spans = %v, want exactly one per request outside the health routes %v (RF-B2)", got, want)
	}
	if points := durationPoints(t, e.metrics); points != uint64(len(want)) {
		t.Fatalf("http.server.request.duration counted %d requests, want %d, none of the health routes (RF-B2, RF-D2)", points, len(want))
	}
	e.stop(t)
}

func TestAnEdgeSeriesDoesNotSplitByTheHostTheClientSends(t *testing.T) {
	e := startEdge(t, devMock, "true")
	for _, host := range []string{"a.example", "b.example", "c.example:1234", "c.example:4321"} {
		if status := e.request(t, http.MethodGet, "/v1/me", "Host", host); status != http.StatusUnauthorized {
			t.Fatalf("GET /v1/me with Host %s = %d, want 401", host, status)
		}
	}
	if err := e.runtime.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush() = %v", err)
	}

	for _, name := range []string{
		httpconv.ServerRequestDuration{}.Name(),
		httpconv.ServerRequestBodySize{}.Name(),
		httpconv.ServerResponseBodySize{}.Name(),
	} {
		series := routeSeries(t, e.metrics, name, "/v1/me")
		if len(series) != 1 || series[0].HasValue(semconv.ServerAddressKey) || series[0].HasValue(semconv.ServerPortKey) {
			encoded := make([]string, len(series))
			for i, set := range series {
				encoded[i] = set.Encoded(attribute.DefaultEncoder())
			}
			t.Errorf("%s for /v1/me = %q, want one series without server.address or server.port, "+
				"which otelhttp takes from the Host the client sends (RF-D3)", name, encoded)
		}
	}
	e.stop(t)
}

func routeSeries(t *testing.T, reader *sdkmetric.ManualReader, name, route string) []attribute.Set {
	t.Helper()
	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("Collect() = %v", err)
	}
	var sets []attribute.Set
	keep := func(set attribute.Set) {
		if value, ok := set.Value(semconv.HTTPRouteKey); ok && value.AsString() == route {
			sets = append(sets, set)
		}
	}
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != name {
				continue
			}
			switch data := m.Data.(type) {
			case metricdata.Histogram[float64]:
				for _, point := range data.DataPoints {
					keep(point.Attributes)
				}
			case metricdata.Histogram[int64]:
				for _, point := range data.DataPoints {
					keep(point.Attributes)
				}
			default:
				t.Fatalf("%s is %T, want a histogram", name, m.Data)
			}
		}
	}
	return sets
}

func durationPoints(t *testing.T, reader *sdkmetric.ManualReader) uint64 {
	t.Helper()
	var collected metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("Collect() = %v", err)
	}
	var count uint64
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			if histogram, ok := m.Data.(metricdata.Histogram[float64]); ok && m.Name == "http.server.request.duration" {
				for _, point := range histogram.DataPoints {
					count += point.Count
				}
			}
		}
	}
	return count
}
