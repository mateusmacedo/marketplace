package app_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	kernelapp "github.com/mateusmacedo/dmpf/libs/backend/go/app"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/app"
)

func lookup(pairs ...string) func(string) string {
	values := map[string]string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		values[pairs[i]] = pairs[i+1]
	}
	return func(name string) string { return values[name] }
}

func TestTheAPIRunsWithItsDefaults(t *testing.T) {
	cfg, err := app.FromEnv(app.RoleAPI, lookup("PG_DSN", "postgres://x", "GRPC_INSECURE", "true", "IAM_OPERATOR_TENANT", "marketplace"))

	if err != nil {
		t.Fatalf("FromEnv() = %v, want nil", err)
	}
	if cfg.API.GRPCAddr != ":9090" || cfg.Service != "iam" || !cfg.API.GRPCInsecure {
		t.Fatalf("cfg = %+v, want :9090, iam, insecure", cfg)
	}
	if cfg.Relay.ShutdownGrace != observability.ShutdownGrace {
		t.Fatalf("Relay.ShutdownGrace = %v, want the kernel's %v", cfg.Relay.ShutdownGrace, observability.ShutdownGrace)
	}
}

func TestTheAPIRequiresATransportPolicy(t *testing.T) {
	_, err := app.FromEnv(app.RoleAPI, lookup("PG_DSN", "postgres://x"))

	if !errors.Is(err, app.ErrMissingVariable) || !strings.Contains(err.Error(), "GRPC_INSECURE") {
		t.Fatalf("FromEnv() = %v, want the missing transport policy named", err)
	}
}

func TestTheAPIRefusesANonPositivePurgeBatchUnderItsOwnSentinel(t *testing.T) {
	cfg := app.Defaults(app.RoleAPI)
	cfg.DSN, cfg.API.GRPCInsecure, cfg.OperatorTenant = "postgres://x", true, "marketplace"
	cfg.Policies.PurgeBatch = 0

	err := cfg.Validate()

	if want := "iam: invalid idempotency or purge policy: PurgeBatch 0"; !errors.Is(err, app.ErrInvalidPolicy) || err.Error() != want {
		t.Fatalf("Validate() = %v, want %q", err, want)
	}
}

func TestTheRelayNamesEachMissingVariable(t *testing.T) {
	full := []string{
		"PG_DSN", "postgres://x", "KAFKA_BROKERS", "b:9092", "KAFKA_SASL_MECHANISM", "SCRAM-SHA-256",
		"KAFKA_IAM_TOPIC", "iam", "KAFKA_IAM_DLQ", "iam.dlq", "KAFKA_GROUP", "g",
	}
	if _, err := app.FromEnv(app.RoleRelay, lookup(full...)); err != nil {
		t.Fatalf("FromEnv(full) = %v, want nil", err)
	}
	for i := 0; i < len(full); i += 2 {
		variable := full[i]
		t.Run("without "+variable, func(t *testing.T) {
			pairs := append(append([]string{}, full[:i]...), full[i+2:]...)

			_, err := app.FromEnv(app.RoleRelay, lookup(pairs...))

			if !errors.Is(err, app.ErrMissingVariable) || !strings.Contains(err.Error(), variable) {
				t.Fatalf("FromEnv() = %v, want %s named", err, variable)
			}
		})
	}
}

func TestIamRefusesARoleItDoesNotHave(t *testing.T) {
	_, err := app.FromEnv("buyer", lookup("PG_DSN", "postgres://x"))

	if !errors.Is(err, app.ErrUnknownRole) || !strings.Contains(err.Error(), "buyer") || !strings.Contains(err.Error(), "api|relay|consumer") {
		t.Fatalf("FromEnv(buyer) = %v, want a refusal naming the role and api|relay|consumer", err)
	}
}

var consumerEnv = []string{
	"PG_DSN", "postgres://x", "KAFKA_BROKERS", "b:9092", "KAFKA_SASL_MECHANISM", "SCRAM-SHA-256",
	"KAFKA_IAM_TOPIC", "iam.events", "KAFKA_IAM_DLQ", "iam.events.dlq", "KAFKA_GROUP", "iam-membership",
	"IAM_OPERATOR_TENANT", "marketplace",
}

