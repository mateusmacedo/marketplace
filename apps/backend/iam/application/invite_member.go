package application

import (
	"context"
	"fmt"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func (s Service) InviteMember(ctx context.Context, cmd InviteMember) (usecase.Outcome[domain.InvitedResponse], error) {
	email := domain.NormalizeEmail(cmd.Email)
	return usecase.Execute(ctx, s.executor(), command[domain.InvitedResponse]{
		Operation:   OperationInviteMember,
		Object:      s.auditedEmail(email),
		Input:       cmd,
		Fingerprint: withList(usecase.NewFingerprint(OperationInviteMember).String(cmd.Email), cmd.Roles),
		Codec:       invitedCodec,
		Run: func(ctx context.Context, res Resources, identity usecase.Identity) (usecase.Outcome[domain.InvitedResponse], error) {
			outcome, err := usecase.Decide(ctx, res.Invitations, res.Outbox, invitationOrigin(email), email, identity,
				loadInvitation,
				func(i *domain.Invitation) (kernel.Accepted[domain.InvitedResponse], *kernel.Rejection) {
					return i.Invite(domain.InviteMember{Email: cmd.Email, Roles: cmd.Roles, At: identity.OccurredAt})
				})
			if err != nil {
				return outcome, fmt.Errorf("application: invite member: %w", err)
			}
			return outcome, nil
		},
	})
}

// auditedEmail keeps the address out of the audit trail while one address
// still joins its records: the SHA-256 the composition root injects in the
// policy, in hex, because this block imports no crypto (RFC 6.2).
func (s Service) auditedEmail(email domain.Email) string {
	if s.Idempotency.Digest == nil {
		return ""
	}
	digest := s.Idempotency.Digest([]byte(email))
	return fmt.Sprintf("%x", digest[:])
}
