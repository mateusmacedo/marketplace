package ports_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/ports"
)

// rolesFunc realizes RolesByStatusReader over a function, so the contract is
// exercised without reaching the provider block.
type rolesFunc func(context.Context, domain.RoleStatus) ([]domain.RoleSnapshot, error)

func (f rolesFunc) LoadByStatus(ctx context.Context, status domain.RoleStatus) ([]domain.RoleSnapshot, error) {
	return f(ctx, status)
}

func TestLoadByStatusAnswersWithEveryRoleInThatStatus(t *testing.T) {
	active := []domain.RoleSnapshot{
		{Ref: "expedicao", Name: "Expedição", Permissions: []domain.Permission{"fulfillment:write"}, Status: domain.RoleStatusActive, AccessVersion: 1},
		{Ref: "catalogo", Name: "Catálogo", Permissions: []domain.Permission{"catalog:write"}, Status: domain.RoleStatusActive, AccessVersion: 1},
	}
	var asked domain.RoleStatus
	var reader ports.RolesByStatusReader = rolesFunc(func(_ context.Context, status domain.RoleStatus) ([]domain.RoleSnapshot, error) {
		asked = status
		return active, nil
	})

	got, err := reader.LoadByStatus(context.Background(), domain.RoleStatusActive)

	if err != nil {
		t.Fatalf("LoadByStatus returned %v, want no error", err)
	}
	if asked != domain.RoleStatusActive {
		t.Fatalf("the reader was keyed on %v, want active", asked)
	}
	if len(got) != len(active) || got[0].Ref != "expedicao" || got[1].Ref != "catalogo" {
		t.Fatalf("LoadByStatus returned %+v, want %+v: the query answers with a collection", got, active)
	}
}

func TestLoadByStatusAnswersWithNoRoleWithoutFailing(t *testing.T) {
	var reader ports.RolesByStatusReader = rolesFunc(func(context.Context, domain.RoleStatus) ([]domain.RoleSnapshot, error) {
		return nil, nil
	})

	got, err := reader.LoadByStatus(context.Background(), domain.RoleStatusRetired)

	if err != nil || len(got) != 0 {
		t.Fatalf("LoadByStatus = %+v, %v; want an empty answer, not a failure", got, err)
	}
}

func TestLoadByStatusKeepsTheCallerContext(t *testing.T) {
	type key struct{}
	parent := context.WithValue(context.Background(), key{}, "marker")
	var seen context.Context
	var reader ports.RolesByStatusReader = rolesFunc(func(ctx context.Context, _ domain.RoleStatus) ([]domain.RoleSnapshot, error) {
		seen = ctx
		return nil, nil
	})

	if _, err := reader.LoadByStatus(parent, domain.RoleStatusActive); err != nil {
		t.Fatalf("LoadByStatus returned %v, want no error", err)
	}

	if seen.Value(key{}) != "marker" {
		t.Fatal("LoadByStatus dropped the caller context; deadline and tenant must reach the provider")
	}
}

func TestLoadByStatusSurfacesTheProviderErrorUntranslated(t *testing.T) {
	cause := errors.New("connection reset by peer")
	var reader ports.RolesByStatusReader = rolesFunc(func(context.Context, domain.RoleStatus) ([]domain.RoleSnapshot, error) {
		return nil, cause
	})

	got, err := reader.LoadByStatus(context.Background(), domain.RoleStatusActive)

	if !errors.Is(err, cause) || got != nil {
		t.Fatalf("LoadByStatus = %+v, %v; want no roles and an error carrying %v", got, err, cause)
	}
}

type invitationsFunc func(context.Context, domain.InvitationStatus) ([]domain.InvitationSnapshot, error)

func (f invitationsFunc) LoadByStatus(ctx context.Context, status domain.InvitationStatus) ([]domain.InvitationSnapshot, error) {
	return f(ctx, status)
}

