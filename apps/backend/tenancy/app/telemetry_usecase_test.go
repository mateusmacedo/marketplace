package app_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/otelboot"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/obstest"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/app"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

type absentStore struct{}

func (absentStore) Load(context.Context, domain.StoreRef) (domain.StoreSnapshot, ports.Version, error) {
	return domain.StoreSnapshot{}, 0, ports.ErrNotFound
}

func TestAFindOfAnAbsentStoreIsNotFoundOnTheUseCaseSpanAndItsRED(t *testing.T) {
	obstest.NotFoundOnTheUseCase(t, "tenancy", application.OperationFindStore, func(ctx context.Context, rt *otelboot.Runtime, pool *pgxpool.Pool) error {
		service, err := app.NewTenancyService(pool, rt, app.Defaults(app.RoleAPI))
		if err != nil {
			return err
		}
		service.Reader = absentStore{}
		service.Authorize = func(context.Context, application.Operation) error { return nil }
		_, err = service.FindStore(ctx, "loja-404")
		return err
	})
}
