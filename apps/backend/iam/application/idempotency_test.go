package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/execctx"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestARepeatedDefineReplaysWithoutASecondEventOrAudit(t *testing.T) {
	h, instr := newInstrumentedHarness(t)
	cmd := application.DefineRole{Ref: testRef, Name: testName, Permissions: testPermissions}

	first, err := h.service.DefineRole(withKey(t, context.Background(), "k-define"), cmd)
	if err != nil {
		t.Fatalf("first DefineRole() = %v, want nil", err)
	}
	ctx := withKey(t, context.Background(), "k-define")
	again, err := h.service.DefineRole(ctx, cmd)
	if err != nil {
		t.Fatalf("repeated DefineRole() = %v, want the stored outcome", err)
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
	h.seedRetired(t)
	retire := application.RetireRole{Ref: testRef}

	if _, err := h.service.RetireRole(withKey(t, context.Background(), "k-retire"), retire); err != nil {
		t.Fatalf("first RetireRole() = %v, want a rejection, not an error", err)
	}
	h.rec.Reset()
	again, err := h.service.RetireRole(withKey(t, context.Background(), "k-retire"), retire)
	if err != nil {
		t.Fatalf("repeated RetireRole() = %v, want the stored rejection", err)
	}

	requireCode(t, again, err, domain.CodeRoleRetired)
	if slices.Contains(h.rec.Observed(), "roles.Load") {
		t.Fatalf("sequence = %v: the replay reached the aggregate", h.rec.Observed())
	}
	if len(instr.audits) != 1 {
		t.Fatalf("Audit called %d times, want 1: a replay is not a new fact", len(instr.audits))
	}
}

func TestTheSameKeyWithAnotherFieldIsAMismatch(t *testing.T) {
	for _, tc := range []struct {
		name          string
		first, second application.DefineRole
	}{
		{"name", application.DefineRole{Ref: testRef, Name: testName, Permissions: testPermissions}, application.DefineRole{Ref: testRef, Name: "Outro nome", Permissions: testPermissions}},
		{"permissions", application.DefineRole{Ref: testRef, Name: testName, Permissions: testPermissions}, application.DefineRole{Ref: testRef, Name: testName, Permissions: []domain.Permission{"fulfillment:write"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			if _, err := h.service.DefineRole(withKey(t, context.Background(), "k-define"), tc.first); err != nil {
				t.Fatalf("first DefineRole() = %v, want nil", err)
			}

			_, err := h.service.DefineRole(withKey(t, context.Background(), "k-define"), tc.second)

			if !errors.Is(err, ports.ErrIdempotencyMismatch) {
				t.Fatalf("second DefineRole() = %v, want ErrIdempotencyMismatch: the %s is part of the request", err, tc.name)
			}
		})
	}
}

func TestTheSameKeyWithOtherPermissionsToChangeIsAMismatch(t *testing.T) {
	h := newHarness(t)
	h.seedActive(t)
	if _, err := h.service.ChangeRolePermissions(withKey(t, context.Background(), "k-change"), application.ChangeRolePermissions{Ref: testRef, Permissions: []domain.Permission{"catalog:write"}}); err != nil {
		t.Fatalf("first ChangeRolePermissions() = %v, want nil", err)
	}

	_, err := h.service.ChangeRolePermissions(withKey(t, context.Background(), "k-change"), application.ChangeRolePermissions{Ref: testRef, Permissions: []domain.Permission{"catalog:read"}})

	if !errors.Is(err, ports.ErrIdempotencyMismatch) {
		t.Fatalf("second ChangeRolePermissions() = %v, want ErrIdempotencyMismatch", err)
	}
}

func TestACommandWaitingPastTheCeilingIsInFlight(t *testing.T) {
	h := newHarness(t, withRegisterError(ports.ErrRegisterTimeout))

	_, err := h.service.DefineRole(withExecution(t, context.Background()), application.DefineRole{Ref: testRef, Name: testName, Permissions: testPermissions})

	if !errors.Is(err, ports.ErrIdempotencyInFlight) {
		t.Fatalf("DefineRole() = %v, want ErrIdempotencyInFlight", err)
	}
}

func TestACommandWithoutAKeyIsRefusedBeforeAnyEffect(t *testing.T) {
	h := newHarness(t)
	ctx := ports.WithExecutionContext(context.Background(), execctx.Execution(t))

	_, err := h.service.DefineRole(ctx, application.DefineRole{Ref: testRef, Name: testName, Permissions: testPermissions})

	if !errors.Is(err, ports.ErrIdempotencyKeyAbsent) {
		t.Fatalf("DefineRole() = %v, want ErrIdempotencyKeyAbsent", err)
	}
	if got := h.outboxLen(); got != 0 {
		t.Fatalf("outbox = %d entries, want none", got)
	}
}

func TestADefinitionOfATakenNameIsRefusedAndReplaysTheRefusalWithoutANewAudit(t *testing.T) {
	h, instr := newInstrumentedHarness(t)
	if _, err := h.service.DefineRole(withKey(t, context.Background(), "k-first"), application.DefineRole{Ref: testRef, Name: testName, Permissions: testPermissions}); err != nil {
		t.Fatalf("first DefineRole() = %v, want nil", err)
	}
	other := application.DefineRole{Ref: "outro-papel", Name: testName, Permissions: testPermissions}

	for range 2 {
		outcome, err := h.service.DefineRole(withKey(t, context.Background(), "k-second"), other)

		requireCode(t, outcome, err, domain.CodeRoleDuplicateName)
	}
	if got := h.outboxLen(); got != 1 {
		t.Fatalf("outbox = %d entries, want 1: the refusal enqueues nothing", got)
	}
	if len(instr.audits) != 2 {
		t.Fatalf("Audit called %d times, want 2: the first definition and the refusal, never the replay", len(instr.audits))
	}
}
