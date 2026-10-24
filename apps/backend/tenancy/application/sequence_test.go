package application_test

import (
	"context"
	"slices"
	"testing"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

var nineSteps = []string{
	"authorize",
	"clock.Now",
	"ids.NewMessageID",
	"within",
	"commands.Register",
	"stores.Load",
	"stores.Save",
	"outbox.Enqueue",
	"commands.Complete",
	"commit",
}

func TestRegisterWalksTheNineStepsInOrder(t *testing.T) {
	h := newHarness(t)

	if _, err := h.service.RegisterStore(withExecution(t, context.Background()), application.RegisterStore{Ref: testRef, Name: testName}); err != nil {
		t.Fatalf("RegisterStore() error = %v, want nil", err)
	}

	if !slices.Equal(h.rec.Observed(), nineSteps) {
		t.Fatalf("sequence mismatch (FND-04 §3.2)\ngot:  %v\nwant: %v", h.rec.Observed(), nineSteps)
	}
}

func TestSuspendWalksTheNineStepsInOrder(t *testing.T) {
	h := newHarness(t)
	h.seedActive(t)

	if _, err := h.service.SuspendStore(withExecution(t, context.Background()), application.SuspendStore{Ref: testRef, Reason: "fraude em análise"}); err != nil {
		t.Fatalf("SuspendStore() error = %v, want nil", err)
	}

	if !slices.Equal(h.rec.Observed(), nineSteps) {
		t.Fatalf("sequence mismatch (FND-04 §3.2)\ngot:  %v\nwant: %v", h.rec.Observed(), nineSteps)
	}
}

func TestReactivateWalksTheNineStepsInOrder(t *testing.T) {
	h := newHarness(t)
	h.seedSuspended(t)

	if _, err := h.service.ReactivateStore(withExecution(t, context.Background()), application.ReactivateStore{Ref: testRef}); err != nil {
		t.Fatalf("ReactivateStore() error = %v, want nil", err)
	}

	if !slices.Equal(h.rec.Observed(), nineSteps) {
		t.Fatalf("sequence mismatch (FND-04 §3.2)\ngot:  %v\nwant: %v", h.rec.Observed(), nineSteps)
	}
}

func TestRegisterEnqueuesTheRegistrationInTheSameTransaction(t *testing.T) {
	h := newHarness(t)

	if _, err := h.service.RegisterStore(withExecution(t, context.Background()), application.RegisterStore{Ref: testRef, Name: testName}); err != nil {
		t.Fatalf("RegisterStore() error = %v, want nil", err)
	}

	entries := h.store.Entries()
	if len(entries) != 1 {
		t.Fatalf("outbox entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	want := domain.StoreRegistered{Ref: testRef, Name: testName, At: testOccurred}
	if entry.Event != want || entry.AggregateType != application.AggregateTypeStore ||
		entry.AggregateID != string(testRef) || entry.AggregateVersion != 1 || entry.Intent.Destination != application.Destination {
		t.Fatalf("outbox entry = %+v, want the registration of the store at version 1", entry)
	}
}

func TestSuspendEnqueuesTheSuspensionInTheSameTransaction(t *testing.T) {
	h := newHarness(t)
	h.seedActive(t)

	if _, err := h.service.SuspendStore(withExecution(t, context.Background()), application.SuspendStore{Ref: testRef, Reason: "fraude em análise"}); err != nil {
		t.Fatalf("SuspendStore() error = %v, want nil", err)
	}

	entries := h.store.Entries()
	want := domain.StoreSuspended{Ref: testRef, Reason: "fraude em análise", At: testOccurred}
	if len(entries) != 1 || entries[0].Event != want || entries[0].AggregateVersion != 2 {
		t.Fatalf("outbox = %+v, want the suspension of the store at version 2", entries)
	}
}

func TestReactivateEnqueuesTheReactivationInTheSameTransaction(t *testing.T) {
	h := newHarness(t)
	h.seedSuspended(t)

	if _, err := h.service.ReactivateStore(withExecution(t, context.Background()), application.ReactivateStore{Ref: testRef}); err != nil {
		t.Fatalf("ReactivateStore() error = %v, want nil", err)
	}

	entries := h.store.Entries()
	want := domain.StoreReactivated{Ref: testRef, At: testOccurred}
	if len(entries) != 1 || entries[0].Event != want || entries[0].AggregateVersion != 2 {
		t.Fatalf("outbox = %+v, want the reactivation of the store at version 2", entries)
	}
}
