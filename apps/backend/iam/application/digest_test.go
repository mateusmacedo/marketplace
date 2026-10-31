package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/serviceskit"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

// The audit Object of an address is the digest of the idempotency policy in
// hex, the form the kernel gives its payload hash; the composition root
// injects SHA-256, which this block cannot import.
func TestTheAuditedEmailIsTheDigestOfThePolicyInHex(t *testing.T) {
	h := newHarness(t)
	fold := serviceskit.FoldDigest([]byte("ana@loja.com"))

	got := application.EmailDigest(h.service, "ana@loja.com")

	if want := fmt.Sprintf("%x", fold[:]); got != want || strings.Contains(got, "@") {
		t.Fatalf("EmailDigest(ana@loja.com) = %s, want %s", got, want)
	}
	if got == application.EmailDigest(h.service, "bia@loja.com") {
		t.Fatal("two addresses share a digest")
	}
}

// Without a digest the command is refused as incomplete before any audit, so
// the Object of an absent policy is never recorded.
func TestAnInvitationWithoutADigestIsRefusedAsIncompleteAndAuditsNothing(t *testing.T) {
	h, instr := newInstrumentedHarness(t)
	h.service.Idempotency.Digest = nil

	_, err := h.service.InviteMember(withKey(t, context.Background(), "k-invite"), application.InviteMember{Email: string(testEmail), Roles: []domain.RoleRef{"expedicao"}})

	if !errors.Is(err, usecase.ErrIncompleteCommand) {
		t.Fatalf("InviteMember() without a digest = %v, want %v", err, usecase.ErrIncompleteCommand)
	}
	if len(instr.audits) != 0 || h.outboxLen() != 0 {
		t.Fatalf("audits = %+v and outbox = %d, want neither: the command never ran", instr.audits, h.outboxLen())
	}
	if got := application.EmailDigest(h.service, testEmail); got != "" {
		t.Fatalf("EmailDigest() without a digest = %q, want empty", got)
	}
}
