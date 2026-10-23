//go:build integration

package provider_test

import (
	"context"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/clock"
	testids "github.com/mateusmacedo/dmpf/libs/backend/go/testkit/ids"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/pg"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/appkit"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/provider"
)

const (
	e2eRef      = domain.StoreRef("loja-e2e")
	e2eOccurred = ports.Instant(1_755_432_000_000_000_000)
)

func TestTheLifecycleOfAStoreEndToEnd(t *testing.T) {
	h := appkit.NewTenancy(t, clock.New(e2eOccurred), &testids.Sequence{Prefix: "m-"})
	pool, service := h.Pool, h.Service
	ctx := context.Background()
	reader := provider.NewStoreReader(postgres.NewReadPool(pool))

	outcome, err := service.RegisterStore(withExecution(t, ctx), application.RegisterStore{Ref: e2eRef, Name: "Loja E2E"})
	if _, refused := outcome.Rejection(); err != nil || refused {
		t.Fatalf("RegisterStore() = (%+v, %v), want accepted", outcome, err)
	}

	t.Run("the store is persisted active", func(t *testing.T) {
		snap, version, err := reader.Load(withExecution(t, ctx), e2eRef)
		want := domain.StoreSnapshot{Ref: e2eRef, Name: "Loja E2E", Status: domain.Active, RegisteredAt: e2eOccurred, StatusChangedAt: e2eOccurred}
		if err != nil || version != 1 || !snap.Equal(want) {
			t.Fatalf("Load() = %+v v%d, %v; want %+v v1", snap, version, err, want)
		}
	})

	t.Run("the outbox row lands at version 1", func(t *testing.T) {
		want := pg.Enqueued{
			MessageID:        "m-000001",
			MessageType:      "com.company.tenancy.store-registered.v1",
			SchemaVersion:    "type.googleapis.com/company.tenancy.event.v1.StoreRegistered",
			AggregateVersion: 1,
			Destination:      "tenancy.events",
			Status:           "pending",
		}
		if outbox := h.Outbox(t); len(outbox) != 1 || outbox[0] != want {
			t.Fatalf("outbox = %+v, want [%+v]", outbox, want)
		}
	})

	t.Run("suspend and reactivate each commit one event with the state", func(t *testing.T) {
		if _, err := service.SuspendStore(withExecution(t, ctx), application.SuspendStore{Ref: e2eRef, Reason: "fraude em análise"}); err != nil {
			t.Fatalf("SuspendStore() = %v", err)
		}
		if snap, version, err := reader.Load(withExecution(t, ctx), e2eRef); err != nil || version != 2 || snap.Status != domain.Suspended || snap.SuspensionReason != "fraude em análise" {
			t.Fatalf("Load() = %+v v%d, %v; want suspended at v2", snap, version, err)
		}
		if _, err := service.ReactivateStore(withExecution(t, ctx), application.ReactivateStore{Ref: e2eRef}); err != nil {
			t.Fatalf("ReactivateStore() = %v", err)
		}
		if snap, version, err := reader.Load(withExecution(t, ctx), e2eRef); err != nil || version != 3 || snap.Status != domain.Active || snap.SuspensionReason != "" {
			t.Fatalf("Load() = %+v v%d, %v; want active without a reason at v3", snap, version, err)
		}

		outbox := h.Outbox(t)
		if len(outbox) != 3 || outbox[1].MessageType != "com.company.tenancy.store-suspended.v1" || outbox[2].MessageType != "com.company.tenancy.store-reactivated.v1" {
			t.Fatalf("outbox = %+v, want registered, suspended and reactivated in order", outbox)
		}
	})
}

func TestTheStoresOfOneStatusAreFoundOutsideAnyUnitOfWork(t *testing.T) {
	h := appkit.NewTenancy(t, clock.New(e2eOccurred), &testids.Sequence{Prefix: "m-"})
	for _, ref := range []domain.StoreRef{"loja-a", "loja-b", "loja-c"} {
		if _, err := h.Service.RegisterStore(withExecution(t, context.Background()), application.RegisterStore{Ref: ref, Name: domain.StoreName("Loja " + ref)}); err != nil {
			t.Fatalf("RegisterStore(%s) = %v", ref, err)
		}
	}
	if _, err := h.Service.SuspendStore(withExecution(t, context.Background()), application.SuspendStore{Ref: "loja-c", Reason: "motivo"}); err != nil {
		t.Fatalf("SuspendStore() = %v", err)
	}

	active, err := h.Service.FindStoresByStatus(withExecution(t, context.Background()), domain.Active)

	if err != nil || len(active) != 2 {
		t.Fatalf("FindStoresByStatus(active) = %+v, %v; want exactly the two active stores", active, err)
	}
}
