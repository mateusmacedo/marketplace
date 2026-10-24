package provider

import (
	"encoding/json"
	"errors"
	"fmt"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

var errUnstoredStatus = errors.New("provider: store status has no stored value")

// The status column holds these values, fixed here so a rename in the domain
// does not rewrite stored rows nor the stores_status_check of the schema.
const (
	statusActive    = "active"
	statusSuspended = "suspended"
)

// storeState is the snapshot column: the fields no query filters by, under
// JSON names fixed here for the same reason.
type storeState struct {
	Name             string `json:"name"`
	SuspensionReason string `json:"suspensionReason"`
	RegisteredAt     int64  `json:"registeredAt"`
	StatusChangedAt  int64  `json:"statusChangedAt"`
}

// storeTable declares how the Store snapshot maps to columns. The statements,
// and with them the tenant predicate, are the kernel's (IDN-14).
var storeTable = postgres.Table[domain.StoreRef, domain.StoreSnapshot]{
	Name:     "stores",
	IDColumn: "store_id",
	Columns:  []string{"status", "snapshot"},

	Encode: func(s domain.StoreSnapshot) ([]any, error) {
		status, err := statusValue(s.Status)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(storeState{
			Name:             string(s.Name),
			SuspensionReason: s.SuspensionReason,
			RegisteredAt:     int64(s.RegisteredAt),
			StatusChangedAt:  int64(s.StatusChangedAt),
		})
		if err != nil {
			return nil, err
		}
		return []any{status, raw}, nil
	},

	Decode: func(scan func(dest ...any) error) (domain.StoreSnapshot, error) {
		var (
			status string
			raw    []byte
		)
		if err := scan(&status, &raw); err != nil {
			return domain.StoreSnapshot{}, err
		}
		parsed, err := statusOf(status)
		if err != nil {
			return domain.StoreSnapshot{}, err
		}
		var state storeState
		if err := json.Unmarshal(raw, &state); err != nil {
			return domain.StoreSnapshot{}, err
		}
		return domain.StoreSnapshot{
			Name:             domain.StoreName(state.Name),
			Status:           parsed,
			SuspensionReason: state.SuspensionReason,
			RegisteredAt:     kernel.Instant(state.RegisteredAt),
			StatusChangedAt:  kernel.Instant(state.StatusChangedAt),
		}, nil
	},

	WithID: func(s domain.StoreSnapshot, ref domain.StoreRef) domain.StoreSnapshot {
		s.Ref = ref
		return s
	},

	Unique: []postgres.Unique{{
		Name:      "stores_tenant_id_name_key",
		Columns:   []string{"tenant_id", "name"},
		Rejection: domain.RejectStoreDuplicateName,
	}},
}

func NewStoreRepository(tx *postgres.Tx) ports.Repository[domain.StoreRef, domain.StoreSnapshot] {
	return storeTable.Repository(tx)
}

func statusValue(status domain.StoreStatus) (string, error) {
	switch status {
	case domain.Active:
		return statusActive, nil
	case domain.Suspended:
		return statusSuspended, nil
	default:
		return "", fmt.Errorf("%w: %s", errUnstoredStatus, status)
	}
}

func statusOf(value string) (domain.StoreStatus, error) {
	switch value {
	case statusActive:
		return domain.Active, nil
	case statusSuspended:
		return domain.Suspended, nil
	default:
		return domain.Unregistered, fmt.Errorf("%w: %q", errUnstoredStatus, value)
	}
}
