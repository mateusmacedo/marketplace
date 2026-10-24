package app

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/mateusmacedo/dmpf/libs/backend/go/authn"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/boot"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/envconfig"
	"github.com/mateusmacedo/dmpf/libs/backend/go/redis"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/admission"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/deadline"
)

var (
	ErrMissingVariable = errors.New("bff: required variable is not set")
	ErrInvalidVariable = envconfig.ErrInvalidVariable

	// ErrInsecureNotDeclared refuses a clear-text dial that no one asked for:
	// without a CA file, GRPC_INSECURE has to say so explicitly.
	ErrInsecureNotDeclared = errors.New("bff: no CA file and GRPC_INSECURE is not set")

	// ErrTokenWithoutIssuer refuses the development token route without an
	// issuer to exchange the credentials at: the mock verifies no password.
	ErrTokenWithoutIssuer = errors.New("bff: AUTH_TOKEN_CLIENT_ID requires OIDC_ISSUER")
)

const (
	envHTTPAddr            = "HTTP_ADDR"
	envAdminAddr           = "ADMIN_ADDR"
	envTenancyTarget       = "TENANCY_GRPC_TARGET"
	envGRPCInsecure        = "GRPC_INSECURE"
	envGRPCCAFile          = "GRPC_CA_FILE"
	envGRPCServerName      = "GRPC_SERVER_NAME"
	envGRPCClientCertFile  = "GRPC_CLIENT_CERT_FILE"
	envGRPCClientKeyFile   = "GRPC_CLIENT_KEY_FILE"
	envCORSOrigins         = "CORS_ORIGINS"
	envMetricTenants       = "METRIC_TENANTS"
	envTenancyContractPath = "OPENAPI_TENANCY_PATH"
	envDrainDelay          = "DRAIN_DELAY"
	envRedisURL            = "REDIS_URL"
	envTokenClientID       = "AUTH_TOKEN_CLIENT_ID"
)

type Config struct {
	HTTPAddr  string
	AdminAddr string

	TenancyTarget string

	// The transport policy of every gRPC client: mutual TLS through CAFile and
	// the client pair, or the development opt-out GRPCInsecure (GRP-15).
	GRPCInsecure   bool
	CAFile         string
	ServerName     string
	ClientCertFile string
	ClientKeyFile  string

	CORSOrigins         []string
	TenancyContractPath string

	Signals boot.Signals

	Service string

	Admission admission.Limit

	// MetricTenants is the allowlist of MET-07: the tenants that keep their own
	// admission bucket and label. Every other tenant shares "other".
	MetricTenants []string
	RouteBudget   deadline.Budget

	// DrainDelay is how long the edge keeps serving, already unready, before
	// closing the listener: the time the load balancer takes to stop routing.
	DrainDelay time.Duration

	Auth  authn.Config
	Redis redis.Config

	// TokenClientID is the public client POST /auth/token exchanges a user's
	// password for; empty, the route does not exist. Development only.
	TokenClientID string
}

func Defaults() Config {
	return Config{
		HTTPAddr:  ":8080",
		AdminAddr: ":8090",
		Service:   "bff",
		Admission: admission.Limit{PerSecond: 50, Burst: 100, Concurrency: 32},
		RouteBudget: deadline.Budget{
			Dependency:        "contexts",
			Method:            "route",
			Limit:             2 * time.Second,
			Slack:             200 * time.Millisecond,
			EstimatedDuration: 200 * time.Millisecond,
		},
	}
}

func FromEnv(lookup func(string) string) (Config, error) {
	cfg := Defaults()

	cfg.HTTPAddr = envconfig.OrDefault(lookup(envHTTPAddr), cfg.HTTPAddr)
	cfg.AdminAddr = envconfig.OrDefault(lookup(envAdminAddr), cfg.AdminAddr)
	cfg.TenancyTarget = lookup(envTenancyTarget)
	cfg.CAFile = lookup(envGRPCCAFile)
	cfg.ServerName = lookup(envGRPCServerName)
	cfg.ClientCertFile = lookup(envGRPCClientCertFile)
	cfg.ClientKeyFile = lookup(envGRPCClientKeyFile)
	cfg.CORSOrigins = envconfig.SplitList(lookup(envCORSOrigins))
	cfg.MetricTenants = envconfig.SplitList(lookup(envMetricTenants))
	cfg.TenancyContractPath = lookup(envTenancyContractPath)
	cfg.TokenClientID = lookup(envTokenClientID)

	var err error
	if cfg.GRPCInsecure, err = envconfig.ParseBool(envGRPCInsecure, lookup(envGRPCInsecure)); err != nil {
		return Config{}, err
	}
	if cfg.Auth, err = authn.ReadEnv(lookup); err != nil {
		return Config{}, err
	}
	if cfg.Redis, err = readRedis(lookup); err != nil {
		return Config{}, err
	}

	if cfg.Signals, err = boot.SignalsFromEnv(lookup); err != nil {
		return Config{}, err
	}
	if value := lookup(envDrainDelay); value != "" {
		if cfg.DrainDelay, err = time.ParseDuration(value); err != nil || cfg.DrainDelay < 0 {
			return Config{}, fmt.Errorf("%w: %s=%q is not a non-negative duration", ErrInvalidVariable, envDrainDelay, value)
		}
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate refuses a start the edge could not serve: an address net.Listen
// would refuse, the public and the administration routes on one port, a missing
// context target, no transport policy (TLS through a trusted authority or the
// explicit development opt-out, GRP-15), a route budget the deadline package
// refuses, no way of verifying identity, no deny-list to consult before
// accepting a token, or a development token route without an issuer.
func (c Config) Validate() error {
	for _, listen := range [][2]string{{envHTTPAddr, c.HTTPAddr}, {envAdminAddr, c.AdminAddr}} {
		if _, _, err := net.SplitHostPort(listen[1]); err != nil {
			return fmt.Errorf("%w: %s=%q is not host:port", ErrInvalidVariable, listen[0], listen[1])
		}
	}
	if c.HTTPAddr == c.AdminAddr && !ephemeral(c.HTTPAddr) {
		return fmt.Errorf("%w: %s and %s share %q", ErrInvalidVariable, envHTTPAddr, envAdminAddr, c.HTTPAddr)
	}
	switch {
	case c.TenancyTarget == "":
		return fmt.Errorf("%w: %s", ErrMissingVariable, envTenancyTarget)
	case !c.GRPCInsecure && c.CAFile == "":
		return fmt.Errorf("%w: %s=true or %s", ErrMissingVariable, envGRPCInsecure, envGRPCCAFile)
	case c.CAFile != "" && (c.ClientCertFile == "" || c.ClientKeyFile == ""):
		return fmt.Errorf("%w: %s and %s", ErrMissingVariable, envGRPCClientCertFile, envGRPCClientKeyFile)
	}
	if err := c.RouteBudget.Validate(); err != nil {
		return err
	}
	if err := c.Auth.Validate(); err != nil {
		return err
	}
	if c.TokenClientID != "" && c.Auth.Issuer == "" {
		return ErrTokenWithoutIssuer
	}
	return validateRedis(c.Redis)
}

func readRedis(lookup func(string) string) (redis.Config, error) {
	if lookup(envRedisURL) == "" {
		return redis.Config{}, nil
	}
	return redis.FromEnv(lookup)
}

func validateRedis(c redis.Config) error {
	if c.Options == nil {
		return fmt.Errorf("%w: %s", ErrMissingVariable, envRedisURL)
	}
	return c.Validate()
}
