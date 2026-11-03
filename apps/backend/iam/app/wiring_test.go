package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/otelboot"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/obstest"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/channel"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/app"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

func TestTheAuditTrailOfTheServiceReachesTheLoggerProviderOfTheRuntime(t *testing.T) {
	obstest.AuditTrail(t, "iam", func(ctx context.Context, rt *otelboot.Runtime, pool *pgxpool.Pool) error {
		service, err := app.NewIamService(pool, rt, app.Defaults(app.RoleAPI))
		if err != nil {
			return err
		}
		service.Instrumentation.Audit(ctx, ports.AuditEvent{Object: "papel-9", Action: "read", Outcome: ports.OutcomeAccepted})
		return nil
	})
}

func TestTheRelayPublishesThroughKafka(t *testing.T) {
	if system := app.Defaults(app.RoleRelay).Relay.System; system != "kafka" {
		t.Fatalf("Relay.System = %q, want %q", system, "kafka")
	}
}

func TestTheRelayAddressesTheChannelByItsPhysicalTopic(t *testing.T) {
	cfg := app.Defaults(app.RoleRelay)
	cfg.IamTopic, cfg.IamDLQ, cfg.Group = "iam.v1", "iam.v1.dlq", "iam-relay"
	catalog, err := channel.NewCatalog(app.IamChannel(cfg))
	if err != nil {
		t.Fatalf("NewCatalog() = %v, want nil", err)
	}

	if topic := catalog.AddressOf(application.Destination); topic != cfg.IamTopic {
		t.Fatalf("AddressOf(%q) = %q, want the physical topic %q (RF-B7)", application.Destination, topic, cfg.IamTopic)
	}
	if topic := catalog.AddressOf("uncatalogued.events"); topic != "" {
		t.Fatalf("AddressOf(uncatalogued.events) = %q, want empty: the send never names a logical channel", topic)
	}
}

func keycloakConfig(operator string) app.Config {
	cfg := app.Defaults(app.RoleConsumer)
	cfg.OperatorTenant = operator
	cfg.Keycloak = app.KeycloakEnv{URL: "http://keycloak:8080", Realm: "dmpf", ClientID: "iam-admin", Secret: provider.NewKeycloakSecret("s3cr3t")}
	return cfg
}

func TestNewKeycloakRefusesAnEnvironmentThatLeavesTheAdapterOff(t *testing.T) {
	k, err := app.NewKeycloak(app.Defaults(app.RoleConsumer), provider.KeycloakDeps{Clock: clock.System()})

	if !errors.Is(err, app.ErrMissingVariable) || !strings.Contains(err.Error(), "KEYCLOAK_IAM_") || k != nil {
		t.Fatalf("NewKeycloak() = %v, %v; want no partner and ErrMissingVariable naming KEYCLOAK_IAM_*", k, err)
	}
}

func TestNewKeycloakAssemblesAPartnerUnderAValidProfile(t *testing.T) {
	k, err := app.NewKeycloak(keycloakConfig("marketplace"), provider.KeycloakDeps{Clock: clock.System()})

	if err != nil || k == nil {
		t.Fatalf("NewKeycloak() = %v, %v; want the partner", k, err)
	}
	if err := usecase.ValidateExternalProfile(k.Profile()); err != nil {
		t.Fatalf("ValidateExternalProfile() = %v, want nil", err)
	}
}

func TestNewKeycloakRefusesAPartnerWithoutTheOperatorTenant(t *testing.T) {
	k, err := app.NewKeycloak(keycloakConfig(""), provider.KeycloakDeps{Clock: clock.System()})

	if !errors.Is(err, provider.ErrKeycloakConfig) || k != nil {
		t.Fatalf("NewKeycloak() = %v, %v; want ErrKeycloakConfig: the operator tenant fences the commands", k, err)
	}
}
