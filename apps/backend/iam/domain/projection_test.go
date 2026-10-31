package domain_test

import (
	"strconv"
	"strings"
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/domainkit"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestRolesMatchTheProjectionFixture(t *testing.T) {
	f, err := tb.DecodeProjection([]byte(roleProjection))
	if err != nil {
		t.Fatalf("decode the role projection: %v", err)
	}
	if f.Identity.Context != "iam" || f.Identity.Aggregate != "role" || len(f.Cases) != 15 {
		t.Fatalf("fixture identity/cases = %+v/%d", f.Identity, len(f.Cases))
	}
	tb.RunProjection(t, f, runRole)
}

func runRole(t *testing.T, c tb.ProjectionCase) (domainkit.Projection, domainkit.Verdict) {
	t.Helper()
	when := kernel.Instant(integer(t, c.Command["at"]))
	before := func() *domain.Role { return roleFromState(t, c.StateBefore) }
	refOf := func(r domain.RoleRef) domainkit.Fields { return domainkit.Fields{"ref": string(r)} }
	switch c.Command["upr"] {
	case "define":
		s := roleSubject(func(r *domain.Role) (kernel.Accepted[domain.DefinedResponse], *kernel.Rejection) {
			return r.Define(domain.DefineRole{Ref: domain.RoleRef(c.Command["ref"]), Name: domain.RoleName(c.Command["name"]), Permissions: permissionsOf(c.Command["permissions"]), At: when})
		}, func(r domain.DefinedResponse) domainkit.Fields { return refOf(r.Ref) })
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	case "change-permissions":
		s := roleSubject(func(r *domain.Role) (kernel.Accepted[domain.PermissionsChangedResponse], *kernel.Rejection) {
			return r.ChangePermissions(domain.ChangeRolePermissions{Permissions: permissionsOf(c.Command["permissions"]), At: when})
		}, func(r domain.PermissionsChangedResponse) domainkit.Fields { return refOf(r.Ref) })
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	case "retire":
		s := roleSubject(func(r *domain.Role) (kernel.Accepted[domain.RetiredResponse], *kernel.Rejection) {
			return r.Retire(domain.RetireRole{At: when})
		}, func(r domain.RetiredResponse) domainkit.Fields { return refOf(r.Ref) })
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	case "confirm-synced":
		s := roleSubject(func(r *domain.Role) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
			return r.ConfirmSynced(domain.ConfirmRoleSynced{Version: integer(t, c.Command["version"])})
		}, func(kernel.Empty) domainkit.Fields { return domainkit.Fields{} })
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	default:
		t.Fatalf("unknown upr %q in case %s", c.Command["upr"], c.Name)
		return domainkit.Projection{}, domainkit.Verdict{}
	}
}

func integer(t *testing.T, s string) int64 {
	t.Helper()
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatalf("integer %q: %v", s, err)
	}
	return n
}

func permissionsOf(joined string) []domain.Permission {
	if joined == "" {
		return nil
	}
	var permissions []domain.Permission
	for _, p := range strings.Split(joined, ",") {
		permissions = append(permissions, domain.Permission(p))
	}
	return permissions
}

func joined(permissions []domain.Permission) string {
	parts := make([]string, 0, len(permissions))
	for _, p := range permissions {
		parts = append(parts, string(p))
	}
	return strings.Join(parts, ",")
}

var roleStatusNames = map[domain.RoleStatus]string{
	domain.RoleStatusUndefined: "undefined",
	domain.RoleStatusActive:    "active",
	domain.RoleStatusRetired:   "retired",
}

func roleState(r *domain.Role) domainkit.Fields {
	snap := r.Snapshot()
	return domainkit.Fields{
		"ref":            string(snap.Ref),
		"name":           string(snap.Name),
		"permissions":    joined(snap.Permissions),
		"status":         roleStatusNames[snap.Status],
		"access_version": strconv.FormatInt(snap.AccessVersion, 10),
		"synced_version": strconv.FormatInt(snap.SyncedVersion, 10),
		"defined_at":     strconv.FormatInt(int64(snap.DefinedAt), 10),
		"changed_at":     strconv.FormatInt(int64(snap.ChangedAt), 10),
	}
}