func TestTheConsumerNamesEachMissingVariable(t *testing.T) {
	if _, err := app.FromEnv(app.RoleConsumer, lookup(consumerEnv...)); err != nil {
		t.Fatalf("FromEnv(full) = %v, want nil", err)
	}
	for i := 0; i < len(consumerEnv); i += 2 {
		variable := consumerEnv[i]
		t.Run("without "+variable, func(t *testing.T) {
			pairs := append(append([]string{}, consumerEnv[:i]...), consumerEnv[i+2:]...)

			_, err := app.FromEnv(app.RoleConsumer, lookup(pairs...))

			if !errors.Is(err, app.ErrMissingVariable) || !strings.Contains(err.Error(), variable) {
				t.Fatalf("FromEnv() = %v, want %s named", err, variable)
			}
		})
	}
}

func TestTheConsumerReadsTheOperatorTenant(t *testing.T) {
	cfg, err := app.FromEnv(app.RoleConsumer, lookup(consumerEnv...))

	if err != nil || cfg.OperatorTenant != "marketplace" || cfg.Group != "iam-membership" {
		t.Fatalf("FromEnv(consumer) = %+v, %v; want the operator tenant and the group of the membership consumer", cfg, err)
	}
}

// INB-14: an inbox entry purged before the broker stops redelivering its
// message would let that message through as new.
func TestTheConsumerRefusesAnInboxRetentionBelowTheRedeliveryWindow(t *testing.T) {
	cfg, err := app.FromEnv(app.RoleConsumer, lookup(consumerEnv...))
	if err != nil {
		t.Fatalf("FromEnv() = %v", err)
	}
	cfg.Policies.InboxRetention = time.Hour

	if err := cfg.Validate(); !errors.Is(err, app.ErrInvalidPolicy) || !strings.Contains(err.Error(), "INB-14") {
		t.Fatalf("Validate() = %v, want the redelivery window named under ErrInvalidPolicy", err)
	}
}

func TestDefaultsAreTheValuesOfARoleWithoutEnvironment(t *testing.T) {
	cfg := app.Defaults(app.RoleRelay)

	if cfg.Role != app.RoleRelay || cfg.API.GRPCAddr != ":9090" || cfg.Relay.BatchSize == 0 {
		t.Fatalf("Defaults(relay) = %+v, want the role, :9090 and a relay configuration", cfg)
	}
}

func TestTheDefaultsKeepCommandsAndThePurgeWithinTheirRetention(t *testing.T) {
	want := kernelapp.Policies{
		IdempotencyWait:      time.Second,
		IdempotencyRetention: 24 * time.Hour,
		OutboxRetention:      168 * time.Hour,
		InboxRetention:       192 * time.Hour,
		PurgeInterval:        15 * time.Minute,
		PurgeBatch:           1000,
	}
	if got := app.Defaults(app.RoleAPI).Policies; got != want {
		t.Fatalf("Policies = %+v, want %+v", got, want)
	}
}

func TestTheIdentityOfTheProcessIsLeftToTheEnvironment(t *testing.T) {
	cfg, err := app.FromEnv(app.RoleAPI, lookup("PG_DSN", "postgres://x", "GRPC_INSECURE", "true", "IAM_OPERATOR_TENANT", "marketplace"))
	if err != nil {
		t.Fatalf("FromEnv() = %v, want nil", err)
	}

	if telemetry := app.TelemetryOf(cfg); telemetry.Version != "" || telemetry.Instance != "" {
		t.Fatalf("TelemetryOf() declares version %q and instance %q, want neither: both come from OTEL_RESOURCE_ATTRIBUTES (RF-E3)", telemetry.Version, telemetry.Instance)
	}
}

