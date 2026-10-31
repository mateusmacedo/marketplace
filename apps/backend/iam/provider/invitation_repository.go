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

var errUnstoredInvitationStatus = errors.New("provider: invitation status has no stored value")

// The status column holds these values, fixed here so a rename in the domain
// does not rewrite stored rows nor the invitations_status_check of the schema.
const (
	invitationStatusPending     = "pending"
	invitationStatusProvisioned = "provisioned"
	invitationStatusFailed      = "failed"
)

// invitationState is the snapshot column, under JSON names fixed here.
type invitationState struct {
	Roles         []string `json:"roles"`
	Round         int64    `json:"round"`
	Subject       string   `json:"subject"`
	FailureReason string   `json:"failureReason"`
	InvitedAt     int64    `json:"invitedAt"`
	SettledAt     int64    `json:"settledAt"`
}

// invitationTable keys the invitation by its normalized address. The
// statements, and with them the tenant predicate, are the kernel's (IDN-14).
var invitationTable = postgres.Table[domain.Email, domain.InvitationSnapshot]{
	Name:     "invitations",
	IDColumn: "invitation_id",
	Columns:  []string{"status", "snapshot"},

	Encode: func(s domain.InvitationSnapshot) ([]any, error) {
		status, err := invitationStatusValue(s.Status)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(invitationState{
			Roles:         names(s.Roles),
			Round:         s.Round,
			Subject:       string(s.Subject),
			FailureReason: s.FailureReason,
			InvitedAt:     int64(s.InvitedAt),
			SettledAt:     int64(s.SettledAt),
		})
		if err != nil {
			return nil, err
		}
		return []any{status, raw}, nil
	},

	Decode: func(scan func(dest ...any) error) (domain.InvitationSnapshot, error) {
		var (
			status string
			raw    []byte
		)
		if err := scan(&status, &raw); err != nil {
			return domain.InvitationSnapshot{}, err
		}
		parsed, err := invitationStatusOf(status)
		if err != nil {
			return domain.InvitationSnapshot{}, err
		}
		var state invitationState
		if err := json.Unmarshal(raw, &state); err != nil {
			return domain.InvitationSnapshot{}, err
		}
		return domain.InvitationSnapshot{
			Roles:         valuesOf[domain.RoleRef](state.Roles),
			Status:        parsed,
			Round:         state.Round,
			Subject:       domain.Subject(state.Subject),
			FailureReason: state.FailureReason,
			InvitedAt:     kernel.Instant(state.InvitedAt),
			SettledAt:     kernel.Instant(state.SettledAt),
		}, nil
	},

	WithID: func(s domain.InvitationSnapshot, email domain.Email) domain.InvitationSnapshot {
		s.Email = email
		return s
	},
}

func NewInvitationRepository(tx *postgres.Tx) ports.Repository[domain.Email, domain.InvitationSnapshot] {
	return invitationTable.Repository(tx)
}

// NewInvitationReader serves the read side without the write side (UOW-11).
func NewInvitationReader(pool postgres.ReadPool) ports.Reader[domain.Email, domain.InvitationSnapshot] {
	return invitationTable.Reader(pool)
}

var invitationsByStatus = invitationTable.Relation("status")

type InvitationsByStatusReader struct{ pool postgres.ReadPool }

func NewInvitationsByStatusReader(pool postgres.ReadPool) *InvitationsByStatusReader {
	return &InvitationsByStatusReader{pool: pool}
}

func (r *InvitationsByStatusReader) LoadByStatus(ctx context.Context, status domain.InvitationStatus) ([]domain.InvitationSnapshot, error) {
	value, err := invitationStatusValue(status)
	if err != nil {
		return nil, err
	}
	return invitationsByStatus.Query(ctx, r.pool, value)
}

func invitationStatusValue(status domain.InvitationStatus) (string, error) {
	switch status {
	case domain.InvitationStatusPending:
		return invitationStatusPending, nil
	case domain.InvitationStatusProvisioned:
		return invitationStatusProvisioned, nil
	case domain.InvitationStatusFailed:
		return invitationStatusFailed, nil
	default:
		return "", fmt.Errorf("%w: %s", errUnstoredInvitationStatus, status)
	}
}

func invitationStatusOf(value string) (domain.InvitationStatus, error) {
	switch value {
	case invitationStatusPending:
		return domain.InvitationStatusPending, nil
	case invitationStatusProvisioned:
		return domain.InvitationStatusProvisioned, nil
	case invitationStatusFailed:
		return domain.InvitationStatusFailed, nil
	default:
		return domain.InvitationStatusUndefined, fmt.Errorf("%w: %q", errUnstoredInvitationStatus, value)
	}
}
