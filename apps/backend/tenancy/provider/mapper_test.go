package provider_test

import (
	"errors"
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventv1 "github.com/mateusmacedo/marketplace/apps/backend/tenancy/contract/gen/go/company/tenancy/event/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/provider"
)

type strayEvent struct{}

func (strayEvent) EventName() string { return "tenancy.stray" }

func requireMapped(t *testing.T, event kernel.DomainEvent, want proto.Message, wantType string) {
	t.Helper()

	mapped, err := provider.Mapper{}.Map(event)

	if err != nil {
		t.Fatalf("Map() = %v, want nil", err)
	}
	if !proto.Equal(mapped.Message, want) || mapped.Type != wantType {
		t.Errorf("Map() = %v %q, want %v %s (PTB-03)", mapped.Message, mapped.Type, want, wantType)
	}
}

func TestMapperMapsStoreRegistered(t *testing.T) {
	t.Parallel()

	requireMapped(t, domain.StoreRegistered{Ref: "loja-acme", Name: "Loja Acme", At: 1_755_432_000_123_456_789},
		&eventv1.StoreRegistered{Ref: "loja-acme", Name: "Loja Acme", RegisteredAt: &timestamppb.Timestamp{Seconds: 1_755_432_000, Nanos: 123_456_789}},
		"com.company.tenancy.store-registered.v1")
}

func TestMapperMapsStoreSuspended(t *testing.T) {
	t.Parallel()

	requireMapped(t, domain.StoreSuspended{Ref: "loja-acme", Reason: "fraude em análise", At: 1_755_432_000_000_000_001},
		&eventv1.StoreSuspended{Ref: "loja-acme", Reason: "fraude em análise", SuspendedAt: &timestamppb.Timestamp{Seconds: 1_755_432_000, Nanos: 1}},
		"com.company.tenancy.store-suspended.v1")
}

func TestMapperMapsStoreReactivated(t *testing.T) {
	t.Parallel()

	requireMapped(t, domain.StoreReactivated{Ref: "loja-acme", At: 1_755_432_000_000_000_002},
		&eventv1.StoreReactivated{Ref: "loja-acme", ReactivatedAt: &timestamppb.Timestamp{Seconds: 1_755_432_000, Nanos: 2}},
		"com.company.tenancy.store-reactivated.v1")
}

func TestMapperReportsUnmappedEvent(t *testing.T) {
	t.Parallel()

	_, err := provider.Mapper{}.Map(strayEvent{})

	if !errors.Is(err, postgres.ErrUnmappedEvent) {
		t.Fatalf("Map() = %v, want ErrUnmappedEvent", err)
	}
}

var _ kernel.DomainEvent = strayEvent{}
