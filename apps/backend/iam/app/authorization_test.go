package app

import (
	"context"
	"errors"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
)

const (
	operator = ports.TenantID("marketplace")
	store    = ports.TenantID("loja-acme")
)

var (
	storeCommands = []application.Operation{
		application.DefineRole{}, application.ChangeRolePermissions{}, application.RetireRole{},
		application.InviteMember{}, application.ClaimOwnership{}, application.GrantRole{}, application.RevokeRole{}, application.RemoveMember{},
	}
	storeQueries = []application.Operation{
		application.FindRole{}, application.FindRolesByStatus{},
		application.FindInvitation{}, application.FindInvitationsByStatus{}, application.FindMembership{}, application.FindMembershipsByStatus{},
	}
	storeOps = append(append([]application.Operation{}, storeCommands...), storeQueries...)

	// The permission of each buyer operation is the one of its route in the
	// bff (spec, Borda): FindBuyer of the operator asks iam:read.
	buyerPermissions = map[application.Operation]ports.Permission{
		application.RegisterBuyer{}:      "profile",
		application.UpdateBuyerProfile{}: "iam:buyer",
		application.BlockBuyer{}:         "iam:write",
		application.UnblockBuyer{}:       "iam:write",
		application.FindBuyer{}:          "iam:read",
		application.FindBuyersByStatus{}: "iam:read",
	}
)

// IDN-16: every operation of this context declares the permission it requires;
// an operation added without one would be denied (IDN-17).
func TestEveryOperationDeclaresItsPermission(t *testing.T) {
	for _, op := range storeCommands {
		if got := permissionOf(op); got != "iam:write" {
			t.Errorf("permissionOf(%T) = %q, want iam:write", op, got)
		}
	}
	for _, op := range storeQueries {
		if got := permissionOf(op); got != "iam:read" {
			t.Errorf("permissionOf(%T) = %q, want iam:read", op, got)
		}
	}
}

func TestEveryBuyerOperationDeclaresThePermissionOfItsRouteAndLivesInTheOperator(t *testing.T) {
	for op, want := range buyerPermissions {
		if got := permissionOf(op); got != want {
			t.Errorf("permissionOf(%T) = %q, want %q", op, got, want)
		}
		if got := tenantKindOf(op); got != operatorTenant {
			t.Errorf("tenantKindOf(%T) = %v, want the operator's tenant", op, got)
		}
	}
}

func TestTheOperatorWithThePermissionIsAuthorizedOnTheBuyers(t *testing.T) {
	authorize := Authorization(operator)

	for op, permission := range buyerPermissions {
		if err := authorize(subjectIn(t, operator, permission), op); err != nil {
			t.Errorf("Authorization()(%T) from the operator with %s = %v, want nil", op, permission, err)
		}
		if err := authorize(subjectIn(t, operator), op); !errors.Is(err, ports.ErrDenied) {
			t.Errorf("Authorization()(%T) from the operator without %s = %v, want ErrDenied", op, permission, err)
		}
	}
}

// A store's own iam:write must not reach the buyers, who are the operator's.
func TestAStoreIsDeniedTheBuyersEvenWithThePermission(t *testing.T) {
	authorize := Authorization(operator)

	for op, permission := range buyerPermissions {
		if err := authorize(subjectIn(t, store, permission, "iam:write", "iam:read"), op); !errors.Is(err, ports.ErrDenied) {
			t.Errorf("Authorization()(%T) from %s = %v, want ErrDenied", op, store, err)
		}
	}
}

// The consumption carries no subject, so the permission is declared (IDN-17)
// but never asked; what holds it is the tenant the envelope carries.
func TestTheConsumptionDeclaresItsPermissionAndLivesInAStore(t *testing.T) {
	op := application.ConsumeInvitationProvisioned{}
	if permissionOf(op) != "iam:consume" || tenantKindOf(op) != storeTenant {
		t.Fatalf("ConsumeInvitationProvisioned declares %q in %v, want iam:consume in a store", permissionOf(op), tenantKindOf(op))
	}
	inStore, inOperator := store, operator
	if err := Authorization(operator)(executionOf(t, ports.ExecutionContextSpec{Tenant: &inStore}), op); err != nil {
		t.Fatalf("Authorization() of a consumption in a store = %v, want nil", err)
	}
	if err := Authorization(operator)(executionOf(t, ports.ExecutionContextSpec{Tenant: &inOperator}), op); !errors.Is(err, ports.ErrDenied) {
		t.Fatalf("Authorization() of a consumption in the operator = %v, want ErrDenied", err)
	}
}

func TestEveryOperationOfAStoreLivesInTheTenantOfAStore(t *testing.T) {
	for _, op := range storeOps {
		if got := tenantKindOf(op); got != storeTenant {
			t.Errorf("tenantKindOf(%T) = %v, want the tenant of a store", op, got)
		}
	}
}

func subjectIn(t *testing.T, tenant ports.TenantID, permissions ...ports.Permission) context.Context {
	t.Helper()
	subject := ports.SubjectID("s-test")
	return executionOf(t, ports.ExecutionContextSpec{Subject: &subject, Tenant: &tenant, Permissions: append([]ports.Permission{}, permissions...)})
}

func executionOf(t *testing.T, spec ports.ExecutionContextSpec) context.Context {
	t.Helper()
	spec.RequestID, spec.CorrelationID, spec.TraceContext, spec.Deadline, spec.Locale = "r-test", "c-test", "t-test", 1, "en"
	execution, err := ports.NewExecutionContext(spec)
	if err != nil {
		t.Fatalf("NewExecutionContext() = %v", err)
	}
	return ports.WithExecutionContext(context.Background(), execution)
}

