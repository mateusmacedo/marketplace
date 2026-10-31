package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	kernelapp "github.com/mateusmacedo/dmpf/libs/backend/go/app"
	"github.com/mateusmacedo/dmpf/libs/backend/go/app/relay"
	"github.com/mateusmacedo/dmpf/libs/backend/go/app/serve"
	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	"github.com/mateusmacedo/dmpf/libs/backend/go/kafka"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/boot"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/envconfig"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/admission"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// Role selects which process the binary becomes (BLK-02: the relay never
// shares a process with the request path).
type Role string

const (
	RoleAPI      Role = "api"
	RoleRelay    Role = "relay"
	RoleConsumer Role = "consumer"
)

// Roles lists the accepted values of --role; the consumer reads the context's
// own channel, where the provisioned invitations arrive.
var Roles = []Role{RoleAPI, RoleRelay, RoleConsumer}

var (
	ErrUnknownRole     = errors.New("iam: unknown role")
	ErrMissingVariable = errors.New("iam: required variable is not set")
	ErrInvalidVariable = envconfig.ErrInvalidVariable
	ErrInvalidPolicy   = errors.New("iam: invalid idempotency or purge policy")
)

const (
	envDSN           = "PG_DSN"
	envMigrateDSN    = "PG_MIGRATE_DSN"
	envMigrate       = "MIGRATE"
	envBrokers       = "KAFKA_BROKERS"
	envKafkaInsecure = "KAFKA_INSECURE"
	envGroup         = "KAFKA_GROUP"

	envIamTopic = "KAFKA_IAM_TOPIC"
	envIamDLQ   = "KAFKA_IAM_DLQ"

	envOperatorTenant = "IAM_OPERATOR_TENANT"

	envMetricTenants = "METRIC_TENANTS"
)

// Config is every operational value the two roles need, resolved once at
// startup from the environment.
type Config struct {
	Role Role

	DSN        string
	MigrateDSN string
	Migrate    bool

	API           kernelgrpc.APIEnv
	Admission     admission.Limit
	MetricTenants []string

	Brokers       []string
	KafkaInsecure bool

	// KafkaAuth is the principal this process presents to the
	// broker, required whenever TLS is on (IDN-04).
	KafkaAuth kafka.ClientAuth
	Group     string

	IamTopic string
	IamDLQ   string

	// OperatorTenant is the reserved tenant of the marketplace: buyers live in
	// it, and roles, invitations and memberships live in any other.
	OperatorTenant string

	// Wait is the INB-17 ceiling of a delivered message; ConsumerTimeout is the
	// consumer's own time policy, which CTX-28 makes mandatory per attempt.
	Wait            time.Duration
	ConsumerTimeout time.Duration

	Signals boot.Signals

	Service string

	Relay relay.Config

	Policies kernelapp.Policies
}

// Defaults are the values a role runs with when the environment says nothing.
func Defaults(role Role) Config {
	return Config{
		Role:    role,
		API:     kernelgrpc.APIEnv{GRPCAddr: ":9090"},
		Service: "iam",
		Relay: relay.Config{
			Source:         "urn:dmpf:reference-iam",
			Interval:       500 * time.Millisecond,
			BatchSize:      50,
			Lease:          30 * time.Second,
			Concurrency:    4,
			MaxAttempts:    10,
			BackoffBase:    200 * time.Millisecond,
			BackoffCeiling: 30 * time.Second,
			ShutdownGrace:  observability.ShutdownGrace,
			System:         semconv.MessagingSystemKafka.Value.AsString(),
		},
		Admission:       admission.Limit{PerSecond: 50, Burst: 100, Concurrency: 32},
		Wait:            2 * time.Second,
		ConsumerTimeout: 30 * time.Second,
		Policies: kernelapp.Policies{
			IdempotencyWait:      time.Second,
			IdempotencyRetention: 24 * time.Hour,
			OutboxRetention:      168 * time.Hour,
			InboxRetention:       192 * time.Hour,
			PurgeInterval:        15 * time.Minute,
			PurgeBatch:           1000,
		},
	}
}

