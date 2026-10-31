package rpc_test

import (
	"context"
	"slices"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/memory"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/execctx"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/service/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func claim(t *testing.T, h *harness, subject string) *servicev1.ClaimOwnershipResponse {
	t.Helper()
	var resp servicev1.ClaimOwnershipResponse
	if err := h.invoke(withTenant(t), "ClaimOwnership", &servicev1.ClaimOwnershipRequest{Subject: subject}, &resp); err != nil {
		t.Fatalf("ClaimOwnership(%s) = %v", subject, err)
	}
	return &resp
}

func membership(t *testing.T, h *harness, subject string) *servicev1.Membership {
	t.Helper()
	var found servicev1.FindMembershipResponse
	if err := h.invoke(withoutKey(t), "FindMembership", &servicev1.FindMembershipRequest{Subject: subject}, &found); err != nil {
		t.Fatalf("FindMembership(%s) = %v", subject, err)
	}
	return found.GetMembership()
}

func TestInviteMemberAnswersTheNormalizedAddressPending(t *testing.T) {
	h := newHarness(t, nil)

	var resp servicev1.InviteMemberResponse
	if err := h.invoke(withTenant(t), "InviteMember", &servicev1.InviteMemberRequest{Email: " Ana@Loja.com ", Roles: []string{"expedicao"}}, &resp); err != nil {
		t.Fatalf("InviteMember() = %v", err)
	}

	if invited := resp.GetInvited(); invited.GetEmail() != "ana@loja.com" || invited.GetStatus() != servicev1.InvitationStatus_INVITATION_STATUS_PENDING {
		t.Fatalf("InviteMember() = %v, want ana@loja.com PENDING", &resp)
	}
	var found servicev1.FindInvitationResponse
	if err := h.invoke(withoutKey(t), "FindInvitation", &servicev1.FindInvitationRequest{Email: "ana@loja.com"}, &found); err != nil {
		t.Fatalf("FindInvitation() = %v", err)
	}
	if inv := found.GetInvitation(); inv.GetRound() != 1 || !slices.Equal(inv.GetRoles(), []string{"expedicao"}) || inv.GetInvitedAt() != int64(occurred) {
		t.Fatalf("FindInvitation() = %v, want round 1 with [expedicao] at the domain instant", inv)
	}
}

func TestTheBodyOfAnInvitationIsJudgedByTheDomain(t *testing.T) {
	for _, tc := range []struct {
		email string
		roles []string
		want  string
	}{
		{"ana", []string{"expedicao"}, string(domain.CodeInvitationEmailInvalid)},
		{"ana@loja.com", nil, string(domain.CodeInvitationRolesEmpty)},
		{"ana@loja.com", []string{"owner"}, string(domain.CodeInvitationRoleReserved)},
	} {
		h := newHarness(t, nil)

		var resp servicev1.InviteMemberResponse
		if err := h.invoke(withTenant(t), "InviteMember", &servicev1.InviteMemberRequest{Email: tc.email, Roles: tc.roles}, &resp); err != nil {
			t.Fatalf("InviteMember(%s, %v) = %v", tc.email, tc.roles, err)
		}
		if got := resp.GetRejection().GetCode(); got != tc.want {
			t.Errorf("InviteMember(%s, %v) = %v, want the rejection %s", tc.email, tc.roles, &resp, tc.want)
		}
	}
}

func TestFindInvitationRefusesAnAddressOutsideTheFormAtTheEdge(t *testing.T) {
	h := newHarness(t, nil)

	err := h.invoke(withoutKey(t), "FindInvitation", &servicev1.FindInvitationRequest{Email: "ana"}, &servicev1.FindInvitationResponse{})

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("FindInvitation(ana) = %v, want InvalidArgument", err)
	}
}

