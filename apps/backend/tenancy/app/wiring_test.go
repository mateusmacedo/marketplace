package app_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/otelboot"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/obstest"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/channel"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/app"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
)

func TestTheAuditTrailOfTheServiceReachesTheLoggerProviderOfTheRuntime(t *testing.T) {
	obstest.AuditTrail(t, "tenancy", func(ctx context.Context, rt *otelboot.Runtime, pool *pgxpool.Pool) error {
		service, err := app.NewTenancyService(pool, rt, app.Defaults(app.RoleAPI))
		if err != nil {
			return err
		}
		service.Instrumentation.Audit(ctx, ports.AuditEvent{Object: "loja-9", Action: "read", Outcome: ports.OutcomeAccepted})
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
	cfg.TenancyTopic, cfg.TenancyDLQ, cfg.Group = "tenancy.v1", "tenancy.v1.dlq", "tenancy-relay"
	catalog, err := channel.NewCatalog(app.TenancyChannel(cfg))
	if err != nil {
		t.Fatalf("NewCatalog() = %v, want nil", err)
	}

	if topic := catalog.AddressOf(application.Destination); topic != cfg.TenancyTopic {
		t.Fatalf("AddressOf(%q) = %q, want the physical topic %q (RF-B7)", application.Destination, topic, cfg.TenancyTopic)
	}
	if topic := catalog.AddressOf("uncatalogued.events"); topic != "" {
		t.Fatalf("AddressOf(uncatalogued.events) = %q, want empty: the send never names a logical channel", topic)
	}
}
