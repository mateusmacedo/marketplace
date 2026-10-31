package domain_test

import (
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

type snapshotter[S any] interface{ Snapshot() S }

type equaler[S any] interface{ Equal(S) bool }

func requireSameDecision[T snapshotter[S], S equaler[S], R any](t *testing.T, newTarget func(*testing.T) T, decide func(T) (kernel.Accepted[R], *kernel.Rejection)) {
	t.Helper()
	first, second := newTarget(t), newTarget(t)
	acc1, rej1 := decide(first)
	acc2, rej2 := decide(second)

	if !first.Snapshot().Equal(second.Snapshot()) {
		t.Fatal("same inputs must produce the same state")
	}
	if !sameEvents(acc1.Events(), acc2.Events()) || !sameRejection(rej1, rej2) {
		t.Fatalf("same inputs must produce the same outcome\nfirst:  %+v %v\nsecond: %+v %v", acc1.Events(), rej1, acc2.Events(), rej2)
	}
}

func TestDefineDeterminism(t *testing.T) {
	cmd := domain.DefineRole{Ref: roleRef, Name: roleName, Permissions: []domain.Permission{"ordering:read", "fulfillment:write"}, At: at}
	requireSameDecision[*domain.Role, domain.RoleSnapshot](t, func(*testing.T) *domain.Role { return domain.NewRole(roleRef) },
		func(r *domain.Role) (kernel.Accepted[domain.DefinedResponse], *kernel.Rejection) {
			return r.Define(cmd)
		})
}

func TestChangePermissionsDeterminism(t *testing.T) {
	cmd := domain.ChangeRolePermissions{Permissions: []domain.Permission{"catalog:write"}, At: later}
	requireSameDecision[*domain.Role, domain.RoleSnapshot](t, newActiveRole,
		func(r *domain.Role) (kernel.Accepted[domain.PermissionsChangedResponse], *kernel.Rejection) {
			return r.ChangePermissions(cmd)
		})
}

func TestRetireDeterminism(t *testing.T) {
	cmd := domain.RetireRole{At: later}
	requireSameDecision[*domain.Role, domain.RoleSnapshot](t, newActiveRole,
		func(r *domain.Role) (kernel.Accepted[domain.RetiredResponse], *kernel.Rejection) {
			return r.Retire(cmd)
		})
}

func TestConfirmSyncedDeterminism(t *testing.T) {
	cmd := domain.ConfirmRoleSynced{Version: 1}
	requireSameDecision[*domain.Role, domain.RoleSnapshot](t, newActiveRole,
		func(r *domain.Role) (kernel.Accepted[kernel.Empty], *kernel.Rejection) { return r.ConfirmSynced(cmd) })
}

func TestInvitationDeterminism(t *testing.T) {
	invite := domain.InviteMember{Email: " Ana@Loja.com ", Roles: []domain.RoleRef{"expedicao", "catalogo"}, At: later}
	requireSameDecision[*domain.Invitation, domain.InvitationSnapshot](t, newFailedInvitation,
		func(i *domain.Invitation) (kernel.Accepted[domain.InvitedResponse], *kernel.Rejection) {
			return i.Invite(invite)
		})
	confirm := domain.ConfirmInvitation{Subject: subject, At: later}
	requireSameDecision[*domain.Invitation, domain.InvitationSnapshot](t, newPendingInvitation,
		func(i *domain.Invitation) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
			return i.Confirm(confirm)
		})
	fail := domain.FailInvitation{Reason: "motivo", At: later}
	requireSameDecision[*domain.Invitation, domain.InvitationSnapshot](t, newPendingInvitation,
		func(i *domain.Invitation) (kernel.Accepted[kernel.Empty], *kernel.Rejection) { return i.Fail(fail) })
}

func TestMembershipDeterminism(t *testing.T) {
	member := func(t *testing.T) *domain.Membership { return newMember(t, "expedicao", "catalogo") }
	establish := domain.EstablishMembership{Subject: subject, Email: email, Roles: []domain.RoleRef{"vendas"}, At: later}
	requireSameDecision[*domain.Membership, domain.MembershipSnapshot](t, newRemovedMember,
		func(m *domain.Membership) (kernel.Accepted[domain.EstablishedResponse], *kernel.Rejection) {
			return m.Establish(establish)
		})
	grant := domain.GrantRole{Role: "vendas", At: later}
	requireSameDecision[*domain.Membership, domain.MembershipSnapshot](t, member,
		func(m *domain.Membership) (kernel.Accepted[domain.GrantedResponse], *kernel.Rejection) {
			return m.Grant(grant)
		})
	revoke := domain.RevokeRole{Role: "catalogo", At: later}
	requireSameDecision[*domain.Membership, domain.MembershipSnapshot](t, member,
		func(m *domain.Membership) (kernel.Accepted[domain.RevokedResponse], *kernel.Rejection) {
			return m.Revoke(revoke)
		})
	remove := domain.RemoveMember{At: later}
	requireSameDecision[*domain.Membership, domain.MembershipSnapshot](t, member,
		func(m *domain.Membership) (kernel.Accepted[domain.RemovedResponse], *kernel.Rejection) {
			return m.Remove(remove)
		})
	confirm := domain.ConfirmMemberAccessSynced{Version: 1}
	requireSameDecision[*domain.Membership, domain.MembershipSnapshot](t, member,
		func(m *domain.Membership) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
			return m.ConfirmAccessSynced(confirm)
		})
}

func TestBuyerDeterminism(t *testing.T) {
	register := domain.RegisterBuyer{Subject: subject, Name: buyerName, At: at}
	requireSameDecision[*domain.Buyer, domain.BuyerSnapshot](t, func(*testing.T) *domain.Buyer { return domain.NewBuyer(subject) },
		func(b *domain.Buyer) (kernel.Accepted[domain.RegisteredResponse], *kernel.Rejection) {
			return b.Register(register)
		})
	update := domain.UpdateBuyerProfile{Name: "Bia Souza", At: later}
	requireSameDecision[*domain.Buyer, domain.BuyerSnapshot](t, newActiveBuyer,
		func(b *domain.Buyer) (kernel.Accepted[domain.ProfileUpdatedResponse], *kernel.Rejection) {
			return b.UpdateProfile(update)
		})
	block := domain.BlockBuyer{Reason: "chargeback recorrente", At: later}
	requireSameDecision[*domain.Buyer, domain.BuyerSnapshot](t, newActiveBuyer,
		func(b *domain.Buyer) (kernel.Accepted[domain.BlockedResponse], *kernel.Rejection) {
			return b.Block(block)
		})
	unblock := domain.UnblockBuyer{At: later}
	requireSameDecision[*domain.Buyer, domain.BuyerSnapshot](t, newBlockedBuyer,
		func(b *domain.Buyer) (kernel.Accepted[domain.UnblockedResponse], *kernel.Rejection) {
			return b.Unblock(unblock)
		})
	confirm := domain.ConfirmBuyerAccessSynced{Version: 1}
	requireSameDecision[*domain.Buyer, domain.BuyerSnapshot](t, newActiveBuyer,
		func(b *domain.Buyer) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
			return b.ConfirmAccessSynced(confirm)
		})
}
