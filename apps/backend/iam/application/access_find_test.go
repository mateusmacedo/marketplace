package application_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestTheInvitationAndTheMembershipAreFoundOutsideAnyUnitOfWork(t *testing.T) {
	h := newHarness(t)
	h.seedInvitation(t, domain.InvitationSnapshot{Email: testEmail, Roles: []domain.RoleRef{"expedicao"}, Status: domain.InvitationStatusPending, Round: 1})
	h.seedMember(t, "expedicao")
	h.rec.Reset()

	invitation, err := h.service.FindInvitation(withExecution(t, context.Background()), testEmail)
	if err != nil || invitation.Status != domain.InvitationStatusPending || invitation.Round != 1 {
		t.Fatalf("FindInvitation() = %+v, %v; want the pending invitation", invitation, err)
	}
	membership, err := h.service.FindMembership(withExecution(t, context.Background()), testSubject)
	if err != nil || membership.Status != domain.MembershipStatusActive || !slices.Equal(membership.Roles, []domain.RoleRef{"expedicao"}) {
		t.Fatalf("FindMembership() = %+v, %v; want the active member", membership, err)
	}
	if slices.Contains(h.rec.Observed(), "within") {
		t.Fatalf("sequence = %v: a query must not open a unit of work (UOW-11)", h.rec.Observed())
	}
}

func TestAnUnknownInvitationOrMembershipIsNotFoundWithThePrefix(t *testing.T) {
	h := newHarness(t)

	if _, err := h.service.FindInvitation(withExecution(t, context.Background()), testEmail); !errors.Is(err, ports.ErrNotFound) ||
		err.Error() != "application: find invitation: ports: aggregate not found" {
		t.Fatalf("FindInvitation(unknown) = %v, want the prefixed ErrNotFound", err)
	}
	if _, err := h.service.FindMembership(withExecution(t, context.Background()), testSubject); !errors.Is(err, ports.ErrNotFound) ||
		err.Error() != "application: find membership kc-123: ports: aggregate not found" {
		t.Fatalf("FindMembership(unknown) = %v, want the prefixed ErrNotFound", err)
	}
}

func TestTheByStatusQueriesAnswerExactlyThatStatusAndNeverNil(t *testing.T) {
	h := newHarness(t)
	h.service.InvitationsByStatus = plantedInvitations{domain.InvitationStatusFailed: {{Email: testEmail, Status: domain.InvitationStatusFailed}}}
	h.service.MembershipsByStatus = plantedMemberships{domain.MembershipStatusRemoved: {{Subject: testSubject, Status: domain.MembershipStatusRemoved}}}

	failed, err := h.service.FindInvitationsByStatus(withExecution(t, context.Background()), domain.InvitationStatusFailed)
	if err != nil || len(failed) != 1 || failed[0].Email != testEmail {
		t.Fatalf("FindInvitationsByStatus(failed) = %+v, %v", failed, err)
	}
	removed, err := h.service.FindMembershipsByStatus(withExecution(t, context.Background()), domain.MembershipStatusRemoved)
	if err != nil || len(removed) != 1 || removed[0].Subject != testSubject {
		t.Fatalf("FindMembershipsByStatus(removed) = %+v, %v", removed, err)
	}
	pending, err := h.service.FindInvitationsByStatus(withExecution(t, context.Background()), domain.InvitationStatusPending)
	if err != nil || pending == nil || len(pending) != 0 {
		t.Fatalf("FindInvitationsByStatus(pending) = %#v, %v; want an empty, non-nil collection", pending, err)
	}
	active, err := h.service.FindMembershipsByStatus(withExecution(t, context.Background()), domain.MembershipStatusActive)
	if err != nil || active == nil || len(active) != 0 {
		t.Fatalf("FindMembershipsByStatus(active) = %#v, %v; want an empty, non-nil collection", active, err)
	}
}

type failingInvitations struct{ err error }

func (r failingInvitations) LoadByStatus(context.Context, domain.InvitationStatus) ([]domain.InvitationSnapshot, error) {
	return nil, r.err
}

type failingMemberships struct{ err error }

func (r failingMemberships) LoadByStatus(context.Context, domain.MembershipStatus) ([]domain.MembershipSnapshot, error) {
	return nil, r.err
}

func TestTheByStatusQueriesCarryThePrefixOfTheirFailure(t *testing.T) {
	h := newHarness(t)
	h.service.InvitationsByStatus = failingInvitations{err: errBoom}
	h.service.MembershipsByStatus = failingMemberships{err: errBoom}

	if _, err := h.service.FindInvitationsByStatus(withExecution(t, context.Background()), domain.InvitationStatusPending); !errors.Is(err, errBoom) ||
		err.Error() != "application: find invitations by status pending: boom" {
		t.Fatalf("FindInvitationsByStatus() = %v, want the prefixed failure", err)
	}
	if _, err := h.service.FindMembershipsByStatus(withExecution(t, context.Background()), domain.MembershipStatusActive); !errors.Is(err, errBoom) ||
		err.Error() != "application: find memberships by status active: boom" {
		t.Fatalf("FindMembershipsByStatus() = %v, want the prefixed failure", err)
	}
}