func roleFromState(t *testing.T, f domainkit.Fields) *domain.Role {
	t.Helper()
	snap := domain.RoleSnapshot{
		Ref:           domain.RoleRef(f["ref"]),
		Name:          domain.RoleName(f["name"]),
		Permissions:   permissionsOf(f["permissions"]),
		AccessVersion: integer(t, f["access_version"]),
		SyncedVersion: integer(t, f["synced_version"]),
		DefinedAt:     kernel.Instant(integer(t, f["defined_at"])),
		ChangedAt:     kernel.Instant(integer(t, f["changed_at"])),
	}
	for status, name := range roleStatusNames {
		if name == f["status"] {
			snap.Status = status
		}
	}
	return domain.FromRoleSnapshot(snap)
}

func roleEvent(e kernel.DomainEvent) (string, domainkit.Fields) {
	at := func(i kernel.Instant) string { return strconv.FormatInt(int64(i), 10) }
	version := func(v int64) string { return strconv.FormatInt(v, 10) }
	switch ev := e.(type) {
	case domain.RoleDefined:
		return ev.EventName(), domainkit.Fields{"ref": string(ev.Ref), "name": string(ev.Name), "permissions": joined(ev.Permissions), "access_version": version(ev.AccessVersion), "at": at(ev.At)}
	case domain.RolePermissionsChanged:
		return ev.EventName(), domainkit.Fields{"ref": string(ev.Ref), "permissions": joined(ev.Permissions), "access_version": version(ev.AccessVersion), "at": at(ev.At)}
	case domain.RoleRetired:
		return ev.EventName(), domainkit.Fields{"ref": string(ev.Ref), "access_version": version(ev.AccessVersion), "at": at(ev.At)}
	default:
		return e.EventName(), domainkit.Fields{}
	}
}

func roleSubject[R any](decide func(*domain.Role) (kernel.Accepted[R], *kernel.Rejection), response func(R) domainkit.Fields) domainkit.Subject[*domain.Role, R] {
	return domainkit.Subject[*domain.Role, R]{
		Decide:   decide,
		Response: response,
		Event:    roleEvent,
		Snapshot: roleState,
		Clone:    func(r *domain.Role) *domain.Role { return domain.FromRoleSnapshot(r.Snapshot()) },
	}
}

