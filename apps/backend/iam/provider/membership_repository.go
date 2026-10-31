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

var errUnstoredMembershipStatus = errors.New("provider: membership status has no stored value")

// The status column holds these values, fixed here so a rename in the domain
// does not rewrite stored rows nor the generated owner column of the schema.
const (
	membershipStatusActive  = "active"
	membershipStatusRemoved = "removed"
)

// membershipState is the snapshot column; the generated owner column reads
// its roles key, so that name is part of the schema too.
type membershipState struct {
	Email         string   `json:"email"`
	Roles         []string `json:"roles"`
	AccessVersion int64    `json:"accessVersion"`
	SyncedVersion int64    `json:"syncedVersion"`
	JoinedAt      int64    `json:"joinedAt"`
	ChangedAt     int64    `json:"changedAt"`
}

// membershipTable keys the membership by its subject. Columns leaves the
// generated owner column out: the database derives it on every write.
var membershipTable = postgres.Table[domain.Subject, domain.MembershipSnapshot]{
	Name:     "memberships",
	IDColumn: "membership_id",
	Columns:  []string{"status", "snapshot"},

	Encode: func(s domain.MembershipSnapshot) ([]any, error) {
		status, err := membershipStatusValue(s.Status)
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(membershipState{
			Email:         string(s.Email),
			Roles:         names(s.Roles),
			AccessVersion: s.AccessVersion,
			SyncedVersion: s.SyncedVersion,
			JoinedAt:      int64(s.JoinedAt),
			ChangedAt:     int64(s.ChangedAt),
		})
		if err != nil {
			return nil, err
		}
		return []any{status, raw}, nil
	},

	Decode: func(scan func(dest ...any) error) (domain.MembershipSnapshot, error) {
		var (
			status string
			raw    []byte
		)
		if err := scan(&status, &raw); err != nil {
			return domain.MembershipSnapshot{}, err
		}
		parsed, err := membershipStatusOf(status)
		if err != nil {
			return domain.MembershipSnapshot{}, err
		}
		var state membershipState
		if err := json.Unmarshal(raw, &state); err != nil {
			return domain.MembershipSnapshot{}, err
		}
		return domain.MembershipSnapshot{
			Email:         domain.Email(state.Email),
			Roles:         valuesOf[domain.RoleRef](state.Roles),
			Status:        parsed,
			AccessVersion: state.AccessVersion,
			SyncedVersion: state.SyncedVersion,
			JoinedAt:      kernel.Instant(state.JoinedAt),
			ChangedAt:     kernel.Instant(state.ChangedAt),
		}, nil
	},

	WithID: func(s domain.MembershipSnapshot, subject domain.Subject) domain.MembershipSnapshot {
		s.Subject = subject
		return s
	},

	Unique: []postgres.Unique{{
		Name:      "memberships_tenant_id_owner_key",
		Columns:   []string{"tenant_id", "owner"},
		Rejection: domain.RejectMembershipOwnerClaimed,
	}},
}

func NewMembershipRepository(tx *postgres.Tx) ports.Repository[domain.Subject, domain.MembershipSnapshot] {
	return membershipTable.Repository(tx)
}

// NewMembershipReader serves the read side without the write side (UOW-11).
func NewMembershipReader(pool postgres.ReadPool) ports.Reader[domain.Subject, domain.MembershipSnapshot] {
	return membershipTable.Reader(pool)
}

var membershipsByStatus = membershipTable.Relation("status")

type MembershipsByStatusReader struct{ pool postgres.ReadPool }

func NewMembershipsByStatusReader(pool postgres.ReadPool) *MembershipsByStatusReader {
	return &MembershipsByStatusReader{pool: pool}
}

func (r *MembershipsByStatusReader) LoadByStatus(ctx context.Context, status domain.MembershipStatus) ([]domain.MembershipSnapshot, error) {
	value, err := membershipStatusValue(status)
	if err != nil {
		return nil, err
	}
	return membershipsByStatus.Query(ctx, r.pool, value)
}

func membershipStatusValue(status domain.MembershipStatus) (string, error) {
	switch status {
	case domain.MembershipStatusActive:
		return membershipStatusActive, nil
	case domain.MembershipStatusRemoved:
		return membershipStatusRemoved, nil
	default:
		return "", fmt.Errorf("%w: %s", errUnstoredMembershipStatus, status)
	}
}

func membershipStatusOf(value string) (domain.MembershipStatus, error) {
	switch value {
	case membershipStatusActive:
		return domain.MembershipStatusActive, nil
	case membershipStatusRemoved:
		return domain.MembershipStatusRemoved, nil
	default:
		return domain.MembershipStatusUndefined, fmt.Errorf("%w: %q", errUnstoredMembershipStatus, value)
	}
}
