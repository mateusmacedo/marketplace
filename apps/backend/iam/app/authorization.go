package app

import (
	"context"
	"fmt"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
)

// tenantKind is where the aggregate of an operation lives: the tenant of a
// store or the tenant of the operator (ADR-013).
type tenantKind int

const (
	undeclaredTenant tenantKind = iota
	storeTenant
	operatorTenant
)

// Authorization is step 1 of this context: each operation declares the
// permission it requires (IDN-16) and the kind of tenant it lives in. WHY: an
// empty operator would read every tenant as a store, so it denies everything.
func Authorization(operator ports.TenantID) usecase.Authorize[application.Operation] {
	permitted := usecase.Permitted(permissionOf)
	return func(ctx context.Context, op application.Operation) error {
		if operator == "" {
			return fmt.Errorf("%w: no operator tenant is configured", ports.ErrDenied)
		}
		if err := permitted(ctx, op); err != nil {
			return err
		}
		execution, err := ports.RequireExecutionContext(ctx)
		if err != nil {
			return err
		}
		tenant, _ := execution.Tenant()
		return admit(tenantKindOf(op), tenant, operator)
	}
}

func admit(kind tenantKind, tenant, operator ports.TenantID) error {
	switch kind {
	case storeTenant:
		if tenant == operator {
			return fmt.Errorf("%w: roles, invitations and memberships live in the tenant of a store", ports.ErrDenied)
		}
		return nil
	case operatorTenant:
		if tenant != operator {
			return fmt.Errorf("%w: buyers live in the operator's tenant", ports.ErrDenied)
		}
		return nil
	default:
		return fmt.Errorf("%w: the operation declares no kind of tenant", ports.ErrDenied)
	}
}

// An operation absent from the switches declares nothing, which is denied
// (IDN-17): adding an operation without its lines closes it, never opens it.
func permissionOf(op application.Operation) ports.Permission {
	switch op.(type) {
	case application.DefineRole, application.ChangeRolePermissions, application.RetireRole,
		application.InviteMember, application.ClaimOwnership, application.GrantRole, application.RevokeRole, application.RemoveMember:
		return "iam:write"
	case application.FindRole, application.FindRolesByStatus,
		application.FindInvitation, application.FindInvitationsByStatus, application.FindMembership, application.FindMembershipsByStatus:
		return "iam:read"
	case application.ConsumeInvitationProvisioned:
		return "iam:consume"
	default:
		return buyerPermissionOf(op)
	}
}

// Each buyer operation declares the permission of its route in the bff; the
// buyer reading itself asks iam:buyer there, the operator here asks iam:read.
func buyerPermissionOf(op application.Operation) ports.Permission {
	switch op.(type) {
	case application.RegisterBuyer:
		return "profile"
	case application.UpdateBuyerProfile:
		return "iam:buyer"
	case application.BlockBuyer, application.UnblockBuyer:
		return "iam:write"
	case application.FindBuyer, application.FindBuyersByStatus:
		return "iam:read"
	default:
		return ""
	}
}

func tenantKindOf(op application.Operation) tenantKind {
	switch op.(type) {
	case application.DefineRole, application.ChangeRolePermissions, application.RetireRole,
		application.FindRole, application.FindRolesByStatus,
		application.InviteMember, application.FindInvitation, application.FindInvitationsByStatus,
		application.ClaimOwnership, application.GrantRole, application.RevokeRole, application.RemoveMember,
		application.FindMembership, application.FindMembershipsByStatus,
		application.ConsumeInvitationProvisioned:
		return storeTenant
	case application.RegisterBuyer, application.UpdateBuyerProfile, application.BlockBuyer, application.UnblockBuyer,
		application.FindBuyer, application.FindBuyersByStatus:
		return operatorTenant
	default:
		return undeclaredTenant
	}
}
