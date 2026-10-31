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

	"github.com/mateusmacedo/marketplace/apps/backend/iam/appkit"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

const (
	e2eRef      = domain.RoleRef("expedicao")
	e2eOccurred = ports.Instant(1_755_432_000_000_000_000)
)

func TestTheLifecycleOfARoleEndToEnd(t *testing.T) {
	h := appkit.NewIam(t, clock.New(e2eOccurred), &testids.Sequence{Prefix: "m-"})
	pool, service := h.Pool, h.Service
	ctx := context.Background()
	reader := provider.NewRoleReader(postgres.NewReadPool(pool))

	outcome, err := service.DefineRole(withExecution(t, ctx), application.DefineRole{
		Ref: e2eRef, Name: "Expedição", Permissions: []domain.Permission{"ordering:read", "fulfillment:write", "ordering:read"},
	})
	if _, refused := outcome.Rejection(); err != nil || refused {
		t.Fatalf("DefineRole() = (%+v, %v), want accepted", outcome, err)
	}

	t.Run("the role is persisted active with its permissions normalized", func(t *testing.T) {
		snap, version, err := reader.Load(withExecution(t, ctx), e2eRef)
		want := domain.RoleSnapshot{
			Ref: e2eRef, Name: "Expedição", Permissions: []domain.Permission{"fulfillment:write", "ordering:read"},
			Status: domain.RoleStatusActive, AccessVersion: 1, DefinedAt: e2eOccurred, ChangedAt: e2eOccurred,
		}
		if err != nil || version != 1 || !snap.Equal(want) {
			t.Fatalf("Load() = %+v v%d, %v; want %+v v1", snap, version, err, want)
		}
	})

	t.Run("the outbox row lands at version 1", func(t *testing.T) {
		want := pg.Enqueued{
			MessageID:        "m-000001",
			MessageType:      "com.company.iam.role-defined.v1",
			SchemaVersion:    "type.googleapis.com/company.iam.event.v1.RoleDefined",
			AggregateVersion: 1,
			Destination:      "iam.events",
			Status:           "pending",
		}
		if outbox := h.Outbox(t); len(outbox) != 1 || outbox[0] != want {
			t.Fatalf("outbox = %+v, want [%+v]", outbox, want)
		}
	})

	t.Run("change and retire each commit one event with the state", func(t *testing.T) {
		if _, err := service.ChangeRolePermissions(withExecution(t, ctx), application.ChangeRolePermissions{Ref: e2eRef, Permissions: []domain.Permission{"catalog:write"}}); err != nil {
			t.Fatalf("ChangeRolePermissions() = %v", err)
		}
		if snap, version, err := reader.Load(withExecution(t, ctx), e2eRef); err != nil || version != 2 || snap.AccessVersion != 2 || len(snap.Permissions) != 1 {
			t.Fatalf("Load() = %+v v%d, %v; want [catalog:write] at access version 2, v2", snap, version, err)
		}
		if _, err := service.RetireRole(withExecution(t, ctx), application.RetireRole{Ref: e2eRef}); err != nil {
			t.Fatalf("RetireRole() = %v", err)
		}
		if snap, version, err := reader.Load(withExecution(t, ctx), e2eRef); err != nil || version != 3 || snap.Status != domain.RoleStatusRetired || snap.AccessVersion != 3 {
			t.Fatalf("Load() = %+v v%d, %v; want retired at access version 3, v3", snap, version, err)
		}

		outbox := h.Outbox(t)
		if len(outbox) != 3 || outbox[1].MessageType != "com.company.iam.role-permissions-changed.v1" || outbox[2].MessageType != "com.company.iam.role-retired.v1" {
			t.Fatalf("outbox = %+v, want defined, permissions changed and retired in order", outbox)
		}
	})
}

func TestTheRolesOfOneStatusAreFoundOutsideAnyUnitOfWork(t *testing.T) {
	h := appkit.NewIam(t, clock.New(e2eOccurred), &testids.Sequence{Prefix: "m-"})
	for _, ref := range []domain.RoleRef{"papel-a", "papel-b", "papel-c"} {
		if _, err := h.Service.DefineRole(withExecution(t, context.Background()), application.DefineRole{Ref: ref, Name: domain.RoleName("Papel " + ref), Permissions: []domain.Permission{"catalog:read"}}); err != nil {
			t.Fatalf("DefineRole(%s) = %v", ref, err)
		}
	}
	if _, err := h.Service.RetireRole(withExecution(t, context.Background()), application.RetireRole{Ref: "papel-c"}); err != nil {
		t.Fatalf("RetireRole() = %v", err)
	}

	active, err := h.Service.FindRolesByStatus(withExecution(t, context.Background()), domain.RoleStatusActive)

	if err != nil || len(active) != 2 {
		t.Fatalf("FindRolesByStatus(active) = %+v, %v; want exactly the two active roles", active, err)
	}
}
