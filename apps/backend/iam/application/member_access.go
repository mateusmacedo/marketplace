package application

import (
	"context"
	"fmt"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func (s Service) GrantRole(ctx context.Context, cmd GrantRole) (usecase.Outcome[domain.GrantedResponse], error) {
	return usecase.Execute(ctx, s.executor(), command[domain.GrantedResponse]{
		Operation:   OperationGrantRole,
		Object:      string(cmd.Subject),
		Input:       cmd,
		Fingerprint: usecase.NewFingerprint(OperationGrantRole).String(string(cmd.Subject)).String(string(cmd.Role)),
		Codec:       grantedCodec,
		Run: func(ctx context.Context, res Resources, identity usecase.Identity) (usecase.Outcome[domain.GrantedResponse], error) {
			outcome, err := usecase.Decide(ctx, res.Memberships, res.Outbox, membershipOrigin(cmd.Subject), cmd.Subject, identity,
				loadExistingMembership,
				func(m *domain.Membership) (kernel.Accepted[domain.GrantedResponse], *kernel.Rejection) {
					return m.Grant(domain.GrantRole{Role: cmd.Role, At: identity.OccurredAt})
				})
			if err != nil {
				return outcome, fmt.Errorf("application: grant role %s: %w", cmd.Subject, err)
			}
			return outcome, nil
		},
	})
}

func (s Service) RevokeRole(ctx context.Context, cmd RevokeRole) (usecase.Outcome[domain.RevokedResponse], error) {
	return usecase.Execute(ctx, s.executor(), command[domain.RevokedResponse]{
		Operation:   OperationRevokeRole,
		Object:      string(cmd.Subject),
		Input:       cmd,
		Fingerprint: usecase.NewFingerprint(OperationRevokeRole).String(string(cmd.Subject)).String(string(cmd.Role)),
		Codec:       revokedCodec,
		Run: func(ctx context.Context, res Resources, identity usecase.Identity) (usecase.Outcome[domain.RevokedResponse], error) {
			outcome, err := usecase.Decide(ctx, res.Memberships, res.Outbox, membershipOrigin(cmd.Subject), cmd.Subject, identity,
				loadExistingMembership,
				func(m *domain.Membership) (kernel.Accepted[domain.RevokedResponse], *kernel.Rejection) {
					return m.Revoke(domain.RevokeRole{Role: cmd.Role, At: identity.OccurredAt})
				})
			if err != nil {
				return outcome, fmt.Errorf("application: revoke role %s: %w", cmd.Subject, err)
			}
			return outcome, nil
		},
	})
}

func (s Service) RemoveMember(ctx context.Context, cmd RemoveMember) (usecase.Outcome[domain.RemovedResponse], error) {
	return usecase.Execute(ctx, s.executor(), command[domain.RemovedResponse]{
		Operation:   OperationRemoveMember,
		Object:      string(cmd.Subject),
		Input:       cmd,
		Fingerprint: usecase.NewFingerprint(OperationRemoveMember).String(string(cmd.Subject)),
		Codec:       removedCodec,
		Run: func(ctx context.Context, res Resources, identity usecase.Identity) (usecase.Outcome[domain.RemovedResponse], error) {
			outcome, err := usecase.Decide(ctx, res.Memberships, res.Outbox, membershipOrigin(cmd.Subject), cmd.Subject, identity,
				loadExistingMembership,
				func(m *domain.Membership) (kernel.Accepted[domain.RemovedResponse], *kernel.Rejection) {
					return m.Remove(domain.RemoveMember{At: identity.OccurredAt})
				})
			if err != nil {
				return outcome, fmt.Errorf("application: remove member %s: %w", cmd.Subject, err)
			}
			return outcome, nil
		},
	})
}
