package provider_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/execctx"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	iamports "github.com/mateusmacedo/marketplace/apps/backend/iam/ports"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

func buyerPayload(t *testing.T, subject string, status domain.BuyerStatus) []byte {
	t.Helper()
	raw, err := provider.KeycloakPayloads{}.BuyerAccess(iamports.BuyerAccess{Subject: domain.Subject(subject), Status: status})
	if err != nil {
		t.Fatalf("BuyerAccess() = %v", err)
	}
	return raw
}

func TestKeycloakTokenIsGrantedOnceAndReusedWithinItsLifetime(t *testing.T) {
	d := newKeycloakDouble(t)
	buyer := d.addUser("comprador@example.com")
	k := d.keycloak(t)

	for range 2 {
		if class, _, err := submit(t, inTenant(t, testOperator), k.BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive)); class != ports.ClassAccepted {
			t.Fatalf("Submit() = %v, %v; want accepted", class, err)
		}
	}

	if grants := d.callsTo("token"); grants != 1 {
		t.Fatalf("token grants = %d, want 1: the token is cached while it lives", grants)
	}
}

func TestKeycloakTokenIsRenewedAheadOfItsExpiryByAtMostHalfItsLifetime(t *testing.T) {
	tests := []struct {
		lifetime int
		cached   bool
	}{
		{lifetime: 0, cached: false},
		{lifetime: 20, cached: true},
		{lifetime: 40, cached: true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("expires_in %d", tt.lifetime), func(t *testing.T) {
			d := newKeycloakDouble(t)
			d.setTokenLifetime(tt.lifetime)
			buyer := d.addUser("comprador@example.com")
			k := d.keycloak(t)

			for range 2 {
				if class, _, err := submit(t, inTenant(t, testOperator), k.BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive)); class != ports.ClassAccepted {
					t.Fatalf("Submit() = %v, %v; want accepted", class, err)
				}
			}

			want := d.adminCalls()
			if tt.cached {
				want = 1
			}
			if grants := d.callsTo("token"); grants != want {
				t.Fatalf("token grants = %d, want %d: the cache keeps a token until min(30 s, half its lifetime) before it expires", grants, want)
			}
		})
	}
}

func TestKeycloakTokenWaiterLeavesWhenItsContextEndsDuringAGrantInFlight(t *testing.T) {
	d := newKeycloakDouble(t)
	first, second := d.addUser("primeiro@example.com"), d.addUser("segundo@example.com")
	k := d.keycloak(t)
	entered, release := d.holdGrants(t)
	firstCtx, firstPayload := inTenant(t, testOperator), buyerPayload(t, first, domain.BuyerStatusActive)
	done := make(chan ports.ExternalClass, 1)
	go func() {
		class, _, _ := k.BuyerAccess().Submit(firstCtx, "0123456789abcdef0123456789abcdef", firstPayload)
		done <- class
	}()
	<-entered

	started := time.Now()
	class, _, err := submit(t, inTenantWithin(t, testOperator, 100*time.Millisecond), k.BuyerAccess(), buyerPayload(t, second, domain.BuyerStatusActive))
	waited := time.Since(started)
	release()

	// WHY: nothing was sent, which the kernel classifies retryable_transport for a
	// deadline spent before the send; the grant is a route a repeat leaves as is.
	if class != ports.ClassRetryableTransport {
		t.Fatalf("Submit() = %v, %v; want retryable_transport", class, err)
	}
	if waited > 5*time.Second {
		t.Fatalf("the waiter left after %v, want it gone at its own deadline, before the grant in flight", waited)
	}
	if class := <-done; class != ports.ClassAccepted {
		t.Fatalf("the command holding the grant = %v, want accepted once released", class)
	}
	if grants := d.callsTo("token"); grants != 1 {
		t.Fatalf("token grants = %d, want 1: the waiter never asked for its own", grants)
	}
}