func TestAStoreWithThePermissionIsAuthorized(t *testing.T) {
	authorize := Authorization(operator)

	for _, op := range storeCommands {
		if err := authorize(subjectIn(t, store, "iam:write"), op); err != nil {
			t.Errorf("Authorization()(%T) = %v, want nil", op, err)
		}
	}
	for _, op := range storeQueries {
		if err := authorize(subjectIn(t, store, "iam:read"), op); err != nil {
			t.Errorf("Authorization()(%T) = %v, want nil", op, err)
		}
	}
}

func TestASubjectOfTheStoreWithoutThePermissionIsDenied(t *testing.T) {
	authorize := Authorization(operator)

	for _, op := range storeCommands {
		if err := authorize(subjectIn(t, store, "iam:read"), op); !errors.Is(err, ports.ErrDenied) {
			t.Errorf("Authorization()(%T) without iam:write = %v, want ErrDenied", op, err)
		}
	}
	for _, op := range storeQueries {
		if err := authorize(subjectIn(t, store), op); !errors.Is(err, ports.ErrDenied) {
			t.Errorf("Authorization()(%T) without iam:read = %v, want ErrDenied", op, err)
		}
	}
}

// The operator holds iam:write as a realm role, so the permission alone would
// let it write the roles of a store it has no Organization in.
func TestTheOperatorIsDeniedTheRolesEvenWithThePermission(t *testing.T) {
	authorize := Authorization(operator)

	for _, op := range storeOps {
		if err := authorize(subjectIn(t, operator, "iam:write", "iam:read"), op); !errors.Is(err, ports.ErrDenied) {
			t.Errorf("Authorization()(%T) from the operator = %v, want ErrDenied", op, err)
		}
	}
}

func TestAnEmptyOperatorDeniesEvenAStoreWithThePermission(t *testing.T) {
	authorize := Authorization("")

	for _, op := range storeOps {
		if err := authorize(subjectIn(t, store, "iam:write", "iam:read"), op); !errors.Is(err, ports.ErrDenied) {
			t.Errorf("Authorization(\"\")(%T) from %s = %v, want ErrDenied", op, store, err)
		}
	}
}

func TestAChainWithoutATenantIsDenied(t *testing.T) {
	subject := ports.SubjectID("s-test")
	ctx := executionOf(t, ports.ExecutionContextSpec{Subject: &subject, Permissions: []ports.Permission{"iam:write", "iam:read"}})

	for _, op := range storeOps {
		if err := Authorization(operator)(ctx, op); !errors.Is(err, ports.ErrDenied) {
			t.Errorf("Authorization()(%T) without a tenant = %v, want ErrDenied", op, err)
		}
	}
	for op := range buyerPermissions {
		if err := Authorization(operator)(ctx, op); !errors.Is(err, ports.ErrDenied) {
			t.Errorf("Authorization()(%T) without a tenant = %v, want ErrDenied", op, err)
		}
	}
}

// IDN-18: a chain without a subject is the workload the channel verified, so
// the permission is not asked of it; the kind of tenant still applies.
func TestAWorkloadWithoutASubjectIsAuthorizedOnlyInAStore(t *testing.T) {
	inStore, inOperator := store, operator

	if err := Authorization(operator)(executionOf(t, ports.ExecutionContextSpec{Tenant: &inStore}), application.DefineRole{}); err != nil {
		t.Fatalf("Authorization() for the workload in a store = %v, want nil", err)
	}
	if err := Authorization(operator)(executionOf(t, ports.ExecutionContextSpec{Tenant: &inOperator}), application.DefineRole{}); !errors.Is(err, ports.ErrDenied) {
		t.Fatalf("Authorization() for the workload in the operator = %v, want ErrDenied", err)
	}
	if err := Authorization(operator)(executionOf(t, ports.ExecutionContextSpec{Tenant: &inOperator}), application.BlockBuyer{}); err != nil {
		t.Fatalf("Authorization() for the workload blocking in the operator = %v, want nil", err)
	}
	if err := Authorization(operator)(executionOf(t, ports.ExecutionContextSpec{Tenant: &inStore}), application.BlockBuyer{}); !errors.Is(err, ports.ErrDenied) {
		t.Fatalf("Authorization() for the workload blocking in a store = %v, want ErrDenied", err)
	}
}

func TestAnExecutionWithoutContextIsRefused(t *testing.T) {
	if err := Authorization(operator)(context.Background(), application.FindRole{}); err == nil {
		t.Fatal("Authorization() without an execution context = nil, want a refusal")
	}
}

func TestEachKindAdmitsOnlyItsOwnTenant(t *testing.T) {
	for _, tc := range []struct {
		kind   tenantKind
		tenant ports.TenantID
		admit  bool
	}{
		{storeTenant, store, true},
		{storeTenant, operator, false},
		{operatorTenant, operator, true},
		{operatorTenant, store, false},
		{undeclaredTenant, store, false},
		{undeclaredTenant, operator, false},
	} {
		err := admit(tc.kind, tc.tenant, operator)
		if admitted := err == nil; admitted != tc.admit || (!admitted && !errors.Is(err, ports.ErrDenied)) {
			t.Errorf("admit(%v, %s) = %v, want admitted %v or ErrDenied", tc.kind, tc.tenant, err, tc.admit)
		}
	}
}
