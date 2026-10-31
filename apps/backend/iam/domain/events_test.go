package domain_test

import (
	"slices"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

// sameEvent compares field by field: most events of the context carry slices,
// which neither == nor the interface comparison of tb.RequireSameEvents accept.
func sameEvent(a, b kernel.DomainEvent) bool {
	switch x := a.(type) {
	case domain.RoleDefined:
		y, ok := b.(domain.RoleDefined)
		return ok && x.Ref == y.Ref && x.Name == y.Name && slices.Equal(x.Permissions, y.Permissions) &&
			x.AccessVersion == y.AccessVersion && x.At == y.At
	case domain.RolePermissionsChanged:
		y, ok := b.(domain.RolePermissionsChanged)
		return ok && x.Ref == y.Ref && slices.Equal(x.Permissions, y.Permissions) && x.AccessVersion == y.AccessVersion && x.At == y.At
	case domain.MemberInvited:
		y, ok := b.(domain.MemberInvited)
		return ok && x.Email == y.Email && slices.Equal(x.Roles, y.Roles) && x.Round == y.Round && x.At == y.At
	case domain.InvitationProvisioned:
		y, ok := b.(domain.InvitationProvisioned)
		return ok && x.Email == y.Email && x.Subject == y.Subject && slices.Equal(x.Roles, y.Roles) && x.At == y.At
	case domain.MembershipEstablished:
		y, ok := b.(domain.MembershipEstablished)
		return ok && x.Subject == y.Subject && x.Email == y.Email && slices.Equal(x.Roles, y.Roles) &&
			x.AccessVersion == y.AccessVersion && x.At == y.At
	case domain.MemberAccessChanged:
		y, ok := b.(domain.MemberAccessChanged)
		return ok && x.Subject == y.Subject && slices.Equal(x.Roles, y.Roles) && x.AccessVersion == y.AccessVersion && x.At == y.At
	case domain.RoleRetired, domain.InvitationFailed, domain.MemberRemoved,
		domain.BuyerRegistered, domain.BuyerProfileUpdated, domain.BuyerBlocked, domain.BuyerUnblocked:
		return a == b
	default:
		return false
	}
}

func sameEvents(a, b []kernel.DomainEvent) bool {
	return slices.EqualFunc(a, b, sameEvent)
}

func sameRejection(a, b *kernel.Rejection) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Code() == b.Code() && a.Message() == b.Message() && slices.Equal(a.Details(), b.Details())
}
