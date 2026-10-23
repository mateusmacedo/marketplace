package domain

import kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

type StoreRef string

type StoreName string

type StoreStatus int

const (
	Unregistered StoreStatus = iota
	Active
	Suspended
)

type RegisterStore struct {
	Ref  StoreRef
	Name StoreName
	At   kernel.Instant
}

type SuspendStore struct {
	Reason string
	At     kernel.Instant
}

type ReactivateStore struct {
	At kernel.Instant
}

type RegisteredResponse struct {
	Ref StoreRef
}

type SuspendedResponse struct {
	Ref StoreRef
}

type ReactivatedResponse struct {
	Ref StoreRef
}

type StoreRegistered struct {
	Ref  StoreRef
	Name StoreName
	At   kernel.Instant
}

func (StoreRegistered) EventName() string { return "tenancy.store-registered" }

type StoreSuspended struct {
	Ref    StoreRef
	Reason string
	At     kernel.Instant
}

func (StoreSuspended) EventName() string { return "tenancy.store-suspended" }

type StoreReactivated struct {
	Ref StoreRef
	At  kernel.Instant
}

func (StoreReactivated) EventName() string { return "tenancy.store-reactivated" }

var (
	_ kernel.DomainEvent = StoreRegistered{}
	_ kernel.DomainEvent = StoreSuspended{}
	_ kernel.DomainEvent = StoreReactivated{}
)

func (s StoreStatus) String() string {
	switch s {
	case Active:
		return "active"
	case Suspended:
		return "suspended"
	default:
		return "unregistered"
	}
}
