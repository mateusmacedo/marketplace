package domain_test

import (
	"strings"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestConfirmProvisionsAPendingInvitationWithTheSubject(t *testing.T) {
	i := newPendingInvitation(t)

	acc, rej := i.Confirm(domain.ConfirmInvitation{Subject: subject, At: later})

	tb.RequireAccepted(t, rej)
	want := domain.InvitationProvisioned{Email: email, Subject: subject, Roles: invitedRoles, At: later}
	if events := acc.Events(); len(events) != 1 || !sameEvent(events[0], want) {
		t.Fatalf("Events() = %+v, want [%+v]", events, want)
	}
	wantState := domain.InvitationSnapshot{
		Email: email, Roles: invitedRoles, Status: domain.InvitationStatusProvisioned, Round: 1,
		Subject: subject, InvitedAt: at, SettledAt: later,
	}
	if got := i.Snapshot(); !got.Equal(wantState) {
		t.Fatalf("Snapshot() = %+v, want %+v", got, wantState)
	}
}

func TestFailClosesAPendingInvitationWithTheReason(t *testing.T) {
	i := newPendingInvitation(t)

	acc, rej := i.Fail(domain.FailInvitation{Reason: "e-mail recusado", At: later})

	tb.RequireAccepted(t, rej)
	want := domain.InvitationFailed{Email: email, Reason: "e-mail recusado", At: later}
	if events := acc.Events(); len(events) != 1 || !sameEvent(events[0], want) {
		t.Fatalf("Events() = %+v, want [%+v]", events, want)
	}
	wantState := domain.InvitationSnapshot{
		Email: email, Roles: invitedRoles, Status: domain.InvitationStatusFailed, Round: 1,
		FailureReason: "e-mail recusado", InvitedAt: at, SettledAt: later,
	}
	if got := i.Snapshot(); !got.Equal(wantState) {
		t.Fatalf("Snapshot() = %+v, want %+v", got, wantState)
	}
}

// A redelivered step finds the invitation already settled and changes nothing.
func TestASettledInvitationRefusesBothStepsAsNotPending(t *testing.T) {
	for name, settled := range map[string]func(*testing.T) *domain.Invitation{"provisioned": newProvisionedInvitation, "failed": newFailedInvitation} {
		t.Run(name, func(t *testing.T) {
			i := settled(t)
			before := i.Snapshot()

			confirmed, rej := i.Confirm(domain.ConfirmInvitation{Subject: "kc-outro", At: later})
			tb.RequireRejected(t, confirmed, rej, domain.CodeInvitationNotPending)
			failed, rej := i.Fail(domain.FailInvitation{Reason: "motivo", At: later})
			tb.RequireRejected(t, failed, rej, domain.CodeInvitationNotPending)

			requireInvitationUnchanged(t, before, i.Snapshot())
		})
	}
}

func TestConfirmRejectsASubjectOutsideItsForm(t *testing.T) {
	for _, s := range []string{"", strings.Repeat("s", 256), "a\u0000b", "a\u0007b", "a\u0085b"} {
		i := newPendingInvitation(t)
		before := i.Snapshot()

		acc, rej := i.Confirm(domain.ConfirmInvitation{Subject: domain.Subject(s), At: later})

		tb.RequireRejected(t, acc, rej, domain.CodeInvitationSubjectInvalid)
		requireInvitationUnchanged(t, before, i.Snapshot())
	}
}

func TestFailRejectsAnEmptyReason(t *testing.T) {
	i := newPendingInvitation(t)

	acc, rej := i.Fail(domain.FailInvitation{At: later})

	tb.RequireRejected(t, acc, rej, domain.CodeInvitationReasonEmpty)
}