// roleProjection is the ORA-30 fixture of the role, kept in the test: the
// domain block may not read the disk, and tb.LoadProjection resolves the root
// from the kit's own path, which a consuming workspace keeps in the module cache.
const roleProjection = `{
  "format_version": "1",
  "identity": {"context": "iam", "aggregate": "role"},
  "cases": [
    {
      "name": "define-accepted",
      "doc": "Ramo Accepted de Define: ref desconhecida na loja nasce active, permissões ordenadas e sem repetição, versão de acesso 1; um role-defined.",
      "state_before": {"ref": "expedicao", "name": "", "permissions": "", "status": "undefined", "access_version": "0", "synced_version": "0", "defined_at": "0", "changed_at": "0"},
      "command": {"upr": "define", "ref": "expedicao", "name": "Expedição", "permissions": "ordering:read,fulfillment:write,ordering:read", "at": "1755432000000000000"},
      "expected": {
        "branch": "accepted",
        "response": {"ref": "expedicao"},
        "events": [
          {"name": "iam.role-defined", "fields": {"ref": "expedicao", "name": "Expedição", "permissions": "fulfillment:write,ordering:read", "access_version": "1", "at": "1755432000000000000"}}
        ],
        "state_after": {"ref": "expedicao", "name": "Expedição", "permissions": "fulfillment:write,ordering:read", "status": "active", "access_version": "1", "synced_version": "0", "defined_at": "1755432000000000000", "changed_at": "1755432000000000000"}
      }
    },
    {
      "name": "define-known-ref",
      "doc": "Ramo Accepted de Define sobre ref já definida: aceita sem alterar nome nem permissões e sem emitir.",
      "state_before": {"ref": "expedicao", "name": "Expedição", "permissions": "fulfillment:write,ordering:read", "status": "active", "access_version": "1", "synced_version": "0", "defined_at": "1755432000000000000", "changed_at": "1755432000000000000"},
      "command": {"upr": "define", "ref": "expedicao", "name": "Outro nome", "permissions": "catalog:write", "at": "1755435600000000000"},
      "expected": {
        "branch": "accepted",
        "response": {"ref": "expedicao"},
        "events": [],
        "state_after": {"ref": "expedicao", "name": "Expedição", "permissions": "fulfillment:write,ordering:read", "status": "active", "access_version": "1", "synced_version": "0", "defined_at": "1755432000000000000", "changed_at": "1755432000000000000"}
      }
    },
    {
      "name": "define-ref-invalid",
      "doc": "Ramo Rejected de Define: ref fora da forma de rótulo DNS; estado inalterado.",
      "state_before": {"ref": "Exp_1", "name": "", "permissions": "", "status": "undefined", "access_version": "0", "synced_version": "0", "defined_at": "0", "changed_at": "0"},
      "command": {"upr": "define", "ref": "Exp_1", "name": "Expedição", "permissions": "fulfillment:write", "at": "1755432000000000000"},
      "expected": {
        "branch": "rejected",
        "rejection": {"code": "iam/role/ref-invalid", "message": "ref must have 3 to 63 characters of a-z, 0-9 and -, without a leading or trailing hyphen", "details": {"ref": "Exp_1"}},
        "events": [],
        "state_after": {"ref": "Exp_1", "name": "", "permissions": "", "status": "undefined", "access_version": "0", "synced_version": "0", "defined_at": "0", "changed_at": "0"}
      }
    },
    {
      "name": "define-reserved",
      "doc": "Ramo Rejected de Define: owner é o papel reservado do dono; estado inalterado.",
      "state_before": {"ref": "owner", "name": "", "permissions": "", "status": "undefined", "access_version": "0", "synced_version": "0", "defined_at": "0", "changed_at": "0"},
      "command": {"upr": "define", "ref": "owner", "name": "Dono", "permissions": "fulfillment:write", "at": "1755432000000000000"},
      "expected": {
        "branch": "rejected",
        "rejection": {"code": "iam/role/reserved", "message": "owner is reserved for the owner of the store", "details": {}},
        "events": [],
        "state_after": {"ref": "owner", "name": "", "permissions": "", "status": "undefined", "access_version": "0", "synced_version": "0", "defined_at": "0", "changed_at": "0"}
      }
    },
    {
      "name": "define-name-empty",
      "doc": "Ramo Rejected de Define: name vazio; estado inalterado.",
      "state_before": {"ref": "expedicao", "name": "", "permissions": "", "status": "undefined", "access_version": "0", "synced_version": "0", "defined_at": "0", "changed_at": "0"},
      "command": {"upr": "define", "ref": "expedicao", "name": "", "permissions": "fulfillment:write", "at": "1755432000000000000"},
      "expected": {
        "branch": "rejected",
        "rejection": {"code": "iam/role/name-empty", "message": "name must not be empty", "details": {}},
        "events": [],
        "state_after": {"ref": "expedicao", "name": "", "permissions": "", "status": "undefined", "access_version": "0", "synced_version": "0", "defined_at": "0", "changed_at": "0"}
      }
    },
    {
      "name": "define-permissions-empty",
      "doc": "Ramo Rejected de Define: permissions vazio; estado inalterado.",
      "state_before": {"ref": "expedicao", "name": "", "permissions": "", "status": "undefined", "access_version": "0", "synced_version": "0", "defined_at": "0", "changed_at": "0"},
      "command": {"upr": "define", "ref": "expedicao", "name": "Expedição", "permissions": "", "at": "1755432000000000000"},
      "expected": {
        "branch": "rejected",
        "rejection": {"code": "iam/role/permissions-empty", "message": "permissions must not be empty", "details": {}},
        "events": [],
        "state_after": {"ref": "expedicao", "name": "", "permissions": "", "status": "undefined", "access_version": "0", "synced_version": "0", "defined_at": "0", "changed_at": "0"}
      }
    },
    {
      "name": "define-permission-unknown",
      "doc": "Ramo Rejected de Define: permissão fora do catálogo, nomeada no detalhe; estado inalterado.",
      "state_before": {"ref": "expedicao", "name": "", "permissions": "", "status": "undefined", "access_version": "0", "synced_version": "0", "defined_at": "0", "changed_at": "0"},
      "command": {"upr": "define", "ref": "expedicao", "name": "Expedição", "permissions": "fulfillment:write,billing:write", "at": "1755432000000000000"},
      "expected": {
        "branch": "rejected",
        "rejection": {"code": "iam/role/permission-unknown", "message": "permission is not in the catalog", "details": {"permission": "billing:write"}},
        "events": [],
        "state_after": {"ref": "expedicao", "name": "", "permissions": "", "status": "undefined", "access_version": "0", "synced_version": "0", "defined_at": "0", "changed_at": "0"}
      }
    },
    {
      "name": "change-permissions-accepted",
      "doc": "Ramo Accepted de ChangePermissions: outro conjunto substitui o atual e a versão de acesso sobe; um role-permissions-changed com ela.",
      "state_before": {"ref": "expedicao", "name": "Expedição", "permissions": "fulfillment:write,ordering:read", "status": "active", "access_version": "1", "synced_version": "1", "defined_at": "1755432000000000000", "changed_at": "1755432000000000000"},
      "command": {"upr": "change-permissions", "permissions": "inventory:write,catalog:write", "at": "1755435600000000000"},
      "expected": {
        "branch": "accepted",
        "response": {"ref": "expedicao"},
        "events": [
          {"name": "iam.role-permissions-changed", "fields": {"ref": "expedicao", "permissions": "catalog:write,inventory:write", "access_version": "2", "at": "1755435600000000000"}}
        ],
        "state_after": {"ref": "expedicao", "name": "Expedição", "permissions": "catalog:write,inventory:write", "status": "active", "access_version": "2", "synced_version": "1", "defined_at": "1755432000000000000", "changed_at": "1755435600000000000"}
      }
    },
    {
      "name": "change-permissions-same-set",
      "doc": "Ramo Accepted de ChangePermissions com o mesmo conjunto normalizado: aceita sem alterar e sem emitir.",
      "state_before": {"ref": "expedicao", "name": "Expedição", "permissions": "fulfillment:write,ordering:read", "status": "active", "access_version": "1", "synced_version": "1", "defined_at": "1755432000000000000", "changed_at": "1755432000000000000"},
      "command": {"upr": "change-permissions", "permissions": "ordering:read,fulfillment:write", "at": "1755435600000000000"},
      "expected": {
        "branch": "accepted",
        "response": {"ref": "expedicao"},
        "events": [],
        "state_after": {"ref": "expedicao", "name": "Expedição", "permissions": "fulfillment:write,ordering:read", "status": "active", "access_version": "1", "synced_version": "1", "defined_at": "1755432000000000000", "changed_at": "1755432000000000000"}
      }
    },
    {
      "name": "change-permissions-retired",
      "doc": "Ramo Rejected de ChangePermissions sobre papel retired; estado inalterado.",
      "state_before": {"ref": "expedicao", "name": "Expedição", "permissions": "fulfillment:write,ordering:read", "status": "retired", "access_version": "2", "synced_version": "2", "defined_at": "1755432000000000000", "changed_at": "1755432000000000000"},
      "command": {"upr": "change-permissions", "permissions": "catalog:write", "at": "1755435600000000000"},
      "expected": {
        "branch": "rejected",
        "rejection": {"code": "iam/role/retired", "message": "role is retired", "details": {}},
        "events": [],
        "state_after": {"ref": "expedicao", "name": "Expedição", "permissions": "fulfillment:write,ordering:read", "status": "retired", "access_version": "2", "synced_version": "2", "defined_at": "1755432000000000000", "changed_at": "1755432000000000000"}
      }
    },
    {
      "name": "retire-accepted",
      "doc": "Ramo Accepted de Retire: o papel active vai a retired, mantém as permissões e a versão de acesso sobe; um role-retired com ela.",
      "state_before": {"ref": "expedicao", "name": "Expedição", "permissions": "fulfillment:write,ordering:read", "status": "active", "access_version": "1", "synced_version": "1", "defined_at": "1755432000000000000", "changed_at": "1755432000000000000"},
      "command": {"upr": "retire", "at": "1755435600000000000"},
      "expected": {
        "branch": "accepted",
        "response": {"ref": "expedicao"},
        "events": [
          {"name": "iam.role-retired", "fields": {"ref": "expedicao", "access_version": "2", "at": "1755435600000000000"}}
        ],
        "state_after": {"ref": "expedicao", "name": "Expedição", "permissions": "fulfillment:write,ordering:read", "status": "retired", "access_version": "2", "synced_version": "1", "defined_at": "1755432000000000000", "changed_at": "1755435600000000000"}
      }
    },
    {
      "name": "retire-retired",
      "doc": "Ramo Rejected de Retire sobre papel retired; estado inalterado.",
      "state_before": {"ref": "expedicao", "name": "Expedição", "permissions": "fulfillment:write,ordering:read", "status": "retired", "access_version": "2", "synced_version": "2", "defined_at": "1755432000000000000", "changed_at": "1755432000000000000"},
      "command": {"upr": "retire", "at": "1755435600000000000"},
      "expected": {
        "branch": "rejected",
        "rejection": {"code": "iam/role/retired", "message": "role is retired", "details": {}},
        "events": [],
        "state_after": {"ref": "expedicao", "name": "Expedição", "permissions": "fulfillment:write,ordering:read", "status": "retired", "access_version": "2", "synced_version": "2", "defined_at": "1755432000000000000", "changed_at": "1755432000000000000"}
      }
    },
    {
      "name": "confirm-synced-accepted",
      "doc": "Ramo Accepted de ConfirmSynced: a versão confirmada sobe até a de acesso, sem evento.",
      "state_before": {"ref": "expedicao", "name": "Expedição", "permissions": "catalog:read", "status": "active", "access_version": "3", "synced_version": "1", "defined_at": "1755432000000000000", "changed_at": "1755435600000000000"},
      "command": {"upr": "confirm-synced", "version": "3"},
      "expected": {
        "branch": "accepted",
        "response": {},
        "events": [],
        "state_after": {"ref": "expedicao", "name": "Expedição", "permissions": "catalog:read", "status": "active", "access_version": "3", "synced_version": "3", "defined_at": "1755432000000000000", "changed_at": "1755435600000000000"}
      }
    },
    {
      "name": "confirm-synced-late",
      "doc": "Ramo Accepted de ConfirmSynced atrasado: a versão confirmada não volta; estado inalterado.",
      "state_before": {"ref": "expedicao", "name": "Expedição", "permissions": "catalog:read", "status": "active", "access_version": "3", "synced_version": "3", "defined_at": "1755432000000000000", "changed_at": "1755435600000000000"},
      "command": {"upr": "confirm-synced", "version": "2"},
      "expected": {
        "branch": "accepted",
        "response": {},
        "events": [],
        "state_after": {"ref": "expedicao", "name": "Expedição", "permissions": "catalog:read", "status": "active", "access_version": "3", "synced_version": "3", "defined_at": "1755432000000000000", "changed_at": "1755435600000000000"}
      }
    },
    {
      "name": "confirm-synced-version-ahead",
      "doc": "Ramo Rejected de ConfirmSynced com versão acima da de acesso; estado inalterado.",
      "state_before": {"ref": "expedicao", "name": "Expedição", "permissions": "catalog:read", "status": "active", "access_version": "2", "synced_version": "0", "defined_at": "1755432000000000000", "changed_at": "1755435600000000000"},
      "command": {"upr": "confirm-synced", "version": "3"},
      "expected": {
        "branch": "rejected",
        "rejection": {"code": "iam/role/version-ahead", "message": "version is ahead of the access version of the role", "details": {"version": "3", "accessVersion": "2"}},
        "events": [],
        "state_after": {"ref": "expedicao", "name": "Expedição", "permissions": "catalog:read", "status": "active", "access_version": "2", "synced_version": "0", "defined_at": "1755432000000000000", "changed_at": "1755435600000000000"}
      }
    }
  ]
}`
