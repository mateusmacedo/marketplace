//go:build integration

package appkit_test

import (
	"context"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/ids"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/appkit"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
)

func harness(t *testing.T) appkit.Harness {
	t.Helper()
	return appkit.NewTenancy(t, clock.New(ports.Instant(1)), &ids.Sequence{Prefix: "m-"})
}

func TestTheUseCaseLeavesOneOutboxRecordPerDecision(t *testing.T) {
	h := harness(t)

	registered, err := h.Service.RegisterStore(withExecution(t, context.Background()), application.RegisterStore{Ref: "loja-appkit", Name: "Loja appkit"})

	if _, refused := registered.Rejection(); err != nil || refused {
		t.Fatalf("RegisterStore() = (%+v, %v), want accepted", registered, err)
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

	registered, err := h.Service.RegisterStore(withExecution(t, context.Background()), application.RegisterStore{Ref: "Loja_1", Name: "Loja appkit"})

	if _, refused := registered.Rejection(); err != nil || !refused {
		t.Fatalf("RegisterStore(Loja_1) = (%+v, %v), want the rejection on the business channel", registered, err)
	}
	if n := len(h.Outbox(t)); n != 0 {
		t.Fatalf("outbox = %d records after a rejection, want none: the transaction never committed", n)
	}
}

func TestARepeatedCommandWithTheSameKeyLeavesOneEffectAndOneEvent(t *testing.T) {
	h := harness(t)
	register := application.RegisterStore{Ref: "loja-appkit", Name: "Loja appkit"}

	first, err := h.Service.RegisterStore(withKey(t, context.Background(), "k-appkit"), register)
	if err != nil {
		t.Fatalf("RegisterStore() = %v, want nil", err)
	}
	again, err := h.Service.RegisterStore(withKey(t, context.Background(), "k-appkit"), register)
	if err != nil {
		t.Fatalf("repeated RegisterStore() = %v, want the stored outcome", err)
	}

	if again.Response() != first.Response() {
		t.Fatalf("replay = %+v, want %+v", again.Response(), first.Response())
	}
	if n := len(h.Outbox(t)); n != 1 {
		t.Fatalf("outbox = %d records, want exactly one: one effect and one event per key", n)
	}
}
