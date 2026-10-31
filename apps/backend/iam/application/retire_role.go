package application

import (
	"context"
	"fmt"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func (s Service) RetireRole(ctx context.Context, cmd RetireRole) (usecase.Outcome[domain.RetiredResponse], error) {
	return usecase.Execute(ctx, s.executor(), command[domain.RetiredResponse]{
		Operation:   OperationRetireRole,
		Object:      string(cmd.Ref),
		Input:       cmd,
		Fingerprint: usecase.NewFingerprint(OperationRetireRole).String(string(cmd.Ref)),
		Codec:       retiredCodec,
		Run: func(ctx context.Context, res Resources, identity usecase.Identity) (usecase.Outcome[domain.RetiredResponse], error) {
			outcome, err := usecase.Decide(ctx, res.Roles, res.Outbox, roleOrigin(cmd.Ref), cmd.Ref, identity,
				loadExistingRole,
				func(r *domain.Role) (kernel.Accepted[domain.RetiredResponse], *kernel.Rejection) {
					return r.Retire(domain.RetireRole{At: identity.OccurredAt})
				})
			if err != nil {
				return outcome, fmt.Errorf("application: retire role %s: %w", cmd.Ref, err)
			}
			return outcome, nil
		},
	})
}
