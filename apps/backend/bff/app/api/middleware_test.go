package api_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/admission"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app/api"
)

type unreadable struct{}

func (unreadable) Read([]byte) (int, error) { panic("the body was read before admission") }

func TestAdmissionRefusesWith429BeforeReadingTheBody(t *testing.T) {
	f := newFixture(t, withLimit(admission.Limit{PerSecond: 0.001, Burst: 1, Concurrency: 10}))

	if rec := f.do(t, http.MethodGet, mePath, nil); rec.Code != http.StatusOK {
		t.Fatalf("first request = %d, want 200", rec.Code)
	}
	rec := f.do(t, http.MethodGet, mePath, unreadable{})
	requireRejection(t, rec, http.StatusTooManyRequests, "admission-refused")
}

// RES-16: the bucket is per tenant, keyed by the tenant the edge authenticated,
// so one tenant exhausting its own does not refuse another's request.
func TestOneTenantExhaustingItsBucketDoesNotRefuseAnother(t *testing.T) {
	f := newFixture(t, withLimit(admission.Limit{PerSecond: 0.001, Burst: 1, Concurrency: 10}))
	other := `Bearer {"sub":"tester","tenant":"globex","permissions":["profile"]}`

	if rec := f.do(t, http.MethodGet, mePath, nil); rec.Code != http.StatusOK {
		t.Fatalf("first request = %d, want 200", rec.Code)
	}
	if rec := f.do(t, http.MethodGet, mePath, nil); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request = %d, want 429", rec.Code)
	}
	if rec := f.do(t, http.MethodGet, mePath, nil, "Authorization", other); rec.Code != http.StatusOK {
		t.Fatalf("another tenant's request = %d, want 200: the buckets are per tenant", rec.Code)
	}
}

func TestAnIncomingTraceparentIsContinued(t *testing.T) {
	f := newFixture(t)
	const incoming = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

	f.do(t, http.MethodGet, mePath, nil, "traceparent", incoming)

	if got := serverSpan(t, f, "GET "+mePath).SpanContext.TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("edge trace = %s, want the incoming trace", got)
	}
}

func TestATracestateFromThePublicClientReachesNoSpan(t *testing.T) {
	f := newFixture(t)
	const incoming = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	members := make([]string, 32)
	for i := range members {
		members[i] = fmt.Sprintf("probe%d=%s", i, strings.Repeat("x", 200))
	}

	f.do(t, http.MethodGet, mePath, nil, "traceparent", incoming, "tracestate", strings.Join(members, ","))

	spans := f.spans.GetSpans()
	if len(spans) == 0 {
		t.Fatal("no span recorded")
	}
	for _, s := range spans {
		if s.SpanContext.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Fatalf("%s trace = %s, want the incoming traceparent continued", s.Name, s.SpanContext.TraceID())
		}
		if s.SpanContext.TraceState().Len() != 0 || s.Parent.TraceState().Len() != 0 {
			t.Fatalf("%s tracestate = %q (parent %q), want none from the public client", s.Name, s.SpanContext.TraceState(), s.Parent.TraceState())
		}
	}
}

func TestTheCorrelationIsEchoedAndAMalformedOneIsReplaced(t *testing.T) {
	f := newFixture(t)

	if got := f.do(t, http.MethodGet, mePath, nil, api.CorrelationHeader, "corr-1").Header().Get(api.CorrelationHeader); got != "corr-1" {
		t.Fatalf("%s = %q, want corr-1 echoed", api.CorrelationHeader, got)
	}
	minted := f.do(t, http.MethodGet, mePath, nil, api.CorrelationHeader, "bad correlation;").Header().Get(api.CorrelationHeader)
	if minted == "" || minted == "bad correlation;" {
		t.Fatalf("%s = %q, want a minted correlation", api.CorrelationHeader, minted)
	}
}

func TestCORSAnswersThePreflightOfDeclaredOrigins(t *testing.T) {
	f := newFixture(t, withCORS("http://localhost:8082"))

	rec := f.do(t, http.MethodOptions, mePath, nil, "Origin", "http://localhost:8082", "Access-Control-Request-Method", "GET")

	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:8082" {
		t.Fatalf("preflight = %d %v, want 204 with the origin allowed", rec.Code, rec.Header())
	}
	for _, header := range []string{api.IdempotencyHeader, "Authorization"} {
		if !strings.Contains(rec.Header().Get("Access-Control-Allow-Headers"), header) {
			t.Fatalf("Access-Control-Allow-Headers = %q, want %s", rec.Header().Get("Access-Control-Allow-Headers"), header)
		}
	}
	if got := rec.Header().Get("Access-Control-Expose-Headers"); got != api.CorrelationHeader {
		t.Fatalf("Access-Control-Expose-Headers = %q, want %s", got, api.CorrelationHeader)
	}

	other := f.do(t, http.MethodOptions, mePath, nil, "Origin", "http://evil.example")
	if other.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("an undeclared origin was allowed")
	}
}

func TestAGetWithAnInvalidIdempotencyKeyNeverReachesTheHandler(t *testing.T) {
	f := newFixture(t)
	for _, key := range []string{"abcé", strings.Repeat("k", 200), " "} {
		t.Run(fmt.Sprintf("%q", key), func(t *testing.T) {
			rec := f.do(t, http.MethodGet, mePath, nil, api.IdempotencyHeader, key)

			requireRejection(t, rec, http.StatusBadRequest, "invalid-idempotency-key")
			requireNoProfile(t, rec.Body.String())
		})
	}
}

func TestTheAccessLogFlagsAnInvalidKeyWithoutRecordingIt(t *testing.T) {
	f := newFixture(t)

	f.do(t, http.MethodGet, mePath, nil, api.IdempotencyHeader, "abcé")

	record := onlyAccessLog(t, f)
	if record["dmpf.idempotency_key.invalid"] != true || record["dmpf.idempotency_key"] != nil {
		t.Fatalf("access log = %v, want the invalid key flagged and not recorded", record)
	}
}

func TestARequestRunsUnderTheRouteBudget(t *testing.T) {
	var remaining time.Duration
	f := newFixture(t, withAuthenticator(deadlineProbe{&remaining}))
	started := time.Now()

	if rec := f.do(t, http.MethodGet, mePath, nil); rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body %s)", rec.Code, rec.Body.String())
	}

	if remaining <= 0 || remaining > routeBudget.Limit || time.Since(started) > routeBudget.Limit {
		t.Fatalf("the request saw %v left, want a deadline within the route budget %v (GRP-05)", remaining, routeBudget.Limit)
	}
}

type deadlineProbe struct{ remaining *time.Duration }

func (p deadlineProbe) Authenticate(ctx context.Context, _ ports.Credential) (ports.Identity, error) {
	if deadline, ok := ctx.Deadline(); ok {
		*p.remaining = time.Until(deadline)
	}
	tenant := ports.TenantID(testTenant)
	return ports.Identity{Subject: "tester", Tenant: &tenant, Permissions: []ports.Permission{api.ProfilePermission}}, nil
}
