package provider

import (
	"fmt"
	"time"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventv1 "github.com/mateusmacedo/marketplace/apps/backend/tenancy/contract/gen/go/company/tenancy/event/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

const (
	storeRegisteredType  = "com.company.tenancy.store-registered.v1"
	storeSuspendedType   = "com.company.tenancy.store-suspended.v1"
	storeReactivatedType = "com.company.tenancy.store-reactivated.v1"
)

type Mapper struct{}

func (Mapper) Map(event kernel.DomainEvent) (postgres.Mapped, error) {
	switch e := event.(type) {
	case domain.StoreRegistered:
		return postgres.Mapped{
			Message: &eventv1.StoreRegistered{
				Ref:          string(e.Ref),
				Name:         string(e.Name),
				RegisteredAt: timestamppb.New(time.Unix(0, int64(e.At))),
			},
			Type: storeRegisteredType,
		}, nil
	case domain.StoreSuspended:
		return postgres.Mapped{
			Message: &eventv1.StoreSuspended{
				Ref:         string(e.Ref),
				Reason:      e.Reason,
				SuspendedAt: timestamppb.New(time.Unix(0, int64(e.At))),
			},
			Type: storeSuspendedType,
		}, nil
	case domain.StoreReactivated:
		return postgres.Mapped{
			Message: &eventv1.StoreReactivated{
				Ref:           string(e.Ref),
				ReactivatedAt: timestamppb.New(time.Unix(0, int64(e.At))),
			},
			Type: storeReactivatedType,
		}, nil
	default:
		return postgres.Mapped{}, fmt.Errorf("%w: %s", postgres.ErrUnmappedEvent, event.EventName())
	}
}
