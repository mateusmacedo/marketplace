//go:build integration

package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/mateusmacedo/dmpf/libs/backend/go/authn"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/redis"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"
)

func startVerifyingEdge(t *testing.T, issuer signingIssuer) (edge, string) {
	t.Helper()
	redisURL := tb.Env(t, "REDIS_URL")
	return startEdge(t, append(issuer.env(), "REDIS_URL", redisURL)...), redisURL
}

func unique(prefix string) string { return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano()) }

func TestMeAnswersWhatTheVerifiedTokenCarries(t *testing.T) {
	issuer := newSigningIssuer(t, nil)
	e, _ := startVerifyingEdge(t, issuer)
	subject := unique("alice")

	status, body := e.call(t, http.MethodGet, "/v1/me", "Authorization", issuer.token(t, member(subject, unique("sid"), "acme", nil)))

	if status != http.StatusOK {
		t.Fatalf("GET /v1/me = %d %s, want 200", status, body)
	}
	var got struct{ Subject, Tenant string }
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("body %s is not the profile: %v", body, err)
	}
	if got.Subject != subject || got.Tenant != "acme" {
		t.Fatalf("profile = %+v, want the sub of the token and the tenant of its one organization", got)
	}
	e.stop(t)
}

func TestMeRefusesAVerifiedTokenWithoutOneTenantOrTheProfileScope(t *testing.T) {
	issuer := newSigningIssuer(t, nil)
	e, _ := startVerifyingEdge(t, issuer)
	two := organization("acme")
	two["globex"] = organization("globex")["globex"]
	cases := []struct {
		name   string
		extra  map[string]any
		status int
		code   string
	}{
		{"no organization", map[string]any{"organization": nil}, http.StatusForbidden, "tenant-unresolved"},
		{"two organizations", map[string]any{"organization": two}, http.StatusForbidden, "tenant-unresolved"},
		{"no profile scope", map[string]any{"scope": "openid email"}, http.StatusForbidden, "permission-denied"},
		{"expired", map[string]any{"exp": time.Now().Add(-time.Minute).Unix()}, http.StatusUnauthorized, "unauthenticated"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bearer := issuer.token(t, member(unique("user"), unique("sid"), "acme", tc.extra))

			status, body := e.call(t, http.MethodGet, "/v1/me", "Authorization", bearer)

			var refused struct{ Code string }
			if err := json.Unmarshal(body, &refused); err != nil || status != tc.status || refused.Code != tc.code {
				t.Fatalf("GET /v1/me = %d %s, want %d %s", status, body, tc.status, tc.code)
			}
		})
	}
	e.stop(t)
}

func TestARevokedSessionIsRefusedByTheEdge(t *testing.T) {
	issuer := newSigningIssuer(t, nil)
	e, redisURL := startVerifyingEdge(t, issuer)
	cfg, err := redis.FromEnv(func(name string) string { return map[string]string{"REDIS_URL": redisURL}[name] })
	if err != nil {
		t.Fatalf("redis.FromEnv() = %v", err)
	}
	client, err := redis.NewClient(cfg)
	if err != nil {
		t.Fatalf("redis.NewClient() = %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	store := redis.NewRevocationStore(client, clock.System())
	subject, session := unique("user"), unique("sid")
	bearer := issuer.token(t, member(subject, session, "acme", nil))

	if status := e.request(t, http.MethodGet, "/v1/me", "Authorization", bearer); status != http.StatusOK {
		t.Fatalf("GET /v1/me before the logout = %d, want the token accepted", status)
	}
	revoked := authn.Revocation{EventID: "ev-" + session, Kind: authn.RevocationSession, Subject: ports.SubjectID(subject),
		SessionID: session, OccurredAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}
	if _, err := store.Revoke(context.Background(), revoked); err != nil {
		t.Fatalf("Revoke() = %v", err)
	}

	if status := e.request(t, http.MethodGet, "/v1/me", "Authorization", bearer); status != http.StatusUnauthorized {
		t.Fatalf("GET /v1/me after the logout = %d, want 401", status)
	}
	e.stop(t)

	refusal := recordNamed(t, e.records(t), "auth: token rejected")
	if got := attributesOf(refusal)["dmpf.auth.refusal_reason"].AsString(); got != "session_revoked" {
		t.Fatalf("dmpf.auth.refusal_reason = %q, want session_revoked", got)
	}
}
