package application

import (
	"context"
	"fmt"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

func (s Service) SuspendStore(ctx context.Context, cmd SuspendStore) (usecase.Outcome[domain.SuspendedResponse], error) {
	return usecase.Execute(ctx, s.executor(), command[domain.SuspendedResponse]{
		Operation:   OperationSuspendStore,
		Object:      string(cmd.Ref),
		Input:       cmd,
		Fingerprint: usecase.NewFingerprint(OperationSuspendStore).String(string(cmd.Ref)).String(cmd.Reason),
		Codec:       suspendedCodec,
		Run: func(ctx context.Context, res Resources, identity usecase.Identity) (usecase.Outcome[domain.SuspendedResponse], error) {
			outcome, err := usecase.Decide(ctx, res.Stores, res.Outbox, origin(cmd.Ref), cmd.Ref, identity,
				loadExistingStore,
				func(st *domain.Store) (kernel.Accepted[domain.SuspendedResponse], *kernel.Rejection) {
					return st.Suspend(domain.SuspendStore{Reason: cmd.Reason, At: identity.OccurredAt})
				})
			if err != nil {
				return outcome, fmt.Errorf("application: suspend %s: %w", cmd.Ref, err)
			}
			return outcome, nil
		},
	})
}
