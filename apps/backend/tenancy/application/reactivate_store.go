package application

import (
	"context"
	"fmt"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

func (s Service) ReactivateStore(ctx context.Context, cmd ReactivateStore) (usecase.Outcome[domain.ReactivatedResponse], error) {
	return usecase.Execute(ctx, s.executor(), command[domain.ReactivatedResponse]{
		Operation:   OperationReactivateStore,
		Object:      string(cmd.Ref),
		Input:       cmd,
		Fingerprint: usecase.NewFingerprint(OperationReactivateStore).String(string(cmd.Ref)),
		Codec:       reactivatedCodec,
		Run: func(ctx context.Context, res Resources, identity usecase.Identity) (usecase.Outcome[domain.ReactivatedResponse], error) {
			outcome, err := usecase.Decide(ctx, res.Stores, res.Outbox, origin(cmd.Ref), cmd.Ref, identity,
				loadExistingStore,
				func(st *domain.Store) (kernel.Accepted[domain.ReactivatedResponse], *kernel.Rejection) {
					return st.Reactivate(domain.ReactivateStore{At: identity.OccurredAt})
				})
			if err != nil {
				return outcome, fmt.Errorf("application: reactivate %s: %w", cmd.Ref, err)
			}
			return outcome, nil
		},
	})
}