func TestKeycloakConcurrentCommandsShareOneGrant(t *testing.T) {
	d := newKeycloakDouble(t)
	k := d.keycloak(t)
	payloads := make([][]byte, 8)
	for i := range payloads {
		payloads[i] = buyerPayload(t, d.addUser(fmt.Sprintf("comprador-%d@example.com", i)), domain.BuyerStatusActive)
	}
	ctx := inTenant(t, testOperator)
	classes := make([]ports.ExternalClass, len(payloads))
	errs := make([]error, len(payloads))

	var wg sync.WaitGroup
	for i := range payloads {
		wg.Go(func() {
			classes[i], _, errs[i] = k.BuyerAccess().Submit(ctx, "0123456789abcdef0123456789abcdef", payloads[i])
		})
	}
	wg.Wait()

	for i, class := range classes {
		if class != ports.ClassAccepted {
			t.Errorf("command %d Submit() = %v, %v; want accepted", i, class, errs[i])
		}
	}
	if grants := d.callsTo("token"); grants != 1 {
		t.Fatalf("token grants = %d, want 1: concurrent commands share one grant", grants)
	}
}

func TestKeycloakTokenIsRenewedOnceWhenTheAdminAPINoLongerHonorsIt(t *testing.T) {
	d := newKeycloakDouble(t)
	buyer := d.addUser("comprador@example.com")
	k := d.keycloak(t)
	if class, _, err := submit(t, inTenant(t, testOperator), k.BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive)); class != ports.ClassAccepted {
		t.Fatalf("Submit() = %v, %v; want accepted", class, err)
	}
	d.revokeTokens()

	class, _, err := submit(t, inTenant(t, testOperator), k.BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusBlocked))

	if class != ports.ClassAccepted {
		t.Fatalf("Submit() after the revocation = %v, %v; want accepted on a renewed token", class, err)
	}
	if grants := d.callsTo("token"); grants != 2 {
		t.Fatalf("token grants = %d, want 2: one renewal on the 401", grants)
	}
	if d.hasBuyerRole(buyer) {
		t.Fatal("the buyer still holds iam:buyer, want the blocked state applied on the renewed token")
	}
}

func TestKeycloakAdminSecondUnauthorizedIsAFinalAuthorization(t *testing.T) {
	d := newKeycloakDouble(t)
	buyer := d.addUser("comprador@example.com")
	d.fail("read_realm_role", http.StatusUnauthorized, 2, false)

	class, _, err := submit(t, inTenant(t, testOperator), d.keycloak(t).BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))

	if class != ports.ClassFinalAuthorization {
		t.Fatalf("Submit() = %v, %v; want final_authorization after the renewed token is refused too", class, err)
	}
	if grants, reads := d.callsTo("token"), d.callsTo("read_realm_role"); grants != 2 || reads != 2 {
		t.Fatalf("token grants = %d, reads = %d; want one renewal and one repeat", grants, reads)
	}
}

func TestKeycloakAdminForbiddenIsAFinalAuthorizationAtOnce(t *testing.T) {
	d := newKeycloakDouble(t)
	buyer := d.addUser("comprador@example.com")
	d.fail("add_user_realm_roles", http.StatusForbidden, 1, false)

	class, _, err := submit(t, inTenant(t, testOperator), d.keycloak(t).BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))

	if class != ports.ClassFinalAuthorization {
		t.Fatalf("Submit() = %v, %v; want final_authorization: the FGAP refuses for good", class, err)
	}
	if grants, writes := d.callsTo("token"), d.callsTo("add_user_realm_roles"); grants != 1 || writes != 1 {
		t.Fatalf("token grants = %d, writes = %d; want neither a renewal nor a repeat on 403", grants, writes)
	}
}

