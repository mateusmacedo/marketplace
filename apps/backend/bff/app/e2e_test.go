//go:build integration

package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mateusmacedo/dmpf/libs/backend/go/contracts/envelope"
	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	tenancyevent "github.com/mateusmacedo/marketplace/apps/backend/tenancy/contract/gen/go/company/tenancy/event/v1"
)

const (
	storeRegisteredType  = "com.company.tenancy.store-registered.v1"
	storeSuspendedType   = "com.company.tenancy.store-suspended.v1"
	storeReactivatedType = "com.company.tenancy.store-reactivated.v1"

	correlation = "corr-e2e-1"
	traceID     = "4bf92f3577b34da6a3ce929d0e0e4736"
	traceparent = "00-" + traceID + "-00f067aa0ba902b7-01"

	e2eStore      = "loja-e2e"
	replayedStore = "loja-e2e-replayed"
)

func TestTopologyEndToEnd(t *testing.T) {
	top := newTopology(t)
	bin := buildBinaries(t)
	top.boot(t, bin)

	t.Log("1. a store registered through the BFF crosses to the tenancy context and leaves as a fact with the correlation and the trace of the edge")
	headers := map[string]string{"X-Correlation-ID": correlation, "traceparent": traceparent}
	if status, body := top.call(t, http.MethodPost, "/tenancy/store", `{"ref":"`+e2eStore+`","name":"Loja e2e"}`, headers); status != http.StatusCreated || string(body) != `{"ref":"`+e2eStore+`"}`+"\n" {
		t.Fatalf("POST store: status = %d, body %s, want 201 with the ref", status, body)
	}
	registered := top.envelopeOf(t, top.tenancyTopic, storeRegisteredType, e2eStore)
	if registered.CorrelationID != correlation {
		t.Fatalf("StoreRegistered correlationid = %q, want %q (CTX-07)", registered.CorrelationID, correlation)
	}
	if cause := registered.CausationID; cause == "" || cause == registered.ID {
		t.Fatalf("StoreRegistered causationid = %q, want the request id of the context (CTX-08)", cause)
	}
	if got := traceOf(registered.TraceParent); got != traceID {
		t.Fatalf("StoreRegistered trace id = %q, want %q from the BFF edge", got, traceID)
	}
	var storeRegistered tenancyevent.StoreRegistered
	requireOccurredAt(t, registered, &storeRegistered, storeRegistered.GetRegisteredAt)
	if storeRegistered.GetName() != "Loja e2e" {
		t.Fatalf("StoreRegistered name = %q, want Loja e2e", storeRegistered.GetName())
	}

	t.Log("2. the store reads as active, and the refusal of the domain reaches the client as a 422")
	if view := top.store(t, e2eStore); view.Status != "active" || view.Name != "Loja e2e" || view.RegisteredAt == 0 {
		t.Fatalf("GET store = %+v, want the active store", view)
	}
	if status, body := top.call(t, http.MethodPost, "/tenancy/store/"+e2eStore+"/reactivate", "", nil); status != http.StatusUnprocessableEntity || !strings.Contains(string(body), "tenancy/store/not-suspended") {
		t.Fatalf("POST reactivate of an active store: status = %d, body %s, want 422 tenancy/store/not-suspended", status, body)
	}

	t.Log("3. suspend and reactivate leave as facts too, each instant equal to its OccurredAt to the nanosecond")
	if status, body := top.call(t, http.MethodPost, "/tenancy/store/"+e2eStore+"/suspend", `{"reason":"fraude em análise"}`, nil); status != http.StatusOK {
		t.Fatalf("POST suspend: status = %d, body %s", status, body)
	}
	var suspended tenancyevent.StoreSuspended
	requireOccurredAt(t, top.envelopeOf(t, top.tenancyTopic, storeSuspendedType, e2eStore), &suspended, suspended.GetSuspendedAt)
	if status, body := top.call(t, http.MethodGet, "/tenancy/store?status=suspended", "", nil); status != http.StatusOK || !strings.Contains(string(body), `"ref":"`+e2eStore+`"`) || !strings.Contains(string(body), "fraude em análise") {
		t.Fatalf("GET stores suspended: status = %d, body %s, want the suspended store with its reason", status, body)
	}
	if status, body := top.call(t, http.MethodPost, "/tenancy/store/"+e2eStore+"/reactivate", "", nil); status != http.StatusOK {
		t.Fatalf("POST reactivate: status = %d, body %s", status, body)
	}
	var reactivated tenancyevent.StoreReactivated
	requireOccurredAt(t, top.envelopeOf(t, top.tenancyTopic, storeReactivatedType, e2eStore), &reactivated, reactivated.GetReactivatedAt)
	if view := top.store(t, e2eStore); view.Status != "active" || view.SuspensionReason != "" {
		t.Fatalf("GET store after reactivate = %+v, want active without a reason", view)
	}
	requireTypePrefix(t, top.recordsUntilEnd(t, top.tenancyTopic), "com.company.tenancy.")

	t.Log("4. a command repeated with its key answers the stored response, and the key reused with another body is refused")
	key := map[string]string{"Idempotency-Key": "e2e-replay"}
	firstStatus, firstBody, firstHeader := top.callWithHeader(t, http.MethodPost, "/tenancy/store", `{"ref":"`+replayedStore+`","name":"Loja replay"}`, key)
	againStatus, againBody, againHeader := top.callWithHeader(t, http.MethodPost, "/tenancy/store", `{"ref":"`+replayedStore+`","name":"Loja replay"}`, key)
	if firstStatus != http.StatusCreated || againStatus != http.StatusCreated || string(againBody) != string(firstBody) {
		t.Fatalf("repeated POST store: %d %s then %d %s, want the same 201 twice", firstStatus, firstBody, againStatus, againBody)
	}
	if firstHeader.Get("Idempotent-Replayed") != "" || againHeader.Get("Idempotent-Replayed") != "true" {
		t.Fatalf("Idempotent-Replayed = %q then %q, want none then true", firstHeader.Get("Idempotent-Replayed"), againHeader.Get("Idempotent-Replayed"))
	}
	top.envelopeOf(t, top.tenancyTopic, storeRegisteredType, replayedStore)
	if n := top.countOnTopic(t, top.tenancyTopic, storeRegisteredType, replayedStore); n != 1 {
		t.Fatalf("StoreRegistered facts for the replayed store = %d, want 1: the replay registered it again", n)
	}
	if status, body, _ := top.callWithHeader(t, http.MethodPost, "/tenancy/store", `{"ref":"`+replayedStore+`","name":"Outra loja"}`, key); status != http.StatusUnprocessableEntity || !strings.Contains(string(body), "reused-idempotency-key") {
		t.Fatalf("POST store with the key and another body: status = %d, body %s, want 422 reused-idempotency-key", status, body)
	}

	t.Log("5. an unknown store is 404, a tenant other than the operator is 403, and no credential is 401")
	if status, body := top.call(t, http.MethodGet, "/tenancy/store/loja-nenhuma", "", nil); status != http.StatusNotFound {
		t.Fatalf("GET unknown store: status = %d, body %s, want 404", status, body)
	}
	if status, body := top.callAs(t, outsiderCredential, http.MethodGet, "/tenancy/store/"+e2eStore); status != http.StatusForbidden || !strings.Contains(string(body), "permission-denied") {
		t.Fatalf("GET store as another tenant: status = %d, body %s, want 403 permission-denied from the context", status, body)
	}
	if status, body := top.callAnonymous(t, http.MethodGet, "/tenancy/store/"+e2eStore); status != http.StatusUnauthorized {
		t.Fatalf("GET store without credential: status = %d, want %d (IDN-01, IDN-06), body %s", status, http.StatusUnauthorized, body)
	}
}

