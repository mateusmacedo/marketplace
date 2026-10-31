//go:build integration

package provider_test

import (
	"context"
	"slices"
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

func activeBuyer(marker int) domain.BuyerSnapshot {
	return domain.BuyerSnapshot{
		Name: domain.BuyerName("Kit " + strconv.Itoa(marker)), Status: domain.BuyerStatusActive,
		AccessVersion: 1, RegisteredAt: kernel.Instant(marker), ChangedAt: kernel.Instant(marker),
	}
}

func TestBuyerRepositoryConformsToTheKit(t *testing.T) {
	pool := appkit.OpenPool(t)
	v := providerkit.Repository(func() providerkit.RepositorySubject[domain.Subject, domain.BuyerSnapshot] {
		pg.ResetTables(t, pool, appkit.Tables...)
		return providerkit.RepositorySubject[domain.Subject, domain.BuyerSnapshot]{
			Within: func(ctx context.Context, fn func(context.Context, ports.Repository[domain.Subject, domain.BuyerSnapshot]) error) error {
				return pg.Within(ctx, pool, provider.NewBuyerRepository, fn)
			},
			Reader:           provider.NewBuyerReader(postgres.NewReadPool(pool)),
			NewID:            func(n int) domain.Subject { return domain.Subject("kit-" + strconv.Itoa(n)) },
			NewState:         activeBuyer,
			Marker:           func(s domain.BuyerSnapshot) int { return int(s.RegisteredAt) },
			TenantUnresolved: postgres.ErrTenantUnresolved,
			Concurrent:       true,
		}
	})
	tb.Require(t, v)
	if len(v.Skipped) != 0 {
		t.Fatalf("nothing should be skipped: %v", v.Skipped)
	}
}

func TestBuyerRoundTripsTheWholeStateAndRefusesAnUndefinedStatus(t *testing.T) {
	pool := appkit.OpenPool(t)
	ctx := withExecution(t, context.Background())
	blocked := domain.BuyerSnapshot{
		Subject: "kc-123", Name: "Bia", Status: domain.BuyerStatusBlocked, BlockReason: "chargeback recorrente",
		AccessVersion: 2, SyncedVersion: 1, RegisteredAt: 1_755_432_000_000_000_001, ChangedAt: 1_755_435_600_000_000_002,
	}

	pg.Seed(t, ctx, pool, provider.NewBuyerRepository, blocked.Subject, blocked)

	if got, version, err := provider.NewBuyerReader(postgres.NewReadPool(pool)).Load(ctx, blocked.Subject); err != nil || version != 1 || !got.Equal(blocked) {
		t.Fatalf("Load() = %+v v%d, %v; want %+v", got, version, err, blocked)
	}
	err := pg.Within(ctx, pool, provider.NewBuyerRepository, func(ctx context.Context, repo ports.Repository[domain.Subject, domain.BuyerSnapshot]) error {
		return repo.Save(ctx, "kc-456", domain.BuyerSnapshot{Subject: "kc-456", Name: "Caio"}, 0)
	})
	if err == nil {
		t.Fatal("Save(undefined) = nil, want a refusal: only a decided status reaches the table")
	}
}

func TestTheBuyersOfOneStatusAreReadExactly(t *testing.T) {
	pool := appkit.OpenPool(t)
	ctx := withExecution(t, context.Background())
	for subject, status := range map[domain.Subject]domain.BuyerStatus{
		"kc-a": domain.BuyerStatusActive, "kc-b": domain.BuyerStatusBlocked, "kc-c": domain.BuyerStatusActive,
	} {
		snap := activeBuyer(1)
		snap.Subject, snap.Status = subject, status
		if status == domain.BuyerStatusBlocked {
			snap.BlockReason = "motivo"
		}
		pg.Seed(t, ctx, pool, provider.NewBuyerRepository, subject, snap)
	}

	active, err := provider.NewBuyersByStatusReader(postgres.NewReadPool(pool)).LoadByStatus(withExecution(t, context.Background()), domain.BuyerStatusActive)

	subjects := make([]string, 0, len(active))
	for _, s := range active {
		subjects = append(subjects, string(s.Subject))
	}
	slices.Sort(subjects)
	if err != nil || !slices.Equal(subjects, []string{"kc-a", "kc-c"}) {
		t.Fatalf("LoadByStatus(active) = %v, %v; want kc-a and kc-c", subjects, err)
	}
}
