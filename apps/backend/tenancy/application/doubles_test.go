package application_test

import (
	"context"
	"testing"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	"github.com/mateusmacedo/dmpf/libs/backend/go/memory"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/serviceskit"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

const (
	testRef  = domain.StoreRef("loja-acme")
	testName = domain.StoreName("Loja Acme")
)

var testOccurred = ports.Instant(1_755_432_000_000_000_000)

var storeTable = memory.Table[domain.StoreRef, domain.StoreSnapshot]{Name: "stores", Unique: []memory.Unique[domain.StoreSnapshot]{{
	Name:      "stores_tenant_id_name_key",
	Key:       func(s domain.StoreSnapshot) ([]string, bool) { return []string{string(s.Name)}, true },
	Rejection: domain.RejectStoreDuplicateName,
}}}

type harness struct {
	fakes   *serviceskit.Fakes
	store   *memory.Store
	rec     *serviceskit.Steps
	service application.Service
}

type recordingStoreRepo struct {
	inner ports.Repository[domain.StoreRef, domain.StoreSnapshot]
	rec   *serviceskit.Steps
}

func (r recordingStoreRepo) Load(ctx context.Context, ref domain.StoreRef) (domain.StoreSnapshot, ports.Version, error) {
	r.rec.Record("stores.Load")
	return r.inner.Load(ctx, ref)
}

func (r recordingStoreRepo) Save(ctx context.Context, ref domain.StoreRef, s domain.StoreSnapshot, expected ports.Version) error {
	r.rec.Record("stores.Save")
	return r.inner.Save(ctx, ref, s, expected)
}

// plantedStatus answers FindStoresByStatus with what the test planted: the
// memory store has no relation query, and the use case is what is under test.
type plantedStatus map[domain.StoreStatus][]domain.StoreSnapshot

func (p plantedStatus) LoadByStatus(_ context.Context, status domain.StoreStatus) ([]domain.StoreSnapshot, error) {
	return p[status], nil
}

type option func(*setup)

type setup struct {
	faults      serviceskit.Faults
	registerErr error
}

func withRegisterError(err error) option {
	return func(s *setup) { s.registerErr = err }
}

func withLoadError(err error) option {
	return func(s *setup) { s.faults.Load = err }
}

func withSaveError(err error) option {
	return func(s *setup) { s.faults.Save = err }
}

func withEnqueueError(err error) option {
	return func(s *setup) { s.faults.Enqueue = err }
}

func newHarness(t *testing.T, options ...option) *harness {
	t.Helper()

	fakes := serviceskit.NewFakes()
	h := &harness{fakes: fakes, store: fakes.Store, rec: fakes.Steps}
	cfg := &setup{}
	for _, apply := range options {
		apply(cfg)
	}
	fakes.FailRegister = cfg.registerErr

	bind := func(tx serviceskit.Tx) application.Resources {
		return application.Resources{
			Stores:   recordingStoreRepo{inner: serviceskit.FaultyRepository(storeTable.Repository(tx.Memory()), cfg.faults), rec: h.rec},
			Outbox:   serviceskit.FaultyOutbox(tx.Outbox(), cfg.faults),
			Commands: tx.CommandInbox(application.CommandConsumer),
		}
	}

	h.service = application.Service{
		UoW:          serviceskit.UnitOfWork(fakes, bind),
		Reader:       storeTable.Reader(h.store),
		StatusReader: plantedStatus{},
		Clock:        fakes.Clock(memory.FixedClock{At: testOccurred}),
		IDs:          fakes.IDs(&memory.SequenceIDs{Prefix: "m-"}),
		Authorize:    serviceskit.Authorize(fakes, usecase.AllowAll[application.Operation]()),
		Idempotency:  usecase.IdempotencyPolicy{Wait: 1_000_000_000, Retention: 86_400_000_000_000, Digest: serviceskit.FoldDigest},
	}
	return h
}

func (h *harness) seedStore(t *testing.T, snap domain.StoreSnapshot) {
	t.Helper()
	uow := memory.NewUnitOfWork(h.store, func(tx *memory.Tx) application.Resources {
		return application.Resources{Stores: storeTable.Repository(tx)}
	})
	err := uow.Within(withExecution(t, context.Background()), func(ctx context.Context, res application.Resources) error {
		return res.Stores.Save(ctx, snap.Ref, snap, 0)
	})
	if err != nil {
		t.Fatalf("seedStore: %v", err)
	}
}

func (h *harness) seedActive(t *testing.T) {
	t.Helper()
	h.seedStore(t, domain.StoreSnapshot{Ref: testRef, Name: testName, Status: domain.Active, RegisteredAt: 1000, StatusChangedAt: 1000})
}

func (h *harness) seedSuspended(t *testing.T) {
	t.Helper()
	h.seedStore(t, domain.StoreSnapshot{
		Ref: testRef, Name: testName, Status: domain.Suspended, SuspensionReason: "fraude em análise",
		RegisteredAt: 1000, StatusChangedAt: 2000,
	})
}

func (h *harness) outboxLen() int {
	return len(h.store.Entries())
}

func (h *harness) stored(t *testing.T, ref domain.StoreRef) domain.StoreSnapshot {
	t.Helper()
	snap, _, err := storeTable.Reader(h.store).Load(withExecution(t, context.Background()), ref)
	if err != nil {
		t.Fatalf("Load(%s) = %v", ref, err)
	}
	return snap
}
