package rpc_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	obsclock "github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/retry"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/deadline"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app/rpc"
	tenancyv1 "github.com/mateusmacedo/marketplace/apps/backend/tenancy/contract/gen/go/company/tenancy/service/v1"
)

type harness struct {
	fake    *fakeContexts
	tenancy rpc.Tenancy
	spans   *tracetest.InMemoryExporter
	tracer  trace.Tracer
}

func newHarness(t *testing.T, fake *fakeContexts) harness {
	t.Helper()
	spans := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spans))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	opts := rpc.Options{Insecure: true, Clock: obsclock.System(), Tracer: provider.Tracer("rpc_test"), TracerProvider: provider, Propagator: propagation.TraceContext{}}
	conn, err := rpc.Dial("passthrough:///tenancy", rpc.TenancyConfig(opts), grpc.WithContextDialer(fake.serveBuffered(t)))
	if err != nil {
		t.Fatalf("Dial(tenancy) = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return harness{fake: fake, tenancy: rpc.NewTenancy(conn), spans: spans, tracer: provider.Tracer("edge")}
}

func withDeadline(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}

func TestMethodNamesComeFromTheGeneratedDescriptor(t *testing.T) {
	want := map[string]string{
		rpc.MethodRegisterStore:      "/company.tenancy.service.v1.TenancyService/RegisterStore",
		rpc.MethodSuspendStore:       "/company.tenancy.service.v1.TenancyService/SuspendStore",
		rpc.MethodReactivateStore:    "/company.tenancy.service.v1.TenancyService/ReactivateStore",
		rpc.MethodFindStore:          "/company.tenancy.service.v1.TenancyService/FindStore",
		rpc.MethodFindStoresByStatus: "/company.tenancy.service.v1.TenancyService/FindStoresByStatus",
	}
	for got, expected := range want {
		if got != expected {
			t.Fatalf("method name = %q, want %q", got, expected)
		}
	}
}

func TestAMethodTheDescriptorDoesNotDeclareRefusesTheStart(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("FullMethod() of an undeclared method did not panic")
		}
	}()
	rpc.FullMethod(tenancyService, "Absent")
}

func TestEveryMethodIsIdempotentAndRetriesOnlyAnUnavailableContext(t *testing.T) {
	cfg := rpc.TenancyConfig(rpc.Options{Insecure: true, Clock: obsclock.System()})

	if len(cfg.Methods) != 5 {
		t.Fatalf("declared %d methods, want 5", len(cfg.Methods))
	}
	for method, policy := range cfg.Methods {
		if !policy.Idempotent {
			t.Fatalf("%s: Idempotent = false; a command carries its key, a read has no effect (GRP-09, IDM-01)", method)
		}
		if len(policy.RetryableCodes) != 1 || policy.RetryableCodes[0] != codes.Unavailable {
			t.Fatalf("%s: RetryableCodes = %v, want [Unavailable] (GRP-08)", method, policy.RetryableCodes)
		}
		if policy.Budget.Limit >= 2*time.Second || policy.Budget.Slack <= 0 {
			t.Fatalf("%s: budget %+v must stay below the route budget with a slack (GRP-17)", method, policy.Budget)
		}
	}
	if cfg.HealthServiceName != rpc.TenancyServiceName || !cfg.InsecureForDevelopmentOnly {
		t.Fatalf("config = %+v, want the health service and the transport the options declared", cfg)
	}
}

func TestFindStoreRetriesAnUnavailableContext(t *testing.T) {
	fake := &fakeContexts{findStore: func(n int) (*tenancyv1.FindStoreResponse, error) {
		if n == 1 {
			return nil, status.Error(codes.Unavailable, "restarting")
		}
		return &tenancyv1.FindStoreResponse{Store: &tenancyv1.Store{Ref: "loja-1"}}, nil
	}}
	h := newHarness(t, fake)

	resp, err := h.tenancy.FindStore(retry.WithBudget(withDeadline(t, 2*time.Second)), &tenancyv1.FindStoreRequest{Ref: "loja-1"})

	if err != nil || resp.GetStore().GetRef() != "loja-1" {
		t.Fatalf("FindStore() = %v, %v; want the store after one retry", resp, err)
	}
	if got := len(fake.callsTo("FindStore")); got != 2 {
		t.Fatalf("FindStore reached the context %d times, want 2", got)
	}
}

