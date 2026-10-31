package rpc

import (
	"context"
	"fmt"
	"unicode"
	"unicode/utf8"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/service/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

const maxTextRunes = 200

// maxListItems bounds each list of a request at the transport only: the domain
// keeps no ceiling of its own on roles or permissions.
const maxListItems = 32

var descriptor = servicev1.File_company_iam_service_v1_iam_service_proto.Services().ByName("IamService")

// ServiceName is the full name the generated descriptor gives the service.
var ServiceName = string(descriptor.FullName())

func Methods() []string { return kernelgrpc.MethodNames(descriptor) }

// Commands lists the methods that require an idempotency key (IDM-01); the
// queries neither require nor read one.
func Commands() []string {
	return []string{
		"DefineRole", "ChangeRolePermissions", "RetireRole",
		"InviteMember", "ClaimOwnership", "GrantRole", "RevokeRole", "RemoveMember",
		"RegisterBuyer", "UpdateBuyerProfile", "BlockBuyer", "UnblockBuyer",
	}
}

// FullMethod is the wire name of a method of the service: /<service>/<method>.
func FullMethod(name string) string { return kernelgrpc.FullMethod(ServiceName, name) }

// IamServer is the handler type ServiceDesc registers.
type IamServer interface {
	DefineRole(context.Context, *servicev1.DefineRoleRequest) (*servicev1.DefineRoleResponse, error)
	ChangeRolePermissions(context.Context, *servicev1.ChangeRolePermissionsRequest) (*servicev1.ChangeRolePermissionsResponse, error)
	RetireRole(context.Context, *servicev1.RetireRoleRequest) (*servicev1.RetireRoleResponse, error)
	FindRole(context.Context, *servicev1.FindRoleRequest) (*servicev1.FindRoleResponse, error)
	FindRolesByStatus(context.Context, *servicev1.FindRolesByStatusRequest) (*servicev1.FindRolesByStatusResponse, error)

	InviteMember(context.Context, *servicev1.InviteMemberRequest) (*servicev1.InviteMemberResponse, error)
	ClaimOwnership(context.Context, *servicev1.ClaimOwnershipRequest) (*servicev1.ClaimOwnershipResponse, error)
	GrantRole(context.Context, *servicev1.GrantRoleRequest) (*servicev1.GrantRoleResponse, error)
	RevokeRole(context.Context, *servicev1.RevokeRoleRequest) (*servicev1.RevokeRoleResponse, error)
	RemoveMember(context.Context, *servicev1.RemoveMemberRequest) (*servicev1.RemoveMemberResponse, error)
	FindInvitation(context.Context, *servicev1.FindInvitationRequest) (*servicev1.FindInvitationResponse, error)
	FindInvitationsByStatus(context.Context, *servicev1.FindInvitationsByStatusRequest) (*servicev1.FindInvitationsByStatusResponse, error)
	FindMembership(context.Context, *servicev1.FindMembershipRequest) (*servicev1.FindMembershipResponse, error)
	FindMembershipsByStatus(context.Context, *servicev1.FindMembershipsByStatusRequest) (*servicev1.FindMembershipsByStatusResponse, error)

	RegisterBuyer(context.Context, *servicev1.RegisterBuyerRequest) (*servicev1.RegisterBuyerResponse, error)
	UpdateBuyerProfile(context.Context, *servicev1.UpdateBuyerProfileRequest) (*servicev1.UpdateBuyerProfileResponse, error)
	BlockBuyer(context.Context, *servicev1.BlockBuyerRequest) (*servicev1.BlockBuyerResponse, error)
	UnblockBuyer(context.Context, *servicev1.UnblockBuyerRequest) (*servicev1.UnblockBuyerResponse, error)
	FindBuyer(context.Context, *servicev1.FindBuyerRequest) (*servicev1.FindBuyerResponse, error)
	FindBuyersByStatus(context.Context, *servicev1.FindBuyersByStatusRequest) (*servicev1.FindBuyersByStatusResponse, error)
}

// ServiceDesc binds the service in the app block: protoc-gen-go-grpc would put
// io.network into the contract block, which admits only wire.codec (ADR-015).
var ServiceDesc = grpc.ServiceDesc{
	ServiceName: ServiceName,
	HandlerType: (*IamServer)(nil),
	Methods: []grpc.MethodDesc{
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "DefineRole"), IamServer.DefineRole),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "ChangeRolePermissions"), IamServer.ChangeRolePermissions),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "RetireRole"), IamServer.RetireRole),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "FindRole"), IamServer.FindRole),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "FindRolesByStatus"), IamServer.FindRolesByStatus),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "InviteMember"), IamServer.InviteMember),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "ClaimOwnership"), IamServer.ClaimOwnership),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "GrantRole"), IamServer.GrantRole),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "RevokeRole"), IamServer.RevokeRole),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "RemoveMember"), IamServer.RemoveMember),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "FindInvitation"), IamServer.FindInvitation),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "FindInvitationsByStatus"), IamServer.FindInvitationsByStatus),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "FindMembership"), IamServer.FindMembership),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "FindMembershipsByStatus"), IamServer.FindMembershipsByStatus),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "RegisterBuyer"), IamServer.RegisterBuyer),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "UpdateBuyerProfile"), IamServer.UpdateBuyerProfile),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "BlockBuyer"), IamServer.BlockBuyer),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "UnblockBuyer"), IamServer.UnblockBuyer),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "FindBuyer"), IamServer.FindBuyer),
		kernelgrpc.Unary(ServiceName, kernelgrpc.Method(descriptor, "FindBuyersByStatus"), IamServer.FindBuyersByStatus),
	},
	Metadata: descriptor.ParentFile().Path(),
}

