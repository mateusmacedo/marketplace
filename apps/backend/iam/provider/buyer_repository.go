package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

var errUnstoredBuyerStatus = errors.New("provider: buyer status has no stored value")

// The status column holds these values, fixed here so a rename in the domain
// does not rewrite stored rows nor the buyers_status_check of the schema.
const (
	buyerStatusActive  = "active"
	buyerStatusBlocked = "blocked"
)

// buyerState is the snapshot column, under JSON names fixed here.
type buyerState struct {
	Name          string `json:"name"`
	BlockReason   string `json:"blockReason"`
	AccessVersion int64  `json:"accessVersion"`
	SyncedVersion int64  `json:"syncedVersion"`
	RegisteredAt  int64  `json:"registeredAt"`
	ChangedAt     int64  `json:"changedAt"`
}

// buyerTable keys the buyer by its subject in the operator's tenant. The
// statements, and with them the tenant predicate, are the kernel's (IDN-14).
var buyerTable = postgres.Table[domain.Subject, domain.BuyerSnapshot]{
	Name:     "buyers",
	IDColumn: "buyer_id",
	Columns:  []string{"status", "snapshot"},

	Encode: func(s domain.BuyerSnapshot) ([]any, error) {
		status, err := buyerStatusValue(s.Status)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(buyerState{
			Name:          string(s.Name),
			BlockReason:   s.BlockReason,
			AccessVersion: s.AccessVersion,
			SyncedVersion: s.SyncedVersion,
			RegisteredAt:  int64(s.RegisteredAt),
			ChangedAt:     int64(s.ChangedAt),
		})
		if err != nil {
			return nil, err
		}
		return []any{status, raw}, nil
	},

	Decode: func(scan func(dest ...any) error) (domain.BuyerSnapshot, error) {
		var (
			status string
			raw    []byte
		)
		if err := scan(&status, &raw); err != nil {
			return domain.BuyerSnapshot{}, err
		}
		parsed, err := buyerStatusOf(status)
		if err != nil {
			return domain.BuyerSnapshot{}, err
		}
		var state buyerState
		if err := json.Unmarshal(raw, &state); err != nil {
			return domain.BuyerSnapshot{}, err
		}
		return domain.BuyerSnapshot{
			Name:          domain.BuyerName(state.Name),
			Status:        parsed,
			BlockReason:   state.BlockReason,
			AccessVersion: state.AccessVersion,
			SyncedVersion: state.SyncedVersion,
			RegisteredAt:  kernel.Instant(state.RegisteredAt),
			ChangedAt:     kernel.Instant(state.ChangedAt),
		}, nil
	},

	WithID: func(s domain.BuyerSnapshot, subject domain.Subject) domain.BuyerSnapshot {
		s.Subject = subject
		return s
	},
}

func NewBuyerRepository(tx *postgres.Tx) ports.Repository[domain.Subject, domain.BuyerSnapshot] {
	return buyerTable.Repository(tx)
}

// NewBuyerReader serves the read side without the write side (UOW-11).
func NewBuyerReader(pool postgres.ReadPool) ports.Reader[domain.Subject, domain.BuyerSnapshot] {
	return buyerTable.Reader(pool)
}

var buyersByStatus = buyerTable.Relation("status")

type BuyersByStatusReader struct{ pool postgres.ReadPool }

func NewBuyersByStatusReader(pool postgres.ReadPool) *BuyersByStatusReader {
	return &BuyersByStatusReader{pool: pool}
}

func (r *BuyersByStatusReader) LoadByStatus(ctx context.Context, status domain.BuyerStatus) ([]domain.BuyerSnapshot, error) {
	value, err := buyerStatusValue(status)
	if err != nil {
		return nil, err
	}
	return buyersByStatus.Query(ctx, r.pool, value)
}

func buyerStatusValue(status domain.BuyerStatus) (string, error) {
	switch status {
	case domain.BuyerStatusActive:
		return buyerStatusActive, nil
	case domain.BuyerStatusBlocked:
		return buyerStatusBlocked, nil
	default:
		return "", fmt.Errorf("%w: %s", errUnstoredBuyerStatus, status)
	}
}

func buyerStatusOf(value string) (domain.BuyerStatus, error) {
	switch value {
	case buyerStatusActive:
		return domain.BuyerStatusActive, nil
	case buyerStatusBlocked:
		return domain.BuyerStatusBlocked, nil
	default:
		return domain.BuyerStatusUndefined, fmt.Errorf("%w: %q", errUnstoredBuyerStatus, value)
	}
}