func TestKeycloakTokenRefusalsAreClassifiedByTheirOAuthError(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   ports.ExternalClass
	}{
		{"invalid client", http.StatusUnauthorized, `{"error":"invalid_client"}`, ports.ClassFinalAuthorization},
		{"unauthorized client", http.StatusBadRequest, `{"error":"unauthorized_client"}`, ports.ClassFinalAuthorization},
		{"invalid request", http.StatusBadRequest, `{"error":"invalid_request"}`, ports.ClassFinalValidation},
		{"unsupported grant type", http.StatusBadRequest, `{"error":"unsupported_grant_type"}`, ports.ClassFinalValidation},
		{"invalid scope", http.StatusBadRequest, `{"error":"invalid_scope"}`, ports.ClassFinalValidation},
		{"server error", http.StatusInternalServerError, `{"error":"invalid_client"}`, ports.ClassRetryableUpstream},
		{"unavailable", http.StatusServiceUnavailable, "", ports.ClassRetryableUpstream},
		{"rate limited", http.StatusTooManyRequests, "", ports.ClassRetryableUpstream},
		{"error outside the contract", http.StatusBadRequest, `{"error":"invalid_grant"}`, ports.ClassUnknownAmbiguous},
		{"body outside the contract", http.StatusUnauthorized, "<html>no</html>", ports.ClassUnknownAmbiguous},
		{"status outside the contract", http.StatusFound, "", ports.ClassUnknownAmbiguous},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newKeycloakDouble(t)
			buyer := d.addUser("comprador@example.com")
			d.failWith("token", tt.status, 1, false, tt.body)

			class, _, err := submit(t, inTenant(t, testOperator), d.keycloak(t).BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))

			if class != tt.want {
				t.Fatalf("Submit() = %v, %v; want %v", class, err, tt.want)
			}
			if admin := len(d.mutations()) + d.callsTo("read_realm_role"); admin != 0 {
				t.Fatalf("the Admin API was called %d times without a token, want none", admin)
			}
		})
	}
}

func TestKeycloakTokenWithTheWrongSecretIsAFinalAuthorization(t *testing.T) {
	d := newKeycloakDouble(t)
	d.setSecret("another-secret")
	buyer := d.addUser("comprador@example.com")

	class, _, err := submit(t, inTenant(t, testOperator), d.keycloak(t).BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))

	if class != ports.ClassFinalAuthorization {
		t.Fatalf("Submit() = %v, %v; want final_authorization from invalid_client", class, err)
	}
}

func TestKeycloakTokenThatDoesNotDecodeIsASchemaMismatch(t *testing.T) {
	for name, body := range map[string]string{
		"not JSON":             "access-token",
		"without access token": `{"expires_in":300,"token_type":"Bearer"}`,
		"not a bearer token":   `{"access_token":"t","expires_in":300,"token_type":"MAC"}`,
	} {
		t.Run(name, func(t *testing.T) {
			d := newKeycloakDouble(t)
			buyer := d.addUser("comprador@example.com")
			d.failWith("token", http.StatusOK, 1, false, body)

			class, _, err := submit(t, inTenant(t, testOperator), d.keycloak(t).BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))

			if class != ports.ClassSchemaMismatch {
				t.Fatalf("Submit() = %v, %v; want schema_mismatch", class, err)
			}
		})
	}
}

type recordingExporter struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (e *recordingExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, record := range records {
		e.records = append(e.records, record.Clone())
	}
	return nil
}

func (e *recordingExporter) Shutdown(context.Context) error   { return nil }
func (e *recordingExporter) ForceFlush(context.Context) error { return nil }

func (e *recordingExporter) rendered() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out strings.Builder
	for _, record := range e.records {
		out.WriteString(record.Body().String())
		record.WalkAttributes(func(kv attribute.KeyValue) bool {
			fmt.Fprintf(&out, " %s=%s", kv.Key, kv.Value.String())
			return true
		})
		out.WriteString("\n")
	}
	return out.String()
}

const leakedEmail = "ana@loja.example"

func TestKeycloakSecretAccessTokenAndAddressNeverReachAnErrorNorALog(t *testing.T) {
	reflected := `{"error":"` + testSecret + ` access-token-1 ` + leakedEmail + `","error_description":"` + testSecret + `"}`
	tests := []struct {
		name   string
		route  string
		code   int
		member bool
	}{
		{"token refusal", "token", http.StatusUnauthorized, false},
		{"token outage", "token", http.StatusBadGateway, false},
		{"admin refusal", "read_realm_role", http.StatusBadRequest, false},
		{"admin outage", "add_user_realm_roles", http.StatusInternalServerError, false},
		{"admin success that does not decode", "read_user_realm_roles", http.StatusOK, false},
		{"refusal of the address search", "find_users", http.StatusBadRequest, true},
		{"outage of the user creation", "create_user", http.StatusInternalServerError, true},
		{"address search that does not decode", "find_users", http.StatusOK, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newKeycloakDouble(t)
			buyer := d.addUser("comprador@example.com")
			d.failWith(tt.route, tt.code, 3, false, reflected)
			exporter := &recordingExporter{}
			k := d.keycloakWith(t, provider.KeycloakDeps{
				LoggerProvider: sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter))),
			})
			command, ctx, payload := k.BuyerAccess(), inTenant(t, testOperator), buyerPayload(t, buyer, domain.BuyerStatusActive)
			if tt.member {
				command, ctx, payload = k.ProvisionMember(), inTenant(t, storeTenant), provisionPayload(t, leakedEmail)
			}

			class, _, err := submit(t, ctx, command, payload)
			if class == ports.ClassAccepted {
				t.Fatalf("Submit() accepted, want the fault of %s to surface", tt.route)
			}

			requireNoLeak(t, err, exporter, k, testSecret, "access-token-", leakedEmail)
		})
	}
}

