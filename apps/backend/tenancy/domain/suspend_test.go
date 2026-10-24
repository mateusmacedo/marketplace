package domain_test

import (
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

func TestSuspendTakesAnActiveStoreToSuspended(t *testing.T) {
	s := newActiveStore(t)

	acc, rej := s.Suspend(domain.SuspendStore{Reason: "fraude em análise", At: later})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.SuspendedResponse{Ref: storeRef}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	events := acc.Events()
	if len(events) != 1 {
		t.Fatalf("Events() len = %d, want 1", len(events))
	}
	if ev := events[0].(domain.StoreSuspended); ev != (domain.StoreSuspended{Ref: storeRef, Reason: "fraude em análise", At: later}) {
		t.Fatalf("StoreSuspended = %+v", ev)
	}
	want := domain.StoreSnapshot{
		Ref: storeRef, Name: storeName, Status: domain.Suspended, SuspensionReason: "fraude em análise",
		RegisteredAt: at, StatusChangedAt: later,
	}
	if got := s.Snapshot(); !got.Equal(want) {
		t.Fatalf("Snapshot() = %+v, want %+v", got, want)
	}
}

func TestSuspendRejectsAnEmptyReason(t *testing.T) {
	s := newActiveStore(t)
	before := s.Snapshot()

	acc, rej := s.Suspend(domain.SuspendStore{At: later})

	tb.RequireRejected(t, acc, rej, domain.CodeStoreReasonEmpty)
	requireStoreUnchanged(t, before, s.Snapshot())
}

func TestSuspendRejectsASuspendedStore(t *testing.T) {
	s := newSuspendedStore(t)
	before := s.Snapshot()

	acc, rej := s.Suspend(domain.SuspendStore{Reason: "outro motivo", At: later})

	tb.RequireRejected(t, acc, rej, domain.CodeStoreNotActive)
	requireStoreUnchanged(t, before, s.Snapshot())
}

func TestSuspendJudgesTheStatusBeforeTheReason(t *testing.T) {
	s := newSuspendedStore(t)

	acc, rej := s.Suspend(domain.SuspendStore{At: later})

	tb.RequireRejected(t, acc, rej, domain.CodeStoreNotActive)
}
