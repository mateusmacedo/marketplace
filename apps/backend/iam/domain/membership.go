package domain

import (
	"slices"
	"strconv"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
)

type Membership struct {
	subject       Subject
	email         Email
	roles         []RoleRef
	status        MembershipStatus
	accessVersion int64
	syncedVersion int64
	joinedAt      kernel.Instant
	changedAt     kernel.Instant
}

func NewMembership(subject Subject) *Membership {
	return &Membership{subject: subject}
}

func FromMembershipSnapshot(s MembershipSnapshot) *Membership {
	return &Membership{
		subject:       s.Subject,
		email:         s.Email,
		roles:         slices.Clone(s.Roles),
		status:        s.Status,
		accessVersion: s.AccessVersion,
		syncedVersion: s.SyncedVersion,
		joinedAt:      s.JoinedAt,
		changedAt:     s.ChangedAt,
	}
}

type MembershipSnapshot struct {
	Subject       Subject
	Email         Email
	Roles         []RoleRef
	Status        MembershipStatus
	AccessVersion int64
	SyncedVersion int64
	JoinedAt      kernel.Instant
	ChangedAt     kernel.Instant
}

func (m *Membership) Snapshot() MembershipSnapshot {
	return MembershipSnapshot{
		Subject:       m.subject,
		Email:         m.email,
		Roles:         slices.Clone(m.roles),
		Status:        m.status,
		AccessVersion: m.accessVersion,
		SyncedVersion: m.syncedVersion,
		JoinedAt:      m.joinedAt,
		ChangedAt:     m.changedAt,
	}
}

func (s MembershipSnapshot) Equal(other MembershipSnapshot) bool {
	return s.Subject == other.Subject && s.Email == other.Email && slices.Equal(s.Roles, other.Roles) &&
		s.Status == other.Status && s.AccessVersion == other.AccessVersion && s.SyncedVersion == other.SyncedVersion &&
		s.JoinedAt == other.JoinedAt && s.ChangedAt == other.ChangedAt
}

// Owns is what the one owner per store counts (ADR-066): an active membership
// holding owner. No decision reads it; the unique constraint does.
func (s MembershipSnapshot) Owns() bool {
	return s.Status == MembershipStatusActive && slices.Contains(s.Roles, OwnerRole)
}

func (m *Membership) clone() Membership {
	next := *m
	next.roles = slices.Clone(m.roles)
	return next
}

// Establish never checks the owner of the store: the unique constraint on the
// active owner refuses a second one when the decision is saved (ADR-066).
func (m *Membership) Establish(cmd EstablishMembership) (kernel.Accepted[EstablishedResponse], *kernel.Rejection) {
	return kernel.DecideOver(m, (*Membership).clone, func(next *Membership) (kernel.Accepted[EstablishedResponse], *kernel.Rejection) {
		if !ValidSubject(cmd.Subject) {
			return kernel.Refuse[EstablishedResponse](CodeMembershipSubjectInvalid, SubjectInvalidMessage)
		}
		if len(cmd.Roles) == 0 {
			return kernel.Refuse[EstablishedResponse](CodeMembershipLastRole, "roles must not be empty")
		}
		for _, role := range cmd.Roles {
			if !ValidRef(role) {
				return kernel.Refuse[EstablishedResponse](CodeMembershipRoleInvalid, roleInvalidMessage, kernel.Detail{Key: "role", Value: string(role)})
			}
		}
		response := EstablishedResponse{Subject: next.subject}
		switch next.status {
		case MembershipStatusActive:
			if next.email == "" {
				next.email = cmd.Email
			}
			roles := union(next.roles, cmd.Roles)
			if slices.Equal(roles, next.roles) {
				return kernel.Accept(response), nil
			}
			next.roles = roles
			next.accessVersion++
			next.changedAt = cmd.At
			return kernel.Accept(response, next.accessChanged(cmd.At)), nil
		case MembershipStatusRemoved:
			if cmd.Email != "" {
				next.email = cmd.Email
			}
			next.accessVersion++
		default:
			next.email = cmd.Email
			next.accessVersion = 1
			next.syncedVersion = 0
			next.joinedAt = cmd.At
		}
		next.roles = normalized(cmd.Roles)
		next.status = MembershipStatusActive
		next.changedAt = cmd.At
		return kernel.Accept(response, MembershipEstablished{
			Subject: next.subject, Email: next.email, Roles: slices.Clone(next.roles), AccessVersion: next.accessVersion, At: cmd.At,
		}), nil
	})
}