// FromEnv reads the variables through lookup, applies Defaults and validates
// for the role.
func FromEnv(role Role, lookup func(string) string) (Config, error) {
	cfg := Defaults(role)

	var err error
	cfg.DSN = lookup(envDSN)
	if cfg.API, err = kernelgrpc.ReadAPIEnv(lookup, cfg.API.GRPCAddr); err != nil {
		return Config{}, err
	}
	cfg.Brokers = envconfig.SplitList(lookup(envBrokers))
	cfg.KafkaAuth = kafka.ReadClientAuth(lookup)
	cfg.MetricTenants = envconfig.SplitList(lookup(envMetricTenants))
	cfg.IamTopic = lookup(envIamTopic)
	cfg.IamDLQ = lookup(envIamDLQ)
	cfg.Group = lookup(envGroup)
	if role == RoleAPI || role == RoleConsumer {
		cfg.OperatorTenant = lookup(envOperatorTenant)
	}

	flags := []struct {
		variable string
		into     *bool
	}{
		{envKafkaInsecure, &cfg.KafkaInsecure},
		{envMigrate, &cfg.Migrate},
	}
	for _, flag := range flags {
		if *flag.into, err = envconfig.ParseBool(flag.variable, lookup(flag.variable)); err != nil {
			return Config{}, err
		}
	}
	if role == RoleAPI && cfg.Migrate {
		cfg.MigrateDSN = lookup(envMigrateDSN)
	}

	if cfg.Signals, err = boot.SignalsFromEnv(lookup); err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate refuses a configuration the role could not start with, naming the
// variable that is missing.
func (c Config) Validate() error {
	missing, err := c.requirements()
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", ErrMissingVariable, missing[0])
	}
	if err := c.Policies.Validate(true, ErrInvalidPolicy); err != nil {
		return err
	}
	// INB-14: an entry purged before the broker stops redelivering its message
	// would let that message through as new.
	if window := MembershipChannel(c).Redelivery.UpperBound; c.Role == RoleConsumer && c.Policies.InboxRetention < window {
		return fmt.Errorf("%w: InboxRetention %v is below the redelivery window %v (INB-14)", ErrInvalidPolicy, c.Policies.InboxRetention, window)
	}
	return c.Relay.Validate()
}

func (c Config) requirements() ([]string, error) {
	var missing []string
	require := func(variable string, absent bool) {
		if absent {
			missing = append(missing, variable)
		}
	}
	require(envDSN, c.DSN == "")
	switch c.Role {
	case RoleAPI:
		require(envMigrateDSN, c.Migrate && c.MigrateDSN == "")
		missing = append(missing, c.API.Missing()...)
		require(envOperatorTenant, c.OperatorTenant == "")
		return missing, nil
	case RoleRelay:
		require(envBrokers, len(c.Brokers) == 0)
		require(envIamTopic, c.IamTopic == "")
		require(envIamDLQ, c.IamDLQ == "")
		require(envGroup, c.Group == "")
		return append(missing, c.KafkaAuth.Missing(c.KafkaInsecure)...), nil
	case RoleConsumer:
		require(envBrokers, len(c.Brokers) == 0)
		require(envIamTopic, c.IamTopic == "")
		require(envIamDLQ, c.IamDLQ == "")
		require(envGroup, c.Group == "")
		require(envOperatorTenant, c.OperatorTenant == "")
		return append(missing, c.KafkaAuth.Missing(c.KafkaInsecure)...), nil
	default:
		return nil, fmt.Errorf("%w: %q (use one of %s)", ErrUnknownRole, c.Role, roleList())
	}
}

func roleList() string {
	names := make([]string, len(Roles))
	for i, role := range Roles {
		names[i] = string(role)
	}
	return strings.Join(names, "|")
}

func apiRole(cfg Config) serve.APIRole {
	return serve.APIRole{DSN: cfg.DSN, MigrateDSN: cfg.MigrateDSN, Server: cfg.API, Migrate: cfg.Migrate, Admission: cfg.Admission, MetricTenants: cfg.MetricTenants, Policies: cfg.Policies}
}

func relayRole(cfg Config) serve.RelayRole {
	return serve.RelayRole{DSN: cfg.DSN, Brokers: cfg.Brokers, KafkaInsecure: cfg.KafkaInsecure, KafkaAuth: cfg.KafkaAuth, Relay: cfg.Relay, Policies: cfg.Policies}
}
