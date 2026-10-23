package domain_test

import (
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

func TestFromStoreSnapshotRoundTripsAnActiveStoreWithAnEmptyReason(t *testing.T) {
	before := newActiveStore(t).Snapshot()
	if before.SuspensionReason != "" {
		t.Fatalf("SuspensionReason = %q, want empty while active", before.SuspensionReason)
	}

	got := domain.FromStoreSnapshot(before).Snapshot()

	if !got.Equal(before) {
		t.Fatalf("round trip lost state\nbefore: %+v\nafter:  %+v", before, got)
	}
}

func TestFromStoreSnapshotRoundTripsTheReasonAndBothInstants(t *testing.T) {
	s := newActiveStore(t)
	if _, rej := s.Suspend(domain.SuspendStore{Reason: "fraude em análise", At: later}); rej != nil {
		t.Fatalf("setup: Suspend rejected: %v", rej)
	}
	before := s.Snapshot()

	got := domain.FromStoreSnapshot(before).Snapshot()

	if !got.Equal(before) || got.RegisteredAt != at || got.StatusChangedAt != later {
		t.Fatalf("round trip lost state\nbefore: %+v\nafter:  %+v", before, got)
	}
}

func TestFromStoreSnapshotRestoresTheStatusAsBehaviour(t *testing.T) {
	reconstituted := domain.FromStoreSnapshot(newSuspendedStore(t).Snapshot())

	acc, rej := reconstituted.Reactivate(domain.ReactivateStore{At: later})

	tb.RequireAccepted(t, rej)
	if len(acc.Events()) != 1 {
		t.Fatalf("Events() len = %d, want 1", len(acc.Events()))
	}
}

func TestEqualTellsEveryFieldApart(t *testing.T) {
	base := newSuspendedStore(t).Snapshot()
	for name, mutate := range map[string]func(*domain.StoreSnapshot){
		"ref":             func(s *domain.StoreSnapshot) { s.Ref = "outra-loja" },
		"name":            func(s *domain.StoreSnapshot) { s.Name = "Outro nome" },
		"status":          func(s *domain.StoreSnapshot) { s.Status = domain.Active },
		"reason":          func(s *domain.StoreSnapshot) { s.SuspensionReason = "outro motivo" },
		"registeredAt":    func(s *domain.StoreSnapshot) { s.RegisteredAt++ },
		"statusChangedAt": func(s *domain.StoreSnapshot) { s.StatusChangedAt++ },
	} {
		t.Run(name, func(t *testing.T) {
			other := base
			mutate(&other)
			if base.Equal(other) {
				t.Fatalf("Equal() ignores %s", name)
			}
		})
	}
}
