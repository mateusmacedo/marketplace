package application

import (
	"context"
	"fmt"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

func (s Service) FindStore(ctx context.Context, ref domain.StoreRef) (domain.StoreSnapshot, error) {
	return usecase.Query[Operation](ctx, s.instrumentation(), s.Authorize, OperationFindStore, FindStore{Ref: ref},
		func(ctx context.Context) (domain.StoreSnapshot, error) {
			snapshot, _, err := s.Reader.Load(ctx, ref)
			if err != nil {
				return domain.StoreSnapshot{}, fmt.Errorf("application: find store %s: %w", ref, err)
			}
			return snapshot, nil
		})
}
