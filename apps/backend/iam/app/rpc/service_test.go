package rpc_test

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/app/rpc"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/service/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestServiceDescCoversEveryMethodOfTheDescriptor(t *testing.T) {
	service := servicev1.File_company_iam_service_v1_iam_service_proto.Services().ByName("IamService")

	if rpc.ServiceDesc.ServiceName != string(service.FullName()) || rpc.ServiceName != "company.iam.service.v1.IamService" {
		t.Fatalf("ServiceName = %q, want %q", rpc.ServiceDesc.ServiceName, service.FullName())
	}
	if missing := kernelgrpc.Uncovered(&rpc.ServiceDesc, service); len(missing) != 0 || len(rpc.ServiceDesc.Streams) != 0 {
		t.Fatalf("ServiceDesc leaves %v uncovered and declares %d streams, want every method covered and no stream",
			missing, len(rpc.ServiceDesc.Streams))
	}
	if n := len(rpc.ServiceDesc.Methods); n != 20 {
		t.Fatalf("ServiceDesc serves %d methods, want the 20 RPCs of the spec", n)
	}
}

func TestEveryCommandIsAServedMethod(t *testing.T) {
	served := make([]string, 0, len(rpc.ServiceDesc.Methods))
	for _, method := range rpc.ServiceDesc.Methods {
		served = append(served, method.MethodName)
	}
	for _, command := range rpc.Commands() {
		if !slices.Contains(served, command) {
			t.Errorf("Commands() names %s, which ServiceDesc does not serve", command)
		}
	}
	want := []string{
		"DefineRole", "ChangeRolePermissions", "RetireRole", "InviteMember", "ClaimOwnership", "GrantRole", "RevokeRole", "RemoveMember",
		"RegisterBuyer", "UpdateBuyerProfile", "BlockBuyer", "UnblockBuyer",
	}
	if !slices.Equal(rpc.Commands(), want) {
		t.Fatalf("Commands() = %v, want %v", rpc.Commands(), want)
	}
}

func TestTheServerImplementsTheHandlerType(t *testing.T) {
	handlerType := reflect.TypeOf(rpc.ServiceDesc.HandlerType).Elem()
	if handlerType.Kind() != reflect.Interface || !reflect.TypeOf(rpc.Server{}).Implements(handlerType) {
		t.Fatalf("rpc.Server does not implement %v", handlerType)
	}
}

func define(t *testing.T, h *harness, ref, name string, permissions ...string) *servicev1.DefineRoleResponse {
	t.Helper()
	var resp servicev1.DefineRoleResponse
	if err := h.invoke(withTenant(t), "DefineRole", &servicev1.DefineRoleRequest{Ref: ref, Name: name, Permissions: permissions}, &resp); err != nil {
		t.Fatalf("DefineRole(%s) = %v", ref, err)
	}
	return &resp
}

func find(t *testing.T, h *harness, ref string) *servicev1.Role {
	t.Helper()
	var found servicev1.FindRoleResponse
	if err := h.invoke(withoutKey(t), "FindRole", &servicev1.FindRoleRequest{Ref: ref}, &found); err != nil {
		t.Fatalf("FindRole(%s) = %v", ref, err)
	}
	return found.GetRole()
}

func TestDefineRoleAnswersTheDefinedRefAndTheRoleIsActive(t *testing.T) {
	h := newHarness(t, nil)

	resp := define(t, h, "expedicao", "Expedição", "fulfillment:write", "ordering:read", "ordering:read")

	if got := resp.GetDefined().GetRef(); got != "expedicao" {
		t.Fatalf("Defined.Ref = %q, want expedicao (response %v)", got, resp)
	}
	role := find(t, h, "expedicao")
	if role.GetName() != "Expedição" || role.GetStatus() != servicev1.RoleStatus_ROLE_STATUS_ACTIVE ||
		!slices.Equal(role.GetPermissions(), []string{"fulfillment:write", "ordering:read"}) {
		t.Fatalf("FindRole() = %v, want Expedição, ACTIVE, the permissions normalized", role)
	}
	if role.GetAccessVersion() != 1 || role.GetSyncedVersion() != 0 || role.GetDefinedAt() != int64(occurred) || role.GetChangedAt() != int64(occurred) {
		t.Fatalf("FindRole() = %v, want access version 1, nothing synced and the domain instant %d in nanoseconds", role, occurred)
	}
	if n := len(h.store.Entries()); n != 1 {
		t.Fatalf("outbox = %d entries, want the one definition", n)
	}
}

