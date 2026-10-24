package app_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"maps"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const (
	signingKeyID = "bff-test"
	audience     = "dmpf-bff"
	tenantClaim  = "organization.*.tenant_id"
)

// signingIssuer is an OIDC issuer the verifier of the authn discovers and
// trusts: its JWKS publishes the key every token below is signed with.
type signingIssuer struct {
	url string
	key *rsa.PrivateKey
}

// newSigningIssuer serves discovery and the JWKS; hold, when given, runs before
// the JWKS answers, so a test can keep a request in flight inside verification.
func newSigningIssuer(t *testing.T, hold func()) signingIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() = %v", err)
	}
	server := httptest.NewServer(nil)
	t.Cleanup(server.Close)
	encode := base64.RawURLEncoding.EncodeToString
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": server.URL, "jwks_uri": server.URL + "/jwks",
			"authorization_endpoint": server.URL + "/auth", "token_endpoint": server.URL + "/token",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		if hold != nil {
			hold()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "kid": signingKeyID, "use": "sig", "alg": "RS256",
			"n": encode(key.N.Bytes()), "e": encode(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	issuer := signingIssuer{url: server.URL, key: key}
	mux.HandleFunc("POST "+tokenEndpoint, issuer.passwordGrant)
	server.Config.Handler = mux
	return issuer
}

// tokenEndpoint is where Keycloak answers the password grant, under the issuer.
const tokenEndpoint = "/protocol/openid-connect/token"

const (
	grantClient   = "marketplace-dev"
	grantUser     = "operator"
	grantPassword = "sentinel-grant-password"
)

// passwordGrant answers like Keycloak: a token for the one user and client it
// knows, invalid_grant for any other credential.
func (i signingIssuer) passwordGrant(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.PostFormValue("grant_type") != "password" || r.PostFormValue("client_id") != grantClient ||
		r.PostFormValue("username") != grantUser || r.PostFormValue("password") != grantPassword {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Invalid user credentials"}`))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "granted-" + grantUser, "expires_in": 300, "token_type": "Bearer"})
}

// env points the api role at this issuer, reading the tenant the way the
// Keycloak realm publishes it: one organization carrying its tenant_id.
func (i signingIssuer) env() []string {
	return []string{"OIDC_ISSUER", i.url, "OIDC_AUDIENCE", audience, "OIDC_TENANT_CLAIM", tenantClaim}
}

// organization is the claim Keycloak issues for a member of one organization.
func organization(tenant string) map[string]any {
	return map[string]any{tenant: map[string]any{"tenant_id": []any{tenant}, "id": "org-" + tenant}}
}

// member is a token of a subject in one organization with the profile scope;
// extra overrides or removes (with nil) any of its claims.
func member(subject, session, tenant string, extra map[string]any) map[string]any {
	claims := map[string]any{"sub": subject, "sid": session, "scope": "openid profile", "organization": organization(tenant)}
	maps.Copy(claims, extra)
	maps.DeleteFunc(claims, func(_ string, value any) bool { return value == nil })
	return claims
}

func (i signingIssuer) token(t *testing.T, claims map[string]any) string {
	t.Helper()
	now := time.Now()
	segment := func(value any) string {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("Marshal() = %v", err)
		}
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	payload := map[string]any{"iss": i.url, "aud": audience, "iat": now.Add(-10 * time.Second).Unix(), "exp": now.Add(5 * time.Minute).Unix()}
	maps.Copy(payload, claims)
	signed := segment(map[string]string{"alg": "RS256", "kid": signingKeyID, "typ": "JWT"}) + "." + segment(payload)
	digest := sha256.Sum256([]byte(signed))
	signature, err := rsa.SignPKCS1v15(rand.Reader, i.key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("SignPKCS1v15() = %v", err)
	}
	return "Bearer " + signed + "." + base64.RawURLEncoding.EncodeToString(signature)
}
