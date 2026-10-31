//go:build integration

package provider_test

import (
	"context"
	"slices"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/clock"
	testids "github.com/mateusmacedo/dmpf/libs/backend/go/testkit/ids"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/pg"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/appkit"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

// The command path is the real one: the claim registers in the command inbox,
// and the violation of the owner constraint on Save still answers owner-claimed
// and commits the refusal, leaving every membership as it was.
func TestASecondClaimOverPostgresIsOwnerClaimedAndNoMembershipChanges(t *testing.T) {
	for name, member := range map[string]*domain.MembershipSnapshot{
		"subject without a membership": nil,
		"active member of another role": {
			Subject: "kc-123", Email: "ana@loja.com", Roles: []domain.RoleRef{"expedicao"}, Status: domain.MembershipStatusActive,
			AccessVersion: 1, JoinedAt: 1, ChangedAt: 1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := appkit.NewIam(t, clock.New(e2eOccurred), &testids.Sequence{Prefix: "m-"})
			ctx := context.Background()
			if member != nil {
				pg.Seed(t, withExecution(t, ctx), h.Pool, provider.NewMembershipRepository, member.Subject, *member)
			}
			if outcome, err := h.Service.ClaimOwnership(withExecution(t, ctx), application.ClaimOwnership{Subject: "kc-owner"}); err != nil || outcome.Response().Subject != "kc-owner" {
				t.Fatalf("ClaimOwnership(kc-owner) = (%+v, %v), want the owner established", outcome, err)
			}
			reader := provider.NewMembershipReader(postgres.NewReadPool(h.Pool))

			outcome, err := h.Service.ClaimOwnership(withExecution(t, ctx), application.ClaimOwnership{Subject: "kc-123"})

			rejection, refused := outcome.Rejection()
			if err != nil || !refused || rejection.Code() != domain.CodeMembershipOwnerClaimed {
				t.Fatalf("ClaimOwnership(kc-123) = (%+v, %v), want %s", outcome, err, domain.CodeMembershipOwnerClaimed)
			}
			owner, version, err := reader.Load(withExecution(t, ctx), "kc-owner")
			if err != nil || version != 1 || !slices.Equal(owner.Roles, []domain.RoleRef{domain.OwnerRole}) {
				t.Fatalf("owner = %+v v%d, %v; want it untouched at v1", owner, version, err)
			}
			got, version, err := reader.Load(withExecution(t, ctx), "kc-123")
			switch {
			case member == nil && err == nil:
				t.Fatalf("kc-123 = %+v v%d, want no membership written", got, version)
			case member != nil && (err != nil || version != 1 || !got.Equal(*member)):
				t.Fatalf("kc-123 = %+v v%d, %v; want it untouched at v1", got, version, err)
			}
			if outbox := h.Outbox(t); len(outbox) != 1 {
				t.Fatalf("outbox = %+v, want only the establishment of the owner", outbox)
			}
		})
	}
}

func TestTheOwnerClaimingAgainOverPostgresLeavesNoEvent(t *testing.T) {
	h := appkit.NewIam(t, clock.New(e2eOccurred), &testids.Sequence{Prefix: "m-"})
	for range 2 {
		if outcome, err := h.Service.ClaimOwnership(withExecution(t, context.Background()), application.ClaimOwnership{Subject: "kc-owner"}); err != nil {
			t.Fatalf("ClaimOwnership() = (%+v, %v)", outcome, err)
		}
	}

	if outbox := h.Outbox(t); len(outbox) != 1 || outbox[0].MessageType != "com.company.iam.membership-established.v1" {
		t.Fatalf("outbox = %+v, want the one establishment", outbox)
	}
}
