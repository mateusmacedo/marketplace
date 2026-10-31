package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/serviceskit"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

type instrumentationRecorder struct {
	steps   *serviceskit.Steps
	results []ports.Result
	audits  []ports.AuditEvent
}

func (r *instrumentationRecorder) BeginOperation(ctx context.Context, _ string) (context.Context, ports.EndOperation) {
	r.steps.Record("begin")
	return ctx, func(result ports.Result) {
		r.steps.Record("end")
		r.results = append(r.results, result)
	}
}

func (r *instrumentationRecorder) Audit(_ context.Context, event ports.AuditEvent) {
	r.steps.Record("audit")
	r.audits = append(r.audits, event)
}

func (r *instrumentationRecorder) requireOnlyAudit(t *testing.T, want ports.AuditEvent) {
	t.Helper()
	if len(r.audits) != 1 || r.audits[0] != want {
		t.Fatalf("audits = %+v, want exactly %+v", r.audits, want)
	}
}

func newInstrumentedHarness(t *testing.T) (*harness, *instrumentationRecorder) {
	t.Helper()
	h := newHarness(t)
	instr := &instrumentationRecorder{steps: h.rec}
	h.service.Instrumentation = instr
	return h, instr
}

func instrumentedWith(t *testing.T, authorize func(context.Context, application.Operation) error) (*harness, *instrumentationRecorder) {
	t.Helper()
	h, instr := newInstrumentedHarness(t)
	h.service.Authorize = authorize
	return h, instr
}

func TestADeniedCommandReportsDeniedWithoutTheErrorAndWritesNothing(t *testing.T) {
	h, instr := instrumentedWith(t, func(context.Context, application.Operation) error {
		return errors.Join(errors.New("roles live in the tenant of a store"), ports.ErrDenied)
	})

	_, err := h.service.DefineRole(withExecution(t, context.Background()), application.DefineRole{Ref: testRef, Name: testName, Permissions: testPermissions})

	if !errors.Is(err, ports.ErrDenied) {
		t.Fatalf("DefineRole() error = %v, want a denial", err)
	}
	if len(instr.results) != 1 || instr.results[0].Outcome != ports.OutcomeDenied || instr.results[0].Err != nil {
		t.Fatalf("results = %+v, want one denied result without error: a denial is not a technical failure", instr.results)
	}
	if got := h.outboxLen(); got != 0 {
		t.Fatalf("outbox = %d entries, want none", got)
	}
}

func TestAnAuthorizerFailureReportsFailedNotDenied(t *testing.T) {
	failure := errors.New("policy engine unreachable")
	h, instr := instrumentedWith(t, func(context.Context, application.Operation) error { return failure })

	_, err := h.service.DefineRole(withExecution(t, context.Background()), application.DefineRole{Ref: testRef, Name: testName, Permissions: testPermissions})

	if !errors.Is(err, failure) {
		t.Fatalf("DefineRole() error = %v, want the authorizer failure", err)
	}
	if len(instr.results) != 1 || instr.results[0].Outcome != ports.OutcomeFailed || !errors.Is(instr.results[0].Err, failure) {
		t.Fatalf("results = %+v, want one failed result carrying the error", instr.results)
	}
}

func TestEachAcceptedCommandAuditsOnce(t *testing.T) {
	for _, tc := range []struct {
		action string
		seed   func(*harness, *testing.T)
		run    func(*testing.T, *harness) error
	}{
		{application.OperationDefineRole, nil, func(t *testing.T, h *harness) error {
			_, err := h.service.DefineRole(withExecution(t, context.Background()), application.DefineRole{Ref: testRef, Name: testName, Permissions: testPermissions})
			return err
		}},
		{application.OperationChangeRolePermissions, (*harness).seedActive, func(t *testing.T, h *harness) error {
			_, err := h.service.ChangeRolePermissions(withExecution(t, context.Background()), application.ChangeRolePermissions{Ref: testRef, Permissions: []domain.Permission{"catalog:write"}})
			return err
		}},
		{application.OperationRetireRole, (*harness).seedActive, func(t *testing.T, h *harness) error {
			_, err := h.service.RetireRole(withExecution(t, context.Background()), application.RetireRole{Ref: testRef})
			return err
		}},
	} {
		t.Run(tc.action, func(t *testing.T) {
			h, instr := newInstrumentedHarness(t)
			if tc.seed != nil {
				tc.seed(h, t)
			}

			if err := tc.run(t, h); err != nil {
				t.Fatalf("%s() error = %v, want nil", tc.action, err)
			}

			instr.requireOnlyAudit(t, ports.AuditEvent{Object: string(testRef), Action: tc.action, Outcome: ports.OutcomeAccepted, At: testOccurred})
		})
	}
}

func TestBeginPrecedesAuthorizationAndEndClosesTheSequence(t *testing.T) {
	h, _ := newInstrumentedHarness(t)

	if _, err := h.service.DefineRole(withExecution(t, context.Background()), application.DefineRole{Ref: testRef, Name: testName, Permissions: testPermissions}); err != nil {
		t.Fatalf("DefineRole() error = %v, want nil", err)
	}

	observed := h.rec.Observed()
	if len(observed) < 4 || observed[0] != "begin" || observed[1] != "authorize" {
		t.Fatalf("observed = %v, want begin then authorize — the span opens before step 1 (TRC-16)", observed)
	}
	if got := observed[len(observed)-2:]; got[0] != "end" || got[1] != "audit" {
		t.Fatalf("observed = %v, want the sequence to close with end then audit", observed)
	}
}