func TestDefiningAKnownRefAgainChangesNothingAndEmitsNothing(t *testing.T) {
	h := newHarness(t, nil)
	define(t, h, "expedicao", "Expedição", "fulfillment:write")

	resp := define(t, h, "expedicao", "Outro nome", "catalog:write")

	if resp.GetDefined().GetRef() != "expedicao" || find(t, h, "expedicao").GetName() != "Expedição" {
		t.Fatalf("second DefineRole() = %v, want Defined without changing the name", resp)
	}
	if n := len(h.store.Entries()); n != 1 {
		t.Fatalf("outbox = %d entries, want 1: a known ref emits nothing", n)
	}
}

// The form of ref and permissions in the body belongs to the domain, whose
// rejections the spec declares: they travel in the response, never as a status.
func TestTheBodyOfADefinitionIsJudgedByTheDomain(t *testing.T) {
	for _, tc := range []struct {
		name        string
		ref         string
		permissions []string
		want        string
	}{
		{"ref Exp_1", "Exp_1", []string{"fulfillment:write"}, string(domain.CodeRoleRefInvalid)},
		{"ref owner", "owner", []string{"fulfillment:write"}, string(domain.CodeRoleReserved)},
		{"no permissions", "expedicao", nil, string(domain.CodeRolePermissionsEmpty)},
		{"billing:write", "expedicao", []string{"billing:write"}, string(domain.CodeRolePermissionUnknown)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, nil)

			resp := define(t, h, tc.ref, "Expedição", tc.permissions...)

			if got := resp.GetRejection().GetCode(); got != tc.want {
				t.Fatalf("DefineRole() = %v, want the rejection %s", resp, tc.want)
			}
			if n := len(h.store.Entries()); n != 0 {
				t.Fatalf("outbox = %d entries, want none", n)
			}
		})
	}
}

func TestDefiningATakenNameIsADomainRejection(t *testing.T) {
	h := newHarness(t, nil)
	define(t, h, "expedicao", "Expedição", "fulfillment:write")

	resp := define(t, h, "outro-papel", "Expedição", "catalog:write")

	if got := resp.GetRejection().GetCode(); got != string(domain.CodeRoleDuplicateName) {
		t.Fatalf("DefineRole(taken name) = %v, want the rejection %s", resp, domain.CodeRoleDuplicateName)
	}
}

func TestANameOutsideTheTextRuleIsInvalidArgumentBeforeTheUseCase(t *testing.T) {
	for _, name := range []string{"", " ", strings.Repeat("a", 201), "a\u200bb", "\u0301"} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, nil)

			var resp servicev1.DefineRoleResponse
			err := h.invoke(withTenant(t), "DefineRole", &servicev1.DefineRoleRequest{Ref: "expedicao", Name: name, Permissions: []string{"catalog:read"}}, &resp)

			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("DefineRole(name %q) = %v, want InvalidArgument", name, err)
			}
			if n := len(h.store.Entries()); n != 0 {
				t.Fatalf("outbox = %d entries, want none", n)
			}
		})
	}
}

func TestChangeAndRetireWalkTheLifecycleAndThenReject(t *testing.T) {
	h := newHarness(t, nil)
	define(t, h, "expedicao", "Expedição", "fulfillment:write")

	var changed servicev1.ChangeRolePermissionsResponse
	if err := h.invoke(withTenant(t), "ChangeRolePermissions", &servicev1.ChangeRolePermissionsRequest{Ref: "expedicao", Permissions: []string{"catalog:write"}}, &changed); err != nil {
		t.Fatalf("ChangeRolePermissions() = %v", err)
	}
	if changed.GetPermissionsChanged().GetRef() != "expedicao" {
		t.Fatalf("ChangeRolePermissions() = %v, want PermissionsChanged expedicao", &changed)
	}
	if role := find(t, h, "expedicao"); !slices.Equal(role.GetPermissions(), []string{"catalog:write"}) || role.GetAccessVersion() != 2 {
		t.Fatalf("FindRole() = %v, want [catalog:write] at access version 2", role)
	}

	var retired servicev1.RetireRoleResponse
	if err := h.invoke(withTenant(t), "RetireRole", &servicev1.RetireRoleRequest{Ref: "expedicao"}, &retired); err != nil {
		t.Fatalf("RetireRole() = %v", err)
	}
	if retired.GetRetired().GetRef() != "expedicao" || find(t, h, "expedicao").GetStatus() != servicev1.RoleStatus_ROLE_STATUS_RETIRED {
		t.Fatalf("RetireRole() = %v, want Retired and the role RETIRED", &retired)
	}

	var again servicev1.RetireRoleResponse
	if err := h.invoke(withTenant(t), "RetireRole", &servicev1.RetireRoleRequest{Ref: "expedicao"}, &again); err != nil {
		t.Fatalf("RetireRole(retired) = %v", err)
	}
	if got := again.GetRejection().GetCode(); got != string(domain.CodeRoleRetired) {
		t.Fatalf("RetireRole(retired) = %v, want the rejection %s", &again, domain.CodeRoleRetired)
	}
}

