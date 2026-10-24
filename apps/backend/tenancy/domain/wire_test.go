package domain_test

import (
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

// The codes and event names are published: a rename in Go must not move the
// value on the wire that the bff and the consumers already read.
func TestRejectionCodesKeepTheirPublishedValues(t *testing.T) {
	codes := map[kernel.Code]string{
		domain.CodeStoreRefInvalid:    "tenancy/store/ref-invalid",
		domain.CodeStoreNameEmpty:     "tenancy/store/name-empty",
		domain.CodeStoreReasonEmpty:   "tenancy/store/reason-empty",
		domain.CodeStoreNotActive:     "tenancy/store/not-active",
		domain.CodeStoreNotSuspended:  "tenancy/store/not-suspended",
		domain.CodeStoreDuplicateName: "tenancy/store/duplicate-name",
	}
	for code, want := range codes {
		if string(code) != want {
			t.Errorf("code = %q, want the published value %q", code, want)
		}
	}
}

func TestEventNamesKeepTheirPublishedValues(t *testing.T) {
	names := map[kernel.DomainEvent]string{
		domain.StoreRegistered{}:  "tenancy.store-registered",
		domain.StoreSuspended{}:   "tenancy.store-suspended",
		domain.StoreReactivated{}: "tenancy.store-reactivated",
	}
	for event, want := range names {
		if got := event.EventName(); got != want {
			t.Errorf("EventName() = %q, want the published value %q", got, want)
		}
	}
}

func TestStatusNamesReadAsTheSpecWritesThem(t *testing.T) {
	names := map[domain.StoreStatus]string{
		domain.Unregistered: "unregistered",
		domain.Active:       "active",
		domain.Suspended:    "suspended",
	}
	for status, want := range names {
		if got := status.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}
