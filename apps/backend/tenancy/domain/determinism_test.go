package domain_test

import (
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

func TestRegisterDeterminism(t *testing.T) {
	cmd := domain.RegisterStore{Ref: storeRef, Name: storeName, At: at}

	s1 := domain.NewStore(storeRef)
	acc1, _ := s1.Register(cmd)
	s2 := domain.NewStore(storeRef)
	acc2, _ := s2.Register(cmd)

	if !s1.Snapshot().Equal(s2.Snapshot()) {
		t.Fatal("same inputs must produce the same state")
	}
	tb.RequireSameEvents(t, acc1.Events(), acc2.Events())
}

func TestSuspendDeterminism(t *testing.T) {
	cmd := domain.SuspendStore{Reason: "fraude em análise", At: later}

	s1 := newActiveStore(t)
	acc1, _ := s1.Suspend(cmd)
	s2 := newActiveStore(t)
	acc2, _ := s2.Suspend(cmd)

	if !s1.Snapshot().Equal(s2.Snapshot()) {
		t.Fatal("same inputs must produce the same state")
	}
	tb.RequireSameEvents(t, acc1.Events(), acc2.Events())
}

func TestReactivateDeterminism(t *testing.T) {
	cmd := domain.ReactivateStore{At: later}

	s1 := newSuspendedStore(t)
	acc1, _ := s1.Reactivate(cmd)
	s2 := newSuspendedStore(t)
	acc2, _ := s2.Reactivate(cmd)

	if !s1.Snapshot().Equal(s2.Snapshot()) {
		t.Fatal("same inputs must produce the same state")
	}
	tb.RequireSameEvents(t, acc1.Events(), acc2.Events())
}
