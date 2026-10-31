//go:build integration

package provider_test

import (
	"context"
	"strconv"
	"sync"
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

func pendingInvitation(marker int) domain.InvitationSnapshot {
	return domain.InvitationSnapshot{
		Roles: []domain.RoleRef{"expedicao"}, Status: domain.InvitationStatusPending, Round: 1, InvitedAt: kernel.Instant(marker),
	}
}

func activeMember(marker int) domain.MembershipSnapshot {
	return domain.MembershipSnapshot{
		Email: "kit@loja.com", Roles: []domain.RoleRef{"expedicao"}, Status: domain.MembershipStatusActive,
		AccessVersion: 1, JoinedAt: kernel.Instant(marker), ChangedAt: kernel.Instant(marker),
	}
}

func TestInvitationRepositoryConformsToTheKit(t *testing.T) {
	pool := appkit.OpenPool(t)
	v := providerkit.Repository(func() providerkit.RepositorySubject[domain.Email, domain.InvitationSnapshot] {
		pg.ResetTables(t, pool, appkit.Tables...)
		return providerkit.RepositorySubject[domain.Email, domain.InvitationSnapshot]{
			Within: func(ctx context.Context, fn func(context.Context, ports.Repository[domain.Email, domain.InvitationSnapshot]) error) error {
				return pg.Within(ctx, pool, provider.NewInvitationRepository, fn)
			},
			Reader:           provider.NewInvitationReader(postgres.NewReadPool(pool)),
			NewID:            func(n int) domain.Email { return domain.Email("kit-" + strconv.Itoa(n) + "@loja.com") },
			NewState:         pendingInvitation,
			Marker:           func(s domain.InvitationSnapshot) int { return int(s.InvitedAt) },
			TenantUnresolved: postgres.ErrTenantUnresolved,
			Concurrent:       true,
		}
	})
	tb.Require(t, v)
	if len(v.Skipped) != 0 {
		t.Fatalf("nothing should be skipped: %v", v.Skipped)
	}
}

func TestMembershipRepositoryConformsToTheKit(t *testing.T) {
	pool := appkit.OpenPool(t)
	v := providerkit.Repository(func() providerkit.RepositorySubject[domain.Subject, domain.MembershipSnapshot] {
		pg.ResetTables(t, pool, appkit.Tables...)
		return providerkit.RepositorySubject[domain.Subject, domain.MembershipSnapshot]{
			Within: func(ctx context.Context, fn func(context.Context, ports.Repository[domain.Subject, domain.MembershipSnapshot]) error) error {
				return pg.Within(ctx, pool, provider.NewMembershipRepository, fn)
			},
			Reader:           provider.NewMembershipReader(postgres.NewReadPool(pool)),
			NewID:            func(n int) domain.Subject { return domain.Subject("kit-" + strconv.Itoa(n)) },
			NewState:         activeMember,
			Marker:           func(s domain.MembershipSnapshot) int { return int(s.JoinedAt) },
			TenantUnresolved: postgres.ErrTenantUnresolved,
			Concurrent:       true,
		}
	})
	tb.Require(t, v)
	if len(v.Skipped) != 0 {
		t.Fatalf("nothing should be skipped: %v", v.Skipped)
	}
}

func TestInvitationAndMembershipRoundTripTheWholeState(t *testing.T) {
	pool := appkit.OpenPool(t)
	ctx := withExecution(t, context.Background())
	failed := domain.InvitationSnapshot{
		Email: "ana@loja.com", Roles: []domain.RoleRef{"catalogo", "expedicao"}, Status: domain.InvitationStatusFailed, Round: 2,
		FailureReason: "recusado", InvitedAt: 1_755_432_000_000_000_001, SettledAt: 1_755_435_600_000_000_002,
	}
	removed := domain.MembershipSnapshot{
		Subject: "kc-123", Email: "ana@loja.com", Roles: []domain.RoleRef{"expedicao"}, Status: domain.MembershipStatusRemoved,
		AccessVersion: 3, SyncedVersion: 2, JoinedAt: 1_755_432_000_000_000_001, ChangedAt: 1_755_435_600_000_000_002,
	}

	pg.Seed(t, ctx, pool, provider.NewInvitationRepository, failed.Email, failed)
	pg.Seed(t, ctx, pool, provider.NewMembershipRepository, removed.Subject, removed)

	if got, version, err := provider.NewInvitationReader(postgres.NewReadPool(pool)).Load(ctx, failed.Email); err != nil || version != 1 || !got.Equal(failed) {
		t.Fatalf("Load(invitation) = %+v v%d, %v; want %+v", got, version, err, failed)
	}
	if got, version, err := provider.NewMembershipReader(postgres.NewReadPool(pool)).Load(ctx, removed.Subject); err != nil || version != 1 || !got.Equal(removed) {
		t.Fatalf("Load(membership) = %+v v%d, %v; want %+v", got, version, err, removed)
	}
}

func TestTheRepositoriesRefuseToStoreAnUndefinedStatus(t *testing.T) {
	pool := appkit.OpenPool(t)
	ctx := withExecution(t, context.Background())

	invitation := pg.Within(ctx, pool, provider.NewInvitationRepository, func(ctx context.Context, repo ports.Repository[domain.Email, domain.InvitationSnapshot]) error {
		return repo.Save(ctx, "ana@loja.com", domain.InvitationSnapshot{Email: "ana@loja.com"}, 0)
	})
	membership := pg.Within(ctx, pool, provider.NewMembershipRepository, func(ctx context.Context, repo ports.Repository[domain.Subject, domain.MembershipSnapshot]) error {
		return repo.Save(ctx, "kc-123", domain.MembershipSnapshot{Subject: "kc-123"}, 0)
	})

	if invitation == nil || membership == nil {
		t.Fatalf("Save(undefined) = %v, %v; want both refused: only a decided status reaches the table", invitation, membership)
	}
}

// The owner constraint holds one value, true, so the kit's per-row values map
// onto it: the first value a subject sees makes the active owner, and any
// other value an active member of another role, whose owner column is NULL.
func TestMembershipRepositoryHonoursTheUniqueKit(t *testing.T) {
	pool := appkit.OpenPool(t)
	v := providerkit.Unique(func() providerkit.UniqueSubject[domain.Subject, domain.MembershipSnapshot] {
		pg.ResetTables(t, pool, appkit.Tables...)
		owning := &firstKey{}
		return providerkit.UniqueSubject[domain.Subject, domain.MembershipSnapshot]{
			Within: func(ctx context.Context, fn func(ctx context.Context, w providerkit.UniqueWrites[domain.Subject, domain.MembershipSnapshot]) error) error {
				uow := postgres.NewUnitOfWork(pool, func(tx *postgres.Tx) *postgres.Tx { return tx })
				return uow.Within(ctx, func(ctx context.Context, tx *postgres.Tx) error {
					return fn(ctx, providerkit.UniqueWrites[domain.Subject, domain.MembershipSnapshot]{
						Repo: provider.NewMembershipRepository(tx),
						Enqueue: func(ctx context.Context, n int) error {
							return tx.Outbox(provider.Mapper{}).Enqueue(ctx, membershipEstablishedEntry(n))
						},
					})
				})
			},
			Reader: provider.NewMembershipReader(postgres.NewReadPool(pool)),
			NewID:  func(n int) domain.Subject { return domain.Subject("kit-" + strconv.Itoa(n)) },
			NewState: func(marker int, keys [][]string) domain.MembershipSnapshot {
				s := activeMember(marker)
				if owning.is(keys[0][0]) {
					s.Roles = []domain.RoleRef{domain.OwnerRole}
				}
				return s
			},
			Marker:      func(s domain.MembershipSnapshot) int { return int(s.JoinedAt) },
			Table:       "memberships",
			Constraints: []providerkit.UniqueConstraint{{Name: "memberships_tenant_id_owner_key", Columns: []string{"tenant_id", "owner"}, Code: domain.CodeMembershipOwnerClaimed}},
			Concurrent:  true,
		}
	})
	tb.Require(t, v)
	if len(v.Skipped) != 3 {
		t.Fatalf("skipped = %v, want only the clauses that need a composite key or a second constraint", v.Skipped)
	}
}

// firstKey remembers the first value it is asked about; the kit may build two
// states at once in its concurrent clause.
type firstKey struct {
	mu    sync.Mutex
	value *string
}

func (f *firstKey) is(value string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.value == nil {
		f.value = &value
	}
	return *f.value == value
}

func membershipEstablishedEntry(n int) ports.OutboxEntry {
	subject := domain.Subject("kit-event-" + strconv.Itoa(n))
	return ports.OutboxEntry{
		MessageID: ports.MessageID("evt-" + string(subject)), OccurredAt: 1,
		Intent:        ports.PublishIntent{Destination: "iam.events", PartitionKey: string(subject)},
		AggregateType: "iam.Membership", AggregateID: string(subject), AggregateVersion: 1,
		Event: domain.MembershipEstablished{Subject: subject, Roles: []domain.RoleRef{"expedicao"}, AccessVersion: 1, At: 1},
	}
}
