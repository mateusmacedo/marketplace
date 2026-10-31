package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	kernelapp "github.com/mateusmacedo/dmpf/libs/backend/go/app"
	"github.com/mateusmacedo/dmpf/libs/backend/go/app/serve"
	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	"github.com/mateusmacedo/dmpf/libs/backend/go/kafka"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/audit"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/boot"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/idclock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/logging"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/otelboot"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/retry"
	obsusecase "github.com/mateusmacedo/dmpf/libs/backend/go/observability/usecase"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/channel"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/app/rpc"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
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
		return serve.RunRelay(ctx, rt, relayRole(cfg), serve.Relay{Component: "iam", Channel: IamChannel(cfg), Scope: reflect.TypeFor[Config]().PkgPath()})
	case RoleConsumer:
		return runConsumer(ctx, cfg, rt)
	default:
		return fmt.Errorf("%w: %q", ErrUnknownRole, cfg.Role)
	}
}

// Waits are the ceilings of INB-17 for a message and of IDM-07 for a command;
// a role that runs no command leaves Command zero.
type Waits struct {
	Message time.Duration
	Command time.Duration
}

func bind(waits Waits) func(*postgres.Tx) application.Resources {
	return func(tx *postgres.Tx) application.Resources {
		return application.Resources{
			Inbox:       tx.Inbox(ConsumerName, waits.Message),
			Roles:       provider.NewRoleRepository(tx),
			Invitations: provider.NewInvitationRepository(tx),
			Memberships: provider.NewMembershipRepository(tx),
			Buyers:      provider.NewBuyerRepository(tx),
			Outbox:      tx.Outbox(provider.Mapper{}),
			Commands:    tx.CommandInbox(application.CommandConsumer, waits.Command),
		}
	}
}

// NewService is the one place the concrete providers of this context are
// instantiated (ADR-015).
func NewService(pool *pgxpool.Pool, clock ports.Clock, ids ports.IDGenerator, waits Waits, operator ports.TenantID) application.Service {
	reads := postgres.NewReadPool(pool)
	return application.Service{
		UoW:                 postgres.NewUnitOfWork(pool, bind(waits)),
		Roles:               provider.NewRoleReader(reads),
		RolesByStatus:       provider.NewRolesByStatusReader(reads),
		Invitations:         provider.NewInvitationReader(reads),
		InvitationsByStatus: provider.NewInvitationsByStatusReader(reads),
		Memberships:         provider.NewMembershipReader(reads),
		MembershipsByStatus: provider.NewMembershipsByStatusReader(reads),
		Buyers:              provider.NewBuyerReader(reads),
		BuyersByStatus:      provider.NewBuyersByStatusReader(reads),
		Clock:               clock,
		IDs:                 ids,
		Authorize:           Authorization(operator),
		Consumer:            ConsumerName,
	}
}

// NewIamService assembles the use cases over Postgres, with the
// instrumentation of FND-08 and the audit trail emitted through rt.
func NewIamService(pool *pgxpool.Pool, rt *otelboot.Runtime, cfg Config) (application.Service, error) {
	policy, err := kernelapp.IdempotencyPolicy(cfg.Policies.IdempotencyWait, cfg.Policies.IdempotencyRetention)
	if err != nil {
		return application.Service{}, err
	}
	service := NewService(pool, idclock.SystemClock{}, idclock.NewMessageIDs("iam"), Waits{Message: cfg.Wait, Command: cfg.Policies.IdempotencyWait}, ports.TenantID(cfg.OperatorTenant))
	service.Idempotency = policy
	service.Instrumentation = obsusecase.New(rt,
		audit.NewLogSink(rt.LoggerProvider()),
		ports.SubjectOf, usecase.CategoryOf,
		application.OperationFindRole, application.OperationFindRolesByStatus,
		application.OperationFindInvitation, application.OperationFindInvitationsByStatus,
		application.OperationFindMembership, application.OperationFindMembershipsByStatus,
		application.OperationFindBuyer, application.OperationFindBuyersByStatus)
	return service, nil
}

func api(cfg Config, rt *otelboot.Runtime) serve.API {
	return serve.API{
		Desc: &rpc.ServiceDesc, Commands: rpc.Commands(), Schema: provider.Schema, Grants: provider.Grants, Uniques: provider.Uniques, CommandConsumer: application.CommandConsumer,
		Register: func(server *grpc.Server, pool *pgxpool.Pool) error {
			service, err := NewIamService(pool, rt, cfg)
			if err != nil {
				return err
			}
			server.RegisterService(&rpc.ServiceDesc, rpc.Server{Service: service})
			return nil
		},
	}
}