func requireNoLeak(t *testing.T, err error, exporter *recordingExporter, k *provider.Keycloak, leaks ...string) {
	t.Helper()
	var text, structured strings.Builder
	slog.New(slog.NewTextHandler(&text, nil)).Error("submit", "err", err)
	slog.New(slog.NewJSONHandler(&structured, nil)).Error("submit", "err", err)
	marshalled, _ := json.Marshal(err)
	renderings := map[string]string{
		"Error":     err.Error(),
		"%v":        fmt.Sprintf("%v", err),
		"%+v":       fmt.Sprintf("%+v", err),
		"%#v":       fmt.Sprintf("%#v", err),
		"JSON":      string(marshalled),
		"text log":  text.String(),
		"json log":  structured.String(),
		"telemetry": exporter.rendered(),
		"Keycloak":  fmt.Sprintf("%+v %#v", k, k),
	}
	for name, rendered := range renderings {
		for _, leaked := range leaks {
			if strings.Contains(rendered, leaked) {
				t.Errorf("%s = %q carries %q", name, rendered, leaked)
			}
		}
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// grantingOnly serves the token from the double and hands every request of
// the Admin API to fail.
func grantingOnly(fail transportFunc) http.RoundTripper {
	return transportFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/protocol/openid-connect/token") {
			return http.DefaultTransport.RoundTrip(r)
		}
		return fail(r)
	})
}

func TestKeycloakURLOfAFailedCallNeverReachesAnErrorNorALog(t *testing.T) {
	d := newKeycloakDouble(t)
	exporter := &recordingExporter{}
	k := d.keycloakWith(t, provider.KeycloakDeps{
		LoggerProvider: sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exporter))),
		Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
			if strings.HasSuffix(r.URL.Path, "/users") {
				return nil, &url.Error{Op: "Get", URL: r.URL.String(), Err: errors.New("connection reset by peer")}
			}
			return http.DefaultTransport.RoundTrip(r)
		}),
	})

	class, _, err := submit(t, inTenant(t, storeTenant), k.ProvisionMember(), provisionPayload(t, leakedEmail))

	if class != ports.ClassRetryableTransport {
		t.Fatalf("Submit() = %v, %v; want retryable_transport from the address search", class, err)
	}
	if !strings.Contains(err.Error(), "find_users") || !strings.Contains(err.Error(), "connection reset by peer") {
		t.Fatalf("err = %v, want the step and the cause of the transport kept for diagnosis", err)
	}
	requireNoLeak(t, err, exporter, k, leakedEmail, url.QueryEscape(leakedEmail))
}

func TestKeycloakCommandWithoutDeadlineIsRefusedBeforeAnyCall(t *testing.T) {
	d := newKeycloakDouble(t)
	buyer := d.addUser("comprador@example.com")
	ctx := execctx.WithExecution(t, context.Background(), execctx.WithTenant(testOperator))

	class, _, err := submit(t, ctx, d.keycloak(t).BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))

	if class != ports.ClassFinalValidation {
		t.Fatalf("Submit() = %v, %v; want final_validation: the kernel has no default deadline", class, err)
	}
	if calls := d.callsTo("token"); calls != 0 {
		t.Fatalf("the partner was called %d times, want none", calls)
	}
}

