package domain_test

import (
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

const (
	roleRef  = domain.RoleRef("expedicao")
	roleName = domain.RoleName("Expedição")
	at       = kernel.Instant(1_755_432_000_000_000_000)
	later    = kernel.Instant(1_755_435_600_000_000_000)
)

var rolePermissions = []domain.Permission{"fulfillment:write", "ordering:read"}

func newActiveRole(t *testing.T) *domain.Role {
	t.Helper()
	r := domain.NewRole(roleRef)
	_, rej := r.Define(domain.DefineRole{Ref: roleRef, Name: roleName, Permissions: rolePermissions, At: at})
	tb.RequireAccepted(t, rej)
	return r
}

func newRetiredRole(t *testing.T) *domain.Role {
	t.Helper()
	r := newActiveRole(t)
	_, rej := r.Retire(domain.RetireRole{At: at})
	tb.RequireAccepted(t, rej)
	return r
}

func requireRoleUnchanged(t *testing.T, before, after domain.RoleSnapshot) {
	t.Helper()
	if !before.Equal(after) {
		t.Fatalf("snapshot changed on rejection\nbefore: %+v\nafter:  %+v", before, after)
	}
}

const (
	email   = domain.Email("ana@loja.com")
	subject = domain.Subject("kc-123")
	owner   = domain.Subject("kc-owner")
)

var invitedRoles = []domain.RoleRef{"expedicao"}

func newPendingInvitation(t *testing.T) *domain.Invitation {
	t.Helper()
	i := domain.NewInvitation(email)
	_, rej := i.Invite(domain.InviteMember{Email: string(email), Roles: invitedRoles, At: at})
	tb.RequireAccepted(t, rej)
	return i
}

func newProvisionedInvitation(t *testing.T) *domain.Invitation {
	t.Helper()
	i := newPendingInvitation(t)
	_, rej := i.Confirm(domain.ConfirmInvitation{Subject: subject, At: at})
	tb.RequireAccepted(t, rej)
	return i
}

func newFailedInvitation(t *testing.T) *domain.Invitation {
	t.Helper()
	i := newPendingInvitation(t)
	_, rej := i.Fail(domain.FailInvitation{Reason: "e-mail recusado pelo Keycloak", At: at})
	tb.RequireAccepted(t, rej)
	return i
}

func requireInvitationUnchanged(t *testing.T, before, after domain.InvitationSnapshot) {
	t.Helper()
	if !before.Equal(after) {
		t.Fatalf("snapshot changed on rejection\nbefore: %+v\nafter:  %+v", before, after)
	}
}

// newMember is an active member established by an invitation, with roles.
func newMember(t *testing.T, roles ...domain.RoleRef) *domain.Membership {
	t.Helper()
	m := domain.NewMembership(subject)
	_, rej := m.Establish(domain.EstablishMembership{Subject: subject, Email: email, Roles: roles, At: at})
	tb.RequireAccepted(t, rej)
	return m
}

func newOwner(t *testing.T) *domain.Membership {
	t.Helper()
	m := domain.NewMembership(owner)
	_, rej := m.Establish(domain.EstablishMembership{Subject: owner, Roles: []domain.RoleRef{domain.OwnerRole}, At: at})
	tb.RequireAccepted(t, rej)
	return m
}

func newRemovedMember(t *testing.T) *domain.Membership {
	t.Helper()
	m := newMember(t, "expedicao", "catalogo")
	_, rej := m.Remove(domain.RemoveMember{At: at})
	tb.RequireAccepted(t, rej)
	return m
}

func requireMembershipUnchanged(t *testing.T, before, after domain.MembershipSnapshot) {
	t.Helper()
	if !before.Equal(after) {
		t.Fatalf("snapshot changed on rejection\nbefore: %+v\nafter:  %+v", before, after)
	}
}

const buyerName = domain.BuyerName("Bia")

func newActiveBuyer(t *testing.T) *domain.Buyer {
	t.Helper()
	b := domain.NewBuyer(subject)
	_, rej := b.Register(domain.RegisterBuyer{Subject: subject, Name: buyerName, At: at})
	tb.RequireAccepted(t, rej)
	return b
}

func newBlockedBuyer(t *testing.T) *domain.Buyer {
	t.Helper()
	b := newActiveBuyer(t)
	_, rej := b.Block(domain.BlockBuyer{Reason: "chargeback recorrente", At: at})
	tb.RequireAccepted(t, rej)
	return b
}

func requireBuyerUnchanged(t *testing.T, before, after domain.BuyerSnapshot) {
	t.Helper()
	if !before.Equal(after) {
		t.Fatalf("snapshot changed on rejection\nbefore: %+v\nafter:  %+v", before, after)
	}
}
