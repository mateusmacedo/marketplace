package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestAnUnknownEmailIsInvitedPendingUnderItsNormalizedAddress(t *testing.T) {
	h := newHarness(t)

	outcome, err := h.service.InviteMember(withExecution(t, context.Background()), application.InviteMember{Email: " Ana@Loja.com ", Roles: []domain.RoleRef{"expedicao"}})

	if err != nil || outcome.Response() != (domain.InvitedResponse{Email: testEmail, Status: domain.InvitationStatusPending}) {
		t.Fatalf("InviteMember() = (%+v, %v), want ana@loja.com pending", outcome, err)
	}
	want := domain.InvitationSnapshot{Email: testEmail, Roles: []domain.RoleRef{"expedicao"}, Status: domain.InvitationStatusPending, Round: 1, InvitedAt: testOccurred}
	if got := h.storedInvitation(t, testEmail); !got.Equal(want) {
		t.Fatalf("stored = %+v, want %+v", got, want)
	}
}

func TestAFailedInvitationIsReopenedInTheSecondRound(t *testing.T) {
	h := newHarness(t)
	h.seedInvitation(t, domain.InvitationSnapshot{
		Email: testEmail, Roles: []domain.RoleRef{"expedicao"}, Status: domain.InvitationStatusFailed, Round: 1,
		FailureReason: "recusado", InvitedAt: 1000, SettledAt: 2000,
	})

	if _, err := h.service.InviteMember(withExecution(t, context.Background()), application.InviteMember{Email: string(testEmail), Roles: []domain.RoleRef{"catalogo"}}); err != nil {
		t.Fatalf("InviteMember() = %v", err)
	}

	if got := h.storedInvitation(t, testEmail); got.Status != domain.InvitationStatusPending || got.Round != 2 || got.FailureReason != "" || h.outboxLen() != 1 {
		t.Fatalf("stored = %+v with %d outbox entries, want pending in round 2 and one event", got, h.outboxLen())
	}
}

func TestAnInvitationOutsideTheRulesIsRejectedAndNothingIsWritten(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  application.InviteMember
		want kernel.Code
	}{
		{"email ana", application.InviteMember{Email: "ana", Roles: []domain.RoleRef{"expedicao"}}, domain.CodeInvitationEmailInvalid},
		{"no roles", application.InviteMember{Email: string(testEmail)}, domain.CodeInvitationRolesEmpty},
		{"owner", application.InviteMember{Email: string(testEmail), Roles: []domain.RoleRef{domain.OwnerRole}}, domain.CodeInvitationRoleReserved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)

			outcome, err := h.service.InviteMember(withExecution(t, context.Background()), tc.cmd)

			requireCode(t, outcome, err, tc.want)
			if got := h.outboxLen(); got != 0 {
				t.Fatalf("outbox = %d entries, want none", got)
			}
		})
	}
}

func TestTheSeededOwnerClaimsTheStore(t *testing.T) {
	h := newHarness(t)

	outcome, err := h.service.ClaimOwnership(withExecution(t, context.Background()), application.ClaimOwnership{Subject: testOwner})

	if err != nil || outcome.Response() != (domain.EstablishedResponse{Subject: testOwner}) {
		t.Fatalf("ClaimOwnership() = (%+v, %v), want the owner established", outcome, err)
	}
	want := domain.MembershipSnapshot{
		Subject: testOwner, Roles: []domain.RoleRef{domain.OwnerRole}, Status: domain.MembershipStatusActive,
		AccessVersion: 1, JoinedAt: testOccurred, ChangedAt: testOccurred,
	}
	if got := h.storedMembership(t, testOwner); !got.Equal(want) {
		t.Fatalf("stored = %+v, want %+v", got, want)
	}
}

// ADR-066: no reading decides the owner; the second claim collides on the
// unique owner of the store when saved, and nothing changes.
func TestASecondClaimIsOwnerClaimedAndNoMembershipChanges(t *testing.T) {
	for name, seed := range map[string]func(*harness, *testing.T){
		"subject without a membership": func(*harness, *testing.T) {},
		"active member of another role": func(h *harness, t *testing.T) {
			h.seedMember(t, "expedicao")
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.seedOwner(t)
			seed(h, t)
			before := h.store.Entries()

			outcome, err := h.service.ClaimOwnership(withExecution(t, context.Background()), application.ClaimOwnership{Subject: testSubject})

			requireCode(t, outcome, err, domain.CodeMembershipOwnerClaimed)
			if got := h.storedMembership(t, testOwner); !slices.Equal(got.Roles, []domain.RoleRef{domain.OwnerRole}) || got.AccessVersion != 1 {
				t.Fatalf("owner = %+v, want it unchanged", got)
			}
			if member, _, err := membershipTable.Reader(h.store).Load(withExecution(t, context.Background()), testSubject); err == nil {
				if !slices.Equal(member.Roles, []domain.RoleRef{"expedicao"}) || member.AccessVersion != 1 {
					t.Fatalf("member = %+v, want it unchanged", member)
				}
			} else if !errors.Is(err, ports.ErrNotFound) {
				t.Fatalf("Load(%s) = %v", testSubject, err)
			}
			if got := len(h.store.Entries()); got != len(before) {
				t.Fatalf("outbox = %d entries, want %d: the refusal enqueues nothing", got, len(before))
			}
		})
	}
}

