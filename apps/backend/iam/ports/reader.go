package ports

import (
	"context"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

// RolesByStatusReader serves FindRolesByStatus: the kernel Reader loads one
// aggregate by its identity, so a query filtered by another column is a port
// of the context (FND-04 §3.3).
type RolesByStatusReader interface {
	LoadByStatus(ctx context.Context, status domain.RoleStatus) ([]domain.RoleSnapshot, error)
}

// InvitationsByStatusReader serves FindInvitationsByStatus, for the same
// reason as RolesByStatusReader.
type InvitationsByStatusReader interface {
	LoadByStatus(ctx context.Context, status domain.InvitationStatus) ([]domain.InvitationSnapshot, error)
}

// MembershipsByStatusReader serves FindMembershipsByStatus, for the same
// reason as RolesByStatusReader.
type MembershipsByStatusReader interface {
	LoadByStatus(ctx context.Context, status domain.MembershipStatus) ([]domain.MembershipSnapshot, error)
}

// BuyersByStatusReader serves FindBuyersByStatus, for the same reason as
// RolesByStatusReader.
type BuyersByStatusReader interface {
	LoadByStatus(ctx context.Context, status domain.BuyerStatus) ([]domain.BuyerSnapshot, error)
}
