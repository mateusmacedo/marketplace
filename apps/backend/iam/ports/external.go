package ports

import "github.com/mateusmacedo/marketplace/apps/backend/iam/domain"

// RoleAccess is what SyncRole converges the group of a role to: active holds
// exactly Permissions as client roles, retired holds none.
type RoleAccess struct {
	Ref         domain.RoleRef
	Status      domain.RoleStatus
	Permissions []domain.Permission
}

// MemberProvision is a user with this address, member of the store.
type MemberProvision struct {
	Email domain.Email
}

// MemberAccess is what SyncMemberAccess and RevokeMemberAccess converge to:
// active is in exactly the groups of Roles, removed is out of the store.
type MemberAccess struct {
	Subject domain.Subject
	Status  domain.MembershipStatus
	Roles   []domain.RoleRef
}

// BuyerAccess is what SyncBuyerAccess converges to: active holds the realm
// role of the buyer, blocked does not.
type BuyerAccess struct {
	Subject domain.Subject
	Status  domain.BuyerStatus
}

// ExternalPayloads encodes a desired state as the payload of its command, so
// this block and application never serialize; a status without wire value is refused.
type ExternalPayloads interface {
	RoleAccess(RoleAccess) ([]byte, error)
	MemberProvision(MemberProvision) ([]byte, error)
	MemberAccess(MemberAccess) ([]byte, error)
	BuyerAccess(BuyerAccess) ([]byte, error)
}
