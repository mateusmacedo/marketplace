package domain_test

import (
	"strings"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestEstablishAcceptsAnUnknownSubjectAsAnActiveMember(t *testing.T) {
	m := domain.NewMembership(subject)

	acc, rej := m.Establish(domain.EstablishMembership{Subject: subject, Email: email, Roles: []domain.RoleRef{"expedicao", "catalogo", "expedicao"}, At: at})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.EstablishedResponse{Subject: subject}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	roles := []domain.RoleRef{"catalogo", "expedicao"}
	want := domain.MembershipEstablished{Subject: subject, Email: email, Roles: roles, AccessVersion: 1, At: at}
	if events := acc.Events(); len(events) != 1 || !sameEvent(events[0], want) {
		t.Fatalf("Events() = %+v, want [%+v]", events, want)
	}
	wantState := domain.MembershipSnapshot{
		Subject: subject, Email: email, Roles: roles, Status: domain.MembershipStatusActive,
		AccessVersion: 1, JoinedAt: at, ChangedAt: at,
	}
	if got := m.Snapshot(); !got.Equal(wantState) {
		t.Fatalf("Snapshot() = %+v, want %+v", got, wantState)
	}
}

func TestEstablishTheOwnerWithoutAnEmail(t *testing.T) {
	m := newOwner(t)

	want := domain.MembershipSnapshot{
		Subject: owner, Roles: []domain.RoleRef{domain.OwnerRole}, Status: domain.MembershipStatusActive,
		AccessVersion: 1, JoinedAt: at, ChangedAt: at,
	}
	if got := m.Snapshot(); !got.Equal(want) {
		t.Fatalf("Snapshot() = %+v, want %+v", got, want)
	}
}

func TestEstablishBringsARemovedMemberBackWithTheNewRoles(t *testing.T) {
	m := newRemovedMember(t)

	acc, rej := m.Establish(domain.EstablishMembership{Subject: subject, Email: "ana@outra.com", Roles: []domain.RoleRef{"vendas"}, At: later})

	tb.RequireAccepted(t, rej)
	want := domain.MembershipEstablished{Subject: subject, Email: "ana@outra.com", Roles: []domain.RoleRef{"vendas"}, AccessVersion: 3, At: later}
	if events := acc.Events(); len(events) != 1 || !sameEvent(events[0], want) {
		t.Fatalf("Events() = %+v, want [%+v]", events, want)
	}
	wantState := domain.MembershipSnapshot{
		Subject: subject, Email: "ana@outra.com", Roles: []domain.RoleRef{"vendas"}, Status: domain.MembershipStatusActive,
		AccessVersion: 3, JoinedAt: at, ChangedAt: later,
	}
	if got := m.Snapshot(); !got.Equal(wantState) {
		t.Fatalf("Snapshot() = %+v, want %+v", got, wantState)
	}
}

func TestEstablishKeepsTheEmailOfAReturningMemberWhenTheCommandHasNone(t *testing.T) {
	m := newRemovedMember(t)

	_, rej := m.Establish(domain.EstablishMembership{Subject: subject, Roles: []domain.RoleRef{"vendas"}, At: later})

	tb.RequireAccepted(t, rej)
	if got := m.Snapshot().Email; got != email {
		t.Fatalf("Email = %q, want the one it had, %q", got, email)
	}
}

func TestEstablishAddsTheNewRolesToAnActiveMember(t *testing.T) {
	m := newMember(t, "expedicao")

	acc, rej := m.Establish(domain.EstablishMembership{Subject: subject, Email: "ana@outra.com", Roles: []domain.RoleRef{"catalogo"}, At: later})

	tb.RequireAccepted(t, rej)
	roles := []domain.RoleRef{"catalogo", "expedicao"}
	want := domain.MemberAccessChanged{Subject: subject, Roles: roles, AccessVersion: 2, At: later}
	if events := acc.Events(); len(events) != 1 || !sameEvent(events[0], want) {
		t.Fatalf("Events() = %+v, want [%+v]", events, want)
	}
	wantState := domain.MembershipSnapshot{
		Subject: subject, Email: email, Roles: roles, Status: domain.MembershipStatusActive,
		AccessVersion: 2, JoinedAt: at, ChangedAt: later,
	}
	if got := m.Snapshot(); !got.Equal(wantState) {
		t.Fatalf("Snapshot() = %+v, want %+v: the email it had stays", got, wantState)
	}
}

func TestEstablishOfHeldRolesAcceptsWithoutChangingOrEmitting(t *testing.T) {
	m := newOwner(t)
	before := m.Snapshot()

	acc, rej := m.Establish(domain.EstablishMembership{Subject: owner, Roles: []domain.RoleRef{domain.OwnerRole}, At: later})

	tb.RequireAccepted(t, rej)
	if len(acc.Events()) != 0 {
		t.Fatalf("Events() len = %d, want 0: the owner claiming again changes nothing", len(acc.Events()))
	}
	requireMembershipUnchanged(t, before, m.Snapshot())
}

// The email of a member established by claiming the ownership is filled by
// the first establishment that brings one, without a change of access.
func TestEstablishFillsAMissingEmailWithoutEmitting(t *testing.T) {
	m := newOwner(t)

	acc, rej := m.Establish(domain.EstablishMembership{Subject: owner, Email: email, Roles: []domain.RoleRef{domain.OwnerRole}, At: later})

	tb.RequireAccepted(t, rej)
	if len(acc.Events()) != 0 {
		t.Fatalf("Events() len = %d, want 0: the roles did not change", len(acc.Events()))
	}
	if got := m.Snapshot(); got.Email != email || got.AccessVersion != 1 || got.ChangedAt != at {
		t.Fatalf("Snapshot() = %+v, want the email filled and the access unchanged", got)
	}
}

func TestEstablishRejectsASubjectOutsideItsForm(t *testing.T) {
	for _, s := range []string{"", strings.Repeat("s", 256), "a\u0000b", "a\u0007b", "a\u0085b"} {
		m := domain.NewMembership(domain.Subject(s))
		before := m.Snapshot()

		acc, rej := m.Establish(domain.EstablishMembership{Subject: domain.Subject(s), Roles: invitedRoles, At: at})

		tb.RequireRejected(t, acc, rej, domain.CodeMembershipSubjectInvalid)
		requireMembershipUnchanged(t, before, m.Snapshot())
	}
}

func TestEstablishRejectsEmptyRoles(t *testing.T) {
	acc, rej := domain.NewMembership(subject).Establish(domain.EstablishMembership{Subject: subject, At: at})

	tb.RequireRejected(t, acc, rej, domain.CodeMembershipLastRole)
}

func TestEstablishRejectsARoleOutsideTheRefForm(t *testing.T) {
	acc, rej := domain.NewMembership(subject).Establish(domain.EstablishMembership{Subject: subject, Roles: []domain.RoleRef{"Exp_1"}, At: at})

	tb.RequireRejected(t, acc, rej, domain.CodeMembershipRoleInvalid)
}

func TestTheOwnerClaimedRejectionCarriesItsCode(t *testing.T) {
	if got := domain.RejectMembershipOwnerClaimed.Code(); got != domain.CodeMembershipOwnerClaimed {
		t.Fatalf("Code() = %q, want %q", got, domain.CodeMembershipOwnerClaimed)
	}
}

func TestOwnsTellsTheActiveOwnerApart(t *testing.T) {
	removed := newRemovedMember(t).Snapshot()
	removed.Roles = []domain.RoleRef{domain.OwnerRole}
	for name, tc := range map[string]struct {
		snapshot domain.MembershipSnapshot
		want     bool
	}{
		"active owner":         {newOwner(t).Snapshot(), true},
		"active member":        {newMember(t, "expedicao").Snapshot(), false},
		"removed with owner":   {removed, false},
		"undefined membership": {domain.NewMembership(subject).Snapshot(), false},
	} {
		if got := tc.snapshot.Owns(); got != tc.want {
			t.Errorf("%s: Owns() = %v, want %v", name, got, tc.want)
		}
	}
}
