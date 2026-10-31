package domain_test

import (
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

// roleAtVersion is a role whose permissions changed until accessVersion
// reached version, with nothing confirmed yet.
func roleAtVersion(t *testing.T, version int64) *domain.Role {
	t.Helper()
	r := newActiveRole(t)
	sets := [][]domain.Permission{{"catalog:read"}, {"catalog:write"}}
	for i := int64(1); i < version; i++ {
		_, rej := r.ChangePermissions(domain.ChangeRolePermissions{Permissions: sets[i%2], At: later})
		tb.RequireAccepted(t, rej)
	}
	return r
}

func TestConfirmSyncedRecordsTheConfirmedVersionWithoutEmitting(t *testing.T) {
	r := roleAtVersion(t, 3)

	acc, rej := r.ConfirmSynced(domain.ConfirmRoleSynced{Version: 3})

	tb.RequireAccepted(t, rej)
	if len(acc.Events()) != 0 {
		t.Fatalf("Events() len = %d, want 0", len(acc.Events()))
	}
	if got := r.Snapshot(); got.SyncedVersion != 3 || got.AccessVersion != 3 {
		t.Fatalf("Snapshot() = %+v, want synced at 3 of 3", got)
	}
}

func TestALateConfirmationNeverMovesTheSyncedVersionBack(t *testing.T) {
	r := roleAtVersion(t, 3)
	if _, rej := r.ConfirmSynced(domain.ConfirmRoleSynced{Version: 3}); rej != nil {
		t.Fatalf("setup: ConfirmSynced(3) rejected: %v", rej)
	}
	before := r.Snapshot()

	acc, rej := r.ConfirmSynced(domain.ConfirmRoleSynced{Version: 2})

	tb.RequireAccepted(t, rej)
	if len(acc.Events()) != 0 {
		t.Fatalf("Events() len = %d, want 0", len(acc.Events()))
	}
	requireRoleUnchanged(t, before, r.Snapshot())
}

func TestConfirmSyncedAppliesToARetiredRole(t *testing.T) {
	r := newRetiredRole(t)

	_, rej := r.ConfirmSynced(domain.ConfirmRoleSynced{Version: 2})

	tb.RequireAccepted(t, rej)
	if got := r.Snapshot(); got.Status != domain.RoleStatusRetired || got.SyncedVersion != 2 {
		t.Fatalf("Snapshot() = %+v, want retired and synced at 2", got)
	}
}

func TestConfirmSyncedRejectsAVersionAheadOfTheAccessVersion(t *testing.T) {
	r := roleAtVersion(t, 2)
	before := r.Snapshot()

	acc, rej := r.ConfirmSynced(domain.ConfirmRoleSynced{Version: 3})

	tb.RequireRejected(t, acc, rej, domain.CodeRoleVersionAhead)
	requireRoleUnchanged(t, before, r.Snapshot())
}
