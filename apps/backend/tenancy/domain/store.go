package domain

import kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

const (
	minRefLength = 3
	maxRefLength = 63
)

type Store struct {
	ref              StoreRef
	name             StoreName
	status           StoreStatus
	suspensionReason string
	registeredAt     kernel.Instant
	statusChangedAt  kernel.Instant
}

func NewStore(ref StoreRef) *Store {
	return &Store{ref: ref}
}

func FromStoreSnapshot(s StoreSnapshot) *Store {
	return &Store{
		ref:              s.Ref,
		name:             s.Name,
		status:           s.Status,
		suspensionReason: s.SuspensionReason,
		registeredAt:     s.RegisteredAt,
		statusChangedAt:  s.StatusChangedAt,
	}
}

type StoreSnapshot struct {
	Ref              StoreRef
	Name             StoreName
	Status           StoreStatus
	SuspensionReason string
	RegisteredAt     kernel.Instant
	StatusChangedAt  kernel.Instant
}

func (s *Store) Snapshot() StoreSnapshot {
	return StoreSnapshot{
		Ref:              s.ref,
		Name:             s.name,
		Status:           s.status,
		SuspensionReason: s.suspensionReason,
		RegisteredAt:     s.registeredAt,
		StatusChangedAt:  s.statusChangedAt,
	}
}

func (s StoreSnapshot) Equal(other StoreSnapshot) bool {
	return s == other
}

func (s *Store) clone() Store {
	return *s
}

func (s *Store) Register(cmd RegisterStore) (kernel.Accepted[RegisteredResponse], *kernel.Rejection) {
	return kernel.DecideOver(s, (*Store).clone, func(next *Store) (kernel.Accepted[RegisteredResponse], *kernel.Rejection) {
		if !ValidRef(cmd.Ref) {
			return kernel.Refuse[RegisteredResponse](CodeStoreRefInvalid,
				"ref must have 3 to 63 characters of a-z, 0-9 and -, without a leading or trailing hyphen",
				kernel.Detail{Key: "ref", Value: string(cmd.Ref)},
			)
		}
		if cmd.Name == "" {
			return kernel.Refuse[RegisteredResponse](CodeStoreNameEmpty, "name must not be empty")
		}
		if next.status != Unregistered {
			return kernel.Accept(RegisteredResponse{Ref: next.ref}), nil
		}
		next.name = cmd.Name
		next.status = Active
		next.suspensionReason = ""
		next.registeredAt = cmd.At
		next.statusChangedAt = cmd.At
		return kernel.Accept(
			RegisteredResponse{Ref: next.ref},
			StoreRegistered{Ref: next.ref, Name: next.name, At: cmd.At},
		), nil
	})
}

func (s *Store) Suspend(cmd SuspendStore) (kernel.Accepted[SuspendedResponse], *kernel.Rejection) {
	return kernel.DecideOver(s, (*Store).clone, func(next *Store) (kernel.Accepted[SuspendedResponse], *kernel.Rejection) {
		if next.status != Active {
			return kernel.Refuse[SuspendedResponse](CodeStoreNotActive, "store is not active")
		}
		if cmd.Reason == "" {
			return kernel.Refuse[SuspendedResponse](CodeStoreReasonEmpty, "reason must not be empty")
		}
		next.status = Suspended
		next.suspensionReason = cmd.Reason
		next.statusChangedAt = cmd.At
		return kernel.Accept(
			SuspendedResponse{Ref: next.ref},
			StoreSuspended{Ref: next.ref, Reason: next.suspensionReason, At: cmd.At},
		), nil
	})
}

func (s *Store) Reactivate(cmd ReactivateStore) (kernel.Accepted[ReactivatedResponse], *kernel.Rejection) {
	return kernel.DecideOver(s, (*Store).clone, func(next *Store) (kernel.Accepted[ReactivatedResponse], *kernel.Rejection) {
		if next.status != Suspended {
			return kernel.Refuse[ReactivatedResponse](CodeStoreNotSuspended, "store is not suspended")
		}
		next.status = Active
		next.suspensionReason = ""
		next.statusChangedAt = cmd.At
		return kernel.Accept(
			ReactivatedResponse{Ref: next.ref},
			StoreReactivated{Ref: next.ref, At: cmd.At},
		), nil
	})
}

// ValidRef keeps the ref a DNS label, so it stays usable as a subdomain.
func ValidRef(ref StoreRef) bool {
	if len(ref) < minRefLength || len(ref) > maxRefLength || ref[0] == '-' || ref[len(ref)-1] == '-' {
		return false
	}
	for _, c := range []byte(ref) {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}
