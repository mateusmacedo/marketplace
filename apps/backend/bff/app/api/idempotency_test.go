package api_test

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"

	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	tenancyv1 "github.com/mateusmacedo/marketplace/apps/backend/tenancy/contract/gen/go/company/tenancy/service/v1"
)

type command struct {
	method, path, body, context string
}

var commands = []command{
	{http.MethodPost, "/tenancy/store", `{"ref":"loja-1","name":"Loja 1"}`, "RegisterStore"},
	{http.MethodPost, "/tenancy/store/loja-1/suspend", `{"reason":"fraude"}`, "SuspendStore"},
	{http.MethodPost, "/tenancy/store/loja-1/reactivate", "", "ReactivateStore"},
}

func derived(subject, key string) string {
	digest := sha256.Sum256([]byte(subject + "\x00" + key))
	return hex.EncodeToString(digest[:])
}

func TestTheContextReceivesTheKeyDerivedFromTheSubject(t *testing.T) {
	for _, c := range commands {
		t.Run(c.context, func(t *testing.T) {
			f := newFixture(t)

			f.post(t, c.path, c.body)

			calls := f.fake.callsTo(c.context)
			if len(calls) != 1 {
				t.Fatalf("%s reached the context %d times, want 1", c.context, len(calls))
			}
			got := calls[0].md.Get(kernelgrpc.IdempotencyKey)
			if len(got) != 1 || got[0] != derived("tester", "k-1") {
				t.Fatalf("idempotency-key = %v, want hex(sha256(subject 0x00 key)) %s", got, derived("tester", "k-1"))
			}
			if !ports.ValidIdempotencyKey(got[0]) || len(got[0]) != 64 {
				t.Fatalf("derived key %q is not a 64-character key the context accepts", got[0])
			}
		})
	}
}

func TestAQueryCarriesNoIdempotencyKey(t *testing.T) {
	f := newFixture(t)

	f.do(t, http.MethodGet, "/tenancy/store/loja-1", nil)

	if got := f.fake.callsTo("FindStore")[0].md.Get(kernelgrpc.IdempotencyKey); len(got) != 0 {
		t.Fatalf("idempotency-key = %v on a query, want none", got)
	}
}

func TestTwoSubjectsWithTheSameClientKeyNeverShareAnEntry(t *testing.T) {
	f := newFixture(t)
	other := strings.Replace(testCredential, `"sub":"tester"`, `"sub":"another"`, 1)

	f.post(t, "/tenancy/store", `{"ref":"loja-1","name":"Loja 1"}`)
	f.do(t, http.MethodPost, "/tenancy/store", strings.NewReader(`{"ref":"loja-1","name":"Loja 1"}`),
		"Idempotency-Key", "k-1", "Content-Type", "application/json", "Authorization", other)

	calls := f.fake.callsTo("RegisterStore")
	if len(calls) != 2 {
		t.Fatalf("RegisterStore reached the context %d times, want 2", len(calls))
	}
	if first, second := calls[0].md.Get(kernelgrpc.IdempotencyKey), calls[1].md.Get(kernelgrpc.IdempotencyKey); first[0] == second[0] {
		t.Fatalf("both subjects sent key %q: the second would replay the first one's answer", first[0])
	}
}

func TestAReplayedCommandIsAnsweredWithIdempotentReplayed(t *testing.T) {
	for _, c := range commands {
		t.Run(c.context, func(t *testing.T) {
			first := newFixture(t).post(t, c.path, c.body)
			again := newFixture(t, withContexts((&fakeContexts{}).replay(c.context))).post(t, c.path, c.body)

			if got := first.Header().Get("Idempotent-Replayed"); got != "" {
				t.Fatalf("first answer carries Idempotent-Replayed %q, want none", got)
			}
			if again.Code != first.Code || again.Body.String() != first.Body.String() {
				t.Fatalf("replay = %d %s, want the first answer %d %s", again.Code, again.Body.String(), first.Code, first.Body.String())
			}
			if got := again.Header().Get("Idempotent-Replayed"); got != "true" {
				t.Fatalf("replay carries Idempotent-Replayed %q, want true", got)
			}
		})
	}
}

