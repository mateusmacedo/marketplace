package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestFindRoleAnswersTheSnapshotOutsideAnyUnitOfWork(t *testing.T) {
	h := newHarness(t)
	h.seedRetired(t)
	h.rec.Reset()

	got, err := h.service.FindRole(withExecution(t, context.Background()), testRef)

	if err != nil || got.Ref != testRef || got.Status != domain.RoleStatusRetired || !slices.Equal(got.Permissions, testPermissions) {
		t.Fatalf("FindRole() = %+v, %v; want the retired role with its permissions", got, err)
	}
	if slices.Contains(h.rec.Observed(), "within") {
		t.Fatalf("sequence = %v: a query must not open a unit of work (UOW-11)", h.rec.Observed())
	}
}

func TestFindRoleOfAnUnknownRefIsNotFoundWithThePrefix(t *testing.T) {
	h := newHarness(t)

	_, err := h.service.FindRole(withExecution(t, context.Background()), testRef)

	if !errors.Is(err, ports.ErrNotFound) || err.Error() != "application: find role expedicao: ports: aggregate not found" {
		t.Fatalf("FindRole(unknown) = %v, want the prefixed ErrNotFound", err)
	}
}

func TestFindRolesByStatusAnswersExactlyTheRolesInThatStatus(t *testing.T) {
	h := newHarness(t)
	h.service.RolesByStatus = plantedRoles{
		domain.RoleStatusActive: {
			{Ref: "expedicao", Name: "Expedição", Status: domain.RoleStatusActive},
			{Ref: "catalogo", Name: "Catálogo", Status: domain.RoleStatusActive},
		},
		domain.RoleStatusRetired: {{Ref: "antigo", Name: "Antigo", Status: domain.RoleStatusRetired}},
	}

	got, err := h.service.FindRolesByStatus(withExecution(t, context.Background()), domain.RoleStatusActive)

	if err != nil || len(got) != 2 || got[0].Ref != "expedicao" || got[1].Ref != "catalogo" {
		t.Fatalf("FindRolesByStatus(active) = %+v, %v; want exactly the two active roles", got, err)
	}
	if slices.Contains(h.rec.Observed(), "within") {
		t.Fatalf("sequence = %v: a query must not open a unit of work (UOW-11)", h.rec.Observed())
	}
}

func TestFindRolesByStatusWithoutMatchAnswersAnEmptyCollection(t *testing.T) {
	h := newHarness(t)

	got, err := h.service.FindRolesByStatus(withExecution(t, context.Background()), domain.RoleStatusRetired)

	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("FindRolesByStatus() = %#v, %v; want an empty, non-nil collection", got, err)
	}
}

type crossTenantRoles struct{ access ports.CrossTenantAccess }

func (r crossTenantRoles) LoadByStatus(context.Context, domain.RoleStatus) ([]domain.RoleSnapshot, error) {
	return nil, r.access
}

type failingRoles struct{ err error }

func (r failingRoles) LoadByStatus(context.Context, domain.RoleStatus) ([]domain.RoleSnapshot, error) {
	return nil, r.err
}

type capturedResult struct{ results []ports.Result }

func (c *capturedResult) BeginOperation(ctx context.Context, _ string) (context.Context, ports.EndOperation) {
	return ctx, func(r ports.Result) { c.results = append(c.results, r) }
}

func (c *capturedResult) Audit(context.Context, ports.AuditEvent) {}

// IDN-13 and IDN-12: the caller of a status held only by another tenant gets
// the empty answer of a status nobody holds, and the instrumentation still
// receives the access to record it.
func TestFindRolesByStatusOfAnotherTenantAnswersEmptyAndReportsTheAccess(t *testing.T) {
	h := newHarness(t)
	access := ports.CrossTenantAccess{Object: "roles?status=active", ContextTenant: "loja-acme", DataTenant: "outra"}
	h.service.RolesByStatus = crossTenantRoles{access: access}
	captured := &capturedResult{}
	h.service.Instrumentation = captured

	got, err := h.service.FindRolesByStatus(withExecution(t, context.Background()), domain.RoleStatusActive)

	if err != nil || len(got) != 0 {
		t.Fatalf("FindRolesByStatus() = %v, %v; want an empty answer", got, err)
	}
	var reported ports.CrossTenantAccess
	if len(captured.results) != 1 || !errors.As(captured.results[0].Err, &reported) || reported != access {
		t.Fatalf("EndOperation(%+v), want the access handed over for the security record", captured.results)
	}
}

func TestFindRolesByStatusFailureCarriesThePrefix(t *testing.T) {
	h := newHarness(t)
	h.service.RolesByStatus = failingRoles{err: errBoom}

	_, err := h.service.FindRolesByStatus(withExecution(t, context.Background()), domain.RoleStatusActive)

	if !errors.Is(err, errBoom) || err.Error() != "application: find roles by status active: boom" {
		t.Fatalf("FindRolesByStatus() = %v, want the prefixed failure", err)
	}
}

func TestADeniedQueryNeverReachesTheReader(t *testing.T) {
	h := newHarness(t)
	h.service.Authorize = func(context.Context, application.Operation) error { return ports.ErrDenied }
	h.service.RolesByStatus = failingRoles{err: errBoom}

	if _, err := h.service.FindRolesByStatus(withExecution(t, context.Background()), domain.RoleStatusActive); !errors.Is(err, ports.ErrDenied) {
		t.Fatalf("FindRolesByStatus() = %v, want the denial before the reader", err)
	}
	if _, err := h.service.FindRole(withExecution(t, context.Background()), testRef); !errors.Is(err, ports.ErrDenied) {
		t.Fatalf("FindRole() = %v, want the denial before the reader", err)
	}
}
