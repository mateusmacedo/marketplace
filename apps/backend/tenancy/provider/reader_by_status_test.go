//go:build integration

package provider_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/pg"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/appkit"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/provider"
)

func saveStore(t *testing.T, pool *pgxpool.Pool, ref domain.StoreRef, status domain.StoreStatus) {
	t.Helper()
	snap := domain.StoreSnapshot{Ref: ref, Name: domain.StoreName("Loja " + string(ref)), Status: status, RegisteredAt: 1, StatusChangedAt: 1}
	if status == domain.Suspended {
		snap.SuspensionReason = "motivo"
	}
	err := pg.Within(withExecution(t, context.Background()), pool, provider.NewStoreRepository, func(ctx context.Context, repo ports.Repository[domain.StoreRef, domain.StoreSnapshot]) error {
		return repo.Save(ctx, ref, snap, 0)
	})
	if err != nil {
		t.Fatalf("save %s: %v", ref, err)
	}
}

func refs(snapshots []domain.StoreSnapshot) []string {
	out := make([]string, 0, len(snapshots))
	for _, s := range snapshots {
		out = append(out, string(s.Ref))
	}
	slices.Sort(out)
	return out
}

// The reader takes the pool, never a Tx: the query runs outside any unit of
// work, and constructing it from the pool is what makes that structural.
func TestLoadByStatusReturnsExactlyTheStoresInThatStatus(t *testing.T) {
	pool := appkit.OpenPool(t)
	saveStore(t, pool, "loja-a", domain.Active)
	saveStore(t, pool, "loja-b", domain.Active)
	saveStore(t, pool, "loja-c", domain.Suspended)
	reader := provider.NewStoresByStatusReader(postgres.NewReadPool(pool))

	active, err := reader.LoadByStatus(withExecution(t, context.Background()), domain.Active)
	if err != nil || !slices.Equal(refs(active), []string{"loja-a", "loja-b"}) {
		t.Fatalf("LoadByStatus(active) = %v, %v; want loja-a and loja-b", refs(active), err)
	}
	suspended, err := reader.LoadByStatus(withExecution(t, context.Background()), domain.Suspended)
	if err != nil || !slices.Equal(refs(suspended), []string{"loja-c"}) || suspended[0].SuspensionReason != "motivo" {
		t.Fatalf("LoadByStatus(suspended) = %+v, %v; want loja-c with its reason", suspended, err)
	}
}

func TestLoadByStatusReturnsEmptyWhenNoStoreHoldsIt(t *testing.T) {
	pool := appkit.OpenPool(t)
	saveStore(t, pool, "loja-a", domain.Active)

	found, err := provider.NewStoresByStatusReader(postgres.NewReadPool(pool)).LoadByStatus(withExecution(t, context.Background()), domain.Suspended)

	if err != nil || len(found) != 0 {
		t.Fatalf("LoadByStatus(suspended) = %v, %v; want none", found, err)
	}
}
