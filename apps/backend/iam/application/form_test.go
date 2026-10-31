package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/memory"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

// refusedWithoutLoading is the sequence of a refusal decided before any read:
// the key is registered and completed with the rejection, nothing is loaded.
var refusedWithoutLoading = []string{
	"authorize", "clock.Now", "ids.NewMessageID", "within", "commands.Register", "commands.Complete", "commit",
}

type bodyIDCommand struct {
	name string
	code kernel.Code
	run  func(context.Context, *harness) (*kernel.Rejection, error)
}

func rejectionOf[R any](outcome usecase.Outcome[R], err error) (*kernel.Rejection, error) {
	rejection, _ := outcome.Rejection()
	return rejection, err
}

// The ids of the body carry U+0000 and the other control characters, which
// Postgres refuses to compare against a key: the domain judges them first.
var bodyIDCommands = []bodyIDCommand{
	{"DefineRole", domain.CodeRoleRefInvalid, func(ctx context.Context, h *harness) (*kernel.Rejection, error) {
		return rejectionOf(h.service.DefineRole(ctx, application.DefineRole{Ref: "a\u0000b", Name: testName, Permissions: testPermissions}))
	}},
	{"InviteMember", domain.CodeInvitationEmailInvalid, func(ctx context.Context, h *harness) (*kernel.Rejection, error) {
		return rejectionOf(h.service.InviteMember(ctx, application.InviteMember{Email: "ana\u0000@loja.com", Roles: []domain.RoleRef{"expedicao"}}))
	}},
	{"ClaimOwnership", domain.CodeMembershipSubjectInvalid, func(ctx context.Context, h *harness) (*kernel.Rejection, error) {
		return rejectionOf(h.service.ClaimOwnership(ctx, application.ClaimOwnership{Subject: "a\u0000b"}))
	}},
	{"RegisterBuyer", domain.CodeBuyerSubjectInvalid, func(ctx context.Context, h *harness) (*kernel.Rejection, error) {
		return rejectionOf(h.service.RegisterBuyer(ctx, application.RegisterBuyer{Subject: "a\u001bb", Name: "Bia"}))
	}},
}

func TestAnIDOfTheBodyOutsideTheFormIsRefusedWithItsCodeWithoutReading(t *testing.T) {
	for _, c := range bodyIDCommands {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, withLoadError(errBoom))

			rejection, err := c.run(withKey(t, context.Background(), "k-form"), h)

			if err != nil || rejection == nil || rejection.Code() != c.code {
				t.Fatalf("%s() = (%v, %v), want the rejection %s and no failure", c.name, rejection, err, c.code)
			}
			if !slices.Equal(h.rec.Observed(), refusedWithoutLoading) {
				t.Fatalf("sequence = %v, want %v: an id outside the form is never read", h.rec.Observed(), refusedWithoutLoading)
			}
			if n := h.outboxLen(); n != 0 {
				t.Fatalf("outbox = %d entries, want none", n)
			}
		})
	}
}

// The refusal is the inbox's like any other: the same key replays it.
func TestTheRefusalOfAnIDOutsideTheFormReplaysThroughTheInbox(t *testing.T) {
	for _, c := range bodyIDCommands {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			if _, err := c.run(withKey(t, context.Background(), "k-form"), h); err != nil {
				t.Fatalf("first %s() = %v", c.name, err)
			}
			h.rec.Reset()

			ctx := withKey(t, context.Background(), "k-form")
			rejection, err := c.run(ctx, h)

			if err != nil || rejection == nil || rejection.Code() != c.code {
				t.Fatalf("replayed %s() = (%v, %v), want the stored rejection %s", c.name, rejection, err, c.code)
			}
			if outcome, _ := ports.IdempotencyOutcomeFrom(ctx); outcome != ports.IdempotencyReplayed {
				t.Fatalf("idempotency outcome = %v, want replayed", outcome)
			}
		})
	}
}

type failingMembershipLoads struct {
	ports.Repository[domain.Subject, domain.MembershipSnapshot]
}

func (failingMembershipLoads) Load(context.Context, domain.Subject) (domain.MembershipSnapshot, ports.Version, error) {
	return domain.MembershipSnapshot{}, 0, errBoom
}

func TestAProvisionedSubjectOutsideTheFormIsR1D2WithoutReading(t *testing.T) {
	store := memory.New()
	svc := newConsumeService(store)
	svc.UoW = memory.NewUnitOfWork(store, func(tx *memory.Tx) application.Resources {
		res := consumeBind(tx)
		res.Memberships = failingMembershipLoads{Repository: res.Memberships}
		return res
	})

	disp, err := svc.ConsumeInvitationProvisioned(withExecution(t, context.Background()), provisioned("m-ext-1", "h1", "a\u0000b", "expedicao"))

	if err != nil || disp != usecase.R1D2 {
		t.Fatalf("Consume() = (%v, %v), want (%v, nil): the subject is refused before the read", disp, err, usecase.R1D2)
	}
	if last, _ := store.InboxLastError(membershipConsumer, "m-ext-1"); last != string(domain.CodeMembershipSubjectInvalid) {
		t.Fatalf("InboxLastError() = %q, want %q", last, domain.CodeMembershipSubjectInvalid)
	}
	if errors.Is(err, errBoom) || len(store.Entries()) != 0 {
		t.Fatalf("Entries() = %d, err = %v; want nothing written and no read", len(store.Entries()), err)
	}
}
