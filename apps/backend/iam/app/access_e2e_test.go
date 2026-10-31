//go:build integration

package app_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/execctx"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/pg"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/app/rpc"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/appkit"
	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/service/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

func claimOverGRPC(t *testing.T, conn *grpc.ClientConn, tenant, subject string) *servicev1.ClaimOwnershipResponse {
	t.Helper()
	var resp servicev1.ClaimOwnershipResponse
	if err := conn.Invoke(call(t, tenant), rpc.FullMethod("ClaimOwnership"), &servicev1.ClaimOwnershipRequest{Subject: subject}, &resp); err != nil {
		t.Fatalf("ClaimOwnership(%s) = %v", subject, err)
	}
	return &resp
}

func membershipOverGRPC(t *testing.T, conn *grpc.ClientConn, subject string) *servicev1.Membership {
	t.Helper()
	var found servicev1.FindMembershipResponse
	if err := conn.Invoke(call(t, storeTenant), rpc.FullMethod("FindMembership"), &servicev1.FindMembershipRequest{Subject: subject}, &found); err != nil {
		t.Fatalf("FindMembership(%s) = %v", subject, err)
	}
	return found.GetMembership()
}

// seedMember writes an active member of the store straight to its table: the
// membership of an invitation is established by its consumption, not by an RPC.
func seedMember(t *testing.T, pool *pgxpool.Pool, subject domain.Subject, roles ...domain.RoleRef) {
	t.Helper()
	ctx := execctx.WithExecution(t, context.Background(), execctx.WithTenant(storeTenant), execctx.AutoKey())
	pg.Seed(t, ctx, pool, provider.NewMembershipRepository, subject, domain.MembershipSnapshot{
		Subject: subject, Email: "ana@loja.com", Roles: roles, Status: domain.MembershipStatusActive,
		AccessVersion: 1, JoinedAt: 1, ChangedAt: 1,
	})
}

func TestTheSeededOwnerClaimsTheStoreOverGRPC(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)

	resp := claimOverGRPC(t, conn, storeTenant, "kc-owner")

	if resp.GetClaimed().GetSubject() != "kc-owner" {
		t.Fatalf("ClaimOwnership() = %v, want Claimed kc-owner", resp)
	}
	owner := membershipOverGRPC(t, conn, "kc-owner")
	if owner.GetStatus() != servicev1.MembershipStatus_MEMBERSHIP_STATUS_ACTIVE || !slices.Equal(owner.GetRoles(), []string{"owner"}) || owner.GetAccessVersion() != 1 || owner.GetEmail() != "" {
		t.Fatalf("FindMembership(kc-owner) = %v, want ACTIVE [owner] at access version 1 and no email", owner)
	}
	if n := outboxRows(t, pool); n != 1 {
		t.Fatalf("outbox rows = %d, want the one membership established", n)
	}
}

func TestAnotherMemberClaimingIsOwnerClaimedAndNothingChanges(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)
	seedMember(t, pool, "kc-123", "expedicao")
	claimOverGRPC(t, conn, storeTenant, "kc-owner")
	before := membershipOverGRPC(t, conn, "kc-123")

	resp := claimOverGRPC(t, conn, storeTenant, "kc-123")

	if got := resp.GetRejection().GetCode(); got != string(domain.CodeMembershipOwnerClaimed) {
		t.Fatalf("ClaimOwnership(kc-123) = %v, want the rejection %s by the outcome", resp, domain.CodeMembershipOwnerClaimed)
	}
	if after := membershipOverGRPC(t, conn, "kc-123"); !proto.Equal(after, before) {
		t.Fatalf("kc-123 = %v, want it unchanged %v", after, before)
	}
	if owner := membershipOverGRPC(t, conn, "kc-owner"); owner.GetAccessVersion() != 1 || !slices.Equal(owner.GetRoles(), []string{"owner"}) {
		t.Fatalf("kc-owner = %v, want it unchanged", owner)
	}
	if n := outboxRows(t, pool); n != 1 {
		t.Fatalf("outbox rows = %d, want only the establishment of the owner", n)
	}
}