type membershipsFunc func(context.Context, domain.MembershipStatus) ([]domain.MembershipSnapshot, error)

func (f membershipsFunc) LoadByStatus(ctx context.Context, status domain.MembershipStatus) ([]domain.MembershipSnapshot, error) {
	return f(ctx, status)
}

func TestTheInvitationsAndMembershipsByStatusAnswerTheStatusTheyWereAskedFor(t *testing.T) {
	var invitations ports.InvitationsByStatusReader = invitationsFunc(func(_ context.Context, status domain.InvitationStatus) ([]domain.InvitationSnapshot, error) {
		return []domain.InvitationSnapshot{{Email: "ana@loja.com", Status: status}}, nil
	})
	var memberships ports.MembershipsByStatusReader = membershipsFunc(func(_ context.Context, status domain.MembershipStatus) ([]domain.MembershipSnapshot, error) {
		return []domain.MembershipSnapshot{{Subject: "kc-123", Status: status}}, nil
	})

	pending, err := invitations.LoadByStatus(context.Background(), domain.InvitationStatusPending)
	if err != nil || len(pending) != 1 || pending[0].Status != domain.InvitationStatusPending {
		t.Fatalf("invitations.LoadByStatus(pending) = %+v, %v", pending, err)
	}
	removed, err := memberships.LoadByStatus(context.Background(), domain.MembershipStatusRemoved)
	if err != nil || len(removed) != 1 || removed[0].Status != domain.MembershipStatusRemoved {
		t.Fatalf("memberships.LoadByStatus(removed) = %+v, %v", removed, err)
	}
}

func TestTheInvitationsAndMembershipsByStatusSurfaceTheProviderError(t *testing.T) {
	cause := errors.New("connection reset by peer")
	var invitations ports.InvitationsByStatusReader = invitationsFunc(func(context.Context, domain.InvitationStatus) ([]domain.InvitationSnapshot, error) {
		return nil, cause
	})
	var memberships ports.MembershipsByStatusReader = membershipsFunc(func(context.Context, domain.MembershipStatus) ([]domain.MembershipSnapshot, error) {
		return nil, cause
	})

	if _, err := invitations.LoadByStatus(context.Background(), domain.InvitationStatusFailed); !errors.Is(err, cause) {
		t.Fatalf("invitations.LoadByStatus() = %v, want %v", err, cause)
	}
	if _, err := memberships.LoadByStatus(context.Background(), domain.MembershipStatusActive); !errors.Is(err, cause) {
		t.Fatalf("memberships.LoadByStatus() = %v, want %v", err, cause)
	}
}

type buyersFunc func(context.Context, domain.BuyerStatus) ([]domain.BuyerSnapshot, error)

func (f buyersFunc) LoadByStatus(ctx context.Context, status domain.BuyerStatus) ([]domain.BuyerSnapshot, error) {
	return f(ctx, status)
}

func TestTheBuyersByStatusAnswerTheStatusTheyWereAskedForAndSurfaceTheError(t *testing.T) {
	cause := errors.New("connection reset by peer")
	var reader ports.BuyersByStatusReader = buyersFunc(func(_ context.Context, status domain.BuyerStatus) ([]domain.BuyerSnapshot, error) {
		if status == domain.BuyerStatusActive {
			return nil, cause
		}
		return []domain.BuyerSnapshot{{Subject: "kc-123", Status: status}}, nil
	})

	blocked, err := reader.LoadByStatus(context.Background(), domain.BuyerStatusBlocked)
	if err != nil || len(blocked) != 1 || blocked[0].Status != domain.BuyerStatusBlocked {
		t.Fatalf("LoadByStatus(blocked) = %+v, %v", blocked, err)
	}
	if _, err := reader.LoadByStatus(context.Background(), domain.BuyerStatusActive); !errors.Is(err, cause) {
		t.Fatalf("LoadByStatus(active) = %v, want %v", err, cause)
	}
}