// Server realizes IamServer over the use cases of the context.
type Server struct {
	Service application.Service
}

// DefineRole leaves the form of the ref and of the permissions to the domain,
// whose rejections the spec declares; the edge shapes the name and the count.
func (s Server) DefineRole(ctx context.Context, req *servicev1.DefineRoleRequest) (*servicev1.DefineRoleResponse, error) {
	if !validText(req.GetName()) {
		return nil, invalidText("name")
	}
	if len(req.GetPermissions()) > maxListItems {
		return nil, tooManyItems("permissions")
	}
	return kernelgrpc.Command(ctx,
		func(ctx context.Context) (usecase.Outcome[domain.DefinedResponse], error) {
			return s.Service.DefineRole(ctx, application.DefineRole{
				Ref: domain.RoleRef(req.GetRef()), Name: domain.RoleName(req.GetName()), Permissions: permissionsOf(req.GetPermissions()),
			})
		},
		defineRoleRefused,
		roleDefinedOf)
}

func (s Server) ChangeRolePermissions(ctx context.Context, req *servicev1.ChangeRolePermissionsRequest) (*servicev1.ChangeRolePermissionsResponse, error) {
	ref, err := roleRef(req.GetRef())
	if err != nil {
		return nil, err
	}
	if len(req.GetPermissions()) > maxListItems {
		return nil, tooManyItems("permissions")
	}
	return kernelgrpc.Command(ctx,
		func(ctx context.Context) (usecase.Outcome[domain.PermissionsChangedResponse], error) {
			return s.Service.ChangeRolePermissions(ctx, application.ChangeRolePermissions{Ref: ref, Permissions: permissionsOf(req.GetPermissions())})
		},
		changeRolePermissionsRefused,
		rolePermissionsChangedOf)
}

func (s Server) RetireRole(ctx context.Context, req *servicev1.RetireRoleRequest) (*servicev1.RetireRoleResponse, error) {
	ref, err := roleRef(req.GetRef())
	if err != nil {
		return nil, err
	}
	return kernelgrpc.Command(ctx,
		func(ctx context.Context) (usecase.Outcome[domain.RetiredResponse], error) {
			return s.Service.RetireRole(ctx, application.RetireRole{Ref: ref})
		},
		retireRoleRefused,
		roleRetiredOf)
}

func (s Server) FindRole(ctx context.Context, req *servicev1.FindRoleRequest) (*servicev1.FindRoleResponse, error) {
	ref, err := roleRef(req.GetRef())
	if err != nil {
		return nil, err
	}
	if _, err := kernelgrpc.ExecutionOf(ctx); err != nil {
		return nil, err
	}
	snapshot, err := s.Service.FindRole(ctx, ref)
	if err != nil {
		return nil, kernelgrpc.StatusOf(err)
	}
	return &servicev1.FindRoleResponse{Role: roleOf(snapshot)}, nil
}

func (s Server) FindRolesByStatus(ctx context.Context, req *servicev1.FindRolesByStatusRequest) (*servicev1.FindRolesByStatusResponse, error) {
	wanted, ok := roleStatusOf(req.GetStatus())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "status must be ROLE_STATUS_ACTIVE or ROLE_STATUS_RETIRED")
	}
	if _, err := kernelgrpc.ExecutionOf(ctx); err != nil {
		return nil, err
	}
	snapshots, err := s.Service.FindRolesByStatus(ctx, wanted)
	if err != nil {
		return nil, kernelgrpc.StatusOf(err)
	}
	roles := make([]*servicev1.Role, 0, len(snapshots))
	for _, snapshot := range snapshots {
		roles = append(roles, roleOf(snapshot))
	}
	return &servicev1.FindRolesByStatusResponse{Roles: roles}, nil
}

func defineRoleRefused(r *kernel.Rejection) *servicev1.DefineRoleResponse {
	return &servicev1.DefineRoleResponse{Result: &servicev1.DefineRoleResponse_Rejection{Rejection: rejectionOf(r)}}
}