func TestKeycloakCommandWithoutExecutionContextOrTenantIsAFinalValidation(t *testing.T) {
	deadlined := func(t *testing.T) context.Context {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(cancel)
		return ctx
	}
	tests := map[string]func(t *testing.T) context.Context{
		"no execution context": deadlined,
		"no tenant": func(t *testing.T) context.Context {
			execution, err := ports.NewExecutionContext(ports.ExecutionContextSpec{
				RequestID: "r", CorrelationID: "c", TraceContext: "t", Deadline: 1, Locale: "en",
			})
			if err != nil {
				t.Fatalf("NewExecutionContext() = %v", err)
			}
			return ports.WithExecutionContext(deadlined(t), execution)
		},
	}
	for name, ctxOf := range tests {
		t.Run(name, func(t *testing.T) {
			d := newKeycloakDouble(t)
			k := d.keycloak(t)
			commands := map[string]struct {
				command ports.ExternalCommand
				payload []byte
			}{
				"BuyerAccess": {k.BuyerAccess(), buyerPayload(t, "sub-1", domain.BuyerStatusActive)},
				"SyncRole":    {k.SyncRole(), rolePayload(t, "expedicao", domain.RoleStatusActive, "catalog:read")},
			}
			for command, c := range commands {
				class, _, err := submit(t, ctxOf(t), c.command, c.payload)

				if class != ports.ClassFinalValidation {
					t.Errorf("%s Submit() = %v, %v; want final_validation", command, class, err)
				}
			}
			if calls := d.callsTo("token"); calls != 0 {
				t.Fatalf("the partner was called %d times, want none", calls)
			}
		})
	}
}

func TestKeycloakTokenNetworkFailureIsClassifiedByTheKernel(t *testing.T) {
	d := newKeycloakDouble(t)
	k := d.keycloak(t)
	d.server.Close()

	class, _, err := submit(t, inTenant(t, testOperator), k.BuyerAccess(), buyerPayload(t, "sub-1", domain.BuyerStatusActive))

	if class != ports.ClassRetryableTransport {
		t.Fatalf("Submit() = %v, %v; want retryable_transport: a refused dial proves nothing was sent", class, err)
	}
}

func TestKeycloakAdminNetworkFailureIsClassifiedByTheKernel(t *testing.T) {
	d := newKeycloakDouble(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() = %v", err)
	}
	closed := listener.Addr().String()
	_ = listener.Close()
	k := d.keycloakWith(t, provider.KeycloakDeps{Transport: grantingOnly(func(r *http.Request) (*http.Response, error) {
		elsewhere := r.Clone(r.Context())
		elsewhere.URL.Host, elsewhere.Host = closed, closed
		return http.DefaultTransport.RoundTrip(elsewhere)
	})})

	class, _, err := submit(t, inTenant(t, testOperator), k.BuyerAccess(), buyerPayload(t, "sub-1", domain.BuyerStatusActive))

	if class != ports.ClassRetryableTransport {
		t.Fatalf("Submit() = %v, %v; want retryable_transport: a refused dial proves nothing was sent", class, err)
	}
	if grants, admin := d.callsTo("token"), d.adminCalls(); grants != 1 || admin != 0 {
		t.Fatalf("token grants = %d, admin calls = %d; want the token granted and the Admin API never reached", grants, admin)
	}
	if !strings.Contains(err.Error(), "read_realm_role") {
		t.Fatalf("err = %v, want the failure on the Admin API route", err)
	}
}

// The kernel calls unknown_sent what lost the connection after the request
// left, and unknown_ambiguous an answer whose body could not be read; only a
// read, or the grant, is sure to change nothing when repeated.
func TestKeycloakConnectionLostAfterTheSendIsRetryableOnlyWhereARepeatChangesNothing(t *testing.T) {
	tests := []struct {
		name   string
		route  string
		broken connectionFault
		want   ports.ExternalClass
	}{
		{"read dropped", "read_realm_role", connectionDropped, ports.ClassRetryableTransport},
		{"read truncated", "read_realm_role", connectionTruncated, ports.ClassRetryableTransport},
		{"grant dropped", "token", connectionDropped, ports.ClassRetryableTransport},
		{"grant truncated", "token", connectionTruncated, ports.ClassRetryableTransport},
		{"write dropped", "add_user_realm_roles", connectionDropped, ports.ClassUnknownSent},
		{"write truncated", "add_user_realm_roles", connectionTruncated, ports.ClassUnknownAmbiguous},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newKeycloakDouble(t)
			buyer := d.addUser("comprador@example.com")
			d.breakConnection(tt.route, tt.broken, 10)

			class, _, err := submit(t, inTenant(t, testOperator), d.keycloak(t).BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))

			if class != tt.want {
				t.Fatalf("Submit() = %v, %v; want %v", class, err, tt.want)
			}
			if d.hasBuyerRole(buyer) {
				t.Fatal("the buyer holds iam:buyer, want the broken call to have changed nothing")
			}
		})
	}
}

