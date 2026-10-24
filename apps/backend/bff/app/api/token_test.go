package api_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	kernelhttp "github.com/mateusmacedo/dmpf/libs/backend/go/http"
	"go.opentelemetry.io/otel/attribute"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app/api"
)

const (
	tokenClientID   = "marketplace-dev"
	sentinelPass    = "sentinel-password"
	sentinelToken   = "sentinel-access-token"
	tokenEndpoint   = "/realms/dmpf/protocol/openid-connect/token"
	validTokenInput = `{"username":"operator","password":"` + sentinelPass + `"}`
)

// fakeIssuer is the token endpoint of a Keycloak realm: it records each form
// it receives and answers what the test declares.
type fakeIssuer struct {
	mu      sync.Mutex
	forms   []url.Values
	paths   []string
	status  int
	body    string
	delay   time.Duration
	baseURL string
}

func newFakeIssuer(t *testing.T, status int, body string) *fakeIssuer {
	t.Helper()
	f := &fakeIssuer{status: status, body: body}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		f.forms = append(f.forms, r.PostForm)
		f.paths = append(f.paths, r.URL.Path)
		delay := f.delay
		f.mu.Unlock()
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.body))
	}))
	t.Cleanup(srv.Close)
	f.baseURL = srv.URL
	return f
}

func (f *fakeIssuer) issuer() string { return f.baseURL + "/realms/dmpf" }

func (f *fakeIssuer) grant(timeout time.Duration) *api.PasswordGrant {
	return api.NewPasswordGrant(f.issuer(), tokenClientID, api.NewTokenClient(), timeout)
}

func (f *fakeIssuer) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.forms)
}

func granted(token string) string {
	return fmt.Sprintf(`{"access_token":%q,"expires_in":300,"refresh_expires_in":1800,"refresh_token":"sentinel-refresh","token_type":"Bearer","scope":"profile email"}`, token)
}

func (f fixture) requestToken(t *testing.T, body string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	base := []string{"Authorization", "", "Content-Type", "application/json", api.IdempotencyHeader, "token-1"}
	return f.do(t, http.MethodPost, api.TokenPath, strings.NewReader(body), append(base, headers...)...)
}

func TestTheTokenRouteDemandsNoIdentityAndPassesTheEdgeValidation(t *testing.T) {
	route := api.TokenRoute(routeBudget)

	if err := route.ValidateEdge(); err != nil {
		t.Fatalf("ValidateEdge() = %v", err)
	}
	if route.Requires != kernelhttp.RequireNeither || route.RequiresSubject() || route.RequiresTenant() {
		t.Fatalf("Requires = %v, want neither subject nor tenant", route.Requires)
	}
	if route.Method != http.MethodPost || route.Path != api.TokenPath || route.IdempotencyKey != api.IdempotencyHeader {
		t.Fatalf("route = %+v, want POST %s declaring %s (RST-02)", route, api.TokenPath, api.IdempotencyHeader)
	}
	for _, served := range api.Routes(routeBudget) {
		if served.Path == api.TokenPath {
			t.Fatalf("Routes() serves %s, want it only through TokenRoute when configured", api.TokenPath)
		}
	}
}

func TestTheTokenRouteDoesNotExistWithoutAnIssuer(t *testing.T) {
	f := newFixture(t)

	rec := f.requestToken(t, validTokenInput)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 without AUTH_TOKEN_CLIENT_ID (body %s)", rec.Code, rec.Body.String())
	}
}

func TestTheTokenRouteExchangesThePasswordAtTheIssuer(t *testing.T) {
	issuer := newFakeIssuer(t, http.StatusOK, granted(sentinelToken))
	f := newFixture(t, withTokenIssuer(issuer.grant(time.Second)))

	rec := f.requestToken(t, validTokenInput)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	decode(t, rec, &body)
	want := map[string]any{"access_token": sentinelToken, "token_type": "Bearer", "expires_in": float64(300)}
	if len(body) != len(want) {
		t.Fatalf("body = %v, want only %v: nothing else of the issuer reaches the caller", body, want)
	}
	for key, value := range want {
		if body[key] != value {
			t.Fatalf("body %s = %v, want %v", key, body[key], value)
		}
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store (RFC 6749 §5.1)", got)
	}
	if issuer.calls() != 1 || issuer.paths[0] != tokenEndpoint {
		t.Fatalf("issuer calls = %d at %v, want one at %s", issuer.calls(), issuer.paths, tokenEndpoint)
	}
	form := issuer.forms[0]
	for key, value := range map[string]string{"grant_type": "password", "client_id": tokenClientID, "username": "operator", "password": sentinelPass} {
		if form.Get(key) != value {
			t.Fatalf("form %s = %q, want %q", key, form.Get(key), value)
		}
	}
}