type storeView struct {
	Status           string `json:"status"`
	Name             string `json:"name"`
	SuspensionReason string `json:"suspensionReason"`
	RegisteredAt     int64  `json:"registeredAt"`
}

func (top *topology) store(t *testing.T, ref string) storeView {
	t.Helper()
	status, body := top.call(t, http.MethodGet, "/tenancy/store/"+ref, "", nil)
	if status != http.StatusOK {
		t.Fatalf("GET store %s: status = %d, body %s", ref, status, body)
	}
	var view storeView
	if err := json.Unmarshal(body, &view); err != nil {
		t.Fatalf("GET store body %s: %v", body, err)
	}
	return view
}

// e2eCredential is what the development authenticator reads back: the
// operator tenant, the only one the tenancy context serves.
const e2eCredential = `Bearer {"sub":"e2e-tester","tenant":"` + operatorTenant + `","permissions":["tenancy:write","tenancy:read"]}`

// outsiderCredential holds the permissions of the routes under a tenant that
// is not the operator: the edge lets it through and the context refuses it.
const outsiderCredential = `Bearer {"sub":"e2e-outsider","tenant":"acme","permissions":["tenancy:write","tenancy:read"]}`

func (top *topology) callAs(t *testing.T, credential, method, path string) (int, []byte) {
	t.Helper()
	req := top.request(t, method, path, "")
	req.Header.Set("Authorization", credential)
	return send(t, req)
}

func (top *topology) call(t *testing.T, method, path, body string, headers map[string]string) (int, []byte) {
	t.Helper()
	req := top.request(t, method, path, body)
	req.Header.Set("Authorization", e2eCredential)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return send(t, req)
}

func (top *topology) callWithHeader(t *testing.T, method, path, body string, headers map[string]string) (int, []byte, http.Header) {
	t.Helper()
	req := top.request(t, method, path, body)
	req.Header.Set("Authorization", e2eCredential)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	got, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("ReadAll() = %v", err)
	}
	return res.StatusCode, got, res.Header
}