func TestTheOwnerClaimingAgainHasNoEffectNorEvent(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)
	claimOverGRPC(t, conn, storeTenant, "kc-owner")
	before := membershipOverGRPC(t, conn, "kc-owner")

	resp := claimOverGRPC(t, conn, storeTenant, "kc-owner")

	if resp.GetClaimed().GetSubject() != "kc-owner" {
		t.Fatalf("ClaimOwnership() again = %v, want Claimed", resp)
	}
	if after := membershipOverGRPC(t, conn, "kc-owner"); !proto.Equal(after, before) {
		t.Fatalf("kc-owner = %v, want it unchanged %v", after, before)
	}
	if n := outboxRows(t, pool); n != 1 {
		t.Fatalf("outbox rows = %d, want 1: claiming again emits nothing", n)
	}
}

func TestEachStoreHasItsOwnOwner(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)
	claimOverGRPC(t, conn, storeTenant, "kc-owner")

	resp := claimOverGRPC(t, conn, "loja-outra", "kc-outro")

	if resp.GetClaimed().GetSubject() != "kc-outro" {
		t.Fatalf("ClaimOwnership() in loja-outra = %v, want Claimed: the owner is unique per store", resp)
	}
}

func TestInvitingAndManagingAMemberOverGRPC(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)
	seedMember(t, pool, "kc-123", "expedicao")

	var invited servicev1.InviteMemberResponse
	if err := conn.Invoke(call(t, storeTenant), rpc.FullMethod("InviteMember"), &servicev1.InviteMemberRequest{Email: " Bia@Loja.com ", Roles: []string{"catalogo"}}, &invited); err != nil {
		t.Fatalf("InviteMember() = %v", err)
	}
	var pending servicev1.FindInvitationsByStatusResponse
	if err := conn.Invoke(call(t, storeTenant), rpc.FullMethod("FindInvitationsByStatus"), &servicev1.FindInvitationsByStatusRequest{Status: servicev1.InvitationStatus_INVITATION_STATUS_PENDING}, &pending); err != nil {
		t.Fatalf("FindInvitationsByStatus() = %v", err)
	}
	if got := pending.GetInvitations(); len(got) != 1 || got[0].GetEmail() != "bia@loja.com" || got[0].GetRound() != 1 {
		t.Fatalf("FindInvitationsByStatus(pending) = %v, want bia@loja.com in round 1", got)
	}

	var granted servicev1.GrantRoleResponse
	if err := conn.Invoke(call(t, storeTenant), rpc.FullMethod("GrantRole"), &servicev1.GrantRoleRequest{Subject: "kc-123", Role: "catalogo"}, &granted); err != nil {
		t.Fatalf("GrantRole() = %v", err)
	}
	if granted.GetGranted().GetSubject() != "kc-123" || granted.GetRejection() != nil {
		t.Fatalf("GrantRole() = %v, want Granted kc-123", &granted)
	}
	var revoked servicev1.RevokeRoleResponse
	if err := conn.Invoke(call(t, storeTenant), rpc.FullMethod("RevokeRole"), &servicev1.RevokeRoleRequest{Subject: "kc-123", Role: "expedicao"}, &revoked); err != nil {
		t.Fatalf("RevokeRole() = %v", err)
	}
	if revoked.GetRevoked().GetSubject() != "kc-123" || revoked.GetRejection() != nil {
		t.Fatalf("RevokeRole() = %v, want Revoked kc-123", &revoked)
	}
	var removedMember servicev1.RemoveMemberResponse
	if err := conn.Invoke(call(t, storeTenant), rpc.FullMethod("RemoveMember"), &servicev1.RemoveMemberRequest{Subject: "kc-123"}, &removedMember); err != nil {
		t.Fatalf("RemoveMember() = %v", err)
	}
	if removedMember.GetRemoved().GetSubject() != "kc-123" || removedMember.GetRejection() != nil {
		t.Fatalf("RemoveMember() = %v, want Removed kc-123", &removedMember)
	}
	var removed servicev1.FindMembershipsByStatusResponse
	if err := conn.Invoke(call(t, storeTenant), rpc.FullMethod("FindMembershipsByStatus"), &servicev1.FindMembershipsByStatusRequest{Status: servicev1.MembershipStatus_MEMBERSHIP_STATUS_REMOVED}, &removed); err != nil {
		t.Fatalf("FindMembershipsByStatus() = %v", err)
	}
	if got := removed.GetMemberships(); len(got) != 1 || !slices.Equal(got[0].GetRoles(), []string{"catalogo"}) || got[0].GetAccessVersion() != 4 {
		t.Fatalf("FindMembershipsByStatus(removed) = %v, want kc-123 with [catalogo] at access version 4", got)
	}
	if n := outboxRows(t, pool); n != 4 {
		t.Fatalf("outbox rows = %d, want 4: invited, granted, revoked and removed", n)
	}
}

