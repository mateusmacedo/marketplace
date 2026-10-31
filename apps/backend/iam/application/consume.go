package application

import (
	"context"
	"errors"
	"fmt"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

// ConsumeInvitationProvisioned walks the seven steps of FND-04 §6.3: the
// subject Keycloak returned for an invitation becomes a member of the store
// with the email and the roles of the invitation. The broker effect of §6.4 is
// the adapter's, after the transaction (INB-08).
func (s Service) ConsumeInvitationProvisioned(ctx context.Context, cmd ConsumeInvitationProvisioned) (usecase.Disposition, error) {
	if err := s.Authorize(ctx, cmd); err != nil {
		return usecase.Classify(err), err
	}

	identity := usecase.ResolveIdentity(s.Clock, s.IDs, maxEventsPerCommand)

	var disposition usecase.Disposition
	err := s.UoW.Within(ctx, func(ctx context.Context, res Resources) error {
		reception, err := res.Inbox.Register(ctx, ports.Receipt{
			Consumer:    s.Consumer,
			MessageID:   cmd.MessageID,
			MessageType: cmd.MessageType,
			PayloadHash: cmd.PayloadHash,
			ReceivedAt:  cmd.ReceivedAt,
		})
		if err != nil {
			return err
		}
		return reception.Match(
			func(p ports.Pending) error { return s.establishFirst(ctx, res, cmd, identity, p, &disposition) },
			func() error { disposition = usecase.R2; return nil },
			func() error { disposition = usecase.R3; return nil },
			func() error { disposition = usecase.R4; return nil },
		)
	})
	if err != nil {
		return usecase.Classify(err), err
	}
	return disposition, nil
}

// establishFirst is axis 2 of §6.4, reached only under R1: D1, D2 or a
// technical failure, completing the pending reception accordingly.
func (s Service) establishFirst(ctx context.Context, res Resources, cmd ConsumeInvitationProvisioned, identity usecase.Identity, pending ports.Pending, disposition *usecase.Disposition) error {
	membership, stored, err := loadMembership(ctx, res.Memberships, cmd.Subject)
	if err != nil {
		return fmt.Errorf("application: consume invitation provisioned %s: %w", cmd.Subject, err)
	}

	accepted, rejection := membership.Establish(domain.EstablishMembership{Subject: cmd.Subject, Email: cmd.Email, Roles: cmd.Roles, At: identity.OccurredAt})
	if rejection == nil {
		err = res.Memberships.Save(ctx, cmd.Subject, membership.Snapshot(), stored)
		rejection, _ = usecase.DeclaredRejection(err)
	}
	if rejection != nil {
		*disposition = usecase.R1D2
		return pending.Complete(ctx, ports.Completion{Status: ports.StatusRejected, At: identity.OccurredAt, LastError: string(rejection.Code())})
	}
	if err != nil {
		if errors.Is(err, ports.ErrVersionConflict) {
			// MAP-07: a reread replays the decision instead of repeating one
			// already taken, so the conflict is retryable here.
			err = usecase.NewFailure(usecase.Conflict, true, err)
		}
		return fmt.Errorf("application: consume invitation provisioned %s: %w", cmd.Subject, err)
	}
	if err := usecase.Enqueue(ctx, res.Outbox, identity, membershipOrigin(cmd.Subject), stored+1, accepted.Events()); err != nil {
		return fmt.Errorf("application: consume invitation provisioned %s: enqueue: %w", cmd.Subject, err)
	}

	*disposition = usecase.R1D1
	return pending.Complete(ctx, ports.Completion{Status: ports.StatusProcessed, At: identity.OccurredAt})
}
