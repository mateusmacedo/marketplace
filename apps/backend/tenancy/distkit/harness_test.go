//go:build integration && distributed

package distkit_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/ids"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/dist"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/appkit"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/distkit"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

// registrations is how many decisions the harness enqueues before the drains
// start, so both have something to compete for.
const registrations = 6

// TestDistkitRole is the entry point of a re-executed child; it is inert in a
// normal run.
func TestDistkitRole(t *testing.T) { distkit.RunRole(t) }

func TestTwoDrainsPublishEveryRecordExactlyOnce(t *testing.T) {
	h := dist.New(t, distkit.Spec)
	enqueue(t, registrations)

	tb.Require(t, dist.RelayVector(t, h, registrations))
}

// enqueue drives the real use case, so what the drains carry is what the
// application authored and not a row a test wrote by hand. The message context
// is what makes a record drainable (OBX-08).
func enqueue(t *testing.T, n int) {
	t.Helper()
	h := appkit.NewTenancy(t, clock.New(ports.Instant(1)), &ids.Sequence{Prefix: "m-"})
	ctx := ports.WithMessageContext(context.Background(), ports.MessageContext{
		CorrelationID: "corr-distkit",
		CausationID:   "caus-distkit",
		Traceparent:   "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
	})

	for i := range n {
		ref := domain.StoreRef(fmt.Sprintf("loja-distkit-%d", i))
		registered, err := h.Service.RegisterStore(withExecution(t, ctx), application.RegisterStore{Ref: ref, Name: domain.StoreName(fmt.Sprintf("Loja distkit %d", i))})
		if err != nil {
			t.Fatalf("RegisterStore(%s) = %v, want nil", ref, err)
		}
		if rejection, refused := registered.Rejection(); refused {
			t.Fatalf("RegisterStore(%s) was rejected with %v, want accepted", ref, rejection)
		}
	}
}
