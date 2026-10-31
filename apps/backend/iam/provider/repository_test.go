//go:build integration

package provider_test

import (
	"context"
	"strconv"
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/providerkit"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/pg"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/appkit"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

func activeRole(marker int) domain.RoleSnapshot {
	return domain.RoleSnapshot{
		Name:          domain.RoleName("kit-name-" + strconv.Itoa(marker)),
		Permissions:   []domain.Permission{"catalog:read"},
		Status:        domain.RoleStatusActive,
		AccessVersion: 1,
		DefinedAt:     kernel.Instant(marker),
		ChangedAt:     kernel.Instant(marker),
	}
}

func TestRoleRepositoryConformsToTheKit(t *testing.T) {
	pool := appkit.OpenPool(t)
	v := providerkit.Repository(func() providerkit.RepositorySubject[domain.RoleRef, domain.RoleSnapshot] {
		pg.ResetTables(t, pool, appkit.Tables...)
		return providerkit.RepositorySubject[domain.RoleRef, domain.RoleSnapshot]{
			Within: func(ctx context.Context, fn func(context.Context, ports.Repository[domain.RoleRef, domain.RoleSnapshot]) error) error {
				return pg.Within(ctx, pool, provider.NewRoleRepository, fn)
			},
			Reader:           provider.NewRoleReader(postgres.NewReadPool(pool)),
			NewID:            func(n int) domain.RoleRef { return domain.RoleRef("kit-" + strconv.Itoa(n)) },
			NewState:         activeRole,
			Marker:           func(s domain.RoleSnapshot) int { return int(s.DefinedAt) },
			TenantUnresolved: postgres.ErrTenantUnresolved,
			Concurrent:       true,
		}
	})
	tb.Require(t, v)
	if len(v.Skipped) != 0 {
		t.Fatalf("postgres scopes by construction and runs transactions at once; nothing should be skipped: %v", v.Skipped)
	}
}

// The permissions, both versions and both instants in nanoseconds must survive
// the snapshot column and the typed status column alike.
func TestRoleRoundTripsTheWholeState(t *testing.T) {
	pool := appkit.OpenPool(t)
	ctx := withExecution(t, context.Background())
	reader := provider.NewRoleReader(postgres.NewReadPool(pool))
	active := domain.RoleSnapshot{
		Ref: "expedicao", Name: "Expedição", Permissions: []domain.Permission{"fulfillment:write", "ordering:read"},
		Status: domain.RoleStatusActive, AccessVersion: 1,
		DefinedAt: 1_755_432_000_000_000_001, ChangedAt: 1_755_432_000_000_000_001,
	}

	pg.Seed(t, ctx, pool, provider.NewRoleRepository, active.Ref, active)
	if got, version, err := reader.Load(ctx, active.Ref); err != nil || version != 1 || !got.Equal(active) {
		t.Fatalf("Load() = %+v v%d, %v; want %+v v1", got, version, err, active)
	}

	retired := active
	retired.Permissions = []domain.Permission{"catalog:write"}
	retired.Status, retired.AccessVersion, retired.SyncedVersion, retired.ChangedAt = domain.RoleStatusRetired, 3, 2, 1_755_435_600_000_000_002
	err := pg.Within(ctx, pool, provider.NewRoleRepository, func(ctx context.Context, repo ports.Repository[domain.RoleRef, domain.RoleSnapshot]) error {
		return repo.Save(ctx, active.Ref, retired, 1)
	})
	if err != nil {
		t.Fatalf("Save(update) = %v", err)
	}
	if got, version, err := reader.Load(ctx, active.Ref); err != nil || version != 2 || !got.Equal(retired) {
		t.Fatalf("Load() = %+v v%d, %v; want %+v v2", got, version, err, retired)
	}
}

func TestRoleRepositoryRefusesToStoreAnUndefinedStatus(t *testing.T) {
	pool := appkit.OpenPool(t)

	err := pg.Within(withExecution(t, context.Background()), pool, provider.NewRoleRepository, func(ctx context.Context, repo ports.Repository[domain.RoleRef, domain.RoleSnapshot]) error {
		return repo.Save(ctx, "expedicao", domain.RoleSnapshot{Ref: "expedicao", Name: "Expedição"}, 0)
	})

	if err == nil {
		t.Fatal("Save(undefined) = nil, want a refusal: only a decided status reaches the table")
	}
}

func TestRoleRepositoryHonoursTheUniqueKit(t *testing.T) {
	pool := appkit.OpenPool(t)
	v := providerkit.Unique(func() providerkit.UniqueSubject[domain.RoleRef, domain.RoleSnapshot] {
		pg.ResetTables(t, pool, appkit.Tables...)
		return providerkit.UniqueSubject[domain.RoleRef, domain.RoleSnapshot]{
			Within: func(ctx context.Context, fn func(ctx context.Context, w providerkit.UniqueWrites[domain.RoleRef, domain.RoleSnapshot]) error) error {
				uow := postgres.NewUnitOfWork(pool, func(tx *postgres.Tx) *postgres.Tx { return tx })
				return uow.Within(ctx, func(ctx context.Context, tx *postgres.Tx) error {
					return fn(ctx, providerkit.UniqueWrites[domain.RoleRef, domain.RoleSnapshot]{
						Repo: provider.NewRoleRepository(tx),
						Enqueue: func(ctx context.Context, n int) error {
							return tx.Outbox(provider.Mapper{}).Enqueue(ctx, roleDefinedEntry(n))
						},
					})
				})
			},
			Reader: provider.NewRoleReader(postgres.NewReadPool(pool)),
			NewID:  func(n int) domain.RoleRef { return domain.RoleRef("kit-" + strconv.Itoa(n)) },
			NewState: func(marker int, keys [][]string) domain.RoleSnapshot {
				s := activeRole(marker)
				s.Name = domain.RoleName(keys[0][0])
				return s
			},
			Marker:      func(s domain.RoleSnapshot) int { return int(s.DefinedAt) },
			Table:       "roles",
			Constraints: []providerkit.UniqueConstraint{{Name: "roles_tenant_id_name_key", Columns: []string{"tenant_id", "name"}, Code: domain.CodeRoleDuplicateName}},
			Concurrent:  true,
		}
	})
	tb.Require(t, v)
	if len(v.Skipped) != 3 {
		t.Fatalf("skipped = %v, want only the clauses that need a composite key or a second constraint", v.Skipped)
	}
}

func roleDefinedEntry(n int) ports.OutboxEntry {
	ref := domain.RoleRef("kit-event-" + strconv.Itoa(n))
	return ports.OutboxEntry{
		MessageID: ports.MessageID("evt-" + string(ref)), OccurredAt: 1,
		Intent:        ports.PublishIntent{Destination: "iam.events", PartitionKey: string(ref)},
		AggregateType: "iam.Role", AggregateID: string(ref), AggregateVersion: 1,
		Event: domain.RoleDefined{Ref: ref, Name: "Kit event", Permissions: []domain.Permission{"catalog:read"}, AccessVersion: 1, At: 1},
	}
}
