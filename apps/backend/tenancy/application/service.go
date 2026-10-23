package application

import (
	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	port "github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/ports"
)

const (
	AggregateTypeStore = "tenancy.Store"

	Destination = "tenancy.events"

	OperationRegisterStore      = "tenancy.RegisterStore"
	OperationSuspendStore       = "tenancy.SuspendStore"
	OperationReactivateStore    = "tenancy.ReactivateStore"
	OperationFindStore          = "tenancy.FindStore"
	OperationFindStoresByStatus = "tenancy.FindStoresByStatus"
)

const maxEventsPerCommand = 1

type Resources struct {
	Stores   port.Repository[domain.StoreRef, domain.StoreSnapshot]
	Outbox   port.Outbox
	Commands port.Inbox
}

type Operation interface{ isOperation() }

type RegisterStore struct {
	Ref  domain.StoreRef
	Name domain.StoreName
}

func (RegisterStore) isOperation() {}

type SuspendStore struct {
	Ref    domain.StoreRef
	Reason string
}

func (SuspendStore) isOperation() {}

type ReactivateStore struct {
	Ref domain.StoreRef
}

func (ReactivateStore) isOperation() {}

// FindStore and FindStoresByStatus ask for state without changing it, and are
// authorized like any other entry point: reaching the edge is not permission
// to read what the operation returns.
type FindStore struct {
	Ref domain.StoreRef
}

func (FindStore) isOperation() {}

type FindStoresByStatus struct {
	Status domain.StoreStatus
}

func (FindStoresByStatus) isOperation() {}

type Service struct {
	UoW          port.UnitOfWork[Resources]
	Reader       port.Reader[domain.StoreRef, domain.StoreSnapshot]
	StatusReader ports.StoresByStatusReader
	Clock        port.Clock
	IDs          port.IDGenerator
	Authorize    usecase.Authorize[Operation]
	Idempotency  usecase.IdempotencyPolicy

	Instrumentation port.Instrumentation
}

type command[R any] = usecase.Command[Resources, Operation, R]

func (s Service) executor() usecase.Executor[Resources, Operation] {
	return usecase.Executor[Resources, Operation]{
		UoW:             s.UoW,
		Inbox:           func(res Resources) port.Inbox { return res.Commands },
		Consumer:        CommandConsumer,
		Clock:           s.Clock,
		IDs:             s.IDs,
		MaxEvents:       maxEventsPerCommand,
		Policy:          s.Idempotency,
		Authorize:       s.Authorize,
		Instrumentation: s.Instrumentation,
	}
}

// A nil hook is the inert realization, so every operation opens and closes
// unconditionally.
func (s Service) instrumentation() port.Instrumentation {
	if s.Instrumentation == nil {
		return port.NoInstrumentation()
	}
	return s.Instrumentation
}

func origin(ref domain.StoreRef) usecase.Origin {
	return usecase.Origin{Destination: Destination, AggregateType: AggregateTypeStore, AggregateID: string(ref)}
}

var (
	loadStore         = usecase.OrNew(domain.NewStore, domain.FromStoreSnapshot)
	loadExistingStore = usecase.Existing[domain.StoreRef](domain.FromStoreSnapshot)
)