// The ref of an existing role travels like a path segment of the bff: outside
// the label form it is refused at the edge, before any use case.
func TestARefOutsideTheLabelFormIsInvalidArgumentOnEveryMethodThatNamesARole(t *testing.T) {
	h := newHarness(t, nil)
	for method, call := range map[string]func() error{
		"ChangeRolePermissions": func() error {
			return h.invoke(withTenant(t), "ChangeRolePermissions", &servicev1.ChangeRolePermissionsRequest{Ref: "Exp_1", Permissions: []string{"catalog:write"}}, &servicev1.ChangeRolePermissionsResponse{})
		},
		"RetireRole": func() error {
			return h.invoke(withTenant(t), "RetireRole", &servicev1.RetireRoleRequest{Ref: "Exp_1"}, &servicev1.RetireRoleResponse{})
		},
		"FindRole": func() error {
			return h.invoke(withoutKey(t), "FindRole", &servicev1.FindRoleRequest{Ref: "Exp_1"}, &servicev1.FindRoleResponse{})
		},
	} {
		if err := call(); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s(Exp_1) = %v, want InvalidArgument", method, err)
		}
	}
}

func TestAnUnknownRoleIsNotFound(t *testing.T) {
	h := newHarness(t, nil)

	if err := h.invoke(withoutKey(t), "FindRole", &servicev1.FindRoleRequest{Ref: "expedicao"}, &servicev1.FindRoleResponse{}); status.Code(err) != codes.NotFound {
		t.Fatalf("FindRole(unknown) = %v, want NotFound", err)
	}
	if err := h.invoke(withTenant(t), "RetireRole", &servicev1.RetireRoleRequest{Ref: "expedicao"}, &servicev1.RetireRoleResponse{}); status.Code(err) != codes.NotFound {
		t.Fatalf("RetireRole(unknown) = %v, want NotFound", err)
	}
}

func TestFindRolesByStatusAnswersThePlantedRolesOfThatStatus(t *testing.T) {
	h := newHarness(t, byStatus{
		domain.RoleStatusRetired: {{Ref: "antigo", Name: "Antigo", Permissions: []domain.Permission{"catalog:read"}, Status: domain.RoleStatusRetired, AccessVersion: 2}},
	})

	var resp servicev1.FindRolesByStatusResponse
	if err := h.invoke(withoutKey(t), "FindRolesByStatus", &servicev1.FindRolesByStatusRequest{Status: servicev1.RoleStatus_ROLE_STATUS_RETIRED}, &resp); err != nil {
		t.Fatalf("FindRolesByStatus() = %v", err)
	}

	if roles := resp.GetRoles(); len(roles) != 1 || roles[0].GetRef() != "antigo" || roles[0].GetStatus() != servicev1.RoleStatus_ROLE_STATUS_RETIRED {
		t.Fatalf("FindRolesByStatus(retired) = %v, want antigo RETIRED", roles)
	}
}

func TestFindRolesByStatusRefusesAnUnspecifiedStatus(t *testing.T) {
	h := newHarness(t, nil)

	err := h.invoke(withoutKey(t), "FindRolesByStatus", &servicev1.FindRolesByStatusRequest{}, &servicev1.FindRolesByStatusResponse{})

	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("FindRolesByStatus(unspecified) = %v, want InvalidArgument", err)
	}
}

func TestADeniedCallIsPermissionDenied(t *testing.T) {
	h := newHarnessAuthorizedBy(t, nil, func(_ context.Context, _ application.Operation) error { return ports.ErrDenied })

	err := h.invoke(withTenant(t), "DefineRole", &servicev1.DefineRoleRequest{Ref: "expedicao", Name: "Expedição", Permissions: []string{"catalog:read"}}, &servicev1.DefineRoleResponse{})

	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("DefineRole() denied = %v, want PermissionDenied", err)
	}
	if n := len(h.store.Entries()); n != 0 {
		t.Fatalf("outbox = %d entries, want none", n)
	}
}

func TestACommandWithoutAKeyIsRefusedBeforeTheHandler(t *testing.T) {
	h := newHarness(t, nil)

	err := h.invoke(withoutKey(t), "DefineRole", &servicev1.DefineRoleRequest{Ref: "expedicao", Name: "Expedição", Permissions: []string{"catalog:read"}}, &servicev1.DefineRoleResponse{})

	if status.Code(err) != codes.InvalidArgument || kernelgrpc.ReasonOf(err) != kernelgrpc.ReasonMissingIdempotencyKey {
		t.Fatalf("DefineRole() without a key = %v, want InvalidArgument with %s", err, kernelgrpc.ReasonMissingIdempotencyKey)
	}
}
