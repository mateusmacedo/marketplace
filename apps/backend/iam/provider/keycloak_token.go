package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	kernelhttp "github.com/mateusmacedo/dmpf/libs/backend/go/http"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
)

// keycloakTokenSlack renews the token before Keycloak stops honoring it, so a
// call that starts near the expiry does not travel with a dying token. It
// never exceeds half the lifetime, so a short-lived token is still reused.
const keycloakTokenSlack = 30 * time.Second

// keycloakTokens holds the access token of the service account until its
// expiry; concurrent commands share one grant.
type keycloakTokens struct {
	client   *kernelhttp.Client
	clock    clock.Clock
	url      string
	clientID string
	secret   KeycloakSecret

	// granting has capacity one: a waiter leaves when its context ends, so a
	// grant in flight holds no command back past its own deadline.
	granting chan struct{}

	mu      sync.Mutex
	current string
	expires time.Time
}

type keycloakGrant struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func (t *keycloakTokens) get(ctx context.Context) (string, error) {
	if token, live := t.cached(); live {
		return token, nil
	}
	select {
	case t.granting <- struct{}{}:
	case <-ctx.Done():
		// WHY: the kernel gives retryable_transport to a deadline spent before the
		// send, and the grant is a repeatable route, so a cancellation is too.
		return "", &keycloakFailure{class: ports.ClassRetryableTransport, step: routeToken, detail: "the context ended waiting for the grant in flight", cause: ctx.Err()}
	}
	defer func() { <-t.granting }()
	if token, live := t.cached(); live {
		return token, nil
	}
	issued := t.clock.Now()
	grant, err := t.grant(ctx)
	if err != nil {
		return "", err
	}
	lifetime := time.Duration(grant.ExpiresIn) * time.Second
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current = grant.AccessToken
	t.expires = issued.Add(lifetime - min(keycloakTokenSlack, lifetime/2))
	return t.current, nil
}

func (t *keycloakTokens) cached() (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.current, t.current != "" && t.clock.Now().Before(t.expires)
}

// invalidate drops the token the Admin API refused, and only that one: a
// concurrent command may already hold a fresher grant.
func (t *keycloakTokens) invalidate(token string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current == token {
		t.current, t.expires = "", time.Time{}
	}
}

func (t *keycloakTokens) grant(ctx context.Context) (keycloakGrant, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {t.clientID},
		"client_secret": {t.secret.reveal()},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, strings.NewReader(form.Encode()))
	if err != nil {
		return keycloakGrant{}, &keycloakFailure{class: ports.ClassFinalValidation, step: routeToken, detail: "request could not be built"}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	// WHY: the grant of client_credentials is a POST that changes nothing in the realm.
	resp, err := t.client.Do(ctx, routeToken, req)
	if err != nil {
		return keycloakGrant{}, sent(routeToken, true, err)
	}
	reply, err := readReply(routeToken, true, resp)
	if err != nil {
		return keycloakGrant{}, err
	}
	if reply.status != http.StatusOK {
		class, detail := tokenRefusal(reply)
		return keycloakGrant{}, &keycloakFailure{class: class, step: routeToken, status: reply.status, detail: detail}
	}
	var grant keycloakGrant
	if json.Unmarshal(reply.body, &grant) != nil || grant.AccessToken == "" || !strings.EqualFold(grant.TokenType, "bearer") || grant.ExpiresIn < 0 {
		return keycloakGrant{}, &keycloakFailure{class: ports.ClassSchemaMismatch, step: routeToken, status: reply.status, detail: "the grant is no bearer token"}
	}
	return grant, nil
}

// oauthErrors are the error codes of RFC 6749 §5.2 the contract classifies;
// the detail is the constant here, never the text the partner sent.
var oauthErrors = map[string]ports.ExternalClass{
	"invalid_client":         ports.ClassFinalAuthorization,
	"unauthorized_client":    ports.ClassFinalAuthorization,
	"invalid_request":        ports.ClassFinalValidation,
	"unsupported_grant_type": ports.ClassFinalValidation,
	"invalid_scope":          ports.ClassFinalValidation,
}

func tokenRefusal(reply keycloakReply) (ports.ExternalClass, string) {
	if reply.status == http.StatusTooManyRequests || reply.status >= http.StatusInternalServerError {
		return ports.ClassRetryableUpstream, ""
	}
	var refusal struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(reply.body, &refusal) == nil {
		if class, ok := oauthErrors[refusal.Error]; ok {
			return class, refusal.Error
		}
	}
	return ports.ClassUnknownAmbiguous, ""
}