// callAnonymous sends the same request without the Authorization header, which
// is the only way to observe the refusal across the whole topology.
func (top *topology) callAnonymous(t *testing.T, method, path string) (int, []byte) {
	t.Helper()
	return send(t, top.request(t, method, path, ""))
}

func (top *topology) request(t *testing.T, method, path, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, "http://"+top.bffAddr+path, strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() = %v", err)
	}
	if method == http.MethodPost {
		req.Header.Set("Idempotency-Key", fmt.Sprintf("k-%d", time.Now().UnixNano()))
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func send(t *testing.T, req *http.Request) (int, []byte) {
	t.Helper()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer res.Body.Close()
	got, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("ReadAll() = %v", err)
	}
	return res.StatusCode, got
}

func requireTypePrefix(t *testing.T, envelopes []envelope.Envelope, prefix string) {
	t.Helper()
	if len(envelopes) == 0 {
		t.Fatalf("no fact published; want facts of type %s*", prefix)
	}
	for _, env := range envelopes {
		if !strings.HasPrefix(env.Type, prefix) {
			t.Fatalf("fact of type %q on a topic that should carry only %s*", env.Type, prefix)
		}
	}
}

func traceOf(parent string) string {
	parts := strings.Split(parent, "-")
	if len(parts) != 4 {
		return ""
	}
	return parts[1]
}

func requireOccurredAt(t *testing.T, env envelope.Envelope, payload proto.Message, at func() *timestamppb.Timestamp) {
	t.Helper()
	if err := envelope.Unpack(env, payload); err != nil {
		t.Fatalf("unpack %s: %v", env.Type, err)
	}
	if got, want := at().AsTime(), env.Time.AsTime(); !got.Equal(want) {
		t.Fatalf("%s instant = %s, want the OccurredAt %s", env.Type, got.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}

func (top *topology) envelopeOf(t *testing.T, topic, eventType, aggregate string) envelope.Envelope {
	t.Helper()
	env, _ := top.findRecord(t, topic, eventType, aggregate)
	return env
}

func (top *topology) findRecord(t *testing.T, topic, eventType, aggregate string) (envelope.Envelope, *kgo.Record) {
	t.Helper()
	cl, err := kgo.NewClient(kgo.SeedBrokers(top.brokers...), kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatalf("kgo.NewClient() = %v", err)
	}
	defer cl.Close()
	deadline := time.Now().Add(e2eTimeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		fetches := cl.PollFetches(ctx)
		cancel()
		var (
			found  envelope.Envelope
			record *kgo.Record
		)
		fetches.EachRecord(func(r *kgo.Record) {
			if record != nil {
				return
			}
			if env, err := envelope.Unmarshal(r.Value); err == nil && env.Type == eventType && env.PartitionKey == aggregate {
				found, record = env, r
			}
		})
		if record != nil {
			return found, record
		}
	}
	t.Fatalf("no %s for %s arrived on %s within %v", eventType, aggregate, topic, e2eTimeout)
	return envelope.Envelope{}, nil
}

func (top *topology) countOnTopic(t *testing.T, topic, eventType, aggregate string) int {
	t.Helper()
	n := 0
	for _, env := range top.recordsUntilEnd(t, topic) {
		if env.Type == eventType && env.PartitionKey == aggregate {
			n++
		}
	}
	return n
}

// recordsUntilEnd drains the topic up to the end offsets read before the first
// fetch: an absence is proven over everything published so far, not over
// whatever one poll happened to return.
func (top *topology) recordsUntilEnd(t *testing.T, topic string) []envelope.Envelope {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), adminWindow)
	defer cancel()
	ends, err := top.admin.ListEndOffsets(ctx, topic)
	if err != nil {
		t.Fatalf("ListEndOffsets(%s) = %v", topic, err)
	}
	remaining := map[int32]int64{}
	ends.Each(func(o kadm.ListedOffset) {
		if o.Offset > 0 {
			remaining[o.Partition] = o.Offset
		}
	})
	if len(remaining) == 0 {
		return nil
	}
	cl, err := kgo.NewClient(kgo.SeedBrokers(top.brokers...), kgo.ConsumeTopics(topic), kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()))
	if err != nil {
		t.Fatalf("kgo.NewClient() = %v", err)
	}
	defer cl.Close()
	var envelopes []envelope.Envelope
	deadline := time.Now().Add(e2eTimeout)
	for len(remaining) > 0 && time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		fetches := cl.PollFetches(ctx)
		cancel()
		fetches.EachRecord(func(r *kgo.Record) {
			end, open := remaining[r.Partition]
			if !open {
				return
			}
			if env, err := envelope.Unmarshal(r.Value); err == nil {
				envelopes = append(envelopes, env)
			} else {
				envelopes = append(envelopes, envelope.Envelope{Type: "<unreadable>"})
			}
			if r.Offset+1 >= end {
				delete(remaining, r.Partition)
			}
		})
	}
	if len(remaining) > 0 {
		t.Fatalf("%s was not read to its end offsets within %v", topic, e2eTimeout)
	}
	return envelopes
}
