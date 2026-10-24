package app

import (
	"context"
	"errors"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
)

const operator = ports.TenantID("marketplace")

var (
	commands = []application.Operation{application.RegisterStore{}, application.SuspendStore{}, application.ReactivateStore{}}
	queries  = []application.Operation{application.FindStore{}, application.FindStoresByStatus{}}
)

// IDN-16: every operation of this context declares the permission it requires;
// an operation added without one would be denied (IDN-17).
func TestEveryOperationDeclaresItsPermission(t *testing.T) {
	for _, op := range commands {
		if got := permissionOf(op); got != "tenancy:write" {
			t.Errorf("permissionOf(%T) = %q, want tenancy:write", op, got)
		}
	}
	for _, op := range queries {
		if got := permissionOf(op); got != "tenancy:read" {
			t.Errorf("permissionOf(%T) = %q, want tenancy:read", op, got)
		}
	}
}

func subjectIn(t *testing.T, tenant ports.TenantID, permissions ...ports.Permission) context.Context {
	t.Helper()
	subject := ports.SubjectID("s-test")
	execution, err := ports.NewExecutionContext(ports.ExecutionContextSpec{
		RequestID: "r-test", CorrelationID: "c-test", TraceContext: "t-test",
		Subject: &subject, Tenant: &tenant, Permissions: append([]ports.Permission{}, permissions...),
		Deadline: 1, Locale: "en",
	})
	if err != nil {
		t.Fatalf("NewExecutionContext() = %v", err)
	}
	return ports.WithExecutionContext(context.Background(), execution)
}

func TestTheOperatorWithThePermissionIsAuthorized(t *testing.T) {
	authorize := Authorization(operator)

	for _, op := range commands {
		if err := authorize(subjectIn(t, operator, "tenancy:write"), op); err != nil {
			t.Errorf("Authorization()(%T) = %v, want nil", op, err)
		}
	}
	for _, op := range queries {
		if err := authorize(subjectIn(t, operator, "tenancy:read"), op); err != nil {
			t.Errorf("Authorization()(%T) = %v, want nil", op, err)
		}
	}
}

func TestASubjectOfTheOperatorWithoutThePermissionIsDenied(t *testing.T) {
	authorize := Authorization(operator)

	for _, op := range commands {
		if err := authorize(subjectIn(t, operator, "tenancy:read"), op); !errors.Is(err, ports.ErrDenied) {
			t.Errorf("Authorization()(%T) without tenancy:write = %v, want ErrDenied", op, err)
		}
	}
	for _, op := range queries {
		if err := authorize(subjectIn(t, operator), op); !errors.Is(err, ports.ErrDenied) {
			t.Errorf("Authorization()(%T) without tenancy:read = %v, want ErrDenied", op, err)
		}
	}
}

func TestATenantThatIsNotTheOperatorIsDeniedEvenWithThePermission(t *testing.T) {
	authorize := Authorization(operator)

	for _, op := range commands {
		if err := authorize(subjectIn(t, "loja-acme", "tenancy:write", "tenancy:read"), op); !errors.Is(err, ports.ErrDenied) {
			t.Errorf("Authorization()(%T) from loja-acme = %v, want ErrDenied", op, err)
		}
	}
}

func TestAnEmptyOperatorDeniesEvenATenantWithThePermission(t *testing.T) {
	authorize := Authorization("")
	subject := ports.SubjectID("s-test")
	withoutTenant := executionOf(t, ports.ExecutionContextSpec{Subject: &subject, Permissions: []ports.Permission{"tenancy:write", "tenancy:read"}})

	for _, op := range append(append([]application.Operation{}, commands...), queries...) {
		if err := authorize(subjectIn(t, operator, "tenancy:write", "tenancy:read"), op); !errors.Is(err, ports.ErrDenied) {
			t.Errorf("Authorization(\"\")(%T) from %s = %v, want ErrDenied", op, operator, err)
		}
		if err := authorize(withoutTenant, op); !errors.Is(err, ports.ErrDenied) {
			t.Errorf("Authorization(\"\")(%T) without a tenant = %v, want ErrDenied", op, err)
		}
	}
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

func TestAChainWithoutATenantIsDenied(t *testing.T) {
	subject := ports.SubjectID("s-test")
	ctx := executionOf(t, ports.ExecutionContextSpec{Subject: &subject, Permissions: []ports.Permission{"tenancy:write", "tenancy:read"}})

	for _, op := range append(append([]application.Operation{}, commands...), queries...) {
		if err := Authorization(operator)(ctx, op); !errors.Is(err, ports.ErrDenied) {
			t.Errorf("Authorization()(%T) without a tenant = %v, want ErrDenied", op, err)
		}
	}
}

// IDN-18: a chain without a subject is the workload the channel verified, so
// the permission is not asked of it; the operator check still applies.
func TestAWorkloadWithoutASubjectIsAuthorizedOnlyInTheOperator(t *testing.T) {
	inOperator, inStore := operator, ports.TenantID("loja-acme")

	if err := Authorization(operator)(executionOf(t, ports.ExecutionContextSpec{Tenant: &inOperator}), application.RegisterStore{}); err != nil {
		t.Fatalf("Authorization() for the workload in the operator = %v, want nil", err)
	}
	if err := Authorization(operator)(executionOf(t, ports.ExecutionContextSpec{Tenant: &inStore}), application.RegisterStore{}); !errors.Is(err, ports.ErrDenied) {
		t.Fatalf("Authorization() for the workload in loja-acme = %v, want ErrDenied", err)
	}
}

func TestAnExecutionWithoutContextIsRefused(t *testing.T) {
	if err := Authorization(operator)(context.Background(), application.FindStore{}); err == nil {
		t.Fatal("Authorization() without an execution context = nil, want a refusal")
	}
}
