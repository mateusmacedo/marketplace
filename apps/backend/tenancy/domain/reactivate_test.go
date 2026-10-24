package domain_test

import (
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

func TestReactivateReturnsASuspendedStoreToActiveAndClearsTheReason(t *testing.T) {
	s := newSuspendedStore(t)

	acc, rej := s.Reactivate(domain.ReactivateStore{At: later})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.ReactivatedResponse{Ref: storeRef}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	events := acc.Events()
	if len(events) != 1 {
		t.Fatalf("Events() len = %d, want 1", len(events))
	}
	if ev := events[0].(domain.StoreReactivated); ev != (domain.StoreReactivated{Ref: storeRef, At: later}) {
		t.Fatalf("StoreReactivated = %+v", ev)
	}
	want := domain.StoreSnapshot{Ref: storeRef, Name: storeName, Status: domain.Active, RegisteredAt: at, StatusChangedAt: later}
	if got := s.Snapshot(); !got.Equal(want) {
		t.Fatalf("Snapshot() = %+v, want %+v", got, want)
	}
}

func TestReactivateRejectsAnActiveStore(t *testing.T) {
	s := newActiveStore(t)
	before := s.Snapshot()

	acc, rej := s.Reactivate(domain.ReactivateStore{At: later})

	tb.RequireRejected(t, acc, rej, domain.CodeStoreNotSuspended)
	requireStoreUnchanged(t, before, s.Snapshot())
}
