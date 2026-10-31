package domain

import (
	"slices"
	"strconv"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
)

const (
	minRefLength = 3
	maxRefLength = 63
)

// OwnerRole is the reserved role of the owner of a store: it holds the whole
// catalog, names the group seeded with the Organization and is never defined,
// granted nor revoked by a command.
const OwnerRole RoleRef = "owner"

type Role struct {
	ref           RoleRef
	name          RoleName
	permissions   []Permission
	status        RoleStatus
	accessVersion int64
	syncedVersion int64
	definedAt     kernel.Instant
	changedAt     kernel.Instant
}

func NewRole(ref RoleRef) *Role {
	return &Role{ref: ref}
}

func FromRoleSnapshot(s RoleSnapshot) *Role {
	return &Role{
		ref:           s.Ref,
		name:          s.Name,
		permissions:   slices.Clone(s.Permissions),
		status:        s.Status,
		accessVersion: s.AccessVersion,
		syncedVersion: s.SyncedVersion,
		definedAt:     s.DefinedAt,
		changedAt:     s.ChangedAt,
	}
}

type RoleSnapshot struct {
	Ref           RoleRef
	Name          RoleName
	Permissions   []Permission
	Status        RoleStatus
	AccessVersion int64
	SyncedVersion int64
	DefinedAt     kernel.Instant
	ChangedAt     kernel.Instant
}

func (r *Role) Snapshot() RoleSnapshot {
	return RoleSnapshot{
		Ref:           r.ref,
		Name:          r.name,
		Permissions:   slices.Clone(r.permissions),
		Status:        r.status,
		AccessVersion: r.accessVersion,
		SyncedVersion: r.syncedVersion,
		DefinedAt:     r.definedAt,
		ChangedAt:     r.changedAt,
	}
}

func (s RoleSnapshot) Equal(other RoleSnapshot) bool {
	return s.Ref == other.Ref && s.Name == other.Name && slices.Equal(s.Permissions, other.Permissions) &&
		s.Status == other.Status && s.AccessVersion == other.AccessVersion && s.SyncedVersion == other.SyncedVersion &&
		s.DefinedAt == other.DefinedAt && s.ChangedAt == other.ChangedAt
}

func (r *Role) clone() Role {
	next := *r
	next.permissions = slices.Clone(r.permissions)
	return next
}

func (r *Role) Define(cmd DefineRole) (kernel.Accepted[DefinedResponse], *kernel.Rejection) {
	return kernel.DecideOver(r, (*Role).clone, func(next *Role) (kernel.Accepted[DefinedResponse], *kernel.Rejection) {
		if !ValidRef(cmd.Ref) {
			return kernel.Refuse[DefinedResponse](CodeRoleRefInvalid,
				"ref must have 3 to 63 characters of a-z, 0-9 and -, without a leading or trailing hyphen",
				kernel.Detail{Key: "ref", Value: string(cmd.Ref)},
			)
		}
		if cmd.Ref == OwnerRole {
			return kernel.Refuse[DefinedResponse](CodeRoleReserved, "owner is reserved for the owner of the store")
		}
		if cmd.Name == "" {
			return kernel.Refuse[DefinedResponse](CodeRoleNameEmpty, "name must not be empty")
		}
		if rejection := refusePermissions(cmd.Permissions); rejection != nil {
			return kernel.Accepted[DefinedResponse]{}, rejection
		}
		if next.status != RoleStatusUndefined {
			return kernel.Accept(DefinedResponse{Ref: next.ref}), nil
		}
		next.name = cmd.Name
		next.permissions = normalized(cmd.Permissions)
		next.status = RoleStatusActive
		next.accessVersion = 1
		next.syncedVersion = 0
		next.definedAt = cmd.At
		next.changedAt = cmd.At
		return kernel.Accept(
			DefinedResponse{Ref: next.ref},
			RoleDefined{Ref: next.ref, Name: next.name, Permissions: slices.Clone(next.permissions), AccessVersion: next.accessVersion, At: cmd.At},
		), nil
	})
}

func (r *Role) ChangePermissions(cmd ChangeRolePermissions) (kernel.Accepted[PermissionsChangedResponse], *kernel.Rejection) {
	return kernel.DecideOver(r, (*Role).clone, func(next *Role) (kernel.Accepted[PermissionsChangedResponse], *kernel.Rejection) {
		if next.status != RoleStatusActive {
			return kernel.Refuse[PermissionsChangedResponse](CodeRoleRetired, "role is retired")
		}
		if rejection := refusePermissions(cmd.Permissions); rejection != nil {
			return kernel.Accepted[PermissionsChangedResponse]{}, rejection
		}
		permissions := normalized(cmd.Permissions)
		if slices.Equal(permissions, next.permissions) {
			return kernel.Accept(PermissionsChangedResponse{Ref: next.ref}), nil
		}
		next.permissions = permissions
		next.accessVersion++
		next.changedAt = cmd.At
		return kernel.Accept(
			PermissionsChangedResponse{Ref: next.ref},
			RolePermissionsChanged{Ref: next.ref, Permissions: slices.Clone(next.permissions), AccessVersion: next.accessVersion, At: cmd.At},
		), nil
	})
}

func (r *Role) Retire(cmd RetireRole) (kernel.Accepted[RetiredResponse], *kernel.Rejection) {
	return kernel.DecideOver(r, (*Role).clone, func(next *Role) (kernel.Accepted[RetiredResponse], *kernel.Rejection) {
		if next.status != RoleStatusActive {
			return kernel.Refuse[RetiredResponse](CodeRoleRetired, "role is retired")
		}
		next.status = RoleStatusRetired
		next.accessVersion++
		next.changedAt = cmd.At
		return kernel.Accept(
			RetiredResponse{Ref: next.ref},
			RoleRetired{Ref: next.ref, AccessVersion: next.accessVersion, At: cmd.At},
		), nil
	})
}

// ConfirmSynced keeps the highest version confirmed, so a confirmation that
// arrives late never undoes a newer one.
func (r *Role) ConfirmSynced(cmd ConfirmRoleSynced) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
	return kernel.DecideOver(r, (*Role).clone, func(next *Role) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
		if cmd.Version > next.accessVersion {
			return kernel.Refuse[kernel.Empty](CodeRoleVersionAhead, "version is ahead of the access version of the role",
				kernel.Detail{Key: "version", Value: strconv.FormatInt(cmd.Version, 10)},
				kernel.Detail{Key: "accessVersion", Value: strconv.FormatInt(next.accessVersion, 10)},
			)
		}
		next.syncedVersion = max(next.syncedVersion, cmd.Version)
		return kernel.Accept(kernel.Empty{}), nil
	})
}

func refusePermissions(permissions []Permission) *kernel.Rejection {
	if len(permissions) == 0 {
		return kernel.Reject(CodeRolePermissionsEmpty, "permissions must not be empty")
	}
	for _, permission := range permissions {
		if !inCatalog(permission) {
			return kernel.Reject(CodeRolePermissionUnknown, "permission is not in the catalog",
				kernel.Detail{Key: "permission", Value: string(permission)})
		}
	}
	return nil
}

// ValidRef keeps the ref a DNS label, the form the name of its group in the
// Organization takes.
func ValidRef(ref RoleRef) bool {
	if len(ref) < minRefLength || len(ref) > maxRefLength || ref[0] == '-' || ref[len(ref)-1] == '-' {
		return false
	}
	for _, c := range []byte(ref) {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}
