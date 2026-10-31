package domain_test

import (
	"strings"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestRegisterAcceptsAnUnknownSubjectAsAnActiveBuyer(t *testing.T) {
	b := domain.NewBuyer(subject)

	acc, rej := b.Register(domain.RegisterBuyer{Subject: subject, Name: buyerName, At: at})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.RegisteredResponse{Subject: subject, Status: domain.BuyerStatusActive}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	want := domain.BuyerRegistered{Subject: subject, Name: buyerName, AccessVersion: 1, At: at}
	if events := acc.Events(); len(events) != 1 || !sameEvent(events[0], want) {
		t.Fatalf("Events() = %+v, want [%+v]", events, want)
	}
	wantState := domain.BuyerSnapshot{Subject: subject, Name: buyerName, Status: domain.BuyerStatusActive, AccessVersion: 1, RegisteredAt: at, ChangedAt: at}
	if got := b.Snapshot(); !got.Equal(wantState) {
		t.Fatalf("Snapshot() = %+v, want %+v", got, wantState)
	}
}

func TestRegisterOfAKnownBuyerAcceptsWithoutChangingOrEmittingEvenWhenBlocked(t *testing.T) {
	for name, known := range map[string]func(*testing.T) *domain.Buyer{"active": newActiveBuyer, "blocked": newBlockedBuyer} {
		t.Run(name, func(t *testing.T) {
			b := known(t)
			before := b.Snapshot()

			acc, rej := b.Register(domain.RegisterBuyer{Subject: subject, Name: "Outro nome", At: later})

			tb.RequireAccepted(t, rej)
			if got, want := acc.Response(), (domain.RegisteredResponse{Subject: subject, Status: before.Status}); got != want {
				t.Fatalf("Response() = %+v, want %+v: the status it already had", got, want)
			}
			if len(acc.Events()) != 0 {
				t.Fatalf("Events() len = %d, want 0", len(acc.Events()))
			}
			requireBuyerUnchanged(t, before, b.Snapshot())
		})
	}
}

func TestRegisterRejectsASubjectOutsideItsForm(t *testing.T) {
	for _, s := range []string{"", strings.Repeat("s", 256), "a\u0000b", "a\u0007b", "a\u0085b"} {
		b := domain.NewBuyer(domain.Subject(s))
		before := b.Snapshot()

		acc, rej := b.Register(domain.RegisterBuyer{Subject: domain.Subject(s), Name: buyerName, At: at})

		tb.RequireRejected(t, acc, rej, domain.CodeBuyerSubjectInvalid)
		requireBuyerUnchanged(t, before, b.Snapshot())
	}
}

func TestRegisterRejectsAnEmptyName(t *testing.T) {
	acc, rej := domain.NewBuyer(subject).Register(domain.RegisterBuyer{Subject: subject, At: at})

	tb.RequireRejected(t, acc, rej, domain.CodeBuyerNameEmpty)
}

func TestUpdateProfileRenamesAnActiveBuyer(t *testing.T) {
	b := newActiveBuyer(t)

	acc, rej := b.UpdateProfile(domain.UpdateBuyerProfile{Name: "Bia Souza", At: later})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.ProfileUpdatedResponse{Subject: subject}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	want := domain.BuyerProfileUpdated{Subject: subject, Name: "Bia Souza", At: later}
	if events := acc.Events(); len(events) != 1 || !sameEvent(events[0], want) {
		t.Fatalf("Events() = %+v, want [%+v]", events, want)
	}
	if got := b.Snapshot(); got.Name != "Bia Souza" || got.ChangedAt != later || got.AccessVersion != 1 {
		t.Fatalf("Snapshot() = %+v, want renamed at %d without a change of access", got, later)
	}
}

func TestUpdateProfileRejectsABlockedBuyerAndAnEmptyName(t *testing.T) {
	blocked := newBlockedBuyer(t)
	before := blocked.Snapshot()
	acc, rej := blocked.UpdateProfile(domain.UpdateBuyerProfile{Name: "Bia Souza", At: later})
	tb.RequireRejected(t, acc, rej, domain.CodeBuyerBlocked)
	requireBuyerUnchanged(t, before, blocked.Snapshot())

	acc, rej = newActiveBuyer(t).UpdateProfile(domain.UpdateBuyerProfile{At: later})
	tb.RequireRejected(t, acc, rej, domain.CodeBuyerNameEmpty)
}

func TestBlockTakesAnActiveBuyerToBlockedWithTheReason(t *testing.T) {
	b := newActiveBuyer(t)

	acc, rej := b.Block(domain.BlockBuyer{Reason: "chargeback recorrente", At: later})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.BlockedResponse{Subject: subject}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	want := domain.BuyerBlocked{Subject: subject, Reason: "chargeback recorrente", AccessVersion: 2, At: later}
	if events := acc.Events(); len(events) != 1 || !sameEvent(events[0], want) {
		t.Fatalf("Events() = %+v, want [%+v]", events, want)
	}
	wantState := domain.BuyerSnapshot{
		Subject: subject, Name: buyerName, Status: domain.BuyerStatusBlocked, BlockReason: "chargeback recorrente",
		AccessVersion: 2, RegisteredAt: at, ChangedAt: later,
	}
	if got := b.Snapshot(); !got.Equal(wantState) {
		t.Fatalf("Snapshot() = %+v, want %+v", got, wantState)
	}
}

func TestBlockRejectsABlockedBuyerAndAnEmptyReason(t *testing.T) {
	blocked := newBlockedBuyer(t)
	before := blocked.Snapshot()
	acc, rej := blocked.Block(domain.BlockBuyer{Reason: "outro motivo", At: later})
	tb.RequireRejected(t, acc, rej, domain.CodeBuyerNotActive)
	requireBuyerUnchanged(t, before, blocked.Snapshot())

	acc, rej = newActiveBuyer(t).Block(domain.BlockBuyer{At: later})
	tb.RequireRejected(t, acc, rej, domain.CodeBuyerReasonEmpty)
}

func TestUnblockGivesTheBlockedBuyerTheRightToBuyBack(t *testing.T) {
	b := newBlockedBuyer(t)

	acc, rej := b.Unblock(domain.UnblockBuyer{At: later})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.UnblockedResponse{Subject: subject}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	want := domain.BuyerUnblocked{Subject: subject, AccessVersion: 3, At: later}
	if events := acc.Events(); len(events) != 1 || !sameEvent(events[0], want) {
		t.Fatalf("Events() = %+v, want [%+v]", events, want)
	}
	if got := b.Snapshot(); got.Status != domain.BuyerStatusActive || got.BlockReason != "" || got.AccessVersion != 3 || got.ChangedAt != later {
		t.Fatalf("Snapshot() = %+v, want active without a reason at access version 3", got)
	}
}

func TestUnblockRejectsAnActiveBuyer(t *testing.T) {
	b := newActiveBuyer(t)
	before := b.Snapshot()

	acc, rej := b.Unblock(domain.UnblockBuyer{At: later})

	tb.RequireRejected(t, acc, rej, domain.CodeBuyerNotBlocked)
	requireBuyerUnchanged(t, before, b.Snapshot())
}

func TestConfirmAccessSyncedOfABuyerKeepsTheHighestVersion(t *testing.T) {
	b := newBlockedBuyer(t)

	if _, rej := b.ConfirmAccessSynced(domain.ConfirmBuyerAccessSynced{Version: 2}); rej != nil {
		t.Fatalf("ConfirmAccessSynced(2) rejected: %v", rej)
	}
	acc, rej := b.ConfirmAccessSynced(domain.ConfirmBuyerAccessSynced{Version: 1})

	tb.RequireAccepted(t, rej)
	if len(acc.Events()) != 0 {
		t.Fatalf("Events() len = %d, want 0", len(acc.Events()))
	}
	if got := b.Snapshot(); got.SyncedVersion != 2 || got.Status != domain.BuyerStatusBlocked {
		t.Fatalf("Snapshot() = %+v, want blocked and synced at 2", got)
	}
}

func TestConfirmAccessSyncedOfABuyerRejectsAVersionAhead(t *testing.T) {
	b := newActiveBuyer(t)
	before := b.Snapshot()

	acc, rej := b.ConfirmAccessSynced(domain.ConfirmBuyerAccessSynced{Version: 2})

	tb.RequireRejected(t, acc, rej, domain.CodeBuyerVersionAhead)
	requireBuyerUnchanged(t, before, b.Snapshot())
}

func TestBuyerEqualTellsEveryFieldApart(t *testing.T) {
	base := newBlockedBuyer(t).Snapshot()
	for name, mutate := range map[string]func(*domain.BuyerSnapshot){
		"subject":       func(s *domain.BuyerSnapshot) { s.Subject = "kc-outro" },
		"name":          func(s *domain.BuyerSnapshot) { s.Name = "Outro" },
		"status":        func(s *domain.BuyerSnapshot) { s.Status = domain.BuyerStatusActive },
		"blockReason":   func(s *domain.BuyerSnapshot) { s.BlockReason = "outro" },
		"accessVersion": func(s *domain.BuyerSnapshot) { s.AccessVersion++ },
		"syncedVersion": func(s *domain.BuyerSnapshot) { s.SyncedVersion++ },
		"registeredAt":  func(s *domain.BuyerSnapshot) { s.RegisteredAt++ },
		"changedAt":     func(s *domain.BuyerSnapshot) { s.ChangedAt++ },
	} {
		other := base
		mutate(&other)
		if base.Equal(other) {
			t.Errorf("Equal() ignores %s", name)
		}
	}
}

func TestFromBuyerSnapshotRoundTripsTheWholeState(t *testing.T) {
	before := newBlockedBuyer(t).Snapshot()

	if got := domain.FromBuyerSnapshot(before).Snapshot(); !got.Equal(before) {
		t.Fatalf("round trip lost state\nbefore: %+v\nafter:  %+v", before, got)
	}
}