func TestTheOwnerClaimsOnceAndASecondSubjectIsOwnerClaimed(t *testing.T) {
	h := newHarness(t, nil)

	if got := claim(t, h, "kc-owner").GetClaimed().GetSubject(); got != "kc-owner" {
		t.Fatalf("ClaimOwnership(kc-owner) = %q, want Claimed kc-owner", got)
	}
	owner := membership(t, h, "kc-owner")
	if owner.GetStatus() != servicev1.MembershipStatus_MEMBERSHIP_STATUS_ACTIVE || !slices.Equal(owner.GetRoles(), []string{"owner"}) || owner.GetAccessVersion() != 1 {
		t.Fatalf("FindMembership(kc-owner) = %v, want ACTIVE [owner] at access version 1", owner)
	}

	if got := claim(t, h, "kc-123").GetRejection().GetCode(); got != string(domain.CodeMembershipOwnerClaimed) {
		t.Fatalf("ClaimOwnership(kc-123) = %q, want %s", got, domain.CodeMembershipOwnerClaimed)
	}
	if got := claim(t, h, "kc-owner").GetClaimed().GetSubject(); got != "kc-owner" {
		t.Fatalf("ClaimOwnership(kc-owner) again = %q, want Claimed", got)
	}
	if n := len(h.store.Entries()); n != 1 {
		t.Fatalf("outbox = %d entries, want only the first establishment", n)
	}
}

// seedMember writes an active member of the store straight to its table: the
// membership of an invitation is established by its consumption, not by an RPC.
func seedMember(t *testing.T, h *harness, subject domain.Subject, roles ...domain.RoleRef) {
	t.Helper()
	uow := memory.NewUnitOfWork(h.store, func(tx *memory.Tx) application.Resources {
		return application.Resources{Memberships: membershipsTable.Repository(tx)}
	})
	ctx := execctx.WithExecution(t, context.Background(), execctx.WithTenant(testTenant), execctx.AutoKey())
	if err := uow.Within(ctx, func(ctx context.Context, res application.Resources) error {
		return res.Memberships.Save(ctx, subject, domain.MembershipSnapshot{
			Subject: subject, Email: "ana@loja.com", Roles: roles, Status: domain.MembershipStatusActive, AccessVersion: 1, JoinedAt: 1, ChangedAt: 1,
		}, 0)
	}); err != nil {
		t.Fatalf("seedMember(%s) = %v", subject, err)
	}
}

func TestGrantRevokeAndRemoveOverTheEdge(t *testing.T) {
	h := newHarness(t, nil)
	seedMember(t, h, "kc-123", "expedicao")

	var granted servicev1.GrantRoleResponse
	if err := h.invoke(withTenant(t), "GrantRole", &servicev1.GrantRoleRequest{Subject: "kc-123", Role: "catalogo"}, &granted); err != nil {
		t.Fatalf("GrantRole() = %v", err)
	}
	if granted.GetGranted().GetSubject() != "kc-123" || !slices.Equal(membership(t, h, "kc-123").GetRoles(), []string{"catalogo", "expedicao"}) {
		t.Fatalf("GrantRole() = %v, want Granted kc-123 with catalogo next to expedicao", &granted)
	}

	var revoked servicev1.RevokeRoleResponse
	if err := h.invoke(withTenant(t), "RevokeRole", &servicev1.RevokeRoleRequest{Subject: "kc-123", Role: "expedicao"}, &revoked); err != nil {
		t.Fatalf("RevokeRole() = %v", err)
	}
	if revoked.GetRevoked().GetSubject() != "kc-123" || !slices.Equal(membership(t, h, "kc-123").GetRoles(), []string{"catalogo"}) {
		t.Fatalf("RevokeRole() = %v, want Revoked kc-123 left with catalogo", &revoked)
	}

	var removed servicev1.RemoveMemberResponse
	if err := h.invoke(withTenant(t), "RemoveMember", &servicev1.RemoveMemberRequest{Subject: "kc-123"}, &removed); err != nil {
		t.Fatalf("RemoveMember() = %v", err)
	}
	if removed.GetRemoved().GetSubject() != "kc-123" || membership(t, h, "kc-123").GetStatus() != servicev1.MembershipStatus_MEMBERSHIP_STATUS_REMOVED {
		t.Fatalf("RemoveMember() = %v, want Removed kc-123 and the membership REMOVED", &removed)
	}
	if n := len(h.store.Entries()); n != 3 {
		t.Fatalf("outbox = %d entries, want granted, revoked and removed", n)
	}
}

