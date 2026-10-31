//go:build integration

package appkit_test

import (
	"context"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/ids"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/appkit"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

var defineExpedicao = application.DefineRole{Ref: "expedicao", Name: "Expedição", Permissions: []domain.Permission{"fulfillment:write"}}

func harness(t *testing.T) appkit.Harness {
	t.Helper()
	return appkit.NewIam(t, clock.New(ports.Instant(1)), &ids.Sequence{Prefix: "m-"})
}

func TestTheUseCaseLeavesOneOutboxRecordPerDecision(t *testing.T) {
	h := harness(t)

	defined, err := h.Service.DefineRole(withExecution(t, context.Background()), defineExpedicao)

	if _, refused := defined.Rejection(); err != nil || refused {
		t.Fatalf("DefineRole() = (%+v, %v), want accepted", defined, err)
	}
	enqueued := h.Outbox(t)
	if len(enqueued) != 1 {
		t.Fatalf("outbox = %d records, want 1: the event goes to the outbox in the same transaction as the state", len(enqueued))
	}
	if enqueued[0].Destination != application.Destination || enqueued[0].Status != "pending" {
		t.Fatalf("record = %+v, want pending to %s", enqueued[0], application.Destination)
	}
}

func TestARejectedDecisionLeavesNothingBehind(t *testing.T) {
	h := harness(t)

	defined, err := h.Service.DefineRole(withExecution(t, context.Background()), application.DefineRole{Ref: "Exp_1", Name: "Expedição", Permissions: []domain.Permission{"fulfillment:write"}})

	if _, refused := defined.Rejection(); err != nil || !refused {
		t.Fatalf("DefineRole(Exp_1) = (%+v, %v), want the rejection on the business channel", defined, err)
	}
	if n := len(h.Outbox(t)); n != 0 {
		t.Fatalf("outbox = %d records after a rejection, want none: the transaction never committed", n)
	}
}

func TestARepeatedCommandWithTheSameKeyLeavesOneEffectAndOneEvent(t *testing.T) {
	h := harness(t)

	first, err := h.Service.DefineRole(withKey(t, context.Background(), "k-appkit"), defineExpedicao)
	if err != nil {
		t.Fatalf("DefineRole() = %v, want nil", err)
	}
	again, err := h.Service.DefineRole(withKey(t, context.Background(), "k-appkit"), defineExpedicao)
	if err != nil {
		t.Fatalf("repeated DefineRole() = %v, want the stored outcome", err)
	}

	if again.Response() != first.Response() {
		t.Fatalf("replay = %+v, want %+v", again.Response(), first.Response())
	}
	if n := len(h.Outbox(t)); n != 1 {
		t.Fatalf("outbox = %d records, want exactly one: one effect and one event per key", n)
	}
}
