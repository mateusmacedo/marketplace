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
)

const (
	envHTTPAddr      = "HTTP_ADDR"
	envAdminAddr     = "ADMIN_ADDR"
	envCORSOrigins   = "CORS_ORIGINS"
	envMetricTenants = "METRIC_TENANTS"
	envDrainDelay    = "DRAIN_DELAY"
	envRedisURL      = "REDIS_URL"
)

type Config struct {
	HTTPAddr  string
	AdminAddr string

	CORSOrigins []string

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
	cfg.CORSOrigins = envconfig.SplitList(lookup(envCORSOrigins))
	cfg.MetricTenants = envconfig.SplitList(lookup(envMetricTenants))

	var err error
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

// Validate refuses a start the edge could not serve: a health address the
// probe cannot reach, a route budget the deadline package refuses, no way of
// verifying identity, or no deny-list to consult before accepting a token.
func (c Config) Validate() error {
	if _, _, err := net.SplitHostPort(c.AdminAddr); err != nil {
		return fmt.Errorf("%w: %s=%q is not host:port", ErrInvalidVariable, envAdminAddr, c.AdminAddr)
	}
	if err := c.RouteBudget.Validate(); err != nil {
		return err
	}
	if err := c.Auth.Validate(); err != nil {
		return err
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