func TestTheOwnerIsProtectedOverTheEdge(t *testing.T) {
	h := newHarness(t, nil)
	claim(t, h, "kc-owner")

	var revoked servicev1.RevokeRoleResponse
	if err := h.invoke(withTenant(t), "RevokeRole", &servicev1.RevokeRoleRequest{Subject: "kc-owner", Role: "owner"}, &revoked); err != nil {
		t.Fatalf("RevokeRole() = %v", err)
	}
	if got := revoked.GetRejection().GetCode(); got != string(domain.CodeMembershipOwnerProtected) || revoked.GetRevoked() != nil {
		t.Fatalf("RevokeRole(owner) = %v, want only the rejection %s", &revoked, domain.CodeMembershipOwnerProtected)
	}

	var removed servicev1.RemoveMemberResponse
	if err := h.invoke(withTenant(t), "RemoveMember", &servicev1.RemoveMemberRequest{Subject: "kc-owner"}, &removed); err != nil {
		t.Fatalf("RemoveMember() = %v", err)
	}
	if got := removed.GetRejection().GetCode(); got != string(domain.CodeMembershipOwnerProtected) || removed.GetRemoved() != nil {
		t.Fatalf("RemoveMember(owner) = %v, want only the rejection %s", &removed, domain.CodeMembershipOwnerProtected)
	}
}

func TestAMembershipCommandOverAnUnknownSubjectIsNotFound(t *testing.T) {
	h := newHarness(t, nil)

	err := h.invoke(withTenant(t), "RemoveMember", &servicev1.RemoveMemberRequest{Subject: "kc-nenhum"}, &servicev1.RemoveMemberResponse{})

	if status.Code(err) != codes.NotFound {
		t.Fatalf("RemoveMember(unknown) = %v, want NotFound", err)
	}
	if err := h.invoke(withoutKey(t), "FindMembership", &servicev1.FindMembershipRequest{Subject: "kc-nenhum"}, &servicev1.FindMembershipResponse{}); status.Code(err) != codes.NotFound {
		t.Fatalf("FindMembership(unknown) = %v, want NotFound", err)
	}
}

func TestTheByStatusQueriesOfAccessRefuseAnUnspecifiedStatus(t *testing.T) {
	h := newHarness(t, nil)

	if err := h.invoke(withoutKey(t), "FindInvitationsByStatus", &servicev1.FindInvitationsByStatusRequest{}, &servicev1.FindInvitationsByStatusResponse{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("FindInvitationsByStatus(unspecified) = %v, want InvalidArgument", err)
	}
	if err := h.invoke(withoutKey(t), "FindMembershipsByStatus", &servicev1.FindMembershipsByStatusRequest{}, &servicev1.FindMembershipsByStatusResponse{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("FindMembershipsByStatus(unspecified) = %v, want InvalidArgument", err)
	}
}

func TestTheByStatusQueriesOfAccessAnswerAnEmptyStatusWithNothing(t *testing.T) {
	h := newHarness(t, nil)

	var invitations servicev1.FindInvitationsByStatusResponse
	if err := h.invoke(withoutKey(t), "FindInvitationsByStatus", &servicev1.FindInvitationsByStatusRequest{Status: servicev1.InvitationStatus_INVITATION_STATUS_FAILED}, &invitations); err != nil || len(invitations.GetInvitations()) != 0 {
		t.Fatalf("FindInvitationsByStatus(failed) = %v, %v; want none", &invitations, err)
	}
	var memberships servicev1.FindMembershipsByStatusResponse
	if err := h.invoke(withoutKey(t), "FindMembershipsByStatus", &servicev1.FindMembershipsByStatusRequest{Status: servicev1.MembershipStatus_MEMBERSHIP_STATUS_REMOVED}, &memberships); err != nil || len(memberships.GetMemberships()) != 0 {
		t.Fatalf("FindMembershipsByStatus(removed) = %v, %v; want none", &memberships, err)
	}
}