func roleDefinedOf(defined domain.DefinedResponse) *servicev1.DefineRoleResponse {
	return &servicev1.DefineRoleResponse{Result: &servicev1.DefineRoleResponse_Defined{
		Defined: &servicev1.Defined{Ref: string(defined.Ref)},
	}}
}

func changeRolePermissionsRefused(r *kernel.Rejection) *servicev1.ChangeRolePermissionsResponse {
	return &servicev1.ChangeRolePermissionsResponse{Result: &servicev1.ChangeRolePermissionsResponse_Rejection{Rejection: rejectionOf(r)}}
}

func rolePermissionsChangedOf(changed domain.PermissionsChangedResponse) *servicev1.ChangeRolePermissionsResponse {
	return &servicev1.ChangeRolePermissionsResponse{Result: &servicev1.ChangeRolePermissionsResponse_PermissionsChanged{
		PermissionsChanged: &servicev1.PermissionsChanged{Ref: string(changed.Ref)},
	}}
}

func retireRoleRefused(r *kernel.Rejection) *servicev1.RetireRoleResponse {
	return &servicev1.RetireRoleResponse{Result: &servicev1.RetireRoleResponse_Rejection{Rejection: rejectionOf(r)}}
}

func roleRetiredOf(retired domain.RetiredResponse) *servicev1.RetireRoleResponse {
	return &servicev1.RetireRoleResponse{Result: &servicev1.RetireRoleResponse_Retired{
		Retired: &servicev1.Retired{Ref: string(retired.Ref)},
	}}
}

func roleRef(raw string) (domain.RoleRef, error) {
	ref := domain.RoleRef(raw)
	if !domain.ValidRef(ref) {
		return "", status.Error(codes.InvalidArgument, "ref must have 3 to 63 characters of a-z, 0-9 and -, without a leading or trailing hyphen")
	}
	return ref, nil
}

func permissionsOf(names []string) []domain.Permission {
	permissions := make([]domain.Permission, 0, len(names))
	for _, name := range names {
		permissions = append(permissions, domain.Permission(name))
	}
	return permissions
}

func tooManyItems(field string) error {
	return status.Error(codes.InvalidArgument, fmt.Sprintf("%s must have at most %d items", field, maxListItems))
}

func invalidText(field string) error {
	return status.Error(codes.InvalidArgument, field+" must have 1 to 200 characters of valid UTF-8 without control, format, line or paragraph separator, variation selector or default ignorable characters and with at least one character that is neither a space nor a combining mark")
}

var refusedInText = []*unicode.RangeTable{
	unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp, unicode.Variation_Selector, unicode.Other_Default_Ignorable_Code_Point,
}

// validText is the name rule of the golden bookings, which the spec applies to
// the name of a role and of a buyer and to the reason of a block alike.
func validText(text string) bool {
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > maxTextRunes {
		return false
	}
	visible := false
	for _, r := range text {
		if unicode.In(r, refusedInText...) {
			return false
		}
		visible = visible || !unicode.IsSpace(r) && !unicode.IsMark(r)
	}
	return visible
}

func roleOf(s domain.RoleSnapshot) *servicev1.Role {
	permissions := make([]string, 0, len(s.Permissions))
	for _, p := range s.Permissions {
		permissions = append(permissions, string(p))
	}
	return &servicev1.Role{
		Ref:           string(s.Ref),
		Name:          string(s.Name),
		Permissions:   permissions,
		Status:        roleStatus(s.Status),
		AccessVersion: s.AccessVersion,
		SyncedVersion: s.SyncedVersion,
		DefinedAt:     int64(s.DefinedAt),
		ChangedAt:     int64(s.ChangedAt),
	}
}

func roleStatus(s domain.RoleStatus) servicev1.RoleStatus {
	switch s {
	case domain.RoleStatusActive:
		return servicev1.RoleStatus_ROLE_STATUS_ACTIVE
	case domain.RoleStatusRetired:
		return servicev1.RoleStatus_ROLE_STATUS_RETIRED
	default:
		return servicev1.RoleStatus_ROLE_STATUS_UNSPECIFIED
	}
}

func roleStatusOf(s servicev1.RoleStatus) (domain.RoleStatus, bool) {
	switch s {
	case servicev1.RoleStatus_ROLE_STATUS_ACTIVE:
		return domain.RoleStatusActive, true
	case servicev1.RoleStatus_ROLE_STATUS_RETIRED:
		return domain.RoleStatusRetired, true
	default:
		return domain.RoleStatusUndefined, false
	}
}

func rejectionOf(r *kernel.Rejection) *servicev1.Rejection {
	return &servicev1.Rejection{Code: string(r.Code()), Message: r.Message()}
}
