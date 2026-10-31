package application

import (
	"context"
	"fmt"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

// ClaimOwnership establishes the subject as the owner without reading who owns
// the store: a second owner collides on the unique active owner when saved,
// and Decide answers that collision as owner-claimed (ADR-066).
func (s Service) ClaimOwnership(ctx context.Context, cmd ClaimOwnership) (usecase.Outcome[domain.EstablishedResponse], error) {
	return usecase.Execute(ctx, s.executor(), command[domain.EstablishedResponse]{
		Operation:   OperationClaimOwnership,
		Object:      string(cmd.Subject),
		Input:       cmd,
		Fingerprint: usecase.NewFingerprint(OperationClaimOwnership).String(string(cmd.Subject)),
		Codec:       claimedCodec,
		Run: func(ctx context.Context, res Resources, identity usecase.Identity) (usecase.Outcome[domain.EstablishedResponse], error) {
			outcome, err := usecase.Decide(ctx, res.Memberships, res.Outbox, membershipOrigin(cmd.Subject), cmd.Subject, identity,
				loadMembership,
				func(m *domain.Membership) (kernel.Accepted[domain.EstablishedResponse], *kernel.Rejection) {
					return m.Establish(domain.EstablishMembership{Subject: cmd.Subject, Roles: []domain.RoleRef{domain.OwnerRole}, At: identity.OccurredAt})
				})
			if err != nil {
				return outcome, fmt.Errorf("application: claim ownership %s: %w", cmd.Subject, err)
			}
			return outcome, nil
		},
	})
}
