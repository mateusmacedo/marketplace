//go:build integration && keycloak

package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/obstest"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app"
)

const (
	acmeTenant      = "t-acme"
	sessionInterval = 2 * time.Second
	revokedWithin   = sessionInterval + time.Second
)

// startKeycloakEdge serves the api role against the realm of the test Keycloak,
// reading the tenant as the realm publishes it, with redisURL as its deny-list.
func startKeycloakEdge(t *testing.T, redisURL string) edge {
	t.Helper()
	return startEdge(t, "OIDC_ISSUER", keycloakIssuer(t), "OIDC_AUDIENCE", audience, "OIDC_TENANT_CLAIM", tenantClaim, "REDIS_URL", redisURL)
}

// startKeycloakSessionEvents runs the session-events role as its own process,
// as it is deployed: the OpenTelemetry runtime of the api already lives in this one.
func startKeycloakSessionEvents(t *testing.T, redisURL string) {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "bff")
	if out, err := exec.Command("go", "build", "-o", binary, "../cmd").CombinedOutput(); err != nil {
		t.Fatalf("go build ../cmd = %v: %s", err, out)
	}
	admin := obstest.UnusedLoopbackAddr(t)
	cmd := exec.Command(binary, "session-events")
	cmd.Env = append(os.Environ(),
		"OTEL_SERVICE_NAME=bff",
		"OTEL_RESOURCE_ATTRIBUTES=service.version=test,service.instance.id=bff-session-events-test",
		"OTEL_TRACES_EXPORTER=none", "OTEL_METRICS_EXPORTER=none", "OTEL_LOGS_EXPORTER=none",
		"OIDC_ISSUER="+keycloakIssuer(t),
		"OIDC_REVOCATION_TTL_SECONDS=300",
		"OIDC_BACKCHANNEL_CLIENT_ID="+portalClient,
		"OIDC_SESSION_EVENTS_CLIENT_ID=dmpf-session-events",
		"OIDC_SESSION_EVENTS_CLIENT_SECRET="+tb.Env(t, "KEYCLOAK_SESSION_EVENTS_SECRET"),
		"OIDC_SESSION_EVENTS_INTERVAL_SECONDS="+strconv.Itoa(int(sessionInterval/time.Second)),
		"SESSION_EVENTS_ADDR="+obstest.UnusedLoopbackAddr(t),
		"ADMIN_ADDR="+admin,
		"REDIS_URL="+redisURL,
	)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start session-events = %v", err)
	}
	var waitErr error
	exited := make(chan struct{})
	go func() {
		waitErr = cmd.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-exited:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-exited
		}
	})

	for deadline := time.Now().Add(15 * time.Second); ; time.Sleep(100 * time.Millisecond) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := app.Probe(ctx, app.Config{AdminAddr: admin})
		cancel()
		if err == nil {
			return
		}
		select {
		case <-exited:
			t.Fatalf("session-events exited before ready (%v): %s", waitErr, output.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("session-events not ready in 15s (%v): %s", err, output.String())
		}
	}
}

func signInAlice(t *testing.T) (*browser, tokenSet) {
	t.Helper()
	b := newBrowser(t)
	tokens, _ := b.signIn(seedAccount(t, "alice"), "organization:acme")
	return b, tokens
}

func TestKeycloakAliceReadsTheIdentityOfHerToken(t *testing.T) {
	e := startKeycloakEdge(t, tb.Env(t, "REDIS_URL"))
	_, tokens := signInAlice(t)

	status, body := e.call(t, http.MethodGet, "/v1/me", "Authorization", "Bearer "+tokens.AccessToken)

	if status != http.StatusOK {
		t.Fatalf("GET /v1/me = %d %s, want 200 for a token of the seeded realm", status, body)
	}
	var profile struct{ Subject, Tenant string }
	if err := json.Unmarshal(body, &profile); err != nil {
		t.Fatalf("body %s is not the profile: %v", body, err)
	}
	if sub, _ := tokenClaims(t, tokens.AccessToken)["sub"].(string); profile.Subject != sub || profile.Tenant != acmeTenant {
		t.Fatalf("profile = %+v, want subject %q of the token and tenant %q of the acme organization", profile, sub, acmeTenant)
	}
	e.stop(t)
}

