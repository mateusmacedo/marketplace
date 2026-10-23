package domain_test

import (
	"strings"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

func TestRegisterAcceptsAnUnknownRefAsAnActiveStore(t *testing.T) {
	s := domain.NewStore(storeRef)

	acc, rej := s.Register(domain.RegisterStore{Ref: storeRef, Name: storeName, At: at})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.RegisteredResponse{Ref: storeRef}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	events := acc.Events()
	if len(events) != 1 {
		t.Fatalf("Events() len = %d, want 1", len(events))
	}
	if ev := events[0].(domain.StoreRegistered); ev != (domain.StoreRegistered{Ref: storeRef, Name: storeName, At: at}) {
		t.Fatalf("StoreRegistered = %+v", ev)
	}
	want := domain.StoreSnapshot{Ref: storeRef, Name: storeName, Status: domain.Active, RegisteredAt: at, StatusChangedAt: at}
	if got := s.Snapshot(); !got.Equal(want) {
		t.Fatalf("Snapshot() = %+v, want %+v", got, want)
	}
}

func TestRegisterOfAKnownRefAcceptsWithoutChangingOrEmitting(t *testing.T) {
	s := newSuspendedStore(t)
	before := s.Snapshot()

	acc, rej := s.Register(domain.RegisterStore{Ref: storeRef, Name: "Outro nome", At: later})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.RegisteredResponse{Ref: storeRef}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	if len(acc.Events()) != 0 {
		t.Fatalf("Events() len = %d, want 0: a known ref registers nothing again", len(acc.Events()))
	}
	requireStoreUnchanged(t, before, s.Snapshot())
}

func TestRegisterRejectsARefOutsideTheLabelForm(t *testing.T) {
	for _, ref := range []string{"Loja_1", "ab", "-loja", "loja-", "loja acme", "", strings.Repeat("a", 64)} {
		t.Run(ref, func(t *testing.T) {
			s := domain.NewStore(domain.StoreRef(ref))
			before := s.Snapshot()

			acc, rej := s.Register(domain.RegisterStore{Ref: domain.StoreRef(ref), Name: storeName, At: at})

			tb.RequireRejected(t, acc, rej, domain.CodeStoreRefInvalid)
			requireStoreUnchanged(t, before, s.Snapshot())
		})
	}
}

func TestRegisterAcceptsARefAtTheBoundsOfTheLabelForm(t *testing.T) {
	for _, ref := range []string{"abc", "a-1", "0-loja-9", strings.Repeat("z", 63)} {
		t.Run(ref, func(t *testing.T) {
			s := domain.NewStore(domain.StoreRef(ref))

			_, rej := s.Register(domain.RegisterStore{Ref: domain.StoreRef(ref), Name: storeName, At: at})

			tb.RequireAccepted(t, rej)
		})
	}
}

func TestRegisterRejectsAnEmptyName(t *testing.T) {
	s := domain.NewStore(storeRef)
	before := s.Snapshot()

	acc, rej := s.Register(domain.RegisterStore{Ref: storeRef, At: at})

	tb.RequireRejected(t, acc, rej, domain.CodeStoreNameEmpty)
	requireStoreUnchanged(t, before, s.Snapshot())
}

func TestTheDuplicateNameRejectionCarriesItsCode(t *testing.T) {
	if got := domain.RejectStoreDuplicateName.Code(); got != domain.CodeStoreDuplicateName {
		t.Fatalf("Code() = %q, want %q", got, domain.CodeStoreDuplicateName)
	}
}

func TestValidRefIsTheRuleRegisterApplies(t *testing.T) {
	for ref, want := range map[domain.StoreRef]bool{"loja-acme": true, "abc": true, "ab": false, "Loja_1": false, "-loja": false, "loja-": false} {
		if got := domain.ValidRef(ref); got != want {
			t.Errorf("ValidRef(%q) = %v, want %v", ref, got, want)
		}
	}
}
