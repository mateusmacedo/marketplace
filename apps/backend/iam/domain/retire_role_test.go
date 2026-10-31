package domain_test

import (
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestRetireTakesAnActiveRoleToRetiredAndRaisesTheAccessVersion(t *testing.T) {
	r := newActiveRole(t)

	acc, rej := r.Retire(domain.RetireRole{At: later})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.RetiredResponse{Ref: roleRef}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	want := domain.RoleRetired{Ref: roleRef, AccessVersion: 2, At: later}
	if events := acc.Events(); len(events) != 1 || !sameEvent(events[0], want) {
		t.Fatalf("Events() = %+v, want [%+v]", events, want)
	}
	wantState := domain.RoleSnapshot{
		Ref: roleRef, Name: roleName, Permissions: rolePermissions, Status: domain.RoleStatusRetired,
		AccessVersion: 2, DefinedAt: at, ChangedAt: later,
	}
	if got := r.Snapshot(); !got.Equal(wantState) {
		t.Fatalf("Snapshot() = %+v, want %+v", got, wantState)
	}
}

func TestRetireRejectsARetiredRole(t *testing.T) {
	r := newRetiredRole(t)
	before := r.Snapshot()

	acc, rej := r.Retire(domain.RetireRole{At: later})

	tb.RequireRejected(t, acc, rej, domain.CodeRoleRetired)
	requireRoleUnchanged(t, before, r.Snapshot())
}