// NewIamConsumer is the consumer adapter with the attempt limit of the channel
// it consumes (ADR-039: the two must agree).
func NewIamConsumer(pool *pgxpool.Pool, cfg Config, ch channel.Channel, authenticated bool, tracer trace.Tracer, meters metric.MeterProvider, logs log.LoggerProvider) kernelapp.Consumer {
	service := NewService(pool, idclock.SystemClock{}, idclock.NewMessageIDs("iam"), Waits{Message: cfg.Wait}, ports.TenantID(cfg.OperatorTenant))
	return NewConsumer(service, pool, idclock.SystemClock{}, cfg.ConsumerTimeout, ch.Retry.MaxAttempts, MembershipBoundary(cfg, authenticated), ConsumerTelemetry{
		Tracer:         tracer,
		MeterProvider:  meters,
		LoggerProvider: logs,
		System:         semconv.MessagingSystemKafka.Value.AsString(),
		Channel:        kernelapp.Channel{Address: ch.Address, Group: ch.Group},
	})
}

const (
	processingDeadline = 5 * time.Second
	revokeGrace        = 10 * time.Second
	rebalanceTimeout   = 30 * time.Second
	queuePerPartition  = 32
)

func runConsumer(ctx context.Context, cfg Config, rt *otelboot.Runtime) error {
	ctx, abort := context.WithCancelCause(ctx)
	defer abort(nil)
	pool, err := postgres.NewPool(ctx, cfg.DSN, rt.Tracer(), postgres.WithMeterProvider(rt.MeterProvider()))
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	if err := postgres.WaitForGrants(ctx, pool, time.Second, rt.LoggerProvider(), append(postgres.KernelGrants(postgres.Outbox, postgres.Inbox), provider.Grants...)...); err != nil {
		return err
	}
	stopPurge, err := kernelapp.StartPurge(ctx, abort,
		kernelapp.PurgeConfig{Name: "message-inbox", Interval: cfg.Policies.PurgeInterval, Batch: cfg.Policies.PurgeBatch, Retention: cfg.Policies.InboxRetention},
		idclock.SystemClock{}, rt.LoggerProvider(), kernelapp.PurgeMessageInbox(pool, ConsumerName))
	if err != nil {
		return err
	}
	defer func() { _ = stopPurge() }()

	ch := MembershipChannel(cfg)
	catalog, err := channel.NewCatalog(ch)
	if err != nil {
		return err
	}
	kafkaConfig, err := kafka.NewConfig(ctx, rt, catalog, cfg.Brokers, cfg.KafkaInsecure, cfg.KafkaAuth)
	if err != nil {
		return err
	}
	kafkaConfig.RevokeGrace = revokeGrace
	logger := rt.LoggerFor(reflect.TypeFor[Config]().PkgPath())
	consumer := &kafka.Consumer{
		Config:  kafkaConfig,
		Channel: ch,
		Sink: Sink{
			Consumer:  NewIamConsumer(pool, cfg, ch, kafkaConfig.ClientAuthenticated(), rt.Tracer(), rt.MeterProvider(), rt.LoggerProvider()),
			EventType: channel.EventTypeOf(ch),
			Logger:    logger,
		},
		Backoff:            retry.Backoff{Base: 10 * time.Millisecond, Factor: 2, Cap: time.Second},
		ProcessingDeadline: processingDeadline,
		RebalanceTimeout:   rebalanceTimeout,
		QueuePerPartition:  queuePerPartition,
	}
	if err := consumer.Validate(); err != nil {
		return err
	}
	logger.LogAttrs(ctx, slog.LevelInfo, "consumer joining",
		slog.String(string(semconv.MessagingSystemKey), semconv.MessagingSystemKafka.Value.AsString()),
		slog.String(string(semconv.MessagingDestinationNameKey), ch.Address),
		slog.String(string(semconv.MessagingConsumerGroupNameKey), ch.Group),
		slog.String(logging.KeyChannelName, ch.Name))
	// WHY: only the cancellation itself is a clean exit; guarding by ctx.Err()
	// would also swallow a broker failure that landed during the drain.
	if err := consumer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return stopPurge()
}
