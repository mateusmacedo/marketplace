package app

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	kernelapp "github.com/mateusmacedo/dmpf/libs/backend/go/app"
	"github.com/mateusmacedo/dmpf/libs/backend/go/app/serve"
	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/audit"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/boot"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/idclock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/otelboot"
	obsusecase "github.com/mateusmacedo/dmpf/libs/backend/go/observability/usecase"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"google.golang.org/grpc"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/app/rpc"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/provider"
)

func Run(ctx context.Context, cfg Config) error {
	return boot.Boot(ctx, TelemetryOf(cfg), func(ctx context.Context, rt *otelboot.Runtime) error {
		return RunWith(ctx, cfg, rt)
	})
}

// RunWith is the delegate a caller that already owns the telemetry enters by,
// which is what the harnesses use.
func RunWith(ctx context.Context, cfg Config, rt *otelboot.Runtime) error {
	switch cfg.Role {
	case RoleAPI:
		return serve.ServeAPI(ctx, rt, apiRole(cfg), api(cfg, rt))
	case RoleRelay:
		return serve.RunRelay(ctx, rt, relayRole(cfg), serve.Relay{Component: "tenancy", Channel: TenancyChannel(cfg), Scope: reflect.TypeFor[Config]().PkgPath()})
	default:
		return fmt.Errorf("%w: %q", ErrUnknownRole, cfg.Role)
	}
}

// Waits are the ceiling of IDM-07 for a command.
type Waits struct {
	Command time.Duration
}

func bind(waits Waits) func(*postgres.Tx) application.Resources {
	return func(tx *postgres.Tx) application.Resources {
		return application.Resources{
			Stores:   provider.NewStoreRepository(tx),
			Outbox:   tx.Outbox(provider.Mapper{}),
			Commands: tx.CommandInbox(application.CommandConsumer, waits.Command),
		}
	}
}

// NewService is the one place the concrete providers of this context are
// instantiated (ADR-015).
func NewService(pool *pgxpool.Pool, clock ports.Clock, ids ports.IDGenerator, waits Waits, operator ports.TenantID) application.Service {
	return application.Service{
		UoW:          postgres.NewUnitOfWork(pool, bind(waits)),
		Reader:       provider.NewStoreReader(postgres.NewReadPool(pool)),
		StatusReader: provider.NewStoresByStatusReader(postgres.NewReadPool(pool)),
		Clock:        clock,
		IDs:          ids,
		Authorize:    Authorization(operator),
	}
}

// NewTenancyService assembles the use cases over Postgres, with the
// instrumentation of FND-08 and the audit trail emitted through rt.
func NewTenancyService(pool *pgxpool.Pool, rt *otelboot.Runtime, cfg Config) (application.Service, error) {
	policy, err := kernelapp.IdempotencyPolicy(cfg.Policies.IdempotencyWait, cfg.Policies.IdempotencyRetention)
	if err != nil {
		return application.Service{}, err
	}
	service := NewService(pool, idclock.SystemClock{}, idclock.NewMessageIDs("tenancy"), Waits{Command: cfg.Policies.IdempotencyWait}, ports.TenantID(cfg.OperatorTenant))
	service.Idempotency = policy
	service.Instrumentation = obsusecase.New(rt,
		audit.NewLogSink(rt.LoggerProvider()),
		ports.SubjectOf, usecase.CategoryOf, application.OperationFindStore, application.OperationFindStoresByStatus)
	return service, nil
}

func api(cfg Config, rt *otelboot.Runtime) serve.API {
	return serve.API{
		Desc: &rpc.ServiceDesc, Commands: rpc.Commands(), Schema: provider.Schema, Grants: provider.Grants, Uniques: provider.Uniques, CommandConsumer: application.CommandConsumer,
		Register: func(server *grpc.Server, pool *pgxpool.Pool) error {
			service, err := NewTenancyService(pool, rt, cfg)
			if err != nil {
				return err
			}
			server.RegisterService(&rpc.ServiceDesc, rpc.Server{Service: service})
			return nil
		},
	}
}