// A refusal is classified by its status even when its body was cut: only an
// accepted answer carries the body a step reads.
func TestKeycloakRefusalWhoseBodyWasCutIsClassifiedByItsStatus(t *testing.T) {
	tests := []struct {
		name   string
		route  string
		status int
		want   ports.ExternalClass
	}{
		{"read forbidden", "read_realm_role", http.StatusForbidden, ports.ClassFinalAuthorization},
		{"grant unavailable", "token", http.StatusServiceUnavailable, ports.ClassRetryableUpstream},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newKeycloakDouble(t)
			buyer := d.addUser("comprador@example.com")
			d.truncate(tt.route, tt.status, 10, false)

			class, _, err := submit(t, inTenant(t, testOperator), d.keycloak(t).BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))

			if class != tt.want {
				t.Fatalf("Submit() = %v, %v; want %v", class, err, tt.want)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", tt.status)) {
				t.Fatalf("err = %v, want the status named for diagnosis", err)
			}
			if d.hasBuyerRole(buyer) {
				t.Fatal("the buyer holds iam:buyer, want the refused call to have changed nothing")
			}
		})
	}
}

func TestKeycloakAdminRetriesATransientReadWithinTheCall(t *testing.T) {
	d := newKeycloakDouble(t)
	buyer := d.addUser("comprador@example.com")
	d.fail("read_realm_role", http.StatusServiceUnavailable, 1, false)

	class, _, err := submit(t, inTenant(t, testOperator), d.keycloak(t).BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))

	if class != ports.ClassAccepted {
		t.Fatalf("Submit() = %v, %v; want accepted after the kernel retried the read", class, err)
	}
	if reads := d.callsTo("read_realm_role"); reads != 2 {
		t.Fatalf("reads = %d, want 2", reads)
	}
}

func TestKeycloakAdminNeverRetriesAPostWithinTheCall(t *testing.T) {
	d := newKeycloakDouble(t)
	buyer := d.addUser("comprador@example.com")
	d.fail("add_user_realm_roles", http.StatusServiceUnavailable, 1, false)

	class, _, err := submit(t, inTenant(t, testOperator), d.keycloak(t).BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))

	if class != ports.ClassRetryableUpstream {
		t.Fatalf("Submit() = %v, %v; want retryable_upstream", class, err)
	}
	if writes := d.callsTo("add_user_realm_roles"); writes != 1 {
		t.Fatalf("writes = %d, want 1: a POST carries no key the partner deduplicates on", writes)
	}
}

func TestKeycloakAdminSuccessThatDoesNotDecodeIsASchemaMismatch(t *testing.T) {
	for name, body := range map[string]string{
		"not JSON":           "roles",
		"over the body cap":  `[{"name":"` + strings.Repeat("x", 1<<20) + `"}]`,
		"of the wrong shape": `{"name":"iam:buyer"}`,
	} {
		t.Run(name, func(t *testing.T) {
			d := newKeycloakDouble(t)
			buyer := d.addUser("comprador@example.com")
			d.failWith("read_user_realm_roles", http.StatusOK, 1, false, body)

			class, _, err := submit(t, inTenant(t, testOperator), d.keycloak(t).BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))

			if class != ports.ClassSchemaMismatch {
				t.Fatalf("Submit() = %v, %v; want schema_mismatch", class, err)
			}
		})
	}
}

func TestKeycloakAdminStatusOutsideTheTableIsUnknownAmbiguous(t *testing.T) {
	d := newKeycloakDouble(t)
	buyer := d.addUser("comprador@example.com")
	d.fail("add_user_realm_roles", http.StatusTeapot, 1, false)

	class, _, err := submit(t, inTenant(t, testOperator), d.keycloak(t).BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))

	if class != ports.ClassUnknownAmbiguous {
		t.Fatalf("Submit() = %v, %v; want unknown_ambiguous", class, err)
	}
	if !strings.Contains(err.Error(), "418") {
		t.Fatalf("err = %v, want the status named for diagnosis", err)
	}
}