func TestTheTokenRouteRefusesABodyOutsideTheContract(t *testing.T) {
	issuer := newFakeIssuer(t, http.StatusOK, granted(sentinelToken))
	f := newFixture(t, withTokenIssuer(issuer.grant(time.Second)))
	cases := []struct {
		name, body, code string
		headers          []string
	}{
		{"not JSON", `username=operator`, "malformed-body", nil},
		{"unknown field", `{"username":"operator","password":"x","client_id":"other"}`, "malformed-body", nil},
		{"trailing data", validTokenInput + `{}`, "malformed-body", nil},
		{"empty username", `{"username":"","password":"x"}`, "invalid-request", nil},
		{"missing password", `{"username":"operator"}`, "invalid-request", nil},
		{"username too long", `{"username":"` + strings.Repeat("u", 256) + `","password":"x"}`, "invalid-request", nil},
		{"password too long", `{"username":"operator","password":"` + strings.Repeat("p", 1025) + `"}`, "invalid-request", nil},
		{"body over the limit", `{"username":"operator","password":"` + strings.Repeat("p", 1<<16) + `"}`, "malformed-body", nil},
		{"no idempotency key", validTokenInput, "missing-idempotency-key", []string{api.IdempotencyHeader, ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := f.requestToken(t, c.body, c.headers...)

			requireRejection(t, rec, http.StatusBadRequest, c.code)
		})
	}
	if issuer.calls() != 0 {
		t.Fatalf("issuer calls = %d, want none for a refused body", issuer.calls())
	}
}

func TestTheTokenRouteAnswersTheRefusalOfTheCredentialsWithoutTheIssuerMessage(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusBadRequest} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			issuer := newFakeIssuer(t, status, `{"error":"invalid_grant","error_description":"Invalid user credentials for operator"}`)
			f := newFixture(t, withTokenIssuer(issuer.grant(time.Second)))

			rec := f.requestToken(t, validTokenInput)

			requireRejection(t, rec, http.StatusUnauthorized, "invalid-credentials")
			if strings.Contains(rec.Body.String(), "operator") || strings.Contains(rec.Body.String(), "invalid_grant") {
				t.Fatalf("body = %s, want nothing the issuer said", rec.Body.String())
			}
		})
	}
}

func TestTheTokenRouteAnswersUnavailableWhenTheIssuerDoesNotIssue(t *testing.T) {
	unreachable := newFakeIssuer(t, http.StatusOK, granted(sentinelToken))
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	unreachable.baseURL = closed.URL
	cases := map[string]*fakeIssuer{
		"server error":    newFakeIssuer(t, http.StatusInternalServerError, `{"error":"unknown_error"}`),
		"client refused":  newFakeIssuer(t, http.StatusUnauthorized, `{"error":"invalid_client","error_description":"Invalid client marketplace-dev"}`),
		"no access token": newFakeIssuer(t, http.StatusOK, `{"token_type":"Bearer"}`),
		"not JSON":        newFakeIssuer(t, http.StatusOK, `<html>maintenance</html>`),
		"redirected":      newFakeIssuer(t, http.StatusFound, `{}`),
		"unreachable":     unreachable,
	}
	for name, issuer := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, withTokenIssuer(issuer.grant(time.Second)))

			rec := f.requestToken(t, validTokenInput)

			requireRejection(t, rec, http.StatusServiceUnavailable, "unavailable")
			if strings.Contains(rec.Body.String(), "marketplace-dev") || strings.Contains(rec.Body.String(), "127.0.0.1") {
				t.Fatalf("body = %s, want nothing of the issuer or the transport", rec.Body.String())
			}
		})
	}
}

func TestTheTokenRouteAnswersTimeoutWhenTheIssuerIsSilent(t *testing.T) {
	issuer := newFakeIssuer(t, http.StatusOK, granted(sentinelToken))
	issuer.delay = time.Second
	f := newFixture(t, withTokenIssuer(issuer.grant(50*time.Millisecond)))

	started := time.Now()
	rec := f.requestToken(t, validTokenInput)

	requireRejection(t, rec, http.StatusGatewayTimeout, "deadline-exceeded")
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("answered after %v, want the exchange timeout to bound the call", elapsed)
	}
}

func TestTheTokenRouteNeverRecordsThePasswordNorTheToken(t *testing.T) {
	issuer := newFakeIssuer(t, http.StatusOK, granted(sentinelToken))
	f := newFixture(t, withTokenIssuer(issuer.grant(time.Second)))

	if rec := f.requestToken(t, validTokenInput); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if err := f.logProvider.ForceFlush(t.Context()); err != nil {
		t.Fatalf("ForceFlush() = %v", err)
	}

	var exported strings.Builder
	for _, record := range f.records.all() {
		exported.WriteString(record.Body().String())
		record.WalkAttributes(func(kv attribute.KeyValue) bool {
			exported.WriteString(string(kv.Key) + "=" + kv.Value.String() + "\n")
			return true
		})
	}
	spans := fmt.Sprintf("%+v", f.spans.GetSpans())
	if !strings.Contains(f.logs.String(), api.TokenPath) {
		t.Fatalf("access log = %s, want the request logged", f.logs.String())
	}
	for _, sentinel := range []string{sentinelPass, sentinelToken, "sentinel-refresh"} {
		for where, recorded := range map[string]string{"log": f.logs.String(), "exported log": exported.String(), "span": spans} {
			if strings.Contains(recorded, sentinel) {
				t.Fatalf("the %s carries %q: %s", where, sentinel, recorded)
			}
		}
	}
}

func TestPasswordGrantReturnsOnlyItsSentinels(t *testing.T) {
	issuer := newFakeIssuer(t, http.StatusUnauthorized, `{"error":"invalid_grant","error_description":"Invalid user credentials"}`)

	_, err := issuer.grant(time.Second).Issue(t.Context(), "operator", sentinelPass)

	if !errors.Is(err, api.ErrInvalidCredentials) || err.Error() != api.ErrInvalidCredentials.Error() {
		t.Fatalf("Issue() = %v, want exactly ErrInvalidCredentials", err)
	}
}