func TestTheOwnerClaimingAgainChangesNothing(t *testing.T) {
	h := newHarness(t)
	h.seedOwner(t)
	before := h.storedMembership(t, testOwner)

	outcome, err := h.service.ClaimOwnership(withExecution(t, context.Background()), application.ClaimOwnership{Subject: testOwner})

	if _, rejected := outcome.Rejection(); err != nil || rejected {
		t.Fatalf("ClaimOwnership() = (%+v, %v), want accepted", outcome, err)
	}
	if got := h.storedMembership(t, testOwner); !got.Equal(before) || h.outboxLen() != 0 {
		t.Fatalf("stored = %+v with %d outbox entries, want it unchanged and no event", got, h.outboxLen())
	}
}

func TestAClaimWithoutASubjectIsRejected(t *testing.T) {
	h := newHarness(t)

	outcome, err := h.service.ClaimOwnership(withExecution(t, context.Background()), application.ClaimOwnership{})

	requireCode(t, outcome, err, domain.CodeMembershipSubjectInvalid)
}

func TestGrantRevokeAndRemoveWalkTheAccessOfAMember(t *testing.T) {
	h := newHarness(t)
	h.seedMember(t, "expedicao")
	ctx := func() context.Context { return withExecution(t, context.Background()) }

	if outcome, err := h.service.GrantRole(ctx(), application.GrantRole{Subject: testSubject, Role: "catalogo"}); err != nil || outcome.Response() != (domain.GrantedResponse{Subject: testSubject}) {
		t.Fatalf("GrantRole() = (%+v, %v)", outcome, err)
	}
	if got := h.storedMembership(t, testSubject); !slices.Equal(got.Roles, []domain.RoleRef{"catalogo", "expedicao"}) || got.AccessVersion != 2 {
		t.Fatalf("after grant = %+v, want both roles at access version 2", got)
	}
	if outcome, err := h.service.RevokeRole(ctx(), application.RevokeRole{Subject: testSubject, Role: "expedicao"}); err != nil || outcome.Response() != (domain.RevokedResponse{Subject: testSubject}) {
		t.Fatalf("RevokeRole() = (%+v, %v)", outcome, err)
	}
	if outcome, err := h.service.RemoveMember(ctx(), application.RemoveMember{Subject: testSubject}); err != nil || outcome.Response() != (domain.RemovedResponse{Subject: testSubject}) {
		t.Fatalf("RemoveMember() = (%+v, %v)", outcome, err)
	}
	if got := h.storedMembership(t, testSubject); got.Status != domain.MembershipStatusRemoved || got.AccessVersion != 4 {
		t.Fatalf("after remove = %+v, want removed at access version 4", got)
	}
	if got := h.outboxLen(); got != 3 {
		t.Fatalf("outbox = %d entries, want 3: granted, revoked and removed", got)
	}
}

func TestTheOwnerIsProtectedAndOwnerIsNeverGranted(t *testing.T) {
	h := newHarness(t)
	h.seedOwner(t)
	h.seedMember(t, "expedicao")
	ctx := func() context.Context { return withExecution(t, context.Background()) }

	revoked, err := h.service.RevokeRole(ctx(), application.RevokeRole{Subject: testOwner, Role: domain.OwnerRole})
	requireCode(t, revoked, err, domain.CodeMembershipOwnerProtected)
	removed, err := h.service.RemoveMember(ctx(), application.RemoveMember{Subject: testOwner})
	requireCode(t, removed, err, domain.CodeMembershipOwnerProtected)
	granted, err := h.service.GrantRole(ctx(), application.GrantRole{Subject: testSubject, Role: domain.OwnerRole})
	requireCode(t, granted, err, domain.CodeMembershipOwnerReserved)
	last, err := h.service.RevokeRole(ctx(), application.RevokeRole{Subject: testSubject, Role: "expedicao"})
	requireCode(t, last, err, domain.CodeMembershipLastRole)
}

func TestAMembershipCommandOverAnUnknownSubjectIsNotFound(t *testing.T) {
	h := newHarness(t)
	ctx := func() context.Context { return withExecution(t, context.Background()) }

	if _, err := h.service.GrantRole(ctx(), application.GrantRole{Subject: testSubject, Role: "catalogo"}); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("GrantRole(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := h.service.RevokeRole(ctx(), application.RevokeRole{Subject: testSubject, Role: "catalogo"}); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("RevokeRole(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := h.service.RemoveMember(ctx(), application.RemoveMember{Subject: testSubject}); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("RemoveMember(unknown) = %v, want ErrNotFound", err)
	}
}
