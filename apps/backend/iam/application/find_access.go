package application

import (
	"context"
	"fmt"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func (s Service) FindInvitation(ctx context.Context, email domain.Email) (domain.InvitationSnapshot, error) {
	return usecase.Query[Operation](ctx, s.instrumentation(), s.Authorize, OperationFindInvitation, FindInvitation{Email: email},
		func(ctx context.Context) (domain.InvitationSnapshot, error) {
			snapshot, _, err := s.Invitations.Load(ctx, email)
			if err != nil {
				return domain.InvitationSnapshot{}, fmt.Errorf("application: find invitation: %w", err)
			}
			return snapshot, nil
		})
}

func (s Service) FindInvitationsByStatus(ctx context.Context, status domain.InvitationStatus) ([]domain.InvitationSnapshot, error) {
	return findByStatus(ctx, s, OperationFindInvitationsByStatus, FindInvitationsByStatus{Status: status}, "find invitations by status", status, s.InvitationsByStatus.LoadByStatus)
}

func (s Service) FindMembership(ctx context.Context, subject domain.Subject) (domain.MembershipSnapshot, error) {
	return usecase.Query[Operation](ctx, s.instrumentation(), s.Authorize, OperationFindMembership, FindMembership{Subject: subject},
		func(ctx context.Context) (domain.MembershipSnapshot, error) {
			snapshot, _, err := s.Memberships.Load(ctx, subject)
			if err != nil {
				return domain.MembershipSnapshot{}, fmt.Errorf("application: find membership %s: %w", subject, err)
			}
			return snapshot, nil
		})
}

func (s Service) FindMembershipsByStatus(ctx context.Context, status domain.MembershipStatus) ([]domain.MembershipSnapshot, error) {
	return findByStatus(ctx, s, OperationFindMembershipsByStatus, FindMembershipsByStatus{Status: status}, "find memberships by status", status, s.MembershipsByStatus.LoadByStatus)
}
