package domain

import kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

type RoleRef string

type RoleName string

type RoleStatus int

const (
	RoleStatusUndefined RoleStatus = iota
	RoleStatusActive
	RoleStatusRetired
)

type DefineRole struct {
	Ref         RoleRef
	Name        RoleName
	Permissions []Permission
	At          kernel.Instant
}

type ChangeRolePermissions struct {
	Permissions []Permission
	At          kernel.Instant
}

type RetireRole struct {
	At kernel.Instant
}

// ConfirmRoleSynced is the external step of the projection of the role into
// Keycloak: Version is the access version the projection converged to.
type ConfirmRoleSynced struct {
	Version int64
}

type DefinedResponse struct {
	Ref RoleRef
}

type PermissionsChangedResponse struct {
	Ref RoleRef
}

type RetiredResponse struct {
	Ref RoleRef
}

type RoleDefined struct {
	Ref           RoleRef
	Name          RoleName
	Permissions   []Permission
	AccessVersion int64
	At            kernel.Instant
}

func (RoleDefined) EventName() string { return "iam.role-defined" }

type RolePermissionsChanged struct {
	Ref           RoleRef
	Permissions   []Permission
	AccessVersion int64
	At            kernel.Instant
}

func (RolePermissionsChanged) EventName() string { return "iam.role-permissions-changed" }

type RoleRetired struct {
	Ref           RoleRef
	AccessVersion int64
	At            kernel.Instant
}

func (RoleRetired) EventName() string { return "iam.role-retired" }

var (
	_ kernel.DomainEvent = RoleDefined{}
	_ kernel.DomainEvent = RolePermissionsChanged{}
	_ kernel.DomainEvent = RoleRetired{}
)

func (s RoleStatus) String() string {
	switch s {
	case RoleStatusActive:
		return "active"
	case RoleStatusRetired:
		return "retired"
	default:
		return "undefined"
	}
}

// Email is the identity of an invitation: the address trimmed and in lower
// case, the form NormalizeEmail gives it.
type Email string

// Subject is the sub of a user in Keycloak.
type Subject string

type InvitationStatus int

const (
	InvitationStatusUndefined InvitationStatus = iota
	InvitationStatusPending
	InvitationStatusProvisioned
	InvitationStatusFailed
)

type MembershipStatus int

const (
	MembershipStatusUndefined MembershipStatus = iota
	MembershipStatusActive
	MembershipStatusRemoved
)

// InviteMember carries the address as received: the invitation judges its
// form after normalizing it.
type InviteMember struct {
	Email string
	Roles []RoleRef
	At    kernel.Instant
}

type ConfirmInvitation struct {
	Subject Subject
	At      kernel.Instant
}

type FailInvitation struct {
	Reason string
	At     kernel.Instant
}

type EstablishMembership struct {
	Subject Subject
	Email   Email
	Roles   []RoleRef
	At      kernel.Instant
}

type GrantRole struct {
	Role RoleRef
	At   kernel.Instant
}

type RevokeRole struct {
	Role RoleRef
	At   kernel.Instant
}

type RemoveMember struct {
	At kernel.Instant
}

type ConfirmMemberAccessSynced struct {
	Version int64
}

type InvitedResponse struct {
	Email  Email
	Status InvitationStatus
}

type EstablishedResponse struct {
	Subject Subject
}

type GrantedResponse struct {
	Subject Subject
}

type RevokedResponse struct {
	Subject Subject
}

type RemovedResponse struct {
	Subject Subject
}

type MemberInvited struct {
	Email Email
	Roles []RoleRef
	Round int64
	At    kernel.Instant
}

func (MemberInvited) EventName() string { return "iam.member-invited" }

type InvitationProvisioned struct {
	Email   Email
	Subject Subject
	Roles   []RoleRef
	At      kernel.Instant
}

func (InvitationProvisioned) EventName() string { return "iam.invitation-provisioned" }

