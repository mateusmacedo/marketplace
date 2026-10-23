package provider

import (
	"context"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

// NewStoreReader serves the read side without the write side (UOW-11): it
// takes the pool because a query must not open a transaction.
func NewStoreReader(pool postgres.ReadPool) ports.Reader[domain.StoreRef, domain.StoreSnapshot] {
	return storeTable.Reader(pool)
}

// storesByStatus is the one query the generic ports do not express: many rows
// filtered by a column. The statement is the kernel's (IDN-14).
var storesByStatus = storeTable.Relation("status")

type StoresByStatusReader struct{ pool postgres.ReadPool }

func NewStoresByStatusReader(pool postgres.ReadPool) *StoresByStatusReader {
	return &StoresByStatusReader{pool: pool}
}

func (r *StoresByStatusReader) LoadByStatus(ctx context.Context, status domain.StoreStatus) ([]domain.StoreSnapshot, error) {
	value, err := statusValue(status)
	if err != nil {
		return nil, err
	}
	return storesByStatus.Query(ctx, r.pool, value)
}