func TestKeycloakLogoutRevokesTheTokenWithinThePollingInterval(t *testing.T) {
	redisURL := tb.Env(t, "REDIS_URL")
	e := startKeycloakEdge(t, redisURL)
	startKeycloakSessionEvents(t, redisURL)
	b, tokens := signInAlice(t)
	bearer := "Bearer " + tokens.AccessToken

	if status, body := e.call(t, http.MethodGet, "/v1/me", "Authorization", bearer); status != http.StatusOK {
		t.Fatalf("GET /v1/me before the logout = %d %s, want 200", status, body)
	}
	b.logout(tokens.IDToken)
	loggedOut := time.Now()

	for status := http.StatusOK; status != http.StatusUnauthorized; time.Sleep(100 * time.Millisecond) {
		if time.Since(loggedOut) > revokedWithin {
			t.Fatalf("GET /v1/me %v after the logout = %d, want 401 within the polling interval plus 1s", revokedWithin, status)
		}
		status = e.request(t, http.MethodGet, "/v1/me", "Authorization", bearer)
	}
	e.stop(t)

	refusal := recordNamed(t, e.records(t), "auth: token rejected")
	if got := attributesOf(refusal)["dmpf.auth.refusal_reason"].AsString(); got != "session_revoked" {
		t.Fatalf("dmpf.auth.refusal_reason = %q, want session_revoked", got)
	}
}

func TestKeycloakTokenIsRefusedOnceTheDenyListStops(t *testing.T) {
	redisURL := tb.Env(t, "REDIS_URL")
	redis := proxyTo(t, redisURL)
	e := startKeycloakEdge(t, redis.url)
	_, tokens := signInAlice(t)
	bearer := "Bearer " + tokens.AccessToken

	if status, body := e.call(t, http.MethodGet, "/v1/me", "Authorization", bearer); status != http.StatusOK {
		t.Fatalf("GET /v1/me with the deny-list up = %d %s, want 200", status, body)
	}
	redis.stop()

	if status, body := e.call(t, http.MethodGet, "/v1/me", "Authorization", bearer); status != http.StatusUnauthorized {
		t.Fatalf("GET /v1/me with the deny-list stopped = %d %s, want 401: the edge fails closed", status, body)
	}
	e.stop(t)
}

// tcpProxy stands between the edge and the test Redis, so a test can stop the
// deny-list under a running edge without touching the shared container.
type tcpProxy struct {
	url      string
	listener net.Listener
	mu       sync.Mutex
	conns    []net.Conn
	stopped  bool
}

func proxyTo(t *testing.T, target string) *tcpProxy {
	t.Helper()
	upstream, err := url.Parse(target)
	if err != nil {
		t.Fatalf("url.Parse(REDIS_URL) = %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() = %v", err)
	}
	p := &tcpProxy{listener: listener}
	proxied := *upstream
	proxied.Host = listener.Addr().String()
	p.url = proxied.String()
	go p.accept(upstream.Host)
	t.Cleanup(p.stop)
	return p
}

func (p *tcpProxy) accept(upstream string) {
	for {
		client, err := p.listener.Accept()
		if err != nil {
			return
		}
		server, err := net.Dial("tcp", upstream)
		if err != nil {
			_ = client.Close()
			continue
		}
		if !p.track(client, server) {
			return
		}
		go pipe(client, server)
		go pipe(server, client)
	}
}

func (p *tcpProxy) track(conns ...net.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		for _, conn := range conns {
			_ = conn.Close()
		}
		return false
	}
	p.conns = append(p.conns, conns...)
	return true
}

func (p *tcpProxy) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return
	}
	p.stopped = true
	_ = p.listener.Close()
	for _, conn := range p.conns {
		_ = conn.Close()
	}
}

func pipe(to, from net.Conn) {
	_, _ = io.Copy(to, from)
	_ = to.Close()
}
