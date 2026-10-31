package rpc_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/service/v1"
)

func refs(n int) []string {
	out := make([]string, 0, n)
	for i := range n {
		out = append(out, fmt.Sprintf("papel-%02d", i))
	}
	return out
}

func requireLimit(t *testing.T, method string, err error, field string) {
	t.Helper()
	if status.Code(err) != codes.InvalidArgument || !strings.Contains(status.Convert(err).Message(), field) || !strings.Contains(status.Convert(err).Message(), "32") {
		t.Fatalf("%s with 33 %s = %v, want InvalidArgument naming %s and the ceiling 32", method, field, err, field)
	}
}

// The ceilings are of the transport, not of the domain: 32 items cross the
// edge, the 33rd is refused before any use case.
func TestThirtyTwoRolesOfAnInvitationCrossTheEdgeAndThirtyThreeDoNot(t *testing.T) {
	h := newHarness(t, nil)

	err := h.invoke(withTenant(t), "InviteMember", &servicev1.InviteMemberRequest{Email: "ana@loja.com", Roles: refs(33)}, &servicev1.InviteMemberResponse{})
	requireLimit(t, "InviteMember", err, "roles")

	var invited servicev1.InviteMemberResponse
	if err := h.invoke(withTenant(t), "InviteMember", &servicev1.InviteMemberRequest{Email: "ana@loja.com", Roles: refs(32)}, &invited); err != nil || invited.GetInvited().GetEmail() != "ana@loja.com" {
		t.Fatalf("InviteMember(32 roles) = %v, %v; want Invited", &invited, err)
	}
	if n := len(h.store.Entries()); n != 1 {
		t.Fatalf("outbox = %d entries, want only the accepted invitation", n)
	}
}

func TestThirtyTwoPermissionsOfARoleCrossTheEdgeAndThirtyThreeDoNot(t *testing.T) {
	h := newHarness(t, nil)
	thirtyTwo := slices.Repeat([]string{"catalog:read"}, 32)
	thirtyThree := slices.Repeat([]string{"catalog:read"}, 33)

	err := h.invoke(withTenant(t), "DefineRole", &servicev1.DefineRoleRequest{Ref: "expedicao", Name: "Expedição", Permissions: thirtyThree}, &servicev1.DefineRoleResponse{})
	requireLimit(t, "DefineRole", err, "permissions")
	if got := define(t, h, "expedicao", "Expedição", thirtyTwo...); got.GetDefined().GetRef() != "expedicao" {
		t.Fatalf("DefineRole(32 permissions) = %v, want Defined", got)
	}

	err = h.invoke(withTenant(t), "ChangeRolePermissions", &servicev1.ChangeRolePermissionsRequest{Ref: "expedicao", Permissions: thirtyThree}, &servicev1.ChangeRolePermissionsResponse{})
	requireLimit(t, "ChangeRolePermissions", err, "permissions")
	var changed servicev1.ChangeRolePermissionsResponse
	if err := h.invoke(withTenant(t), "ChangeRolePermissions", &servicev1.ChangeRolePermissionsRequest{Ref: "expedicao", Permissions: slices.Repeat([]string{"catalog:write"}, 32)}, &changed); err != nil || changed.GetPermissionsChanged().GetRef() != "expedicao" {
		t.Fatalf("ChangeRolePermissions(32 permissions) = %v, %v; want PermissionsChanged", &changed, err)
	}
	if n := len(h.store.Entries()); n != 2 {
		t.Fatalf("outbox = %d entries, want the definition and the change only", n)
	}
}