func TestADeniedAccessQueryNeverReachesTheReader(t *testing.T) {
	h := newHarness(t)
	h.service.Authorize = func(context.Context, application.Operation) error { return ports.ErrDenied }
	h.service.InvitationsByStatus = failingInvitations{err: errBoom}
	h.service.MembershipsByStatus = failingMemberships{err: errBoom}
	ctx := func() context.Context { return withExecution(t, context.Background()) }

	for name, err := range map[string]error{
		"FindInvitation":          second(h.service.FindInvitation(ctx(), testEmail)),
		"FindInvitationsByStatus": second(h.service.FindInvitationsByStatus(ctx(), domain.InvitationStatusPending)),
		"FindMembership":          second(h.service.FindMembership(ctx(), testSubject)),
		"FindMembershipsByStatus": second(h.service.FindMembershipsByStatus(ctx(), domain.MembershipStatusActive)),
	} {
		if !errors.Is(err, ports.ErrDenied) {
			t.Errorf("%s() = %v, want the denial before the reader", name, err)
		}
	}
}

func second[T any](_ T, err error) error { return err }

func TestTheSameKeyWithOtherRolesToInviteIsAMismatch(t *testing.T) {
	h := newHarness(t)
	if _, err := h.service.InviteMember(withKey(t, context.Background(), "k-invite"), application.InviteMember{Email: string(testEmail), Roles: []domain.RoleRef{"expedicao"}}); err != nil {
		t.Fatalf("first InviteMember() = %v, want nil", err)
	}

	_, err := h.service.InviteMember(withKey(t, context.Background(), "k-invite"), application.InviteMember{Email: string(testEmail), Roles: []domain.RoleRef{"catalogo"}})

	if !errors.Is(err, ports.ErrIdempotencyMismatch) {
		t.Fatalf("second InviteMember() = %v, want ErrIdempotencyMismatch", err)
	}
}

func TestARepeatedRefusedClaimReplaysTheRejectionWithoutANewAudit(t *testing.T) {
	h, instr := newInstrumentedHarness(t)
	h.seedOwner(t)

	for range 2 {
		outcome, err := h.service.ClaimOwnership(withKey(t, context.Background(), "k-claim"), application.ClaimOwnership{Subject: testSubject})

		requireCode(t, outcome, err, domain.CodeMembershipOwnerClaimed)
	}
	if len(instr.audits) != 1 {
		t.Fatalf("Audit called %d times, want 1: the refusal, never the replay", len(instr.audits))
	}
}

// The stored format is frozen: an inbox entry written by one release must
// still decode in the next.
func TestTheAccessResponsesKeepTheirStoredFormat(t *testing.T) {
	subject := []byte{0x01, 'a', 0x06, 'k', 'c', '-', '1', '2', '3'}
	for name, stored := range map[string][]byte{
		"claimed": usecase.EncodeOutcome(application.ClaimedCodec, usecase.Accepted(domain.EstablishedResponse{Subject: testSubject})),
		"granted": usecase.EncodeOutcome(application.GrantedCodec, usecase.Accepted(domain.GrantedResponse{Subject: testSubject})),
		"revoked": usecase.EncodeOutcome(application.RevokedCodec, usecase.Accepted(domain.RevokedResponse{Subject: testSubject})),
		"removed": usecase.EncodeOutcome(application.RemovedCodec, usecase.Accepted(domain.RemovedResponse{Subject: testSubject})),
	} {
		if !bytes.Equal(stored, subject) {
			t.Errorf("%s: stored = %x, want %x", name, stored, subject)
		}
	}
	invited := append([]byte{0x01, 'a', 0x0c}, append([]byte("ana@loja.com"), append([]byte{0x07}, []byte("pending")...)...)...)
	if stored := usecase.EncodeOutcome(application.InvitedCodec, usecase.Accepted(domain.InvitedResponse{Email: testEmail, Status: domain.InvitationStatusPending})); !bytes.Equal(stored, invited) {
		t.Errorf("invited: stored = %x, want %x", stored, invited)
	}
}

func TestTheInvitedResponseRoundTripsEveryStatus(t *testing.T) {
	for _, status := range []domain.InvitationStatus{domain.InvitationStatusPending, domain.InvitationStatusProvisioned, domain.InvitationStatusFailed} {
		want := domain.InvitedResponse{Email: testEmail, Status: status}

		got, err := usecase.DecodeOutcome(application.InvitedCodec, usecase.EncodeOutcome(application.InvitedCodec, usecase.Accepted(want)))

		if err != nil || got.Response() != want {
			t.Errorf("round trip of %v = (%+v, %v)", status, got.Response(), err)
		}
	}
}
