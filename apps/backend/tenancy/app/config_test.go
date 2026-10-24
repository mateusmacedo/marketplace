package app_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	kernelapp "github.com/mateusmacedo/dmpf/libs/backend/go/app"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/app"
)

func lookup(pairs ...string) func(string) string {
	values := map[string]string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		values[pairs[i]] = pairs[i+1]
	}
	return func(name string) string { return values[name] }
}

func TestTheAPIRunsWithItsDefaults(t *testing.T) {
	cfg, err := app.FromEnv(app.RoleAPI, lookup("PG_DSN", "postgres://x", "GRPC_INSECURE", "true", "TENANCY_OPERATOR_TENANT", "marketplace"))

	if err != nil {
		t.Fatalf("FromEnv() = %v, want nil", err)
	}
	if cfg.API.GRPCAddr != ":9090" || cfg.Service != "tenancy" || !cfg.API.GRPCInsecure {
		t.Fatalf("cfg = %+v, want :9090, tenancy, insecure", cfg)
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

	if want := "tenancy: invalid idempotency or purge policy: PurgeBatch 0"; !errors.Is(err, app.ErrInvalidPolicy) || err.Error() != want {
		t.Fatalf("Validate() = %v, want %q", err, want)
	}
}

func TestTheRelayNamesEachMissingVariable(t *testing.T) {
	full := []string{
		"PG_DSN", "postgres://x", "KAFKA_BROKERS", "b:9092", "KAFKA_SASL_MECHANISM", "SCRAM-SHA-256",
		"KAFKA_TENANCY_TOPIC", "tenancy", "KAFKA_TENANCY_DLQ", "tenancy.dlq", "KAFKA_GROUP", "g",
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

func TestTenancyRefusesTheConsumerRole(t *testing.T) {
	_, err := app.FromEnv("consumer", lookup("PG_DSN", "postgres://x"))

	if !errors.Is(err, app.ErrUnknownRole) || !strings.Contains(err.Error(), "consumer") || !strings.Contains(err.Error(), "api|relay") {
		t.Fatalf("FromEnv(consumer) = %v, want a refusal naming the role and api|relay", err)
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
		PurgeInterval:        15 * time.Minute,
		PurgeBatch:           1000,
	}
	if got := app.Defaults(app.RoleAPI).Policies; got != want {
		t.Fatalf("Policies = %+v, want %+v", got, want)
	}
}

func TestTheIdentityOfTheProcessIsLeftToTheEnvironment(t *testing.T) {
	cfg, err := app.FromEnv(app.RoleAPI, lookup("PG_DSN", "postgres://x", "GRPC_INSECURE", "true", "TENANCY_OPERATOR_TENANT", "marketplace"))
	if err != nil {
		t.Fatalf("FromEnv() = %v, want nil", err)
	}

	if telemetry := app.TelemetryOf(cfg); telemetry.Version != "" || telemetry.Instance != "" {
		t.Fatalf("TelemetryOf() declares version %q and instance %q, want neither: both come from OTEL_RESOURCE_ATTRIBUTES (RF-E3)", telemetry.Version, telemetry.Instance)
	}
}

func TestFromEnvHandsTheMigratingDSNToTheAPIOnly(t *testing.T) {
	const migrator = "postgres://tenancy_migrator:secret@db/tenancy"
	api, err := app.FromEnv(app.RoleAPI, lookup("PG_DSN", "postgres://x", "MIGRATE", "true", "PG_MIGRATE_DSN", migrator, "GRPC_INSECURE", "true", "TENANCY_OPERATOR_TENANT", "marketplace"))
	if err != nil {
		t.Fatalf("FromEnv(api) = %v, want nil", err)
	}
	if api.MigrateDSN != migrator {
		t.Fatalf("FromEnv(api).MigrateDSN = %q, want %q", api.MigrateDSN, migrator)
	}

	relay, err := app.FromEnv(app.RoleRelay, lookup("PG_DSN", "postgres://x", "MIGRATE", "true", "PG_MIGRATE_DSN", migrator,
		"KAFKA_BROKERS", "b:9092", "KAFKA_INSECURE", "true", "KAFKA_TENANCY_TOPIC", "t", "KAFKA_TENANCY_DLQ", "d", "KAFKA_GROUP", "g"))
	if err != nil {
		t.Fatalf("FromEnv(relay) = %v, want nil", err)
	}
	if relay.MigrateDSN != "" {
		t.Fatalf("FromEnv(relay).MigrateDSN = %q, want it never read by a role that does not migrate", relay.MigrateDSN)
	}
}

func TestTheAPIReadsTheMigratingDSNOnlyWhenItMigrates(t *testing.T) {
	cfg, err := app.FromEnv(app.RoleAPI, lookup("PG_DSN", "postgres://x", "PG_MIGRATE_DSN", "postgres://tenancy_migrator:secret@db/tenancy", "GRPC_INSECURE", "true", "TENANCY_OPERATOR_TENANT", "marketplace"))
	if err != nil {
		t.Fatalf("FromEnv() = %v, want nil", err)
	}
	if cfg.MigrateDSN != "" {
		t.Fatalf("FromEnv().MigrateDSN = %q without MIGRATE, want it left unread", cfg.MigrateDSN)
	}
}

func TestTheMigratingAPIRequiresTheMigratingDSN(t *testing.T) {
	_, err := app.FromEnv(app.RoleAPI, lookup("PG_DSN", "postgres://x", "MIGRATE", "true", "GRPC_INSECURE", "true", "TENANCY_OPERATOR_TENANT", "marketplace"))

	if !errors.Is(err, app.ErrMissingVariable) || !strings.Contains(err.Error(), "PG_MIGRATE_DSN") {
		t.Fatalf("FromEnv() = %v, want the missing PG_MIGRATE_DSN named", err)
	}
}

func TestTheAPIRequiresTheOperatorTenant(t *testing.T) {
	_, err := app.FromEnv(app.RoleAPI, lookup("PG_DSN", "postgres://x", "GRPC_INSECURE", "true"))

	if !errors.Is(err, app.ErrMissingVariable) || !strings.Contains(err.Error(), "TENANCY_OPERATOR_TENANT") {
		t.Fatalf("FromEnv() = %v, want the missing TENANCY_OPERATOR_TENANT named", err)
	}
}

func TestTheAPIReadsTheOperatorTenant(t *testing.T) {
	cfg, err := app.FromEnv(app.RoleAPI, lookup("PG_DSN", "postgres://x", "GRPC_INSECURE", "true", "TENANCY_OPERATOR_TENANT", "marketplace"))

	if err != nil || cfg.OperatorTenant != "marketplace" {
		t.Fatalf("FromEnv() = %+v, %v; want the operator tenant marketplace", cfg.OperatorTenant, err)
	}
}

func TestTheRelayNeitherRequiresNorReadsTheOperatorTenant(t *testing.T) {
	cfg, err := app.FromEnv(app.RoleRelay, lookup("PG_DSN", "postgres://x", "KAFKA_BROKERS", "b:9092", "KAFKA_INSECURE", "true",
		"KAFKA_TENANCY_TOPIC", "t", "KAFKA_TENANCY_DLQ", "d", "KAFKA_GROUP", "g", "TENANCY_OPERATOR_TENANT", "marketplace"))

	if err != nil || cfg.OperatorTenant != "" {
		t.Fatalf("FromEnv(relay) = %q, %v; want it never read by a role that serves no command", cfg.OperatorTenant, err)
	}
}