func TestRegisterStoreRetriesAnUnavailableContextUnderTheSameKey(t *testing.T) {
	fake := &fakeContexts{registerStore: func(n int) (*tenancyv1.RegisterStoreResponse, error) {
		if n == 1 {
			return nil, status.Error(codes.Unavailable, "restarting")
		}
		return &tenancyv1.RegisterStoreResponse{}, nil
	}}
	h := newHarness(t, fake)
	ctx := rpc.WithCall(retry.WithBudget(withDeadline(t, 2*time.Second)), rpc.Call{IdempotencyKey: "k-derived"})

	if _, err := h.tenancy.RegisterStore(ctx, &tenancyv1.RegisterStoreRequest{Ref: "loja-1", Name: "Loja 1"}); err != nil {
		t.Fatalf("RegisterStore() = %v, want success after one retry", err)
	}

	calls := fake.callsTo("RegisterStore")
	if len(calls) != 2 {
		t.Fatalf("RegisterStore reached the context %d times, want 2", len(calls))
	}
	for i, call := range calls {
		if got := call.md.Get(kernelgrpc.IdempotencyKey); len(got) != 1 || got[0] != "k-derived" {
			t.Fatalf("attempt %d carried idempotency-key %v, want the same key on every attempt", i+1, got)
		}
	}
}

func TestTheReplayHeaderOfTheContextReachesTheCaller(t *testing.T) {
	h := newHarness(t, &fakeContexts{replayRegister: true})

	replayed := rpc.WithReplaySlot(withDeadline(t, 2*time.Second))
	if _, err := h.tenancy.RegisterStore(replayed, &tenancyv1.RegisterStoreRequest{Ref: "loja-1", Name: "Loja 1"}); err != nil {
		t.Fatalf("RegisterStore() = %v", err)
	}
	fresh := rpc.WithReplaySlot(withDeadline(t, 2*time.Second))
	if _, err := h.tenancy.SuspendStore(fresh, &tenancyv1.SuspendStoreRequest{Ref: "loja-1", Reason: "fraude"}); err != nil {
		t.Fatalf("SuspendStore() = %v", err)
	}

	if !rpc.Replayed(replayed) {
		t.Fatal("Replayed() = false after the context answered idempotent-replayed: true")
	}
	if rpc.Replayed(fresh) {
		t.Fatal("Replayed() = true for a first answer")
	}
}

func TestMetadataCarriesTheCallAndTheClientSpan(t *testing.T) {
	fake := &fakeContexts{}
	h := newHarness(t, fake)
	ctx, root := h.tracer.Start(withDeadline(t, 2*time.Second), "HTTP GET /tenancy/store/{ref}", trace.WithSpanKind(trace.SpanKindServer))
	ctx = rpc.WithCall(ctx, rpc.Call{CorrelationID: "corr-1", RequestID: "req-1", IdempotencyKey: "k-1", TenantID: "acme", Locale: "pt-BR"})

	if _, err := h.tenancy.FindStore(ctx, &tenancyv1.FindStoreRequest{Ref: "loja-1"}); err != nil {
		t.Fatalf("FindStore() = %v", err)
	}
	root.End()

	calls := fake.callsTo("FindStore")
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	md := calls[0].md
	for key, want := range map[string]string{
		kernelgrpc.CorrelationKey: "corr-1", kernelgrpc.CausationKey: "req-1", kernelgrpc.IdempotencyKey: "k-1",
		kernelgrpc.TenantKey: "acme", kernelgrpc.LocaleKey: "pt-BR",
	} {
		if got := md.Get(key); len(got) != 1 || got[0] != want {
			t.Fatalf("metadata %s = %v, want %q", key, got, want)
		}
	}
	traceparent := md.Get("traceparent")
	if len(traceparent) != 1 {
		t.Fatalf("metadata traceparent = %v, want one W3C value", traceparent)
	}
	parts := strings.Split(traceparent[0], "-")
	if len(parts) != 4 || parts[1] != root.SpanContext().TraceID().String() {
		t.Fatalf("traceparent %q does not continue the edge trace %s", traceparent[0], root.SpanContext().TraceID())
	}
	byID := map[string]tracetest.SpanStub{}
	for _, s := range h.spans.GetSpans() {
		byID[s.SpanContext.SpanID().String()] = s
	}
	sent, ok := byID[parts[2]]
	if !ok {
		t.Fatalf("traceparent span %s was not exported; spans: %v", parts[2], h.spans.GetSpans())
	}
	if sent.SpanContext.SpanID() == root.SpanContext().SpanID() || sent.SpanKind != trace.SpanKindClient || sent.Name != strings.TrimPrefix(rpc.MethodFindStore, "/") {
		t.Fatalf("traceparent names span %q, want a client span of the call, not the edge span", sent.Name)
	}
	for span := sent; span.Parent.SpanID() != root.SpanContext().SpanID(); {
		parent, known := byID[span.Parent.SpanID().String()]
		if !known {
			t.Fatalf("client span %q does not descend from the edge span", sent.Name)
		}
		span = parent
	}
}

