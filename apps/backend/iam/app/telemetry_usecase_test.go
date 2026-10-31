package app_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/otelboot"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/obstest"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/app"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

type absentRole struct{}

func (absentRole) Load(context.Context, domain.RoleRef) (domain.RoleSnapshot, ports.Version, error) {
	return domain.RoleSnapshot{}, 0, ports.ErrNotFound
}

func TestAFindOfAnAbsentRoleIsNotFoundOnTheUseCaseSpanAndItsRED(t *testing.T) {
	obstest.NotFoundOnTheUseCase(t, "iam", application.OperationFindRole, func(ctx context.Context, rt *otelboot.Runtime, pool *pgxpool.Pool) error {
		service, err := app.NewIamService(pool, rt, app.Defaults(app.RoleAPI))
		if err != nil {
			return err
		}
		service.Roles = absentRole{}
		service.Authorize = func(context.Context, application.Operation) error { return nil }
		_, err = service.FindRole(ctx, "papel-404")
		return err
	})
}