func TestFromEnvHandsTheMigratingDSNToTheAPIOnly(t *testing.T) {
	const migrator = "postgres://iam_migrator:secret@db/iam"
	api, err := app.FromEnv(app.RoleAPI, lookup("PG_DSN", "postgres://x", "MIGRATE", "true", "PG_MIGRATE_DSN", migrator, "GRPC_INSECURE", "true", "IAM_OPERATOR_TENANT", "marketplace"))
	if err != nil {
		t.Fatalf("FromEnv(api) = %v, want nil", err)
	}
	if api.MigrateDSN != migrator {
		t.Fatalf("FromEnv(api).MigrateDSN = %q, want %q", api.MigrateDSN, migrator)
	}

	relay, err := app.FromEnv(app.RoleRelay, lookup("PG_DSN", "postgres://x", "MIGRATE", "true", "PG_MIGRATE_DSN", migrator,
		"KAFKA_BROKERS", "b:9092", "KAFKA_INSECURE", "true", "KAFKA_IAM_TOPIC", "t", "KAFKA_IAM_DLQ", "d", "KAFKA_GROUP", "g"))
	if err != nil {
		t.Fatalf("FromEnv(relay) = %v, want nil", err)
	}
	if relay.MigrateDSN != "" {
		t.Fatalf("FromEnv(relay).MigrateDSN = %q, want it never read by a role that does not migrate", relay.MigrateDSN)
	}
}

func TestTheAPIReadsTheMigratingDSNOnlyWhenItMigrates(t *testing.T) {
	cfg, err := app.FromEnv(app.RoleAPI, lookup("PG_DSN", "postgres://x", "PG_MIGRATE_DSN", "postgres://iam_migrator:secret@db/iam", "GRPC_INSECURE", "true", "IAM_OPERATOR_TENANT", "marketplace"))
	if err != nil {
		t.Fatalf("FromEnv() = %v, want nil", err)
	}
	if cfg.MigrateDSN != "" {
		t.Fatalf("FromEnv().MigrateDSN = %q without MIGRATE, want it left unread", cfg.MigrateDSN)
	}
}

func TestTheMigratingAPIRequiresTheMigratingDSN(t *testing.T) {
	_, err := app.FromEnv(app.RoleAPI, lookup("PG_DSN", "postgres://x", "MIGRATE", "true", "GRPC_INSECURE", "true", "IAM_OPERATOR_TENANT", "marketplace"))

	if !errors.Is(err, app.ErrMissingVariable) || !strings.Contains(err.Error(), "PG_MIGRATE_DSN") {
		t.Fatalf("FromEnv() = %v, want the missing PG_MIGRATE_DSN named", err)
	}
}

func TestTheAPIRequiresTheOperatorTenant(t *testing.T) {
	_, err := app.FromEnv(app.RoleAPI, lookup("PG_DSN", "postgres://x", "GRPC_INSECURE", "true"))

	if !errors.Is(err, app.ErrMissingVariable) || !strings.Contains(err.Error(), "IAM_OPERATOR_TENANT") {
		t.Fatalf("FromEnv() = %v, want the missing IAM_OPERATOR_TENANT named", err)
	}
}

func TestTheAPIReadsTheOperatorTenant(t *testing.T) {
	cfg, err := app.FromEnv(app.RoleAPI, lookup("PG_DSN", "postgres://x", "GRPC_INSECURE", "true", "IAM_OPERATOR_TENANT", "marketplace"))

	if err != nil || cfg.OperatorTenant != "marketplace" {
		t.Fatalf("FromEnv() = %+v, %v; want the operator tenant marketplace", cfg.OperatorTenant, err)
	}
}

func TestTheRelayNeitherRequiresNorReadsTheOperatorTenant(t *testing.T) {
	cfg, err := app.FromEnv(app.RoleRelay, lookup("PG_DSN", "postgres://x", "KAFKA_BROKERS", "b:9092", "KAFKA_INSECURE", "true",
		"KAFKA_IAM_TOPIC", "t", "KAFKA_IAM_DLQ", "d", "KAFKA_GROUP", "g", "IAM_OPERATOR_TENANT", "marketplace"))

	if err != nil || cfg.OperatorTenant != "" {
		t.Fatalf("FromEnv(relay) = %q, %v; want it never read by a role that serves no command", cfg.OperatorTenant, err)
	}
}
