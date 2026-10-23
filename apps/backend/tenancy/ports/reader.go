package ports

import (
	"context"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

// StoresByStatusReader reads the stores of the tenant in one status. The
// generic Reader of the kernel loads one aggregate by its own identity, so a
// query filtered by another column belongs to the port block (FND-04 §3.3).
type StoresByStatusReader interface {
	LoadByStatus(ctx context.Context, status domain.StoreStatus) ([]domain.StoreSnapshot, error)
}
