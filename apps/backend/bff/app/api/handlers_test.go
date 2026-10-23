package api_test

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	kernelhttp "github.com/mateusmacedo/dmpf/libs/backend/go/http"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/deadline"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app/api"
	"github.com/mateusmacedo/marketplace/apps/backend/bff/app/rpc"
	tenancyv1 "github.com/mateusmacedo/marketplace/apps/backend/tenancy/contract/gen/go/company/tenancy/service/v1"
)

func TestRoutesDeclareTheMethodPermissionAndKeyOfEachOperation(t *testing.T) {
	want := []string{
		"me GET /v1/me profile -",
		"registerStore POST /tenancy/store tenancy:write Idempotency-Key",
		"findStoresByStatus GET /tenancy/store tenancy:read -",
		"findStore GET /tenancy/store/{ref} tenancy:read -",
		"suspendStore POST /tenancy/store/{ref}/suspend tenancy:write Idempotency-Key",
		"reactivateStore POST /tenancy/store/{ref}/reactivate tenancy:write Idempotency-Key",
	}
	var got []string
	for _, route := range api.Routes(routeBudget) {
		key := cmp.Or(route.IdempotencyKey, "-")
		got = append(got, strings.Join([]string{route.Name, route.Method, route.Path, string(route.Permission), key}, " "))
		if route.Requires != kernelhttp.RequireSubjectAndTenant || route.Budget != routeBudget {
			t.Errorf("%s: Requires = %v, Budget = %v, want subject and tenant over the route budget", route.Name, route.Requires, route.Budget)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Routes() =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestRoutesReferenceThePublishedContracts(t *testing.T) {
	routes := api.Routes(routeBudget)
	if len(routes) != 6 {
		t.Fatalf("Routes() has %d routes, want 6", len(routes))
	}
	for _, route := range routes {
		if err := route.Validate(); err != nil {
			t.Fatalf("%s: Validate() = %v", route.Name, err)
		}
		context := "bff"
		if strings.HasPrefix(route.Path, "/tenancy/") {
			context = "tenancy"
		}
		if !strings.HasPrefix(route.ContractRef, "apps/backend/"+context+"/contract/openapi/v1/openapi.yaml#/paths/") {
			t.Fatalf("%s: ContractRef = %q, want the %s contract (RST-04)", route.Name, route.ContractRef, context)
		}
		if route.Method == http.MethodPost && (route.IdempotencyKey != api.IdempotencyHeader || !route.Idempotent()) {
			t.Fatalf("%s: a POST must declare %s (RST-02)", route.Name, api.IdempotencyHeader)
		}
	}
	stripped := routes[1]
	stripped.ContractRef = ""
	if err := stripped.Validate(); !errors.Is(err, kernelhttp.ErrContractRequired) {
		t.Fatalf("Validate() without ContractRef = %v, want ErrContractRequired", err)
	}
}

func TestAPostWithoutIdempotencyKeyNeverCallsAContext(t *testing.T) {
	f := newFixture(t)

	rec := f.do(t, http.MethodPost, "/tenancy/store/loja-1/reactivate", nil)

	requireRejection(t, rec, http.StatusBadRequest, "missing-idempotency-key")
	if n := f.fake.total(); n != 0 {
		t.Fatalf("calls = %d, want 0", n)
	}
}

func TestContextStatusesMapToHTTP(t *testing.T) {
	cases := []struct {
		code   codes.Code
		status int
		want   string
	}{
		{codes.NotFound, http.StatusNotFound, "not-found"},
		{codes.Aborted, http.StatusConflict, "version-conflict"},
		{codes.PermissionDenied, http.StatusForbidden, "permission-denied"},
		{codes.InvalidArgument, http.StatusBadRequest, "invalid-request"},
		{codes.DeadlineExceeded, http.StatusGatewayTimeout, "deadline-exceeded"},
		{codes.Unavailable, http.StatusServiceUnavailable, "unavailable"},
		{codes.Internal, http.StatusInternalServerError, "internal-failure"},
	}
	for _, tc := range cases {
		t.Run(tc.code.String(), func(t *testing.T) {
			f := newFixture(t, withContexts((&fakeContexts{}).on("FindStore", func(int) (any, error) {
				return nil, status.Error(tc.code, "detail the edge must not echo")
			})))

			rec := f.do(t, http.MethodGet, "/tenancy/store/loja-1", nil)

			requireRejection(t, rec, tc.status, tc.want)
			if strings.Contains(rec.Body.String(), "must not echo") {
				t.Fatalf("body %s leaked the context detail", rec.Body.String())
			}
		})
	}
}

func TestAContextAnswerWithoutResultIsAnInternalFailure(t *testing.T) {
	f := newFixture(t, withContexts((&fakeContexts{}).
		on("RegisterStore", func(int) (any, error) { return &tenancyv1.RegisterStoreResponse{}, nil }).
		on("FindStore", func(int) (any, error) {
			return &tenancyv1.FindStoreResponse{Store: &tenancyv1.Store{Ref: "loja-1"}}, nil
		})))

	requireRejection(t, f.post(t, "/tenancy/store", `{"ref":"loja-1","name":"Loja 1"}`), http.StatusInternalServerError, "internal-failure")
	requireRejection(t, f.do(t, http.MethodGet, "/tenancy/store/loja-1", nil), http.StatusInternalServerError, "internal-failure")
}

func TestTheEdgeGrantsTheRetryBudgetOfARead(t *testing.T) {
	fake := (&fakeContexts{}).on("FindStore", func(n int) (any, error) {
		if n == 1 {
			return nil, status.Error(codes.Unavailable, "restarting")
		}
		return &tenancyv1.FindStoreResponse{Store: &tenancyv1.Store{Ref: "loja-1", Status: tenancyv1.StoreStatus_STORE_STATUS_ACTIVE}}, nil
	})
	f := newFixture(t, withContexts(fake))

	rec := f.do(t, http.MethodGet, "/tenancy/store/loja-1", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 after one retry (body %s)", rec.Code, rec.Body.String())
	}
	if n := len(fake.callsTo("FindStore")); n != 2 {
		t.Fatalf("FindStore reached the context %d times, want 2 (RES-31)", n)
	}
}

func TestAnUnavailableSuspendIsRetriedUnderTheSameKey(t *testing.T) {
	fake := (&fakeContexts{}).on("SuspendStore", func(n int) (any, error) {
		if n == 1 {
			return nil, status.Error(codes.Unavailable, "restarting")
		}
		return &tenancyv1.SuspendStoreResponse{Result: &tenancyv1.SuspendStoreResponse_Suspended{Suspended: &tenancyv1.Suspended{Ref: "loja-1"}}}, nil
	})
	f := newFixture(t, withContexts(fake))

	rec := f.post(t, "/tenancy/store/loja-1/suspend", `{"reason":"fraude"}`)

	if rec.Code >= http.StatusBadRequest {
		t.Fatalf("status = %d, want success after one retry (body %s)", rec.Code, rec.Body.String())
	}
	calls := fake.callsTo("SuspendStore")
	if len(calls) != 2 {
		t.Fatalf("SuspendStore reached the context %d times, want 2: a command with a key is retried (GRP-09, IDM-01)", len(calls))
	}
	first, second := calls[0].md.Get(kernelgrpc.IdempotencyKey), calls[1].md.Get(kernelgrpc.IdempotencyKey)
	if len(first) != 1 || !slices.Equal(first, second) {
		t.Fatalf("keys = %v then %v, want the same key on both attempts", first, second)
	}
}

func TestALocallyExhaustedDeadlineIs504WithoutACall(t *testing.T) {
	tight := deadline.Budget{Dependency: "edge", Method: "route", Limit: 50 * time.Millisecond, Slack: 5 * time.Millisecond, EstimatedDuration: 10 * time.Millisecond}
	f := newFixture(t, withBudget(tight))

	rec := f.post(t, "/tenancy/store/loja-1/suspend", `{"reason":"fraude"}`)

	requireRejection(t, rec, http.StatusGatewayTimeout, "deadline-exceeded")
	if n := f.fake.total(); n != 0 {
		t.Fatalf("calls = %d, want 0 — the method slack does not fit the route deadline", n)
	}
}

func TestARequestWithoutDeadlineReachesTheContextBelowTheRouteBudget(t *testing.T) {
	f := newFixture(t)
	started := time.Now()

	rec := f.do(t, http.MethodGet, "/tenancy/store/loja-1", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}
	calls := f.fake.callsTo("FindStore")
	if len(calls) != 1 || !calls[0].hasDeadline {
		t.Fatalf("the context received no deadline (GRP-04): %+v", calls)
	}
	if budget := calls[0].deadline.Sub(started); budget >= routeBudget.Limit {
		t.Fatalf("the context got %v, want less than the route budget %v (GRP-17)", budget, routeBudget.Limit)
	}
}

func TestInvalidRequestsNeverCallAContext(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		path, body, code string
	}{
		{"/tenancy/store", `{"ref":"loja-1"`, "malformed-body"},
		{"/tenancy/store", `{"ref":"loja-1","name":""}`, "invalid-request"},
		{"/tenancy/store", `{"ref":"loja-1","name":"Loja 1","extra":true}`, "malformed-body"},
		{"/tenancy/store", `{"ref":"loja-1","name":"Loja 1"}{}`, "malformed-body"},
		{"/tenancy/store/loja-1/suspend", `{"reason":""}`, "invalid-request"},
		{"/tenancy/store/loja%201/reactivate", "", "invalid-request"},
		{"/tenancy/store/Loja-1/suspend", `{"reason":"fraude"}`, "invalid-request"},
	}
	for _, tc := range cases {
		t.Run(tc.path+" "+tc.body, func(t *testing.T) {
			requireRejection(t, f.post(t, tc.path, tc.body), http.StatusBadRequest, tc.code)
		})
	}
	if n := f.fake.total(); n != 0 {
		t.Fatalf("calls = %d, want 0", n)
	}
}

func TestTheEdgeSpanParentsTheClientSpanAndTheMetadata(t *testing.T) {
	f := newFixture(t)

	rec := f.do(t, http.MethodGet, "/tenancy/store/loja-1", nil, api.CorrelationHeader, "corr-1")

	if rec.Header().Get(api.CorrelationHeader) != "corr-1" {
		t.Fatalf("%s = %q, want corr-1 echoed", api.CorrelationHeader, rec.Header().Get(api.CorrelationHeader))
	}
	md := f.fake.callsTo("FindStore")[0].md
	if got := md.Get("x-correlation-id"); len(got) != 1 || got[0] != "corr-1" {
		t.Fatalf("x-correlation-id = %v, want corr-1", got)
	}
	if got := md.Get("x-causation-id"); len(got) != 1 || got[0] == "" || got[0] == "corr-1" {
		t.Fatalf("x-causation-id = %v, want the edge request id", got)
	}
	if got := md.Get(kernelgrpc.TenantKey); len(got) != 1 || got[0] != testTenant {
		t.Fatalf("%s = %v, want the tenant the edge resolved", kernelgrpc.TenantKey, got)
	}
	if got := md.Get("authorization"); len(got) != 0 {
		t.Fatalf("authorization = %v: the credential never crosses to a context (CTX-12)", got)
	}

	var edge tracetest.SpanStub
	byID := map[string]tracetest.SpanStub{}
	for _, s := range f.spans.GetSpans() {
		byID[s.SpanContext.SpanID().String()] = s
		if s.Name == "GET /tenancy/store/{ref}" {
			edge = s
		}
	}
	if !edge.SpanContext.IsValid() {
		t.Fatalf("no edge span among %v", f.spans.GetSpans())
	}
	traceparent := md.Get("traceparent")
	parts := strings.Split(strings.Join(traceparent, ""), "-")
	if len(parts) != 4 || parts[1] != edge.SpanContext.TraceID().String() {
		t.Fatalf("traceparent %v does not continue the edge trace", traceparent)
	}
	client, sent := byID[parts[2]]
	if !sent || client.SpanKind != trace.SpanKindClient || client.Name != strings.TrimPrefix(rpc.MethodFindStore, "/") {
		t.Fatalf("traceparent names %q, want a client span", client.Name)
	}
	for span := client; span.Parent.SpanID() != edge.SpanContext.SpanID(); {
		parent, known := byID[span.Parent.SpanID().String()]
		if !known {
			t.Fatalf("client span %q does not descend from the edge span (TRC-05)", client.Name)
		}
		span = parent
	}
}

func TestATracestateFromThePublicClientNeverReachesAContext(t *testing.T) {
	f := newFixture(t)
	const incoming = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	members := make([]string, 32)
	for i := range members {
		members[i] = fmt.Sprintf("probe%d=%s", i, strings.Repeat("x", 200))
	}

	f.do(t, http.MethodGet, "/tenancy/store/loja-1", nil, "traceparent", incoming, "tracestate", strings.Join(members, ","))

	md := f.fake.callsTo("FindStore")[0].md
	if got := md.Get("tracestate"); len(got) != 0 {
		t.Fatalf("tracestate sent to the context = %v, want none from the public client", got)
	}
	if got := strings.Join(md.Get("traceparent"), ""); !strings.Contains(got, "4bf92f3577b34da6a3ce929d0e0e4736") {
		t.Fatalf("traceparent sent to the context = %q, want the incoming trace", got)
	}
}

func TestAMalformedCorrelationIsReplacedOnTheCallToo(t *testing.T) {
	f := newFixture(t)

	rec := f.do(t, http.MethodGet, "/tenancy/store/loja-1", nil, api.CorrelationHeader, "bad correlation;")

	minted := rec.Header().Get(api.CorrelationHeader)
	if minted == "" || minted == "bad correlation;" {
		t.Fatalf("%s = %q, want a minted correlation", api.CorrelationHeader, minted)
	}
	if got := f.fake.callsTo("FindStore")[0].md.Get("x-correlation-id"); len(got) != 1 || got[0] != minted {
		t.Fatalf("x-correlation-id = %v, want %q", got, minted)
	}
}

func TestTheContractsAreServedOnlyWhenProvided(t *testing.T) {
	silent := newFixture(t)
	if rec := silent.do(t, http.MethodGet, api.TenancyContractPath, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("%s without document = %d, want 404", api.TenancyContractPath, rec.Code)
	}

	f := newFixture(t, withTenancyContract("tenancy: yes"))
	rec := f.do(t, http.MethodGet, api.TenancyContractPath, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "tenancy: yes" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/yaml") {
		t.Fatalf("%s = %d %q %v, want 200 with the YAML document", api.TenancyContractPath, rec.Code, rec.Body.String(), rec.Header())
	}
}

func TestAnIdempotencyKeyTheWireRejectsNeverReachesAContext(t *testing.T) {
	f := newFixture(t)
	// WHY: gRPC refuses metadata outside %x20-%x7E (grpc internal/metadata),
	// and net/http accepts those bytes, so an unvalidated header turns into a
	// permanent 500 the caller can trigger at will.
	for _, key := range []string{"abcé", "abc\tdef", strings.Repeat("k", 200), " "} {
		t.Run(fmt.Sprintf("%q", key), func(t *testing.T) {
			rec := f.do(t, http.MethodPost, "/tenancy/store/loja-1/reactivate", nil,
				"Idempotency-Key", key, "Content-Type", "application/json")

			requireRejection(t, rec, http.StatusBadRequest, "invalid-idempotency-key")
		})
	}
	if n := f.fake.total(); n != 0 {
		t.Fatalf("calls = %d, want 0", n)
	}
}

func TestARouteWithoutItsPermissionNeverReachesAContext(t *testing.T) {
	f := newFixture(t)
	reader := `Bearer {"sub":"tester","tenant":"acme","permissions":["tenancy:read"]}`

	rec := f.do(t, http.MethodPost, "/tenancy/store/loja-1/reactivate", nil, "Idempotency-Key", "k-1", "Authorization", reader)

	requireRejection(t, rec, http.StatusForbidden, "permission-denied")
	if n := f.fake.total(); n != 0 {
		t.Fatalf("calls = %d, want 0: the edge refuses a route the caller holds no permission for", n)
	}
}
