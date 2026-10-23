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

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/appkit"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/provider"
)

func activeStore(marker int) domain.StoreSnapshot {
	return domain.StoreSnapshot{
		Name:            domain.StoreName("kit-name-" + strconv.Itoa(marker)),
		Status:          domain.Active,
		RegisteredAt:    kernel.Instant(marker),
		StatusChangedAt: kernel.Instant(marker),
	}
}

func TestStoreRepositoryConformsToTheKit(t *testing.T) {
	pool := appkit.OpenPool(t)
	v := providerkit.Repository(func() providerkit.RepositorySubject[domain.StoreRef, domain.StoreSnapshot] {
		pg.ResetTables(t, pool, appkit.Tables...)
		return providerkit.RepositorySubject[domain.StoreRef, domain.StoreSnapshot]{
			Within: func(ctx context.Context, fn func(context.Context, ports.Repository[domain.StoreRef, domain.StoreSnapshot]) error) error {
				return pg.Within(ctx, pool, provider.NewStoreRepository, fn)
			},
			Reader:           provider.NewStoreReader(postgres.NewReadPool(pool)),
			NewID:            func(n int) domain.StoreRef { return domain.StoreRef("kit-" + strconv.Itoa(n)) },
			NewState:         activeStore,
			Marker:           func(s domain.StoreSnapshot) int { return int(s.RegisteredAt) },
			TenantUnresolved: postgres.ErrTenantUnresolved,
			Concurrent:       true,
		}
	})
	tb.Require(t, v)
	if len(v.Skipped) != 0 {
		t.Fatalf("postgres scopes by construction and runs transactions at once; nothing should be skipped: %v", v.Skipped)
	}
}

// The reason, the empty reason and both instants in nanoseconds must survive
// the snapshot column and the typed status column alike.
func TestStoreRoundTripsTheWholeState(t *testing.T) {
	pool := appkit.OpenPool(t)
	ctx := withExecution(t, context.Background())
	reader := provider.NewStoreReader(postgres.NewReadPool(pool))
	active := domain.StoreSnapshot{
		Ref: "loja-acme", Name: "Loja Acme", Status: domain.Active,
		RegisteredAt: 1_755_432_000_000_000_001, StatusChangedAt: 1_755_432_000_000_000_001,
	}

	pg.Seed(t, ctx, pool, provider.NewStoreRepository, active.Ref, active)
	if got, version, err := reader.Load(ctx, active.Ref); err != nil || version != 1 || !got.Equal(active) {
		t.Fatalf("Load() = %+v v%d, %v; want %+v v1", got, version, err, active)
	}

	suspended := active
	suspended.Status, suspended.SuspensionReason, suspended.StatusChangedAt = domain.Suspended, "fraude em análise", 1_755_435_600_000_000_002
	err := pg.Within(ctx, pool, provider.NewStoreRepository, func(ctx context.Context, repo ports.Repository[domain.StoreRef, domain.StoreSnapshot]) error {
		return repo.Save(ctx, active.Ref, suspended, 1)
	})
	if err != nil {
		t.Fatalf("Save(update) = %v", err)
	}
	if got, version, err := reader.Load(ctx, active.Ref); err != nil || version != 2 || !got.Equal(suspended) {
		t.Fatalf("Load() = %+v v%d, %v; want %+v v2", got, version, err, suspended)
	}
}

func TestStoreRepositoryRefusesToStoreAnUnregisteredStatus(t *testing.T) {
	pool := appkit.OpenPool(t)

	err := pg.Within(withExecution(t, context.Background()), pool, provider.NewStoreRepository, func(ctx context.Context, repo ports.Repository[domain.StoreRef, domain.StoreSnapshot]) error {
		return repo.Save(ctx, "loja-acme", domain.StoreSnapshot{Ref: "loja-acme", Name: "Loja Acme"}, 0)
	})

	if err == nil {
		t.Fatal("Save(unregistered) = nil, want a refusal: only a decided status reaches the table")
	}
}

func TestStoreRepositoryHonoursTheUniqueKit(t *testing.T) {
	pool := appkit.OpenPool(t)
	v := providerkit.Unique(func() providerkit.UniqueSubject[domain.StoreRef, domain.StoreSnapshot] {
		pg.ResetTables(t, pool, appkit.Tables...)
		return providerkit.UniqueSubject[domain.StoreRef, domain.StoreSnapshot]{
			Within: func(ctx context.Context, fn func(ctx context.Context, w providerkit.UniqueWrites[domain.StoreRef, domain.StoreSnapshot]) error) error {
				uow := postgres.NewUnitOfWork(pool, func(tx *postgres.Tx) *postgres.Tx { return tx })
				return uow.Within(ctx, func(ctx context.Context, tx *postgres.Tx) error {
					return fn(ctx, providerkit.UniqueWrites[domain.StoreRef, domain.StoreSnapshot]{
						Repo: provider.NewStoreRepository(tx),
						Enqueue: func(ctx context.Context, n int) error {
							return tx.Outbox(provider.Mapper{}).Enqueue(ctx, storeRegisteredEntry(n))
						},
					})
				})
			},
			Reader: provider.NewStoreReader(postgres.NewReadPool(pool)),
			NewID:  func(n int) domain.StoreRef { return domain.StoreRef("kit-" + strconv.Itoa(n)) },
			NewState: func(marker int, keys [][]string) domain.StoreSnapshot {
				s := activeStore(marker)
				s.Name = domain.StoreName(keys[0][0])
				return s
			},
			Marker:      func(s domain.StoreSnapshot) int { return int(s.RegisteredAt) },
			Table:       "stores",
			Constraints: []providerkit.UniqueConstraint{{Name: "stores_tenant_id_name_key", Columns: []string{"tenant_id", "name"}, Code: domain.CodeStoreDuplicateName}},
			Concurrent:  true,
		}
	})
	tb.Require(t, v)
	if len(v.Skipped) != 3 {
		t.Fatalf("skipped = %v, want only the clauses that need a composite key or a second constraint", v.Skipped)
	}
}

func storeRegisteredEntry(n int) ports.OutboxEntry {
	ref := domain.StoreRef("kit-event-" + strconv.Itoa(n))
	return ports.OutboxEntry{
		MessageID: ports.MessageID("evt-" + string(ref)), OccurredAt: 1,
		Intent:        ports.PublishIntent{Destination: "tenancy.events", PartitionKey: string(ref)},
		AggregateType: "tenancy.Store", AggregateID: string(ref), AggregateVersion: 1,
		Event: domain.StoreRegistered{Ref: ref, Name: "Kit event", At: 1},
	}
}
