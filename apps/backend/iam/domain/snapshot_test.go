package domain_test

import (
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestFromRoleSnapshotRoundTripsTheWholeState(t *testing.T) {
	r := newActiveRole(t)
	if _, rej := r.ChangePermissions(domain.ChangeRolePermissions{Permissions: []domain.Permission{"catalog:write"}, At: later}); rej != nil {
		t.Fatalf("setup: ChangePermissions rejected: %v", rej)
	}
	if _, rej := r.ConfirmSynced(domain.ConfirmRoleSynced{Version: 1}); rej != nil {
		t.Fatalf("setup: ConfirmSynced rejected: %v", rej)
	}
	before := r.Snapshot()

	got := domain.FromRoleSnapshot(before).Snapshot()

	if !got.Equal(before) || got.DefinedAt != at || got.ChangedAt != later || got.SyncedVersion != 1 || got.AccessVersion != 2 {
		t.Fatalf("round trip lost state\nbefore: %+v\nafter:  %+v", before, got)
	}
}

func TestFromRoleSnapshotRestoresTheStatusAsBehaviour(t *testing.T) {
	reconstituted := domain.FromRoleSnapshot(newRetiredRole(t).Snapshot())

	acc, rej := reconstituted.Retire(domain.RetireRole{At: later})

	tb.RequireRejected(t, acc, rej, domain.CodeRoleRetired)
}

// DEC-12: the snapshot and the reconstitution copy the permissions, so no
// caller holding one slice reaches the state of the aggregate.
func TestThePermissionsOfTheSnapshotAreACopy(t *testing.T) {
	r := newActiveRole(t)
	snapshot := r.Snapshot()
	snapshot.Permissions[0] = "billing:write"
	if got := r.Snapshot().Permissions[0]; got != "fulfillment:write" {
		t.Fatalf("Snapshot() shares its slice: the aggregate now holds %q", got)
	}

	source := newActiveRole(t).Snapshot()
	reconstituted := domain.FromRoleSnapshot(source)
	source.Permissions[0] = "billing:write"
	if got := reconstituted.Snapshot().Permissions[0]; got != "fulfillment:write" {
		t.Fatalf("FromRoleSnapshot() shares its slice: the aggregate now holds %q", got)
	}
}

func TestEqualTellsEveryFieldApart(t *testing.T) {
	base := newActiveRole(t).Snapshot()
	for name, mutate := range map[string]func(*domain.RoleSnapshot){
		"ref":           func(s *domain.RoleSnapshot) { s.Ref = "outro" },
		"name":          func(s *domain.RoleSnapshot) { s.Name = "Outro nome" },
		"permissions":   func(s *domain.RoleSnapshot) { s.Permissions = []domain.Permission{"catalog:read"} },
		"status":        func(s *domain.RoleSnapshot) { s.Status = domain.RoleStatusRetired },
		"accessVersion": func(s *domain.RoleSnapshot) { s.AccessVersion++ },
		"syncedVersion": func(s *domain.RoleSnapshot) { s.SyncedVersion++ },
		"definedAt":     func(s *domain.RoleSnapshot) { s.DefinedAt++ },
		"changedAt":     func(s *domain.RoleSnapshot) { s.ChangedAt++ },
	} {
		t.Run(name, func(t *testing.T) {
			other := base
			other.Permissions = append([]domain.Permission(nil), base.Permissions...)
			mutate(&other)
			if base.Equal(other) {
				t.Fatalf("Equal() ignores %s", name)
			}
		})
	}
}

func TestFromInvitationSnapshotRoundTripsTheWholeState(t *testing.T) {
	for name, settled := range map[string]func(*testing.T) *domain.Invitation{"provisioned": newProvisionedInvitation, "failed": newFailedInvitation} {
		before := settled(t).Snapshot()

		got := domain.FromInvitationSnapshot(before).Snapshot()

		if !got.Equal(before) {
			t.Fatalf("%s: round trip lost state\nbefore: %+v\nafter:  %+v", name, before, got)
		}
	}
}

func TestFromMembershipSnapshotRoundTripsTheWholeState(t *testing.T) {
	m := newMember(t, "expedicao", "catalogo")
	if _, rej := m.ConfirmAccessSynced(domain.ConfirmMemberAccessSynced{Version: 1}); rej != nil {
		t.Fatalf("setup: ConfirmAccessSynced rejected: %v", rej)
	}
	before := m.Snapshot()

	got := domain.FromMembershipSnapshot(before).Snapshot()

	if !got.Equal(before) || got.SyncedVersion != 1 {
		t.Fatalf("round trip lost state\nbefore: %+v\nafter:  %+v", before, got)
	}
}

// DEC-12: the roles of the snapshots are copies, so no caller holding one
// slice reaches the state of the aggregate.
func TestTheRolesOfTheSnapshotsAreCopies(t *testing.T) {
	i := newPendingInvitation(t)
	i.Snapshot().Roles[0] = "intruso"
	if got := i.Snapshot().Roles[0]; got != "expedicao" {
		t.Fatalf("Invitation.Snapshot() shares its slice: the aggregate now holds %q", got)
	}

	source := newMember(t, "expedicao").Snapshot()
	m := domain.FromMembershipSnapshot(source)
	source.Roles[0] = "intruso"
	if got := m.Snapshot().Roles[0]; got != "expedicao" {
		t.Fatalf("FromMembershipSnapshot() shares its slice: the aggregate now holds %q", got)
	}
}

func TestInvitationEqualTellsEveryFieldApart(t *testing.T) {
	base := newProvisionedInvitation(t).Snapshot()
	for name, mutate := range map[string]func(*domain.InvitationSnapshot){
		"email":         func(s *domain.InvitationSnapshot) { s.Email = "bia@loja.com" },
		"roles":         func(s *domain.InvitationSnapshot) { s.Roles = []domain.RoleRef{"catalogo"} },
		"status":        func(s *domain.InvitationSnapshot) { s.Status = domain.InvitationStatusFailed },
		"round":         func(s *domain.InvitationSnapshot) { s.Round++ },
		"subject":       func(s *domain.InvitationSnapshot) { s.Subject = "kc-outro" },
		"failureReason": func(s *domain.InvitationSnapshot) { s.FailureReason = "motivo" },
		"invitedAt":     func(s *domain.InvitationSnapshot) { s.InvitedAt++ },
		"settledAt":     func(s *domain.InvitationSnapshot) { s.SettledAt++ },
	} {
		other := base
		other.Roles = append([]domain.RoleRef(nil), base.Roles...)
		mutate(&other)
		if base.Equal(other) {
			t.Errorf("Equal() ignores %s", name)
		}
	}
}

func TestMembershipEqualTellsEveryFieldApart(t *testing.T) {
	base := newMember(t, "expedicao").Snapshot()
	for name, mutate := range map[string]func(*domain.MembershipSnapshot){
		"subject":       func(s *domain.MembershipSnapshot) { s.Subject = "kc-outro" },
		"email":         func(s *domain.MembershipSnapshot) { s.Email = "bia@loja.com" },
		"roles":         func(s *domain.MembershipSnapshot) { s.Roles = []domain.RoleRef{"catalogo"} },
		"status":        func(s *domain.MembershipSnapshot) { s.Status = domain.MembershipStatusRemoved },
		"accessVersion": func(s *domain.MembershipSnapshot) { s.AccessVersion++ },
		"syncedVersion": func(s *domain.MembershipSnapshot) { s.SyncedVersion++ },
		"joinedAt":      func(s *domain.MembershipSnapshot) { s.JoinedAt++ },
		"changedAt":     func(s *domain.MembershipSnapshot) { s.ChangedAt++ },
	} {
		other := base
		other.Roles = append([]domain.RoleRef(nil), base.Roles...)
		mutate(&other)
		if base.Equal(other) {
			t.Errorf("Equal() ignores %s", name)
		}
	}
}