func (m *Membership) Grant(cmd GrantRole) (kernel.Accepted[GrantedResponse], *kernel.Rejection) {
	return kernel.DecideOver(m, (*Membership).clone, func(next *Membership) (kernel.Accepted[GrantedResponse], *kernel.Rejection) {
		if next.status != MembershipStatusActive {
			return kernel.Refuse[GrantedResponse](CodeMembershipRemoved, "member is removed")
		}
		if !ValidRef(cmd.Role) {
			return kernel.Refuse[GrantedResponse](CodeMembershipRoleInvalid, roleInvalidMessage, kernel.Detail{Key: "role", Value: string(cmd.Role)})
		}
		if cmd.Role == OwnerRole {
			return kernel.Refuse[GrantedResponse](CodeMembershipOwnerReserved, "owner is reserved for the owner of the store")
		}
		response := GrantedResponse{Subject: next.subject}
		if slices.Contains(next.roles, cmd.Role) {
			return kernel.Accept(response), nil
		}
		next.roles = union(next.roles, []RoleRef{cmd.Role})
		next.accessVersion++
		next.changedAt = cmd.At
		return kernel.Accept(response, next.accessChanged(cmd.At)), nil
	})
}

func (m *Membership) Revoke(cmd RevokeRole) (kernel.Accepted[RevokedResponse], *kernel.Rejection) {
	return kernel.DecideOver(m, (*Membership).clone, func(next *Membership) (kernel.Accepted[RevokedResponse], *kernel.Rejection) {
		if next.status != MembershipStatusActive {
			return kernel.Refuse[RevokedResponse](CodeMembershipRemoved, "member is removed")
		}
		if cmd.Role == OwnerRole {
			return kernel.Refuse[RevokedResponse](CodeMembershipOwnerProtected, "the owner of the store keeps the owner role")
		}
		if !slices.Contains(next.roles, cmd.Role) {
			return kernel.Refuse[RevokedResponse](CodeMembershipRoleNotHeld, "member does not hold the role", kernel.Detail{Key: "role", Value: string(cmd.Role)})
		}
		roles := without(next.roles, cmd.Role)
		if len(roles) == 0 {
			return kernel.Refuse[RevokedResponse](CodeMembershipLastRole, "the member would be left without a role")
		}
		next.roles = roles
		next.accessVersion++
		next.changedAt = cmd.At
		return kernel.Accept(RevokedResponse{Subject: next.subject}, next.accessChanged(cmd.At)), nil
	})
}

func (m *Membership) Remove(cmd RemoveMember) (kernel.Accepted[RemovedResponse], *kernel.Rejection) {
	return kernel.DecideOver(m, (*Membership).clone, func(next *Membership) (kernel.Accepted[RemovedResponse], *kernel.Rejection) {
		if next.status != MembershipStatusActive {
			return kernel.Refuse[RemovedResponse](CodeMembershipRemoved, "member is removed")
		}
		if slices.Contains(next.roles, OwnerRole) {
			return kernel.Refuse[RemovedResponse](CodeMembershipOwnerProtected, "the owner of the store is never removed")
		}
		next.status = MembershipStatusRemoved
		next.accessVersion++
		next.changedAt = cmd.At
		return kernel.Accept(
			RemovedResponse{Subject: next.subject},
			MemberRemoved{Subject: next.subject, AccessVersion: next.accessVersion, At: cmd.At},
		), nil
	})
}

// ConfirmAccessSynced keeps the highest version confirmed, so a confirmation
// that arrives late never undoes a newer one.
func (m *Membership) ConfirmAccessSynced(cmd ConfirmMemberAccessSynced) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
	return kernel.DecideOver(m, (*Membership).clone, func(next *Membership) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
		if cmd.Version > next.accessVersion {
			return kernel.Refuse[kernel.Empty](CodeMembershipVersionAhead, "version is ahead of the access version of the member",
				kernel.Detail{Key: "version", Value: strconv.FormatInt(cmd.Version, 10)},
				kernel.Detail{Key: "accessVersion", Value: strconv.FormatInt(next.accessVersion, 10)},
			)
		}
		next.syncedVersion = max(next.syncedVersion, cmd.Version)
		return kernel.Accept(kernel.Empty{}), nil
	})
}

func (m *Membership) accessChanged(at kernel.Instant) MemberAccessChanged {
	return MemberAccessChanged{Subject: m.subject, Roles: slices.Clone(m.roles), AccessVersion: m.accessVersion, At: at}
}
