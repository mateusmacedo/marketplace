package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

func TestFindStoreAnswersTheSnapshotOutsideAnyUnitOfWork(t *testing.T) {
	h := newHarness(t)
	h.seedSuspended(t)
	h.rec.Reset()

	got, err := h.service.FindStore(withExecution(t, context.Background()), testRef)

	if err != nil || got.Ref != testRef || got.Status != domain.Suspended || got.SuspensionReason != "fraude em análise" {
		t.Fatalf("FindStore() = %+v, %v; want the suspended store", got, err)
	}
	if slices.Contains(h.rec.Observed(), "within") {
		t.Fatalf("sequence = %v: a query must not open a unit of work (UOW-11)", h.rec.Observed())
	}
}

func TestFindStoreOfAnUnknownRefIsNotFoundWithThePrefix(t *testing.T) {
	h := newHarness(t)

	_, err := h.service.FindStore(withExecution(t, context.Background()), testRef)

	if !errors.Is(err, ports.ErrNotFound) || err.Error() != "application: find store loja-acme: ports: aggregate not found" {
		t.Fatalf("FindStore(unknown) = %v, want the prefixed ErrNotFound", err)
	}
}

func TestFindStoresByStatusAnswersExactlyTheStoresInThatStatus(t *testing.T) {
	h := newHarness(t)
	active := []domain.StoreSnapshot{
		{Ref: "loja-a", Name: "Loja A", Status: domain.Active},
		{Ref: "loja-b", Name: "Loja B", Status: domain.Active},
	}
	h.service.StatusReader = plantedStatus{
		domain.Active:    active,
		domain.Suspended: {{Ref: "loja-c", Name: "Loja C", Status: domain.Suspended}},
	}

	got, err := h.service.FindStoresByStatus(withExecution(t, context.Background()), domain.Active)

	if err != nil || len(got) != 2 || got[0].Ref != "loja-a" || got[1].Ref != "loja-b" {
		t.Fatalf("FindStoresByStatus(active) = %+v, %v; want exactly the two active stores", got, err)
	}
	if slices.Contains(h.rec.Observed(), "within") {
		t.Fatalf("sequence = %v: a query must not open a unit of work (UOW-11)", h.rec.Observed())
	}
}

func TestFindStoresByStatusWithoutMatchAnswersAnEmptyCollection(t *testing.T) {
	h := newHarness(t)

	got, err := h.service.FindStoresByStatus(withExecution(t, context.Background()), domain.Suspended)

	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("FindStoresByStatus() = %#v, %v; want an empty, non-nil collection", got, err)
	}
}

type crossTenantStatus struct{ access ports.CrossTenantAccess }

func (r crossTenantStatus) LoadByStatus(context.Context, domain.StoreStatus) ([]domain.StoreSnapshot, error) {
	return nil, r.access
}

type failingStatus struct{ err error }

func (r failingStatus) LoadByStatus(context.Context, domain.StoreStatus) ([]domain.StoreSnapshot, error) {
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
func TestFindStoresByStatusOfAnotherTenantAnswersEmptyAndReportsTheAccess(t *testing.T) {
	h := newHarness(t)
	access := ports.CrossTenantAccess{Object: "stores?status=active", ContextTenant: "marketplace", DataTenant: "outro"}
	h.service.StatusReader = crossTenantStatus{access: access}
	captured := &capturedResult{}
	h.service.Instrumentation = captured

	got, err := h.service.FindStoresByStatus(withExecution(t, context.Background()), domain.Active)

	if err != nil || len(got) != 0 {
		t.Fatalf("FindStoresByStatus() = %v, %v; want an empty answer", got, err)
	}
	var reported ports.CrossTenantAccess
	if len(captured.results) != 1 || !errors.As(captured.results[0].Err, &reported) || reported != access {
		t.Fatalf("EndOperation(%+v), want the access handed over for the security record", captured.results)
	}
}

func TestFindStoresByStatusFailureCarriesThePrefix(t *testing.T) {
	h := newHarness(t)
	h.service.StatusReader = failingStatus{err: errBoom}

	_, err := h.service.FindStoresByStatus(withExecution(t, context.Background()), domain.Active)

	if !errors.Is(err, errBoom) || err.Error() != "application: find stores by status active: boom" {
		t.Fatalf("FindStoresByStatus() = %v, want the prefixed failure", err)
	}
}

func TestADeniedQueryNeverReachesTheReader(t *testing.T) {
	h := newHarness(t)
	h.service.Authorize = func(context.Context, application.Operation) error { return ports.ErrDenied }
	h.service.StatusReader = failingStatus{err: errBoom}

	if _, err := h.service.FindStoresByStatus(withExecution(t, context.Background()), domain.Active); !errors.Is(err, ports.ErrDenied) {
		t.Fatalf("FindStoresByStatus() = %v, want the denial before the reader", err)
	}
	if _, err := h.service.FindStore(withExecution(t, context.Background()), testRef); !errors.Is(err, ports.ErrDenied) {
		t.Fatalf("FindStore() = %v, want the denial before the reader", err)
	}
}
