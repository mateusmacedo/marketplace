package domain

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
)

const (
	maxEmailLength   = 254
	maxSubjectLength = 255
)

type Invitation struct {
	email         Email
	roles         []RoleRef
	status        InvitationStatus
	round         int64
	subject       Subject
	failureReason string
	invitedAt     kernel.Instant
	settledAt     kernel.Instant
}

func NewInvitation(email Email) *Invitation {
	return &Invitation{email: email}
}

func FromInvitationSnapshot(s InvitationSnapshot) *Invitation {
	return &Invitation{
		email:         s.Email,
		roles:         slices.Clone(s.Roles),
		status:        s.Status,
		round:         s.Round,
		subject:       s.Subject,
		failureReason: s.FailureReason,
		invitedAt:     s.InvitedAt,
		settledAt:     s.SettledAt,
	}
}

type InvitationSnapshot struct {
	Email         Email
	Roles         []RoleRef
	Status        InvitationStatus
	Round         int64
	Subject       Subject
	FailureReason string
	InvitedAt     kernel.Instant
	SettledAt     kernel.Instant
}

func (i *Invitation) Snapshot() InvitationSnapshot {
	return InvitationSnapshot{
		Email:         i.email,
		Roles:         slices.Clone(i.roles),
		Status:        i.status,
		Round:         i.round,
		Subject:       i.subject,
		FailureReason: i.failureReason,
		InvitedAt:     i.invitedAt,
		SettledAt:     i.settledAt,
	}
}

func (s InvitationSnapshot) Equal(other InvitationSnapshot) bool {
	return s.Email == other.Email && slices.Equal(s.Roles, other.Roles) && s.Status == other.Status &&
		s.Round == other.Round && s.Subject == other.Subject && s.FailureReason == other.FailureReason &&
		s.InvitedAt == other.InvitedAt && s.SettledAt == other.SettledAt
}

func (i *Invitation) clone() Invitation {
	next := *i
	next.roles = slices.Clone(i.roles)
	return next
}

func (i *Invitation) Invite(cmd InviteMember) (kernel.Accepted[InvitedResponse], *kernel.Rejection) {
	return kernel.DecideOver(i, (*Invitation).clone, func(next *Invitation) (kernel.Accepted[InvitedResponse], *kernel.Rejection) {
		if address := NormalizeEmail(cmd.Email); !ValidEmail(address) {
			return kernel.Refuse[InvitedResponse](CodeInvitationEmailInvalid,
				EmailInvalidMessage,
				kernel.Detail{Key: "email", Value: string(address)},
			)
		}
		if rejection := refuseInvitedRoles(cmd.Roles); rejection != nil {
			return kernel.Accepted[InvitedResponse]{}, rejection
		}
		if next.status == InvitationStatusPending {
			return kernel.Accept(InvitedResponse{Email: next.email, Status: next.status}), nil
		}
		next.roles = normalized(cmd.Roles)
		next.status = InvitationStatusPending
		next.round++
		next.subject = ""
		next.failureReason = ""
		next.invitedAt = cmd.At
		next.settledAt = 0
		return kernel.Accept(
			InvitedResponse{Email: next.email, Status: next.status},
			MemberInvited{Email: next.email, Roles: slices.Clone(next.roles), Round: next.round, At: cmd.At},
		), nil
	})
}

// Confirm is the external step of the invitation: Keycloak returned the
// subject of the address.
func (i *Invitation) Confirm(cmd ConfirmInvitation) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
	return kernel.DecideOver(i, (*Invitation).clone, func(next *Invitation) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
		if next.status != InvitationStatusPending {
			return kernel.Refuse[kernel.Empty](CodeInvitationNotPending, "invitation is not pending")
		}
		if !ValidSubject(cmd.Subject) {
			return kernel.Refuse[kernel.Empty](CodeInvitationSubjectInvalid, SubjectInvalidMessage)
		}
		next.status = InvitationStatusProvisioned
		next.subject = cmd.Subject
		next.settledAt = cmd.At
		return kernel.Accept(
			kernel.Empty{},
			InvitationProvisioned{Email: next.email, Subject: next.subject, Roles: slices.Clone(next.roles), At: cmd.At},
		), nil
	})
}

// Fail is the external step of the invitation that Keycloak finally refused.
func (i *Invitation) Fail(cmd FailInvitation) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
	return kernel.DecideOver(i, (*Invitation).clone, func(next *Invitation) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
		if next.status != InvitationStatusPending {
			return kernel.Refuse[kernel.Empty](CodeInvitationNotPending, "invitation is not pending")
		}
		if cmd.Reason == "" {
			return kernel.Refuse[kernel.Empty](CodeInvitationReasonEmpty, "reason must not be empty")
		}
		next.status = InvitationStatusFailed
		next.failureReason = cmd.Reason
		next.settledAt = cmd.At
		return kernel.Accept(
			kernel.Empty{},
			InvitationFailed{Email: next.email, Reason: next.failureReason, At: cmd.At},
		), nil
	})
}

func refuseInvitedRoles(roles []RoleRef) *kernel.Rejection {
	if len(roles) == 0 {
		return kernel.Reject(CodeInvitationRolesEmpty, "roles must not be empty")
	}
	for _, role := range roles {
		if !ValidRef(role) {
			return kernel.Reject(CodeInvitationRoleInvalid, roleInvalidMessage, kernel.Detail{Key: "role", Value: string(role)})
		}
	}
	if slices.Contains(roles, OwnerRole) {
		return kernel.Reject(CodeInvitationRoleReserved, "owner is not invitable: the owner is seeded with the store")
	}
	return nil
}

// The messages of the form of an address and of a subject, shared by the
// refusals of the domain and by the InvalidArgument of the edge.
const (
	EmailInvalidMessage   = "email must have up to 254 characters, exactly one @, a non-empty local part, a domain with a dot and no control character"
	SubjectInvalidMessage = "subject must have 1 to 255 characters and no control character"
)

const roleInvalidMessage = "role must be a ref of 3 to 63 characters of a-z, 0-9 and -, without a leading or trailing hyphen"

// NormalizeEmail is the identity of an invitation for an address as typed.
func NormalizeEmail(raw string) Email {
	return Email(strings.ToLower(strings.TrimSpace(raw)))
}

// ValidEmail refuses control characters too: U+0000 is valid UTF-8 for
// proto3 but no Postgres text holds it, and the address is the key read first.
func ValidEmail(email Email) bool {
	local, domainPart, found := strings.Cut(string(email), "@")
	return found && utf8.RuneCountInString(string(email)) <= maxEmailLength && local != "" &&
		!strings.Contains(domainPart, "@") && strings.Contains(domainPart, ".") && !hasControl(string(email))
}

// ValidSubject is the form of every subject, the key of a membership and of a
// buyer, under the same refusal of control characters as ValidEmail.
func ValidSubject(subject Subject) bool {
	n := utf8.RuneCountInString(string(subject))
	return n >= 1 && n <= maxSubjectLength && !hasControl(string(subject))
}

func hasControl(text string) bool { return strings.ContainsFunc(text, unicode.IsControl) }
