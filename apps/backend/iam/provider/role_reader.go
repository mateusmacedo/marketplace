package provider

import (
	"context"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

// NewRoleReader serves the read side without the write side (UOW-11): it
// takes the pool because a query must not open a transaction.
func NewRoleReader(pool postgres.ReadPool) ports.Reader[domain.RoleRef, domain.RoleSnapshot] {
	return roleTable.Reader(pool)
}

// rolesByStatus is the one query the generic ports do not express: many rows
// filtered by a column. The statement is the kernel's (IDN-14).
var rolesByStatus = roleTable.Relation("status")

type RolesByStatusReader struct{ pool postgres.ReadPool }

func NewRolesByStatusReader(pool postgres.ReadPool) *RolesByStatusReader {
	return &RolesByStatusReader{pool: pool}
}

func (r *RolesByStatusReader) LoadByStatus(ctx context.Context, status domain.RoleStatus) ([]domain.RoleSnapshot, error) {
	value, err := roleStatusValue(status)
	if err != nil {
		return nil, err
	}
	return rolesByStatus.Query(ctx, r.pool, value)
}
