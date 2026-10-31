package application

import (
	"context"
	"fmt"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func (s Service) FindRole(ctx context.Context, ref domain.RoleRef) (domain.RoleSnapshot, error) {
	return usecase.Query[Operation](ctx, s.instrumentation(), s.Authorize, OperationFindRole, FindRole{Ref: ref},
		func(ctx context.Context) (domain.RoleSnapshot, error) {
			snapshot, _, err := s.Roles.Load(ctx, ref)
			if err != nil {
				return domain.RoleSnapshot{}, fmt.Errorf("application: find role %s: %w", ref, err)
			}
			return snapshot, nil
		})
}
