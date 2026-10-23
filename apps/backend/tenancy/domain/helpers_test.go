package domain_test

import (
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

const (
	storeRef  = domain.StoreRef("loja-acme")
	storeName = domain.StoreName("Loja Acme")
	at        = kernel.Instant(1_755_432_000_000_000_000)
	later     = kernel.Instant(1_755_435_600_000_000_000)
)

func newActiveStore(t *testing.T) *domain.Store {
	t.Helper()
	s := domain.NewStore(storeRef)
	_, rej := s.Register(domain.RegisterStore{Ref: storeRef, Name: storeName, At: at})
	tb.RequireAccepted(t, rej)
	return s
}

func newSuspendedStore(t *testing.T) *domain.Store {
	t.Helper()
	s := newActiveStore(t)
	_, rej := s.Suspend(domain.SuspendStore{Reason: "fraude em análise", At: at})
	tb.RequireAccepted(t, rej)
	return s
}

func requireStoreUnchanged(t *testing.T, before, after domain.StoreSnapshot) {
	t.Helper()
	if !before.Equal(after) {
		t.Fatalf("snapshot changed on rejection\nbefore: %+v\nafter:  %+v", before, after)
	}
}
