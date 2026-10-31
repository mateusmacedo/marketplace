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

	"github.com/mateusmacedo/marketplace/apps/backend/iam/appkit"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

func saveRole(t *testing.T, pool *pgxpool.Pool, ref domain.RoleRef, status domain.RoleStatus) {
	t.Helper()
	snap := domain.RoleSnapshot{
		Ref: ref, Name: domain.RoleName("Papel " + string(ref)), Permissions: []domain.Permission{"catalog:read"},
		Status: status, AccessVersion: 1, DefinedAt: 1, ChangedAt: 1,
	}
	err := pg.Within(withExecution(t, context.Background()), pool, provider.NewRoleRepository, func(ctx context.Context, repo ports.Repository[domain.RoleRef, domain.RoleSnapshot]) error {
		return repo.Save(ctx, ref, snap, 0)
	})
	if err != nil {
		t.Fatalf("save %s: %v", ref, err)
	}
}

func refs(snapshots []domain.RoleSnapshot) []string {
	out := make([]string, 0, len(snapshots))
	for _, s := range snapshots {
		out = append(out, string(s.Ref))
	}
	slices.Sort(out)
	return out
}

// The reader takes the pool, never a Tx: the query runs outside any unit of
// work, and constructing it from the pool is what makes that structural.
func TestLoadByStatusReturnsExactlyTheRolesInThatStatus(t *testing.T) {
	pool := appkit.OpenPool(t)
	saveRole(t, pool, "papel-a", domain.RoleStatusActive)
	saveRole(t, pool, "papel-b", domain.RoleStatusActive)
	saveRole(t, pool, "papel-c", domain.RoleStatusRetired)
	reader := provider.NewRolesByStatusReader(postgres.NewReadPool(pool))

	active, err := reader.LoadByStatus(withExecution(t, context.Background()), domain.RoleStatusActive)
	if err != nil || !slices.Equal(refs(active), []string{"papel-a", "papel-b"}) {
		t.Fatalf("LoadByStatus(active) = %v, %v; want papel-a and papel-b", refs(active), err)
	}
	retired, err := reader.LoadByStatus(withExecution(t, context.Background()), domain.RoleStatusRetired)
	if err != nil || !slices.Equal(refs(retired), []string{"papel-c"}) || !slices.Equal(retired[0].Permissions, []domain.Permission{"catalog:read"}) {
		t.Fatalf("LoadByStatus(retired) = %+v, %v; want papel-c with its permissions", retired, err)
	}
}

func TestLoadByStatusReturnsEmptyWhenNoRoleHoldsIt(t *testing.T) {
	pool := appkit.OpenPool(t)
	saveRole(t, pool, "papel-a", domain.RoleStatusActive)

	found, err := provider.NewRolesByStatusReader(postgres.NewReadPool(pool)).LoadByStatus(withExecution(t, context.Background()), domain.RoleStatusRetired)

	if err != nil || len(found) != 0 {
		t.Fatalf("LoadByStatus(retired) = %v, %v; want none", found, err)
	}
}
