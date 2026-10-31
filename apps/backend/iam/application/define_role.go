package application

import (
	"context"
	"fmt"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func (s Service) DefineRole(ctx context.Context, cmd DefineRole) (usecase.Outcome[domain.DefinedResponse], error) {
	return usecase.Execute(ctx, s.executor(), command[domain.DefinedResponse]{
		Operation:   OperationDefineRole,
		Object:      string(cmd.Ref),
		Input:       cmd,
		Fingerprint: withList(usecase.NewFingerprint(OperationDefineRole).String(string(cmd.Ref)).String(string(cmd.Name)), cmd.Permissions),
		Codec:       definedCodec,
		Run: func(ctx context.Context, res Resources, identity usecase.Identity) (usecase.Outcome[domain.DefinedResponse], error) {
			outcome, err := usecase.Decide(ctx, res.Roles, res.Outbox, roleOrigin(cmd.Ref), cmd.Ref, identity,
				loadRole,
				func(r *domain.Role) (kernel.Accepted[domain.DefinedResponse], *kernel.Rejection) {
					return r.Define(domain.DefineRole{Ref: cmd.Ref, Name: cmd.Name, Permissions: cmd.Permissions, At: identity.OccurredAt})
				})
			if err != nil {
				return outcome, fmt.Errorf("application: define role %s: %w", cmd.Ref, err)
			}
			return outcome, nil
		},
	})
}
