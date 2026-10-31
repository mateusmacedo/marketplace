package domain_test

import (
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestChangePermissionsReplacesTheSetAndRaisesTheAccessVersion(t *testing.T) {
	r := newActiveRole(t)

	acc, rej := r.ChangePermissions(domain.ChangeRolePermissions{Permissions: []domain.Permission{"inventory:write", "catalog:write", "catalog:write"}, At: later})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.PermissionsChangedResponse{Ref: roleRef}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	changed := []domain.Permission{"catalog:write", "inventory:write"}
	want := domain.RolePermissionsChanged{Ref: roleRef, Permissions: changed, AccessVersion: 2, At: later}
	if events := acc.Events(); len(events) != 1 || !sameEvent(events[0], want) {
		t.Fatalf("Events() = %+v, want [%+v]", events, want)
	}
	wantState := domain.RoleSnapshot{
		Ref: roleRef, Name: roleName, Permissions: changed, Status: domain.RoleStatusActive,
		AccessVersion: 2, DefinedAt: at, ChangedAt: later,
	}
	if got := r.Snapshot(); !got.Equal(wantState) {
		t.Fatalf("Snapshot() = %+v, want %+v", got, wantState)
	}
}

func TestChangePermissionsToTheSameSetAcceptsWithoutChangingOrEmitting(t *testing.T) {
	r := newActiveRole(t)
	before := r.Snapshot()

	acc, rej := r.ChangePermissions(domain.ChangeRolePermissions{Permissions: []domain.Permission{"ordering:read", "fulfillment:write", "ordering:read"}, At: later})

	tb.RequireAccepted(t, rej)
	if len(acc.Events()) != 0 {
		t.Fatalf("Events() len = %d, want 0: the same set changes nothing", len(acc.Events()))
	}
	requireRoleUnchanged(t, before, r.Snapshot())
}

func TestChangePermissionsRejectsARetiredRole(t *testing.T) {
	r := newRetiredRole(t)
	before := r.Snapshot()

	acc, rej := r.ChangePermissions(domain.ChangeRolePermissions{Permissions: []domain.Permission{"catalog:write"}, At: later})

	tb.RequireRejected(t, acc, rej, domain.CodeRoleRetired)
	requireRoleUnchanged(t, before, r.Snapshot())
}

func TestChangePermissionsRejectsEmptyPermissions(t *testing.T) {
	r := newActiveRole(t)
	before := r.Snapshot()

	acc, rej := r.ChangePermissions(domain.ChangeRolePermissions{At: later})

	tb.RequireRejected(t, acc, rej, domain.CodeRolePermissionsEmpty)
	requireRoleUnchanged(t, before, r.Snapshot())
}

func TestChangePermissionsRejectsAPermissionOutsideTheCatalog(t *testing.T) {
	r := newActiveRole(t)
	before := r.Snapshot()

	acc, rej := r.ChangePermissions(domain.ChangeRolePermissions{Permissions: []domain.Permission{"tenancy:write"}, At: later})

	tb.RequireRejected(t, acc, rej, domain.CodeRolePermissionUnknown)
	requireRoleUnchanged(t, before, r.Snapshot())
}

func TestChangePermissionsJudgesTheStatusBeforeThePermissions(t *testing.T) {
	r := newRetiredRole(t)

	acc, rej := r.ChangePermissions(domain.ChangeRolePermissions{At: later})

	tb.RequireRejected(t, acc, rej, domain.CodeRoleRetired)
}