type InvitationFailed struct {
	Email  Email
	Reason string
	At     kernel.Instant
}

func (InvitationFailed) EventName() string { return "iam.invitation-failed" }

type MembershipEstablished struct {
	Subject       Subject
	Email         Email
	Roles         []RoleRef
	AccessVersion int64
	At            kernel.Instant
}

func (MembershipEstablished) EventName() string { return "iam.membership-established" }

type MemberAccessChanged struct {
	Subject       Subject
	Roles         []RoleRef
	AccessVersion int64
	At            kernel.Instant
}

func (MemberAccessChanged) EventName() string { return "iam.member-access-changed" }

type MemberRemoved struct {
	Subject       Subject
	AccessVersion int64
	At            kernel.Instant
}

func (MemberRemoved) EventName() string { return "iam.member-removed" }

var (
	_ kernel.DomainEvent = MemberInvited{}
	_ kernel.DomainEvent = InvitationProvisioned{}
	_ kernel.DomainEvent = InvitationFailed{}
	_ kernel.DomainEvent = MembershipEstablished{}
	_ kernel.DomainEvent = MemberAccessChanged{}
	_ kernel.DomainEvent = MemberRemoved{}
)

func (s InvitationStatus) String() string {
	switch s {
	case InvitationStatusPending:
		return "pending"
	case InvitationStatusProvisioned:
		return "provisioned"
	case InvitationStatusFailed:
		return "failed"
	default:
		return "undefined"
	}
}

func (s MembershipStatus) String() string {
	switch s {
	case MembershipStatusActive:
		return "active"
	case MembershipStatusRemoved:
		return "removed"
	default:
		return "undefined"
	}
}

type BuyerName string

type BuyerStatus int

const (
	BuyerStatusUndefined BuyerStatus = iota
	BuyerStatusActive
	BuyerStatusBlocked
)

// RegisterBuyer carries the subject the bff took from the token, never one the
// client sent.
type RegisterBuyer struct {
	Subject Subject
	Name    BuyerName
	At      kernel.Instant
}

type UpdateBuyerProfile struct {
	Name BuyerName
	At   kernel.Instant
}

type BlockBuyer struct {
	Reason string
	At     kernel.Instant
}

type UnblockBuyer struct {
	At kernel.Instant
}

type ConfirmBuyerAccessSynced struct {
	Version int64
}

type RegisteredResponse struct {
	Subject Subject
	Status  BuyerStatus
}

type ProfileUpdatedResponse struct {
	Subject Subject
}

type BlockedResponse struct {
	Subject Subject
}

type UnblockedResponse struct {
	Subject Subject
}

type BuyerRegistered struct {
	Subject       Subject
	Name          BuyerName
	AccessVersion int64
	At            kernel.Instant
}

func (BuyerRegistered) EventName() string { return "iam.buyer-registered" }

type BuyerProfileUpdated struct {
	Subject Subject
	Name    BuyerName
	At      kernel.Instant
}

func (BuyerProfileUpdated) EventName() string { return "iam.buyer-profile-updated" }

type BuyerBlocked struct {
	Subject       Subject
	Reason        string
	AccessVersion int64
	At            kernel.Instant
}

func (BuyerBlocked) EventName() string { return "iam.buyer-blocked" }

type BuyerUnblocked struct {
	Subject       Subject
	AccessVersion int64
	At            kernel.Instant
}

func (BuyerUnblocked) EventName() string { return "iam.buyer-unblocked" }

var (
	_ kernel.DomainEvent = BuyerRegistered{}
	_ kernel.DomainEvent = BuyerProfileUpdated{}
	_ kernel.DomainEvent = BuyerBlocked{}
	_ kernel.DomainEvent = BuyerUnblocked{}
)

func (s BuyerStatus) String() string {
	switch s {
	case BuyerStatusActive:
		return "active"
	case BuyerStatusBlocked:
		return "blocked"
	default:
		return "undefined"
	}
}
