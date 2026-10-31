//go:build integration && distributed

package distkit

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/dist"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/app"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/appkit"
)

// RoleConsumer is the consumer of the context as the binary runs it, in a
// process of its own next to the relay's.
const RoleConsumer dist.Role = "consumer"

// OperatorTenant is what the consumer child is configured with: the tenant
// the provisioned invitations of the harness never belong to.
const OperatorTenant = "marketplace"

// Spec runs the relay and the consumer of this context from their own
// configuration, so the harness judges the production path end to end.
var Spec = dist.Spec{
	Name: "iam",
	Pool: appkit.PoolOptions,
	Roles: map[dist.Role]dist.RoleFunc{
		dist.RoleRelay: dist.Relay(func(ctx context.Context, getenv func(string) string) error {
			cfg, err := app.FromEnv(app.RoleRelay, getenv)
			if err != nil {
				return err
			}
			return app.Run(ctx, cfg)
		}),
		RoleConsumer: func(t *testing.T, ctx context.Context) {
			t.Helper()
			cfg, err := app.FromEnv(app.RoleConsumer, os.Getenv)
			if err != nil {
				t.Fatalf("distkit: consumer configuration: %v", err)
			}
			if err := app.Run(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("distkit: the consumer returned %v", err)
			}
		},
	},
}

func RunRole(t *testing.T) { dist.RunRole(t, Spec) }

// StartConsumer starts the consumer child with the operator tenant its
// configuration requires.
func StartConsumer(t testing.TB, h *dist.Harness) *dist.Process {
	t.Helper()
	return h.Start(t, RoleConsumer, "IAM_OPERATOR_TENANT="+OperatorTenant)
}
