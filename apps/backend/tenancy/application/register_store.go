package application

import (
	"context"
	"fmt"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

func (s Service) RegisterStore(ctx context.Context, cmd RegisterStore) (usecase.Outcome[domain.RegisteredResponse], error) {
	return usecase.Execute(ctx, s.executor(), command[domain.RegisteredResponse]{
		Operation:   OperationRegisterStore,
		Object:      string(cmd.Ref),
		Input:       cmd,
		Fingerprint: usecase.NewFingerprint(OperationRegisterStore).String(string(cmd.Ref)).String(string(cmd.Name)),
		Codec:       registeredCodec,
		Run: func(ctx context.Context, res Resources, identity usecase.Identity) (usecase.Outcome[domain.RegisteredResponse], error) {
			outcome, err := usecase.Decide(ctx, res.Stores, res.Outbox, origin(cmd.Ref), cmd.Ref, identity,
				loadStore,
				func(st *domain.Store) (kernel.Accepted[domain.RegisteredResponse], *kernel.Rejection) {
					return st.Register(domain.RegisterStore{Ref: cmd.Ref, Name: cmd.Name, At: identity.OccurredAt})
				})
			if err != nil {
				return outcome, fmt.Errorf("application: register %s: %w", cmd.Ref, err)
			}
			return outcome, nil
		},
	})
}
