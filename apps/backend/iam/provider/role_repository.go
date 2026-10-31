package provider

import (
	"encoding/json"
	"errors"
	"fmt"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

var errUnstoredRoleStatus = errors.New("provider: role status has no stored value")

// The status column holds these values, fixed here so a rename in the domain
// does not rewrite stored rows nor the roles_status_check of the schema.
const (
	roleStatusActive  = "active"
	roleStatusRetired = "retired"
)

// roleState is the snapshot column: the fields no query filters by, under
// JSON names fixed here for the same reason.
type roleState struct {
	Name          string   `json:"name"`
	Permissions   []string `json:"permissions"`
	AccessVersion int64    `json:"accessVersion"`
	SyncedVersion int64    `json:"syncedVersion"`
	DefinedAt     int64    `json:"definedAt"`
	ChangedAt     int64    `json:"changedAt"`
}

// roleTable declares how the Role snapshot maps to columns. The statements,
// and with them the tenant predicate, are the kernel's (IDN-14).
var roleTable = postgres.Table[domain.RoleRef, domain.RoleSnapshot]{
	Name:     "roles",
	IDColumn: "role_id",
	Columns:  []string{"status", "snapshot"},

	Encode: func(s domain.RoleSnapshot) ([]any, error) {
		status, err := roleStatusValue(s.Status)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(roleState{
			Name:          string(s.Name),
			Permissions:   names(s.Permissions),
			AccessVersion: s.AccessVersion,
			SyncedVersion: s.SyncedVersion,
			DefinedAt:     int64(s.DefinedAt),
			ChangedAt:     int64(s.ChangedAt),
		})
		if err != nil {
			return nil, err
		}
		return []any{status, raw}, nil
	},

	Decode: func(scan func(dest ...any) error) (domain.RoleSnapshot, error) {
		var (
			status string
			raw    []byte
		)
		if err := scan(&status, &raw); err != nil {
			return domain.RoleSnapshot{}, err
		}
		parsed, err := roleStatusOf(status)
		if err != nil {
			return domain.RoleSnapshot{}, err
		}
		var state roleState
		if err := json.Unmarshal(raw, &state); err != nil {
			return domain.RoleSnapshot{}, err
		}
		return domain.RoleSnapshot{
			Name:          domain.RoleName(state.Name),
			Permissions:   valuesOf[domain.Permission](state.Permissions),
			Status:        parsed,
			AccessVersion: state.AccessVersion,
			SyncedVersion: state.SyncedVersion,
			DefinedAt:     kernel.Instant(state.DefinedAt),
			ChangedAt:     kernel.Instant(state.ChangedAt),
		}, nil
	},

	WithID: func(s domain.RoleSnapshot, ref domain.RoleRef) domain.RoleSnapshot {
		s.Ref = ref
		return s
	},

	Unique: []postgres.Unique{{
		Name:      "roles_tenant_id_name_key",
		Columns:   []string{"tenant_id", "name"},
		Rejection: domain.RejectRoleDuplicateName,
	}},
}

func NewRoleRepository(tx *postgres.Tx) ports.Repository[domain.RoleRef, domain.RoleSnapshot] {
	return roleTable.Repository(tx)
}

func roleStatusValue(status domain.RoleStatus) (string, error) {
	switch status {
	case domain.RoleStatusActive:
		return roleStatusActive, nil
	case domain.RoleStatusRetired:
		return roleStatusRetired, nil
	default:
		return "", fmt.Errorf("%w: %s", errUnstoredRoleStatus, status)
	}
}

func roleStatusOf(value string) (domain.RoleStatus, error) {
	switch value {
	case roleStatusActive:
		return domain.RoleStatusActive, nil
	case roleStatusRetired:
		return domain.RoleStatusRetired, nil
	default:
		return domain.RoleStatusUndefined, fmt.Errorf("%w: %q", errUnstoredRoleStatus, value)
	}
}
