package domain_test

import (
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestGrantAddsARoleAndRaisesTheAccessVersion(t *testing.T) {
	m := newMember(t, "expedicao")

	acc, rej := m.Grant(domain.GrantRole{Role: "catalogo", At: later})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.GrantedResponse{Subject: subject}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	roles := []domain.RoleRef{"catalogo", "expedicao"}
	want := domain.MemberAccessChanged{Subject: subject, Roles: roles, AccessVersion: 2, At: later}
	if events := acc.Events(); len(events) != 1 || !sameEvent(events[0], want) {
		t.Fatalf("Events() = %+v, want [%+v]", events, want)
	}
	if got := m.Snapshot(); got.AccessVersion != 2 || got.ChangedAt != later || len(got.Roles) != 2 {
		t.Fatalf("Snapshot() = %+v, want %v at access version 2", got, roles)
	}
}

func TestGrantOfAHeldRoleAcceptsWithoutChangingOrEmitting(t *testing.T) {
	m := newMember(t, "expedicao")
	before := m.Snapshot()

	acc, rej := m.Grant(domain.GrantRole{Role: "expedicao", At: later})

	tb.RequireAccepted(t, rej)
	if len(acc.Events()) != 0 {
		t.Fatalf("Events() len = %d, want 0", len(acc.Events()))
	}
	requireMembershipUnchanged(t, before, m.Snapshot())
}

func TestGrantRejectsWhatTheSpecRefuses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		member func(*testing.T) *domain.Membership
		role   domain.RoleRef
		want   kernel.Code
	}{
		{"removed member", newRemovedMember, "vendas", domain.CodeMembershipRemoved},
		{"role outside the form", func(t *testing.T) *domain.Membership { return newMember(t, "expedicao") }, "Exp_1", domain.CodeMembershipRoleInvalid},
		{"owner", func(t *testing.T) *domain.Membership { return newMember(t, "expedicao") }, domain.OwnerRole, domain.CodeMembershipOwnerReserved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.member(t)
			before := m.Snapshot()

			acc, rej := m.Grant(domain.GrantRole{Role: tc.role, At: later})

			tb.RequireRejected(t, acc, rej, tc.want)
			requireMembershipUnchanged(t, before, m.Snapshot())
		})
	}
}

func TestRevokeRemovesAHeldRoleAndRaisesTheAccessVersion(t *testing.T) {
	m := newMember(t, "expedicao", "catalogo")

	acc, rej := m.Revoke(domain.RevokeRole{Role: "catalogo", At: later})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.RevokedResponse{Subject: subject}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	want := domain.MemberAccessChanged{Subject: subject, Roles: []domain.RoleRef{"expedicao"}, AccessVersion: 2, At: later}
	if events := acc.Events(); len(events) != 1 || !sameEvent(events[0], want) {
		t.Fatalf("Events() = %+v, want [%+v]", events, want)
	}
}

func TestRevokeRejectsWhatTheSpecRefuses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		member func(*testing.T) *domain.Membership
		role   domain.RoleRef
		want   kernel.Code
	}{
		{"removed member", newRemovedMember, "expedicao", domain.CodeMembershipRemoved},
		{"owner of the store", newOwner, domain.OwnerRole, domain.CodeMembershipOwnerProtected},
		{"role not held", func(t *testing.T) *domain.Membership { return newMember(t, "expedicao", "catalogo") }, "vendas", domain.CodeMembershipRoleNotHeld},
		{"last role", func(t *testing.T) *domain.Membership { return newMember(t, "expedicao") }, "expedicao", domain.CodeMembershipLastRole},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.member(t)
			before := m.Snapshot()

			acc, rej := m.Revoke(domain.RevokeRole{Role: tc.role, At: later})

			tb.RequireRejected(t, acc, rej, tc.want)
			requireMembershipUnchanged(t, before, m.Snapshot())
		})
	}
}

func TestRemoveTakesAnActiveMemberToRemovedKeepingItsRoles(t *testing.T) {
	m := newMember(t, "expedicao")

	acc, rej := m.Remove(domain.RemoveMember{At: later})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.RemovedResponse{Subject: subject}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	want := domain.MemberRemoved{Subject: subject, AccessVersion: 2, At: later}
	if events := acc.Events(); len(events) != 1 || !sameEvent(events[0], want) {
		t.Fatalf("Events() = %+v, want [%+v]", events, want)
	}
	wantState := domain.MembershipSnapshot{
		Subject: subject, Email: email, Roles: []domain.RoleRef{"expedicao"}, Status: domain.MembershipStatusRemoved,
		AccessVersion: 2, JoinedAt: at, ChangedAt: later,
	}
	if got := m.Snapshot(); !got.Equal(wantState) {
		t.Fatalf("Snapshot() = %+v, want %+v", got, wantState)
	}
}

func TestRemoveRejectsARemovedMemberAndTheOwner(t *testing.T) {
	for _, tc := range []struct {
		name   string
		member func(*testing.T) *domain.Membership
		want   kernel.Code
	}{
		{"removed member", newRemovedMember, domain.CodeMembershipRemoved},
		{"owner of the store", newOwner, domain.CodeMembershipOwnerProtected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.member(t)
			before := m.Snapshot()

			acc, rej := m.Remove(domain.RemoveMember{At: later})

			tb.RequireRejected(t, acc, rej, tc.want)
			requireMembershipUnchanged(t, before, m.Snapshot())
		})
	}
}

func TestConfirmAccessSyncedKeepsTheHighestConfirmedVersion(t *testing.T) {
	m := newMember(t, "expedicao")
	if _, rej := m.Grant(domain.GrantRole{Role: "catalogo", At: later}); rej != nil {
		t.Fatalf("setup: Grant rejected: %v", rej)
	}

	if _, rej := m.ConfirmAccessSynced(domain.ConfirmMemberAccessSynced{Version: 2}); rej != nil {
		t.Fatalf("ConfirmAccessSynced(2) rejected: %v", rej)
	}
	acc, rej := m.ConfirmAccessSynced(domain.ConfirmMemberAccessSynced{Version: 1})

	tb.RequireAccepted(t, rej)
	if len(acc.Events()) != 0 {
		t.Fatalf("Events() len = %d, want 0", len(acc.Events()))
	}
	if got := m.Snapshot(); got.SyncedVersion != 2 {
		t.Fatalf("SyncedVersion = %d, want 2: a late confirmation never moves it back", got.SyncedVersion)
	}
}

func TestConfirmAccessSyncedAppliesToARemovedMember(t *testing.T) {
	m := newRemovedMember(t)

	_, rej := m.ConfirmAccessSynced(domain.ConfirmMemberAccessSynced{Version: 2})

	tb.RequireAccepted(t, rej)
	if got := m.Snapshot(); got.Status != domain.MembershipStatusRemoved || got.SyncedVersion != 2 {
		t.Fatalf("Snapshot() = %+v, want removed and synced at 2", got)
	}
}

func TestConfirmAccessSyncedRejectsAVersionAheadOfTheAccessVersion(t *testing.T) {
	m := newMember(t, "expedicao")
	before := m.Snapshot()

	acc, rej := m.ConfirmAccessSynced(domain.ConfirmMemberAccessSynced{Version: 2})

	tb.RequireRejected(t, acc, rej, domain.CodeMembershipVersionAhead)
	requireMembershipUnchanged(t, before, m.Snapshot())
}
