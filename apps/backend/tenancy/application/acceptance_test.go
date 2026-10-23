package application_test

import (
	"context"
	"errors"
	"testing"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

func requireCode[R any](t *testing.T, outcome usecase.Outcome[R], err error, want kernel.Code) {
	t.Helper()
	rejection, rejected := outcome.Rejection()
	if err != nil || !rejected || rejection.Code() != want {
		t.Fatalf("outcome = (%+v, %v), want the rejection %s", outcome, err, want)
	}
}

func TestAnUnknownRefIsRegisteredActive(t *testing.T) {
	h := newHarness(t)

	outcome, err := h.service.RegisterStore(withExecution(t, context.Background()), application.RegisterStore{Ref: testRef, Name: testName})

	if err != nil || outcome.Response() != (domain.RegisteredResponse{Ref: testRef}) {
		t.Fatalf("RegisterStore() = (%+v, %v), want the registered ref", outcome, err)
	}
	want := domain.StoreSnapshot{Ref: testRef, Name: testName, Status: domain.Active, RegisteredAt: testOccurred, StatusChangedAt: testOccurred}
	if got := h.stored(t, testRef); !got.Equal(want) {
		t.Fatalf("stored = %+v, want %+v", got, want)
	}
}

func TestAKnownRefIsAcceptedWithoutChangeOrEvent(t *testing.T) {
	h := newHarness(t)
	h.seedSuspended(t)
	before := h.stored(t, testRef)

	outcome, err := h.service.RegisterStore(withExecution(t, context.Background()), application.RegisterStore{Ref: testRef, Name: "Outro nome"})

	if _, rejected := outcome.Rejection(); err != nil || rejected {
		t.Fatalf("RegisterStore() = (%+v, %v), want accepted", outcome, err)
	}
	if got := h.stored(t, testRef); !got.Equal(before) {
		t.Fatalf("stored = %+v, want it unchanged %+v", got, before)
	}
	if got := h.outboxLen(); got != 0 {
		t.Fatalf("outbox = %d entries, want none", got)
	}
}

func TestAnInvalidRefIsRejectedAndNothingIsWritten(t *testing.T) {
	for _, ref := range []domain.StoreRef{"Loja_1", "ab", "-loja"} {
		t.Run(string(ref), func(t *testing.T) {
			h := newHarness(t)

			outcome, err := h.service.RegisterStore(withExecution(t, context.Background()), application.RegisterStore{Ref: ref, Name: testName})

			requireCode(t, outcome, err, domain.CodeStoreRefInvalid)
			if got := h.outboxLen(); got != 0 {
				t.Fatalf("outbox = %d entries, want none", got)
			}
			if _, _, err := storeTable.Reader(h.store).Load(withExecution(t, context.Background()), ref); !errors.Is(err, ports.ErrNotFound) {
				t.Fatalf("Load(%s) = %v, want ErrNotFound: nothing is written", ref, err)
			}
		})
	}
}

func TestAnEmptyNameIsRejected(t *testing.T) {
	h := newHarness(t)

	outcome, err := h.service.RegisterStore(withExecution(t, context.Background()), application.RegisterStore{Ref: testRef})

	requireCode(t, outcome, err, domain.CodeStoreNameEmpty)
}

func TestSuspendingAnActiveStoreRecordsTheReason(t *testing.T) {
	h := newHarness(t)
	h.seedActive(t)

	outcome, err := h.service.SuspendStore(withExecution(t, context.Background()), application.SuspendStore{Ref: testRef, Reason: "fraude em análise"})

	if err != nil || outcome.Response() != (domain.SuspendedResponse{Ref: testRef}) {
		t.Fatalf("SuspendStore() = (%+v, %v), want the suspended ref", outcome, err)
	}
	if got := h.stored(t, testRef); got.Status != domain.Suspended || got.SuspensionReason != "fraude em análise" || got.StatusChangedAt != testOccurred {
		t.Fatalf("stored = %+v, want suspended with the reason at %d", got, testOccurred)
	}
}

func TestSuspendingWithAnEmptyReasonLeavesTheStateUnchanged(t *testing.T) {
	h := newHarness(t)
	h.seedActive(t)
	before := h.stored(t, testRef)

	outcome, err := h.service.SuspendStore(withExecution(t, context.Background()), application.SuspendStore{Ref: testRef})

	requireCode(t, outcome, err, domain.CodeStoreReasonEmpty)
	if got := h.stored(t, testRef); !got.Equal(before) {
		t.Fatalf("stored = %+v, want it unchanged %+v", got, before)
	}
}

func TestSuspendingASuspendedStoreLeavesTheStateUnchanged(t *testing.T) {
	h := newHarness(t)
	h.seedSuspended(t)
	before := h.stored(t, testRef)

	outcome, err := h.service.SuspendStore(withExecution(t, context.Background()), application.SuspendStore{Ref: testRef, Reason: "outro motivo"})

	requireCode(t, outcome, err, domain.CodeStoreNotActive)
	if got := h.stored(t, testRef); !got.Equal(before) {
		t.Fatalf("stored = %+v, want it unchanged %+v", got, before)
	}
}

func TestReactivatingASuspendedStoreClearsTheReason(t *testing.T) {
	h := newHarness(t)
	h.seedSuspended(t)

	outcome, err := h.service.ReactivateStore(withExecution(t, context.Background()), application.ReactivateStore{Ref: testRef})

	if err != nil || outcome.Response() != (domain.ReactivatedResponse{Ref: testRef}) {
		t.Fatalf("ReactivateStore() = (%+v, %v), want the reactivated ref", outcome, err)
	}
	if got := h.stored(t, testRef); got.Status != domain.Active || got.SuspensionReason != "" || got.StatusChangedAt != testOccurred {
		t.Fatalf("stored = %+v, want active without a reason at %d", got, testOccurred)
	}
}

func TestReactivatingAnActiveStoreLeavesTheStateUnchanged(t *testing.T) {
	h := newHarness(t)
	h.seedActive(t)
	before := h.stored(t, testRef)

	outcome, err := h.service.ReactivateStore(withExecution(t, context.Background()), application.ReactivateStore{Ref: testRef})

	requireCode(t, outcome, err, domain.CodeStoreNotSuspended)
	if got := h.stored(t, testRef); !got.Equal(before) {
		t.Fatalf("stored = %+v, want it unchanged %+v", got, before)
	}
}

func TestSuspendingOrReactivatingAnUnknownRefIsNotFound(t *testing.T) {
	h := newHarness(t)

	if _, err := h.service.SuspendStore(withExecution(t, context.Background()), application.SuspendStore{Ref: testRef, Reason: "motivo"}); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("SuspendStore(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := h.service.ReactivateStore(withExecution(t, context.Background()), application.ReactivateStore{Ref: testRef}); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("ReactivateStore(unknown) = %v, want ErrNotFound", err)
	}
}