// The operator holds iam:write as a realm role; invitations and memberships
// live in the tenant of a store, so its calls are refused before any write.
func TestTheOperatorIsPermissionDeniedOnInvitationsAndMembershipsAndNothingIsWritten(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)

	for method, req := range map[string][2]proto.Message{
		"InviteMember":   {&servicev1.InviteMemberRequest{Email: "ana@loja.com", Roles: []string{"expedicao"}}, &servicev1.InviteMemberResponse{}},
		"ClaimOwnership": {&servicev1.ClaimOwnershipRequest{Subject: "kc-owner"}, &servicev1.ClaimOwnershipResponse{}},
		"GrantRole":      {&servicev1.GrantRoleRequest{Subject: "kc-123", Role: "catalogo"}, &servicev1.GrantRoleResponse{}},
		"RevokeRole":     {&servicev1.RevokeRoleRequest{Subject: "kc-123", Role: "catalogo"}, &servicev1.RevokeRoleResponse{}},
		"RemoveMember":   {&servicev1.RemoveMemberRequest{Subject: "kc-123"}, &servicev1.RemoveMemberResponse{}},
	} {
		if err := conn.Invoke(call(t, operatorTenant), rpc.FullMethod(method), req[0], req[1]); status.Code(err) != codes.PermissionDenied {
			t.Errorf("%s() from the operator = %v, want PermissionDenied", method, err)
		}
	}

	for _, table := range []string{"invitations", "memberships", "outbox"} {
		if n := countRows(t, pool, table); n != 0 {
			t.Errorf("%s rows = %d, want none", table, n)
		}
	}
}

// The policy of the composition root is SHA-256, so the trail records the
// address as the hex digest the kernel gives a revoked subject, never in clear.
func TestTheAuditOfAnInvitationCarriesTheSHA256OfTheAddress(t *testing.T) {
	pool := appkit.OpenPool(t)
	spy := &auditSpy{}
	conn := dialWith(t, pool, spy)

	if err := conn.Invoke(call(t, storeTenant), rpc.FullMethod("InviteMember"), &servicev1.InviteMemberRequest{Email: " Ana@Loja.com ", Roles: []string{"expedicao"}}, &servicev1.InviteMemberResponse{}); err != nil {
		t.Fatalf("InviteMember() = %v", err)
	}

	const digest = "9931188e0342b5eb11fcc6d21c7121f04163e4180e53923b678e8383db55af4e"
	spy.mu.Lock()
	defer spy.mu.Unlock()
	if len(spy.audits) != 1 || spy.audits[0].Object != digest {
		t.Fatalf("audits = %+v, want one with the sha256 of ana@loja.com", spy.audits)
	}
}
