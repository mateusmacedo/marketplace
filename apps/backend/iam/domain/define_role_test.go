package domain_test

import (
	"strings"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestDefineAcceptsAnUnknownRefAsAnActiveRoleWithItsPermissionsNormalized(t *testing.T) {
	r := domain.NewRole(roleRef)

	acc, rej := r.Define(domain.DefineRole{
		Ref: roleRef, Name: roleName, At: at,
		Permissions: []domain.Permission{"ordering:read", "fulfillment:write", "ordering:read"},
	})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.DefinedResponse{Ref: roleRef}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	events := acc.Events()
	want := domain.RoleDefined{Ref: roleRef, Name: roleName, Permissions: rolePermissions, AccessVersion: 1, At: at}
	if len(events) != 1 || !sameEvent(events[0], want) {
		t.Fatalf("Events() = %+v, want [%+v]", events, want)
	}
	wantState := domain.RoleSnapshot{
		Ref: roleRef, Name: roleName, Permissions: rolePermissions, Status: domain.RoleStatusActive,
		AccessVersion: 1, SyncedVersion: 0, DefinedAt: at, ChangedAt: at,
	}
	if got := r.Snapshot(); !got.Equal(wantState) {
		t.Fatalf("Snapshot() = %+v, want %+v", got, wantState)
	}
}

func TestDefineOfAKnownRefAcceptsWithoutChangingOrEmitting(t *testing.T) {
	for name, known := range map[string]func(*testing.T) *domain.Role{"active": newActiveRole, "retired": newRetiredRole} {
		t.Run(name, func(t *testing.T) {
			r := known(t)
			before := r.Snapshot()

			acc, rej := r.Define(domain.DefineRole{Ref: roleRef, Name: "Outro nome", Permissions: []domain.Permission{"catalog:write"}, At: later})

			tb.RequireAccepted(t, rej)
			if got, want := acc.Response(), (domain.DefinedResponse{Ref: roleRef}); got != want {
				t.Fatalf("Response() = %+v, want %+v", got, want)
			}
			if len(acc.Events()) != 0 {
				t.Fatalf("Events() len = %d, want 0: a known ref defines nothing again", len(acc.Events()))
			}
			requireRoleUnchanged(t, before, r.Snapshot())
		})
	}
}

func TestDefineRejectsARefOutsideTheLabelForm(t *testing.T) {
	for _, ref := range []string{"Exp_1", "ab", "-exp", "exp-", "exp edicao", "", strings.Repeat("a", 64)} {
		t.Run(ref, func(t *testing.T) {
			r := domain.NewRole(domain.RoleRef(ref))
			before := r.Snapshot()

			acc, rej := r.Define(domain.DefineRole{Ref: domain.RoleRef(ref), Name: roleName, Permissions: rolePermissions, At: at})

			tb.RequireRejected(t, acc, rej, domain.CodeRoleRefInvalid)
			requireRoleUnchanged(t, before, r.Snapshot())
		})
	}
}

func TestDefineAcceptsARefAtTheBoundsOfTheLabelForm(t *testing.T) {
	for _, ref := range []string{"abc", "a-1", "0-papel-9", strings.Repeat("z", 63)} {
		t.Run(ref, func(t *testing.T) {
			_, rej := domain.NewRole(domain.RoleRef(ref)).Define(domain.DefineRole{Ref: domain.RoleRef(ref), Name: roleName, Permissions: rolePermissions, At: at})

			tb.RequireAccepted(t, rej)
		})
	}
}

func TestDefineRejectsTheReservedOwnerRef(t *testing.T) {
	r := domain.NewRole(domain.OwnerRole)
	before := r.Snapshot()

	acc, rej := r.Define(domain.DefineRole{Ref: domain.OwnerRole, Name: "Dono", Permissions: rolePermissions, At: at})

	tb.RequireRejected(t, acc, rej, domain.CodeRoleReserved)
	requireRoleUnchanged(t, before, r.Snapshot())
}

func TestDefineRejectsAnEmptyName(t *testing.T) {
	r := domain.NewRole(roleRef)

	acc, rej := r.Define(domain.DefineRole{Ref: roleRef, Permissions: rolePermissions, At: at})

	tb.RequireRejected(t, acc, rej, domain.CodeRoleNameEmpty)
}

func TestDefineRejectsEmptyPermissions(t *testing.T) {
	for name, permissions := range map[string][]domain.Permission{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			r := domain.NewRole(roleRef)

			acc, rej := r.Define(domain.DefineRole{Ref: roleRef, Name: roleName, Permissions: permissions, At: at})

			tb.RequireRejected(t, acc, rej, domain.CodeRolePermissionsEmpty)
		})
	}
}

func TestDefineRejectsAPermissionOutsideTheCatalogNamingIt(t *testing.T) {
	r := domain.NewRole(roleRef)
	before := r.Snapshot()

	acc, rej := r.Define(domain.DefineRole{Ref: roleRef, Name: roleName, Permissions: []domain.Permission{"catalog:write", "billing:write"}, At: at})

	tb.RequireRejected(t, acc, rej, domain.CodeRolePermissionUnknown)
	if details := rej.Details(); len(details) != 1 || details[0].Key != "permission" || details[0].Value != "billing:write" {
		t.Fatalf("Details() = %+v, want the unknown permission named", details)
	}
	requireRoleUnchanged(t, before, r.Snapshot())
}

// The pre-conditions belong to the command (ADR-032): a known ref still
// refuses a malformed command instead of accepting it as a no-op.
func TestDefineJudgesTheCommandBeforeTheKnownRef(t *testing.T) {
	r := newActiveRole(t)

	acc, rej := r.Define(domain.DefineRole{Ref: roleRef, Name: roleName, Permissions: []domain.Permission{"billing:write"}, At: later})

	tb.RequireRejected(t, acc, rej, domain.CodeRolePermissionUnknown)
}

func TestTheDuplicateNameRejectionCarriesItsCode(t *testing.T) {
	if got := domain.RejectRoleDuplicateName.Code(); got != domain.CodeRoleDuplicateName {
		t.Fatalf("Code() = %q, want %q", got, domain.CodeRoleDuplicateName)
	}
}

func TestValidRefIsTheRuleDefineApplies(t *testing.T) {
	for ref, want := range map[domain.RoleRef]bool{"expedicao": true, "abc": true, "owner": true, "ab": false, "Exp_1": false, "-exp": false, "exp-": false} {
		if got := domain.ValidRef(ref); got != want {
			t.Errorf("ValidRef(%q) = %v, want %v", ref, got, want)
		}
	}
}
