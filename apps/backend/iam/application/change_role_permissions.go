package application

import (
	"context"
	"fmt"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func (s Service) ChangeRolePermissions(ctx context.Context, cmd ChangeRolePermissions) (usecase.Outcome[domain.PermissionsChangedResponse], error) {
	return usecase.Execute(ctx, s.executor(), command[domain.PermissionsChangedResponse]{
		Operation:   OperationChangeRolePermissions,
		Object:      string(cmd.Ref),
		Input:       cmd,
		Fingerprint: withList(usecase.NewFingerprint(OperationChangeRolePermissions).String(string(cmd.Ref)), cmd.Permissions),
		Codec:       permissionsChangedCodec,
		Run: func(ctx context.Context, res Resources, identity usecase.Identity) (usecase.Outcome[domain.PermissionsChangedResponse], error) {
			outcome, err := usecase.Decide(ctx, res.Roles, res.Outbox, roleOrigin(cmd.Ref), cmd.Ref, identity,
				loadExistingRole,
				func(r *domain.Role) (kernel.Accepted[domain.PermissionsChangedResponse], *kernel.Rejection) {
					return r.ChangePermissions(domain.ChangeRolePermissions{Permissions: cmd.Permissions, At: identity.OccurredAt})
				})
			if err != nil {
				return outcome, fmt.Errorf("application: change role permissions %s: %w", cmd.Ref, err)
			}
			return outcome, nil
		},
	})
}
