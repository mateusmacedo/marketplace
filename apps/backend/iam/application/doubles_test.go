package application_test

import (
	"context"
	"testing"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	"github.com/mateusmacedo/dmpf/libs/backend/go/memory"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/serviceskit"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

const (
	testRef  = domain.RoleRef("expedicao")
	testName = domain.RoleName("Expedição")
)

var (
	testOccurred    = ports.Instant(1_755_432_000_000_000_000)
	testPermissions = []domain.Permission{"fulfillment:write", "ordering:read"}
)

var roleTable = memory.Table[domain.RoleRef, domain.RoleSnapshot]{Name: "roles", Unique: []memory.Unique[domain.RoleSnapshot]{{
	Name:      "roles_tenant_id_name_key",
	Key:       func(s domain.RoleSnapshot) ([]string, bool) { return []string{string(s.Name)}, true },
	Rejection: domain.RejectRoleDuplicateName,
}}}

var invitationTable = memory.Table[domain.Email, domain.InvitationSnapshot]{Name: "invitations"}

var buyerTable = memory.Table[domain.Subject, domain.BuyerSnapshot]{Name: "buyers"}

// The owner key exists only for the active owner, as the generated column of
// the provider is NULL for every other membership.
var membershipTable = memory.Table[domain.Subject, domain.MembershipSnapshot]{Name: "memberships", Unique: []memory.Unique[domain.MembershipSnapshot]{{
	Name:      "memberships_tenant_id_owner_key",
	Key:       func(s domain.MembershipSnapshot) ([]string, bool) { return []string{"owner"}, s.Owns() },
	Rejection: domain.RejectMembershipOwnerClaimed,
}}}

type harness struct {
	fakes   *serviceskit.Fakes
	store   *memory.Store
	rec     *serviceskit.Steps
	service application.Service
}

// recording names each step of a repository after its table, so the sequence
// of FND-04 §3.2 is asserted for every aggregate alike.
type recording[ID comparable, S any] struct {
	table string
	inner ports.Repository[ID, S]
	rec   *serviceskit.Steps
}

func (r recording[ID, S]) Load(ctx context.Context, id ID) (S, ports.Version, error) {
	r.rec.Record(r.table + ".Load")
	return r.inner.Load(ctx, id)
}

func (r recording[ID, S]) Save(ctx context.Context, id ID, s S, expected ports.Version) error {
	r.rec.Record(r.table + ".Save")
	return r.inner.Save(ctx, id, s, expected)
}

// plantedRoles answers FindRolesByStatus with what the test planted: the
// memory store has no relation query, and the use case is what is under test.
type plantedRoles map[domain.RoleStatus][]domain.RoleSnapshot

func (p plantedRoles) LoadByStatus(_ context.Context, status domain.RoleStatus) ([]domain.RoleSnapshot, error) {
	return p[status], nil
}

type plantedInvitations map[domain.InvitationStatus][]domain.InvitationSnapshot

func (p plantedInvitations) LoadByStatus(_ context.Context, status domain.InvitationStatus) ([]domain.InvitationSnapshot, error) {
	return p[status], nil
}

type plantedBuyers map[domain.BuyerStatus][]domain.BuyerSnapshot

func (p plantedBuyers) LoadByStatus(_ context.Context, status domain.BuyerStatus) ([]domain.BuyerSnapshot, error) {
	return p[status], nil
}

type plantedMemberships map[domain.MembershipStatus][]domain.MembershipSnapshot

func (p plantedMemberships) LoadByStatus(_ context.Context, status domain.MembershipStatus) ([]domain.MembershipSnapshot, error) {
	return p[status], nil
}

type option func(*setup)

type setup struct {
	faults      serviceskit.Faults
	registerErr error
}

func withRegisterError(err error) option {
	return func(s *setup) { s.registerErr = err }
}

func withLoadError(err error) option {
	return func(s *setup) { s.faults.Load = err }
}

func withSaveError(err error) option {
	return func(s *setup) { s.faults.Save = err }
}

func withEnqueueError(err error) option {
	return func(s *setup) { s.faults.Enqueue = err }
}

func newHarness(t *testing.T, options ...option) *harness {
	t.Helper()

	fakes := serviceskit.NewFakes()
	h := &harness{fakes: fakes, store: fakes.Store, rec: fakes.Steps}
	cfg := &setup{}
	for _, apply := range options {
		apply(cfg)
	}
	fakes.FailRegister = cfg.registerErr

	bind := func(tx serviceskit.Tx) application.Resources {
		return application.Resources{
			Roles: recording[domain.RoleRef, domain.RoleSnapshot]{
				table: "roles", inner: serviceskit.FaultyRepository(roleTable.Repository(tx.Memory()), cfg.faults), rec: h.rec,
			},
			Invitations: recording[domain.Email, domain.InvitationSnapshot]{
				table: "invitations", inner: serviceskit.FaultyRepository(invitationTable.Repository(tx.Memory()), cfg.faults), rec: h.rec,
			},
			Memberships: recording[domain.Subject, domain.MembershipSnapshot]{
				table: "memberships", inner: serviceskit.FaultyRepository(membershipTable.Repository(tx.Memory()), cfg.faults), rec: h.rec,
			},
			Buyers: recording[domain.Subject, domain.BuyerSnapshot]{
				table: "buyers", inner: serviceskit.FaultyRepository(buyerTable.Repository(tx.Memory()), cfg.faults), rec: h.rec,
			},
			Outbox:   serviceskit.FaultyOutbox(tx.Outbox(), cfg.faults),
			Commands: tx.CommandInbox(application.CommandConsumer),
		}
	}

	h.service = application.Service{
		UoW:           serviceskit.UnitOfWork(fakes, bind),
		Roles:         roleTable.Reader(h.store),
		RolesByStatus: plantedRoles{},

		Invitations:         invitationTable.Reader(h.store),
		InvitationsByStatus: plantedInvitations{},
		Memberships:         membershipTable.Reader(h.store),
		MembershipsByStatus: plantedMemberships{},
		Buyers:              buyerTable.Reader(h.store),
		BuyersByStatus:      plantedBuyers{},

		Clock:       fakes.Clock(memory.FixedClock{At: testOccurred}),
		IDs:         fakes.IDs(&memory.SequenceIDs{Prefix: "m-"}),
		Authorize:   serviceskit.Authorize(fakes, usecase.AllowAll[application.Operation]()),
		Idempotency: usecase.IdempotencyPolicy{Wait: 1_000_000_000, Retention: 86_400_000_000_000, Digest: serviceskit.FoldDigest},
	}
	return h
}

func (h *harness) seedRole(t *testing.T, snap domain.RoleSnapshot) {
	t.Helper()
	uow := memory.NewUnitOfWork(h.store, func(tx *memory.Tx) application.Resources {
		return application.Resources{Roles: roleTable.Repository(tx)}
	})
	err := uow.Within(withExecution(t, context.Background()), func(ctx context.Context, res application.Resources) error {
		return res.Roles.Save(ctx, snap.Ref, snap, 0)
	})
	if err != nil {
		t.Fatalf("seedRole: %v", err)
	}
}

func (h *harness) seedActive(t *testing.T) {
	t.Helper()
	h.seedRole(t, domain.RoleSnapshot{
		Ref: testRef, Name: testName, Permissions: testPermissions, Status: domain.RoleStatusActive,
		AccessVersion: 1, DefinedAt: 1000, ChangedAt: 1000,
	})
}

func (h *harness) seedRetired(t *testing.T) {
	t.Helper()
	h.seedRole(t, domain.RoleSnapshot{
		Ref: testRef, Name: testName, Permissions: testPermissions, Status: domain.RoleStatusRetired,
		AccessVersion: 2, DefinedAt: 1000, ChangedAt: 2000,
	})
}

func (h *harness) outboxLen() int {
	return len(h.store.Entries())
}

func (h *harness) stored(t *testing.T, ref domain.RoleRef) domain.RoleSnapshot {
	t.Helper()
	snap, _, err := roleTable.Reader(h.store).Load(withExecution(t, context.Background()), ref)
	if err != nil {
		t.Fatalf("Load(%s) = %v", ref, err)
	}
	return snap
}

func (h *harness) seedInvitation(t *testing.T, snap domain.InvitationSnapshot) {
	t.Helper()
	uow := memory.NewUnitOfWork(h.store, func(tx *memory.Tx) application.Resources {
		return application.Resources{Invitations: invitationTable.Repository(tx)}
	})
	err := uow.Within(withExecution(t, context.Background()), func(ctx context.Context, res application.Resources) error {
		return res.Invitations.Save(ctx, snap.Email, snap, 0)
	})
	if err != nil {
		t.Fatalf("seedInvitation: %v", err)
	}
}

func (h *harness) seedMembership(t *testing.T, snap domain.MembershipSnapshot) {
	t.Helper()
	uow := memory.NewUnitOfWork(h.store, func(tx *memory.Tx) application.Resources {
		return application.Resources{Memberships: membershipTable.Repository(tx)}
	})
	err := uow.Within(withExecution(t, context.Background()), func(ctx context.Context, res application.Resources) error {
		return res.Memberships.Save(ctx, snap.Subject, snap, 0)
	})
	if err != nil {
		t.Fatalf("seedMembership: %v", err)
	}
}

const (
	testEmail   = domain.Email("ana@loja.com")
	testSubject = domain.Subject("kc-123")
	testOwner   = domain.Subject("kc-owner")
)

func (h *harness) seedMember(t *testing.T, roles ...domain.RoleRef) {
	t.Helper()
	h.seedMembership(t, domain.MembershipSnapshot{
		Subject: testSubject, Email: testEmail, Roles: roles, Status: domain.MembershipStatusActive,
		AccessVersion: 1, JoinedAt: 1000, ChangedAt: 1000,
	})
}

func (h *harness) seedOwner(t *testing.T) {
	t.Helper()
	h.seedMembership(t, domain.MembershipSnapshot{
		Subject: testOwner, Roles: []domain.RoleRef{domain.OwnerRole}, Status: domain.MembershipStatusActive,
		AccessVersion: 1, JoinedAt: 1000, ChangedAt: 1000,
	})
}

func (h *harness) storedInvitation(t *testing.T, email domain.Email) domain.InvitationSnapshot {
	t.Helper()
	snap, _, err := invitationTable.Reader(h.store).Load(withExecution(t, context.Background()), email)
	if err != nil {
		t.Fatalf("Load(%s) = %v", email, err)
	}
	return snap
}

func (h *harness) storedMembership(t *testing.T, subject domain.Subject) domain.MembershipSnapshot {
	t.Helper()
	snap, _, err := membershipTable.Reader(h.store).Load(withExecution(t, context.Background()), subject)
	if err != nil {
		t.Fatalf("Load(%s) = %v", subject, err)
	}
	return snap
}

func (h *harness) seedBuyer(t *testing.T, status domain.BuyerStatus) {
	t.Helper()
	snap := domain.BuyerSnapshot{Subject: testSubject, Name: "Bia", Status: status, AccessVersion: 1, RegisteredAt: 1000, ChangedAt: 1000}
	if status == domain.BuyerStatusBlocked {
		snap.BlockReason, snap.AccessVersion = "chargeback recorrente", 2
	}
	uow := memory.NewUnitOfWork(h.store, func(tx *memory.Tx) application.Resources {
		return application.Resources{Buyers: buyerTable.Repository(tx)}
	})
	if err := uow.Within(withExecution(t, context.Background()), func(ctx context.Context, res application.Resources) error {
		return res.Buyers.Save(ctx, snap.Subject, snap, 0)
	}); err != nil {
		t.Fatalf("seedBuyer: %v", err)
	}
}

func (h *harness) storedBuyer(t *testing.T, subject domain.Subject) domain.BuyerSnapshot {
	t.Helper()
	snap, _, err := buyerTable.Reader(h.store).Load(withExecution(t, context.Background()), subject)
	if err != nil {
		t.Fatalf("Load(%s) = %v", subject, err)
	}
	return snap
}
