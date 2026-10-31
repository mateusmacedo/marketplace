package domain_test

import (
	"strings"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestInviteAcceptsAnUnknownEmailAsAPendingFirstRound(t *testing.T) {
	i := domain.NewInvitation(domain.NormalizeEmail(" Ana@Loja.com "))

	acc, rej := i.Invite(domain.InviteMember{Email: " Ana@Loja.com ", Roles: []domain.RoleRef{"expedicao", "catalogo", "expedicao"}, At: at})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.InvitedResponse{Email: email, Status: domain.InvitationStatusPending}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	roles := []domain.RoleRef{"catalogo", "expedicao"}
	want := domain.MemberInvited{Email: email, Roles: roles, Round: 1, At: at}
	if events := acc.Events(); len(events) != 1 || !sameEvent(events[0], want) {
		t.Fatalf("Events() = %+v, want [%+v]", events, want)
	}
	wantState := domain.InvitationSnapshot{Email: email, Roles: roles, Status: domain.InvitationStatusPending, Round: 1, InvitedAt: at}
	if got := i.Snapshot(); !got.Equal(wantState) {
		t.Fatalf("Snapshot() = %+v, want %+v", got, wantState)
	}
}

func TestInviteReopensAClosedInvitationInANewRound(t *testing.T) {
	for name, closed := range map[string]func(*testing.T) *domain.Invitation{"failed": newFailedInvitation, "provisioned": newProvisionedInvitation} {
		t.Run(name, func(t *testing.T) {
			i := closed(t)

			acc, rej := i.Invite(domain.InviteMember{Email: string(email), Roles: []domain.RoleRef{"catalogo"}, At: later})

			tb.RequireAccepted(t, rej)
			want := domain.MemberInvited{Email: email, Roles: []domain.RoleRef{"catalogo"}, Round: 2, At: later}
			if events := acc.Events(); len(events) != 1 || !sameEvent(events[0], want) {
				t.Fatalf("Events() = %+v, want [%+v]", events, want)
			}
			wantState := domain.InvitationSnapshot{Email: email, Roles: []domain.RoleRef{"catalogo"}, Status: domain.InvitationStatusPending, Round: 2, InvitedAt: later}
			if got := i.Snapshot(); !got.Equal(wantState) {
				t.Fatalf("Snapshot() = %+v, want %+v: subject, reason and settlement cleared", got, wantState)
			}
		})
	}
}

func TestInviteOfAPendingEmailAcceptsWithoutChangingOrEmitting(t *testing.T) {
	i := newPendingInvitation(t)
	before := i.Snapshot()

	acc, rej := i.Invite(domain.InviteMember{Email: string(email), Roles: []domain.RoleRef{"catalogo"}, At: later})

	tb.RequireAccepted(t, rej)
	if got, want := acc.Response(), (domain.InvitedResponse{Email: email, Status: domain.InvitationStatusPending}); got != want {
		t.Fatalf("Response() = %+v, want %+v", got, want)
	}
	if len(acc.Events()) != 0 {
		t.Fatalf("Events() len = %d, want 0", len(acc.Events()))
	}
	requireInvitationUnchanged(t, before, i.Snapshot())
}

func TestInviteRejectsAnEmailOutsideTheForm(t *testing.T) {
	for _, raw := range []string{"ana", "", "@loja.com", "ana@loja", "ana@@loja.com", "ana@lo@ja.com", strings.Repeat("a", 246) + "@loja.com",
		"ana\u0000@loja.com", "ana@lo\u0007ja.com", "ana@loja.com\u007f"} {
		t.Run(raw, func(t *testing.T) {
			i := domain.NewInvitation(domain.NormalizeEmail(raw))
			before := i.Snapshot()

			acc, rej := i.Invite(domain.InviteMember{Email: raw, Roles: invitedRoles, At: at})

			tb.RequireRejected(t, acc, rej, domain.CodeInvitationEmailInvalid)
			requireInvitationUnchanged(t, before, i.Snapshot())
		})
	}
}

func TestInviteAcceptsAnEmailAtTheBoundOfTheForm(t *testing.T) {
	raw := strings.Repeat("a", 245) + "@loja.com"

	_, rej := domain.NewInvitation(domain.Email(raw)).Invite(domain.InviteMember{Email: raw, Roles: invitedRoles, At: at})

	tb.RequireAccepted(t, rej)
}

func TestInviteRejectsEmptyRoles(t *testing.T) {
	acc, rej := domain.NewInvitation(email).Invite(domain.InviteMember{Email: string(email), At: at})

	tb.RequireRejected(t, acc, rej, domain.CodeInvitationRolesEmpty)
}

func TestInviteRejectsARoleOutsideTheRefFormNamingIt(t *testing.T) {
	acc, rej := domain.NewInvitation(email).Invite(domain.InviteMember{Email: string(email), Roles: []domain.RoleRef{"expedicao", "Exp_1"}, At: at})

	tb.RequireRejected(t, acc, rej, domain.CodeInvitationRoleInvalid)
	if details := rej.Details(); len(details) != 1 || details[0].Key != "role" || details[0].Value != "Exp_1" {
		t.Fatalf("Details() = %+v, want the invalid role named", details)
	}
}

func TestInviteRejectsTheOwnerRole(t *testing.T) {
	acc, rej := domain.NewInvitation(email).Invite(domain.InviteMember{Email: string(email), Roles: []domain.RoleRef{domain.OwnerRole}, At: at})

	tb.RequireRejected(t, acc, rej, domain.CodeInvitationRoleReserved)
}

func TestNormalizeEmailTrimsAndLowersTheAddress(t *testing.T) {
	if got := domain.NormalizeEmail(" Ana@Loja.COM\t"); got != "ana@loja.com" {
		t.Fatalf("NormalizeEmail() = %q, want ana@loja.com", got)
	}
}

func TestValidEmailIsTheRuleInviteApplies(t *testing.T) {
	for raw, want := range map[domain.Email]bool{"ana@loja.com": true, "a@b.c": true, "ana": false, "ana@loja": false, "@loja.com": false, "a@b@c.d": false,
		"ana\u0000@loja.com": false, "ana@loja\u001f.com": false, "ana\u0085@loja.com": false} {
		if got := domain.ValidEmail(raw); got != want {
			t.Errorf("ValidEmail(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestValidSubjectIsTheRuleOfEverySubject(t *testing.T) {
	for raw, want := range map[domain.Subject]bool{
		"kc-123": true, "s": true, domain.Subject(strings.Repeat("s", 255)): true,
		"": false, domain.Subject(strings.Repeat("s", 256)): false, "a\u0000b": false, "a\u001bb": false, "a\u009fb": false,
	} {
		if got := domain.ValidSubject(raw); got != want {
			t.Errorf("ValidSubject(%q) = %v, want %v", raw, got, want)
		}
	}
}
