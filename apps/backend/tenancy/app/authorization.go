package app

import (
	"context"
	"fmt"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
)

// Authorization is step 1 of this context: each operation declares the
// permission it requires (IDN-16), and the registry of stores lives only in the
// operator's tenant, so any other tenant is denied before the transaction.
// WHY: NewService and the wiring take a Config that may skip Validate, and an
// empty operator would match a chain without a tenant; it denies everything.
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
		if tenant, _ := execution.Tenant(); tenant != operator {
			return fmt.Errorf("%w: the registry of stores belongs to the operator's tenant", ports.ErrDenied)
		}
		return nil
	}
}

// An operation absent from the switch declares none, which Permitted denies
// (IDN-17): adding an operation without a line here closes it, never opens it.
func permissionOf(op application.Operation) ports.Permission {
	switch op.(type) {
	case application.RegisterStore, application.SuspendStore, application.ReactivateStore:
		return "tenancy:write"
	case application.FindStore, application.FindStoresByStatus:
		return "tenancy:read"
	default:
		return ""
	}
}