func TestAnEmptyFieldOfTheCallIsNotSent(t *testing.T) {
	fake := &fakeContexts{}
	h := newHarness(t, fake)

	if _, err := h.tenancy.FindStore(rpc.WithCall(withDeadline(t, 2*time.Second), rpc.Call{CorrelationID: "corr-1"}), &tenancyv1.FindStoreRequest{Ref: "loja-1"}); err != nil {
		t.Fatalf("FindStore() = %v", err)
	}

	md := fake.callsTo("FindStore")[0].md
	for _, key := range []string{kernelgrpc.IdempotencyKey, kernelgrpc.TenantKey, kernelgrpc.LocaleKey} {
		if got := md.Get(key); len(got) != 0 {
			t.Fatalf("metadata %s = %v, want absent for an empty field", key, got)
		}
	}
}

func TestTheContextReceivesADeadlineBelowTheCaller(t *testing.T) {
	fake := &fakeContexts{}
	h := newHarness(t, fake)
	ctx := withDeadline(t, 2*time.Second)
	callerDeadline, _ := ctx.Deadline()

	if _, err := h.tenancy.FindStore(ctx, &tenancyv1.FindStoreRequest{Ref: "loja-1"}); err != nil {
		t.Fatalf("FindStore() = %v", err)
	}

	calls := fake.callsTo("FindStore")
	if len(calls) != 1 || !calls[0].hasDeadline {
		t.Fatalf("the context received no deadline (GRP-04): %+v", calls)
	}
	if gap := callerDeadline.Sub(calls[0].deadline); gap < 50*time.Millisecond {
		t.Fatalf("the context deadline is %v before the caller's, want the method slack kept (GRP-05, GRP-17)", gap)
	}
}

func TestACallWithoutDeadlineNeverReachesTheWire(t *testing.T) {
	fake := &fakeContexts{}
	h := newHarness(t, fake)

	_, err := h.tenancy.FindStore(context.Background(), &tenancyv1.FindStoreRequest{Ref: "loja-1"})

	if !errors.Is(err, deadline.ErrNoDeadline) {
		t.Fatalf("FindStore() = %v, want ErrNoDeadline (GRP-04)", err)
	}
	if got := len(fake.callsTo("FindStore")); got != 0 {
		t.Fatalf("calls = %d, want 0", got)
	}
}

func TestAnExhaustedDeadlineNeverReachesTheWire(t *testing.T) {
	fake := &fakeContexts{}
	h := newHarness(t, fake)

	_, err := h.tenancy.RegisterStore(withDeadline(t, 20*time.Millisecond), &tenancyv1.RegisterStoreRequest{Ref: "loja-1", Name: "Loja 1"})

	if !errors.Is(err, deadline.ErrDeadlineExhausted) {
		t.Fatalf("RegisterStore() = %v, want ErrDeadlineExhausted (GRP-17)", err)
	}
	if got := len(fake.callsTo("RegisterStore")); got != 0 {
		t.Fatalf("calls = %d, want 0", got)
	}
}