func TestAReplayedRefusalKeepsIts422AndIsMarked(t *testing.T) {
	fake := (&fakeContexts{}).replay("SuspendStore").on("SuspendStore", func(int) (any, error) {
		return &tenancyv1.SuspendStoreResponse{Result: &tenancyv1.SuspendStoreResponse_Rejection{
			Rejection: &tenancyv1.Rejection{Code: "tenancy/store/not-active", Message: "store is not active"},
		}}, nil
	})
	f := newFixture(t, withContexts(fake))

	rec := f.post(t, "/tenancy/store/loja-1/suspend", `{"reason":"fraude"}`)

	requireRejection(t, rec, http.StatusUnprocessableEntity, "tenancy/store/not-active")
	if got := rec.Header().Get("Idempotent-Replayed"); got != "true" {
		t.Fatalf("Idempotent-Replayed = %q, want true on the replayed refusal", got)
	}
}

func TestTheIdempotencyOutcomesOfTheContextReachTheClient(t *testing.T) {
	status := func(err error) error {
		mapped, _ := kernelgrpc.IdempotencyStatus(err)
		return mapped
	}
	for name, c := range map[string]struct {
		err    error
		status int
		code   string
	}{
		"reused key":    {status(ports.ErrIdempotencyMismatch), http.StatusUnprocessableEntity, "reused-idempotency-key"},
		"key in flight": {status(ports.ErrIdempotencyInFlight), http.StatusConflict, "in-flight-idempotency-key"},
		"missing key":   {kernelgrpc.KeyStatus(kernelgrpc.ReasonMissingIdempotencyKey), http.StatusBadRequest, "missing-idempotency-key"},
		"invalid key":   {kernelgrpc.KeyStatus(kernelgrpc.ReasonInvalidIdempotencyKey), http.StatusBadRequest, "invalid-idempotency-key"},
		"exists":        {status(ports.ErrAlreadyExists), http.StatusConflict, "already-exists"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, withContexts((&fakeContexts{}).on("RegisterStore", func(int) (any, error) { return nil, c.err })))

			rec := f.post(t, "/tenancy/store", `{"ref":"loja-1","name":"Loja 1"}`)

			requireRejection(t, rec, c.status, c.code)
		})
	}
}

func TestTheAccessLogCarriesTheClientAndTheDerivedKey(t *testing.T) {
	f := newFixture(t)

	f.post(t, "/tenancy/store", `{"ref":"loja-1","name":"Loja 1"}`)

	record := onlyAccessLog(t, f)
	if record["dmpf.idempotency_key"] != "k-1" || record["dmpf.idempotency_key.derived"] != derived("tester", "k-1") {
		t.Fatalf("access log = %v, want the client key and the key the context received", record)
	}
}

func TestOnlyAReplayedAnswerIsMarked(t *testing.T) {
	status := func(err error) error {
		mapped, _ := kernelgrpc.IdempotencyStatus(err)
		return mapped
	}
	for name, c := range map[string]struct {
		fake   *fakeContexts
		status int
	}{
		"key in flight": {(&fakeContexts{}).on("ReactivateStore", func(int) (any, error) { return nil, status(ports.ErrIdempotencyInFlight) }), http.StatusConflict},
		"reused key":    {(&fakeContexts{}).on("ReactivateStore", func(int) (any, error) { return nil, status(ports.ErrIdempotencyMismatch) }), http.StatusUnprocessableEntity},
		"replay the edge cannot answer": {(&fakeContexts{}).replay("ReactivateStore").on("ReactivateStore", func(int) (any, error) {
			return &tenancyv1.ReactivateStoreResponse{}, nil
		}), http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			rec := newFixture(t, withContexts(c.fake)).post(t, "/tenancy/store/loja-1/reactivate", "")

			if rec.Code != c.status {
				t.Fatalf("status = %d %s, want %d", rec.Code, rec.Body.String(), c.status)
			}
			if got := rec.Header().Get("Idempotent-Replayed"); got != "" {
				t.Fatalf("Idempotent-Replayed = %q on a %d, want none: only a replayed answer below 500 is marked", got, rec.Code)
			}
		})
	}
}

func TestCORSExposesTheReplayHeader(t *testing.T) {
	f := newFixture(t, withCORS("https://app.example"))

	rec := f.do(t, http.MethodGet, "/tenancy/store/loja-1", nil, "Origin", "https://app.example")

	exposed := rec.Header().Get("Access-Control-Expose-Headers")
	if !strings.Contains(exposed, "Idempotent-Replayed") || !strings.Contains(exposed, "X-Correlation-ID") {
		t.Fatalf("Access-Control-Expose-Headers = %q, want X-Correlation-ID and Idempotent-Replayed", exposed)
	}
}
