//go:build integration

package appkit_test

import (
	"context"
	"slices"
	"testing"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/ids"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/execctx"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/pg"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/appkit"
	eventv1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/event/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

func consumerHarness(t *testing.T) appkit.Harness {
	t.Helper()
	return appkit.NewIam(t, clock.New(ports.Instant(1_755_432_000_000_000_000)), &ids.Sequence{Prefix: "m-"})
}

var provisionedAna = &eventv1.InvitationProvisioned{Email: "ana@loja.com", Subject: "kc-123", Roles: []string{"expedicao", "catalogo"}}

func membershipOf(t *testing.T, h appkit.Harness, subject domain.Subject) (domain.MembershipSnapshot, ports.Version) {
	t.Helper()
	ctx := execctx.WithExecution(t, context.Background(), execctx.WithTenant(appkit.Tenant))
	snap, version, err := provider.NewMembershipReader(postgres.NewReadPool(h.Pool)).Load(ctx, subject)
	if err != nil {
		t.Fatalf("Load(%s) = %v", subject, err)
	}
	return snap, version
}

func TestAProvisionedInvitationEstablishesTheMembershipAndAcksAfterTheCommit(t *testing.T) {
	h := consumerHarness(t)

	outcome, ack, err := h.Deliver(context.Background(), "evt-1", appkit.RawInvitationProvisioned(t, "evt-1", appkit.RelaySource, provisionedAna), 1)

	if err != nil || outcome.Disposition != usecase.R1D1 {
		t.Fatalf("Deliver() = (%+v, %v), want R1D1", outcome, err)
	}
	if ack.Acks != 1 || ack.InboxAtAck != 1 {
		t.Fatalf("ack = %+v, want one ack after the inbox row was committed (INB-08)", ack)
	}
	got, version := membershipOf(t, h, "kc-123")
	if got.Status != domain.MembershipStatusActive || got.Email != "ana@loja.com" || !slices.Equal(got.Roles, []domain.RoleRef{"catalogo", "expedicao"}) || version != 1 {
		t.Fatalf("membership = %+v v%d, want active with the email and the roles of the invitation", got, version)
	}
	if outbox := pg.Outbox(t, h.Pool); len(outbox) != 1 || outbox[0].MessageType != "com.company.iam.membership-established.v1" {
		t.Fatalf("outbox = %+v, want the membership established", outbox)
	}
}

func TestTheSameProvisionedInvitationTwiceIsProcessedOnce(t *testing.T) {
	h := consumerHarness(t)
	raw := appkit.RawInvitationProvisioned(t, "evt-1", appkit.RelaySource, provisionedAna)
	if _, _, err := h.Deliver(context.Background(), "evt-1", raw, 1); err != nil {
		t.Fatalf("first Deliver() = %v", err)
	}

	outcome, ack, err := h.Deliver(context.Background(), "evt-1", raw, 1)

	if err != nil || outcome.Disposition != usecase.R2 || ack.Acks != 1 {
		t.Fatalf("redelivery = (%+v, %v) with %+v, want R2 acknowledged", outcome, err, ack)
	}
	if e := h.Effects(t); e != (appkit.Effects{Inbox: 1, Memberships: 1, Outbox: 1}) {
		t.Fatalf("effects = %+v, want the one consumption", e)
	}
}

func TestARemovedMemberComesBackActiveByANewInvitation(t *testing.T) {
	h := consumerHarness(t)
	ctx := execctx.WithExecution(t, context.Background(), execctx.WithTenant(appkit.Tenant))
	pg.Seed(t, ctx, h.Pool, provider.NewMembershipRepository, "kc-123", domain.MembershipSnapshot{
		Subject: "kc-123", Email: "ana@loja.com", Roles: []domain.RoleRef{"expedicao"}, Status: domain.MembershipStatusRemoved,
		AccessVersion: 2, JoinedAt: 1, ChangedAt: 2,
	})

	outcome, _, err := h.Deliver(context.Background(), "evt-2", appkit.RawInvitationProvisioned(t, "evt-2", appkit.RelaySource,
		&eventv1.InvitationProvisioned{Email: "ana@loja.com", Subject: "kc-123", Roles: []string{"vendas"}}), 1)

	if err != nil || outcome.Disposition != usecase.R1D1 {
		t.Fatalf("Deliver() = (%+v, %v), want R1D1", outcome, err)
	}
	if got, _ := membershipOf(t, h, "kc-123"); got.Status != domain.MembershipStatusActive || !slices.Equal(got.Roles, []domain.RoleRef{"vendas"}) || got.AccessVersion != 3 {
		t.Fatalf("membership = %+v, want active again with [vendas] at access version 3", got)
	}
}

func TestADeliveryFromAnotherSourceIsQuarantinedAndNothingElseIsWritten(t *testing.T) {
	h := consumerHarness(t)

	outcome, ack, err := h.Deliver(context.Background(), "evt-1", appkit.RawInvitationProvisioned(t, "evt-1", "urn:dmpf:intruder", provisionedAna), 1)

	if err != nil || !outcome.Contained || outcome.Reason != ports.ReasonUntrustedBoundary || ack.Acks != 1 {
		t.Fatalf("Deliver() = (%+v, %v) with %+v, want contained as untrusted-boundary and acknowledged", outcome, err, ack)
	}
	if e := h.Effects(t); e != (appkit.Effects{Quarantine: 1}) {
		t.Fatalf("effects = %+v, want only the quarantine row", e)
	}
	if reasons := h.QuarantineReasons(t); !slices.Equal(reasons, []string{string(ports.ReasonUntrustedBoundary)}) {
		t.Fatalf("quarantine reasons = %v, want untrusted-boundary", reasons)
	}
}

func TestAnInvalidEnvelopeIsQuarantinedAndNothingElseIsWritten(t *testing.T) {
	h := consumerHarness(t)

	outcome, ack, err := h.Deliver(context.Background(), "evt-1", []byte("not a cloudevent"), 1)

	if err != nil || !outcome.Contained || outcome.Classified || ack.Acks != 1 {
		t.Fatalf("Deliver() = (%+v, %v) with %+v, want contained unclassified and acknowledged (INB-10)", outcome, err, ack)
	}
	if e := h.Effects(t); e != (appkit.Effects{Quarantine: 1}) {
		t.Fatalf("effects = %+v, want only the quarantine row", e)
	}
}

// ERR-11: a payload that is not the contract of its type is terminal, and the
// handler opens no unit of work for it.
func TestAPayloadThatIsNotTheContractIsATerminalValidationWithoutAWrite(t *testing.T) {
	h := consumerHarness(t)
	raw := appkit.RawEnvelope(t, "evt-1", appkit.RelaySource, "com.company.iam.invitation-provisioned.v1", &eventv1.MemberInvited{Email: "ana@loja.com", Round: 1})

	outcome, _, err := h.Deliver(context.Background(), "evt-1", raw, 1)

	if outcome.Disposition != usecase.R1D4 || !outcome.Contained {
		t.Fatalf("Deliver() = (%+v, %v), want R1D4 contained", outcome, err)
	}
	if e := h.Effects(t); e.Memberships != 0 || e.Outbox != 0 || e.Inbox != 0 {
		t.Fatalf("effects = %+v, want no membership, no outbox and no inbox row", e)
	}
}

// A provisioned subject with U+0000 is refused by the domain before the read,
// which Postgres would end in 22021: a business rejection, acked once.
func TestAProvisionedSubjectWithU0000IsRejectedWithoutAWrite(t *testing.T) {
	h := consumerHarness(t)
	malformed := &eventv1.InvitationProvisioned{Email: "ana@loja.com", Subject: "a\u0000b", Roles: []string{"expedicao"}}

	outcome, ack, err := h.Deliver(context.Background(), "evt-1", appkit.RawInvitationProvisioned(t, "evt-1", appkit.RelaySource, malformed), 1)

	if err != nil || outcome.Disposition != usecase.R1D2 || ack.Acks != 1 {
		t.Fatalf("Deliver() = (%+v, %v) with %+v, want R1D2 acked once", outcome, err, ack)
	}
	if e := h.Effects(t); e.Memberships != 0 || e.Outbox != 0 || e.Inbox != 1 {
		t.Fatalf("effects = %+v, want only the inbox row of the refusal", e)
	}
}

// An email with U+0000 is not the contract of the consumer: terminal before
// any unit of work, contained once, never a retried 22P05 of the snapshot.
func TestAProvisionedEmailWithU0000IsQuarantinedWithoutAWrite(t *testing.T) {
	h := consumerHarness(t)
	malformed := &eventv1.InvitationProvisioned{Email: "ana\u0000@loja.com", Subject: "kc-123", Roles: []string{"expedicao"}}

	outcome, ack, err := h.Deliver(context.Background(), "evt-1", appkit.RawInvitationProvisioned(t, "evt-1", appkit.RelaySource, malformed), 1)

	if outcome.Disposition != usecase.R1D4 || !outcome.Contained || ack.Acks != 1 {
		t.Fatalf("Deliver() = (%+v, %v) with %+v, want R1D4 contained and acknowledged once", outcome, err, ack)
	}
	if category := usecase.CategoryOf(err); category != string(usecase.Validation) {
		t.Fatalf("Deliver() error = %v of category %q, want a Validation of the handler, not a failure of the store", err, category)
	}
	if e := h.Effects(t); e != (appkit.Effects{Quarantine: 1}) {
		t.Fatalf("effects = %+v, want only the quarantine row", e)
	}
}
