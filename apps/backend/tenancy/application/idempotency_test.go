package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/execctx"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

func TestARepeatedRegisterReplaysWithoutASecondEventOrAudit(t *testing.T) {
	h, instr := newInstrumentedHarness(t)
	register := application.RegisterStore{Ref: testRef, Name: testName}

	first, err := h.service.RegisterStore(withKey(t, context.Background(), "k-register"), register)
	if err != nil {
		t.Fatalf("first RegisterStore() = %v, want nil", err)
	}
	ctx := withKey(t, context.Background(), "k-register")
	again, err := h.service.RegisterStore(ctx, register)
	if err != nil {
		t.Fatalf("repeated RegisterStore() = %v, want the stored outcome", err)
	}

	if again.Response() != first.Response() {
		t.Fatalf("replay = %+v, want %+v", again.Response(), first.Response())
	}
	if got := h.outboxLen(); got != 1 {
		t.Fatalf("outbox = %d entries, want 1", got)
	}
	if len(instr.audits) != 1 {
		t.Fatalf("Audit called %d times, want 1: a replay is not a new fact", len(instr.audits))
	}
	if outcome, _ := ports.IdempotencyOutcomeFrom(ctx); outcome != ports.IdempotencyReplayed {
		t.Fatalf("idempotency outcome = %v, want replayed", outcome)
	}
}

func TestARepeatedRefusalReplaysTheRejectionWithoutLoading(t *testing.T) {
	h, instr := newInstrumentedHarness(t)
	h.seedSuspended(t)
	suspend := application.SuspendStore{Ref: testRef, Reason: "motivo"}

	if _, err := h.service.SuspendStore(withKey(t, context.Background(), "k-suspend"), suspend); err != nil {
		t.Fatalf("first SuspendStore() = %v, want a rejection, not an error", err)
	}
	h.rec.Reset()
	again, err := h.service.SuspendStore(withKey(t, context.Background(), "k-suspend"), suspend)
	if err != nil {
		t.Fatalf("repeated SuspendStore() = %v, want the stored rejection", err)
	}

	requireCode(t, again, err, domain.CodeStoreNotActive)
	if slices.Contains(h.rec.Observed(), "stores.Load") {
		t.Fatalf("sequence = %v: the replay reached the aggregate", h.rec.Observed())
	}
	if len(instr.audits) != 1 {
		t.Fatalf("Audit called %d times, want 1: a replay is not a new fact", len(instr.audits))
	}
}

func TestTheSameKeyWithAnotherNameIsAMismatch(t *testing.T) {
	h := newHarness(t)
	if _, err := h.service.RegisterStore(withKey(t, context.Background(), "k-register"), application.RegisterStore{Ref: testRef, Name: testName}); err != nil {
		t.Fatalf("first RegisterStore() = %v, want nil", err)
	}

	_, err := h.service.RegisterStore(withKey(t, context.Background(), "k-register"), application.RegisterStore{Ref: testRef, Name: "Outro nome"})

	if !errors.Is(err, ports.ErrIdempotencyMismatch) {
		t.Fatalf("second RegisterStore() = %v, want ErrIdempotencyMismatch: the name is part of the request", err)
	}
}

func TestTheSameKeyWithAnotherReasonIsAMismatch(t *testing.T) {
	h := newHarness(t)
	h.seedActive(t)
	if _, err := h.service.SuspendStore(withKey(t, context.Background(), "k-suspend"), application.SuspendStore{Ref: testRef, Reason: "motivo"}); err != nil {
		t.Fatalf("first SuspendStore() = %v, want nil", err)
	}

	_, err := h.service.SuspendStore(withKey(t, context.Background(), "k-suspend"), application.SuspendStore{Ref: testRef, Reason: "outro motivo"})

	if !errors.Is(err, ports.ErrIdempotencyMismatch) {
		t.Fatalf("second SuspendStore() = %v, want ErrIdempotencyMismatch: the reason is part of the request", err)
	}
}

func TestACommandWaitingPastTheCeilingIsInFlight(t *testing.T) {
	h := newHarness(t, withRegisterError(ports.ErrRegisterTimeout))

	_, err := h.service.RegisterStore(withExecution(t, context.Background()), application.RegisterStore{Ref: testRef, Name: testName})

	if !errors.Is(err, ports.ErrIdempotencyInFlight) {
		t.Fatalf("RegisterStore() = %v, want ErrIdempotencyInFlight", err)
	}
}

func TestACommandWithoutAKeyIsRefusedBeforeAnyEffect(t *testing.T) {
	h := newHarness(t)
	ctx := ports.WithExecutionContext(context.Background(), execctx.Execution(t))

	_, err := h.service.RegisterStore(ctx, application.RegisterStore{Ref: testRef, Name: testName})

	if !errors.Is(err, ports.ErrIdempotencyKeyAbsent) {
		t.Fatalf("RegisterStore() = %v, want ErrIdempotencyKeyAbsent", err)
	}
	if got := h.outboxLen(); got != 0 {
		t.Fatalf("outbox = %d entries, want none", got)
	}
}

func TestARegisterOfATakenNameIsRefusedAndReplaysTheRefusalWithoutANewAudit(t *testing.T) {
	h, instr := newInstrumentedHarness(t)
	if _, err := h.service.RegisterStore(withKey(t, context.Background(), "k-first"), application.RegisterStore{Ref: testRef, Name: testName}); err != nil {
		t.Fatalf("first RegisterStore() = %v, want nil", err)
	}
	other := application.RegisterStore{Ref: "loja-outra", Name: testName}

	for range 2 {
		outcome, err := h.service.RegisterStore(withKey(t, context.Background(), "k-second"), other)

		requireCode(t, outcome, err, domain.CodeStoreDuplicateName)
	}
	if got := h.outboxLen(); got != 1 {
		t.Fatalf("outbox = %d entries, want 1: the refusal enqueues nothing", got)
	}
	if len(instr.audits) != 2 {
		t.Fatalf("Audit called %d times, want 2: the first registration and the refusal, never the replay", len(instr.audits))
	}
}
