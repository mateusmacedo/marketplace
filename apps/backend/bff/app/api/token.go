package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	kernelhttp "github.com/mateusmacedo/dmpf/libs/backend/go/http"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/tracing"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/deadline"
	"go.opentelemetry.io/otel/trace"
)

// TokenPath is the development route that exchanges a user's password for an
// access token of the issuer the edge verifies.
const TokenPath = "/auth/token"

const (
	maxUsernameLength = 255
	maxPasswordLength = 1024
	maxIssuerAnswer   = 1 << 16

	tokenEndpointPath = "/protocol/openid-connect/token"
)

var (
	// ErrInvalidCredentials is the issuer refusing the username or the password.
	ErrInvalidCredentials = errors.New("api: the issuer refused the credentials")
	// ErrIssuerUnavailable is the issuer unreachable or answering anything but a
	// token or a refusal of the credentials.
	ErrIssuerUnavailable = errors.New("api: the issuer is unavailable")
	// ErrIssuerTimeout is the issuer not answering before the exchange timeout.
	ErrIssuerTimeout = errors.New("api: the issuer did not answer in time")
)

// Token is what the issuer granted: the access token and its lifetime.
type Token struct {
	AccessToken string
	ExpiresIn   int
}

// TokenIssuer exchanges a user's credentials for an access token.
type TokenIssuer interface {
	Issue(ctx context.Context, username, password string) (Token, error)
}

// TokenRoute is the route of POST /auth/token. It demands no identity, since
// its caller has none yet.
//
// WHY: the kernel admits RequireNeither only with the reach declared as all
// tenants (IDN-19), and WithVerifiedPlatformReach grants that reach only to a
// route that demands a subject, so this one reads no tenant data at all. The
// idempotency key is declared because ValidateEdge refuses a POST without one
// (RST-02); a replay just issues another token.
func TokenRoute(budget deadline.Budget) kernelhttp.Route {
	return kernelhttp.Route{
		Name:           "issueToken",
		Method:         http.MethodPost,
		Path:           TokenPath,
		ContractRef:    contract + strings.ReplaceAll(TokenPath, "/", "~1") + "/post",
		Budget:         budget,
		Requires:       kernelhttp.RequireNeither,
		PlatformReach:  kernelhttp.ReachAllTenants,
		IdempotencyKey: IdempotencyHeader,
	}
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type issuedToken struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func validCredentials(c credentials) bool {
	return within(c.Username, maxUsernameLength) && within(c.Password, maxPasswordLength)
}

func within(value string, limit int) bool {
	length := utf8.RuneCountInString(value)
	return length > 0 && length <= limit
}

// issueToken answers only what the edge decides: the issuer's own answer,
// which may name the user or the client, never reaches the caller.
func issueToken(issuer TokenIssuer) http.HandlerFunc {
	decode := fromBody("TokenRequest", "username and password must be non-empty strings of up to 255 and 1024 characters",
		validCredentials, func(c credentials) credentials { return c })
	return func(w http.ResponseWriter, r *http.Request) {
		creds, ok := decode(w, r)
		if !ok {
			return
		}
		token, err := issuer.Issue(r.Context(), creds.Username, creds.Password)
		span := trace.SpanFromContext(r.Context())
		switch {
		case err == nil:
			// WHY: RFC 6749 §5.1 forbids caching an answer that carries a token.
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Pragma", "no-cache")
			writeJSON(r, w, http.StatusOK, issuedToken{AccessToken: token.AccessToken, TokenType: "Bearer", ExpiresIn: token.ExpiresIn})
		case errors.Is(err, ErrInvalidCredentials):
			writeRejection(r, w, http.StatusUnauthorized, "invalid-credentials", "the username or the password was not accepted")
		case errors.Is(err, ErrIssuerTimeout):
			tracing.RecordError(span, "DeadlineExceeded")
			writeRejection(r, w, http.StatusGatewayTimeout, "deadline-exceeded", "the identity provider did not answer in time")
		default:
			tracing.RecordError(span, "Unavailable")
			writeRejection(r, w, http.StatusServiceUnavailable, "unavailable", "the identity provider is unavailable")
		}
	}
}

// PasswordGrant exchanges a user's password at the token endpoint of a
// Keycloak realm for a public client (RFC 6749 §4.3). It exists only for
// development: the password passes through the edge.
type PasswordGrant struct {
	endpoint string
	clientID string
	client   *http.Client
	timeout  time.Duration
}

// NewPasswordGrant reuses client for every exchange and bounds each one by
// timeout, on top of the deadline of the request.
func NewPasswordGrant(issuer, clientID string, client *http.Client, timeout time.Duration) *PasswordGrant {
	return &PasswordGrant{
		endpoint: strings.TrimSuffix(issuer, "/") + tokenEndpointPath,
		clientID: clientID,
		client:   client,
		timeout:  timeout,
	}
}

// NewTokenClient is the client a PasswordGrant reuses: its own pool, and no
// redirect followed, since a token endpoint that redirects is not answering.
func NewTokenClient() *http.Client {
	return &http.Client{
		Transport:     http.DefaultTransport.(*http.Transport).Clone(),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

type issuerAnswer struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	Error       string `json:"error"`
}

// Issue returns only the sentinels of this file, never the issuer's message or
// the transport error, which carries the endpoint and may echo the request.
func (g *PasswordGrant) Issue(ctx context.Context, username, password string) (Token, error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	form := url.Values{
		"grant_type": {"password"},
		"client_id":  {g.clientID},
		"username":   {username},
		"password":   {password},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, ErrIssuerUnavailable
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return Token{}, transportFailure(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var answer issuerAnswer
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxIssuerAnswer)).Decode(&answer); err != nil {
		if timedOut(err) {
			return Token{}, ErrIssuerTimeout
		}
		return Token{}, ErrIssuerUnavailable
	}
	switch {
	case resp.StatusCode == http.StatusOK && answer.AccessToken != "":
		return Token{AccessToken: answer.AccessToken, ExpiresIn: answer.ExpiresIn}, nil
	case (resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized) && answer.Error == "invalid_grant":
		return Token{}, ErrInvalidCredentials
	default:
		return Token{}, ErrIssuerUnavailable
	}
}

func transportFailure(err error) error {
	if timedOut(err) {
		return ErrIssuerTimeout
	}
	return ErrIssuerUnavailable
}

func timedOut(err error) bool {
	var network net.Error
	return errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &network) && network.Timeout())
}
