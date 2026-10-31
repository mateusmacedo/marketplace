package application_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	"github.com/mateusmacedo/dmpf/libs/backend/go/memory"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

const membershipConsumer = "iam-membership"

func consumeBind(tx *memory.Tx) application.Resources {
	return application.Resources{
		Inbox:       tx.Inbox(membershipConsumer),
		Memberships: membershipTable.Repository(tx),
		Outbox:      tx.Outbox(),
		Commands:    tx.CommandInbox(application.CommandConsumer),
	}
}

func newConsumeService(store *memory.Store) application.Service {
	return application.Service{
		UoW:       memory.NewUnitOfWork(store, consumeBind),
		Clock:     memory.FixedClock{At: testOccurred},
		IDs:       &memory.SequenceIDs{Prefix: "m-"},
		Authorize: usecase.AllowAll[application.Operation](),
		Consumer:  membershipConsumer,
	}
}

func provisioned(id ports.MessageID, hash string, subject domain.Subject, roles ...domain.RoleRef) application.ConsumeInvitationProvisioned {
	return application.ConsumeInvitationProvisioned{
		MessageID: id, MessageType: "com.company.iam.invitation-provisioned.v1", PayloadHash: hash, ReceivedAt: testOccurred - 1,
		Subject: subject, Email: testEmail, Roles: roles,
	}
}

// failingMembershipSaves wraps a real repository and fails every Save with failure,
// which is how the technical dispositions are reached without a broken aggregate.
type failingMembershipSaves struct {
	ports.Repository[domain.Subject, domain.MembershipSnapshot]
	failure error
}

func (f failingMembershipSaves) Save(context.Context, domain.Subject, domain.MembershipSnapshot, ports.Version) error {
	return f.failure
}

func newConsumeServiceWithFailingSave(store *memory.Store, failure error) application.Service {
	svc := newConsumeService(store)
	svc.UoW = memory.NewUnitOfWork(store, func(tx *memory.Tx) application.Resources {
		res := consumeBind(tx)
		res.Memberships = failingMembershipSaves{Repository: res.Memberships, failure: failure}
		return res
	})
	return svc
}

type neverCompletingInbox struct{}

func (neverCompletingInbox) Register(context.Context, ports.Receipt) (ports.Reception, error) {
	return ports.FirstReception(brokenPending{}), nil
}

type brokenPending struct{}

func (brokenPending) Complete(context.Context, ports.Completion) error { return nil }
func (brokenPending) Completed() bool                                  { return false }

func seedConsumed(t *testing.T, store *memory.Store, snap domain.MembershipSnapshot) {
	t.Helper()
	uow := memory.NewUnitOfWork(store, consumeBind)
	if err := uow.Within(withExecution(t, context.Background()), func(ctx context.Context, res application.Resources) error {
		return res.Memberships.Save(ctx, snap.Subject, snap, 0)
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func consumedMembership(t *testing.T, store *memory.Store, subject domain.Subject) domain.MembershipSnapshot {
	t.Helper()
	snap, _, err := membershipTable.Reader(store).Load(withExecution(t, context.Background()), subject)
	if err != nil {
		t.Fatalf("Load(%s) = %v", subject, err)
	}
	return snap
}

func requireNothingConsumed(t *testing.T, store *memory.Store) {
	t.Helper()
	if got := store.InboxRows(); got != 0 {
		t.Fatalf("InboxRows() = %d, want 0", got)
	}
	if got := len(store.Entries()); got != 0 {
		t.Fatalf("Entries() = %d, want 0", got)
	}
}

func TestTheFirstReceptionEstablishesTheMembershipOfTheInvitation(t *testing.T) {
	store := memory.New()
	svc := newConsumeService(store)

	disp, err := svc.ConsumeInvitationProvisioned(withExecution(t, context.Background()), provisioned("m-ext-1", "h1", testSubject, "expedicao", "catalogo"))

	if err != nil || disp != usecase.R1D1 {
		t.Fatalf("Consume() = (%v, %v), want (%v, nil)", disp, err, usecase.R1D1)
	}
	want := domain.MembershipSnapshot{
		Subject: testSubject, Email: testEmail, Roles: []domain.RoleRef{"catalogo", "expedicao"}, Status: domain.MembershipStatusActive,
		AccessVersion: 1, JoinedAt: testOccurred, ChangedAt: testOccurred,
	}
	if got := consumedMembership(t, store, testSubject); !got.Equal(want) {
		t.Fatalf("membership = %+v, want %+v", got, want)
	}
	entries := store.Entries()
	if len(entries) != 1 || entries[0].AggregateType != application.AggregateTypeMembership || entries[0].AggregateID != string(testSubject) {
		t.Fatalf("outbox = %+v, want the membership established", entries)
	}
	if _, ok := entries[0].Event.(domain.MembershipEstablished); !ok {
		t.Fatalf("event = %T, want MembershipEstablished", entries[0].Event)
	}
	if status, ok := store.InboxStatus(membershipConsumer, "m-ext-1"); !ok || status != ports.StatusProcessed {
		t.Fatalf("InboxStatus() = (%v, %v), want processed", status, ok)
	}
}

func TestARemovedMemberComesBackActiveWithTheRolesOfTheInvitation(t *testing.T) {
	store := memory.New()
	seedConsumed(t, store, domain.MembershipSnapshot{
		Subject: testSubject, Email: testEmail, Roles: []domain.RoleRef{"expedicao"}, Status: domain.MembershipStatusRemoved,
		AccessVersion: 2, JoinedAt: 1000, ChangedAt: 2000,
	})
	svc := newConsumeService(store)

	disp, err := svc.ConsumeInvitationProvisioned(withExecution(t, context.Background()), provisioned("m-ext-1", "h1", testSubject, "vendas"))

	if err != nil || disp != usecase.R1D1 {
		t.Fatalf("Consume() = (%v, %v), want R1D1", disp, err)
	}
	got := consumedMembership(t, store, testSubject)
	if got.Status != domain.MembershipStatusActive || !slices.Equal(got.Roles, []domain.RoleRef{"vendas"}) || got.AccessVersion != 3 {
		t.Fatalf("membership = %+v, want active with [vendas] at access version 3", got)
	}
	if _, ok := store.Entries()[0].Event.(domain.MembershipEstablished); !ok {
		t.Fatalf("event = %T, want MembershipEstablished", store.Entries()[0].Event)
	}
}

func TestAnActiveMemberGainsTheRolesOfANewInvitation(t *testing.T) {
	store := memory.New()
	seedConsumed(t, store, domain.MembershipSnapshot{
		Subject: testSubject, Email: testEmail, Roles: []domain.RoleRef{"expedicao"}, Status: domain.MembershipStatusActive,
		AccessVersion: 1, JoinedAt: 1000, ChangedAt: 1000,
	})
	svc := newConsumeService(store)

	if _, err := svc.ConsumeInvitationProvisioned(withExecution(t, context.Background()), provisioned("m-ext-1", "h1", testSubject, "catalogo")); err != nil {
		t.Fatalf("Consume() = %v", err)
	}

	if got := consumedMembership(t, store, testSubject); !slices.Equal(got.Roles, []domain.RoleRef{"catalogo", "expedicao"}) || got.AccessVersion != 2 {
		t.Fatalf("membership = %+v, want both roles at access version 2", got)
	}
	if _, ok := store.Entries()[0].Event.(domain.MemberAccessChanged); !ok {
		t.Fatalf("event = %T, want MemberAccessChanged", store.Entries()[0].Event)
	}
}

func TestARefusedEstablishmentIsR1D2AndWritesNoMembership(t *testing.T) {
	store := memory.New()
	svc := newConsumeService(store)

	disp, err := svc.ConsumeInvitationProvisioned(withExecution(t, context.Background()), provisioned("m-ext-1", "h1", "", "expedicao"))

	if err != nil || disp != usecase.R1D2 {
		t.Fatalf("Consume() = (%v, %v), want (%v, nil): a refusal is not a technical failure", disp, err, usecase.R1D2)
	}
	if status, _ := store.InboxStatus(membershipConsumer, "m-ext-1"); status != ports.StatusRejected {
		t.Fatalf("InboxStatus() = %v, want rejected", status)
	}
	if last, _ := store.InboxLastError(membershipConsumer, "m-ext-1"); last != string(domain.CodeMembershipSubjectInvalid) {
		t.Fatalf("InboxLastError() = %q, want %q", last, domain.CodeMembershipSubjectInvalid)
	}
	if got := len(store.Entries()); got != 0 {
		t.Fatalf("Entries() = %d, want 0", got)
	}
}

// ADR-066: the refusal a context declared for its unique constraint is a
// business rejection of the message too, never a technical failure.
func TestAViolationOfTheDeclaredOwnerIsR1D2(t *testing.T) {
	store := memory.New()
	violation := ports.UniqueViolation{Table: "memberships", Constraint: "memberships_tenant_id_owner_key", Rejection: domain.RejectMembershipOwnerClaimed}
	svc := newConsumeServiceWithFailingSave(store, violation)

	disp, err := svc.ConsumeInvitationProvisioned(withExecution(t, context.Background()), provisioned("m-ext-1", "h1", testSubject, "expedicao"))

	if err != nil || disp != usecase.R1D2 {
		t.Fatalf("Consume() = (%v, %v), want R1D2", disp, err)
	}
	if last, _ := store.InboxLastError(membershipConsumer, "m-ext-1"); last != string(domain.CodeMembershipOwnerClaimed) {
		t.Fatalf("InboxLastError() = %q, want %q", last, domain.CodeMembershipOwnerClaimed)
	}
}

func TestTheTechnicalFailuresOfTheSaveAreClassifiedAndPersistNothing(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
		want    usecase.Disposition
	}{
		{"transient", usecase.NewFailure(usecase.TransientDependency, true, errors.New("dependency down")), usecase.R1D3},
		{"terminal", usecase.NewFailure(usecase.Unexpected, false, errors.New("boom")), usecase.R1D4},
		{"version conflict", ports.ErrVersionConflict, usecase.R1D3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := memory.New()
			svc := newConsumeServiceWithFailingSave(store, tc.failure)

			disp, err := svc.ConsumeInvitationProvisioned(withExecution(t, context.Background()), provisioned("m-ext-1", "h1", testSubject, "expedicao"))

			if !errors.Is(err, tc.failure) || disp != tc.want {
				t.Fatalf("Consume() = (%v, %v), want (%v, the failure)", disp, err, tc.want)
			}
			if !strings.HasPrefix(err.Error(), "application: consume invitation provisioned kc-123: ") {
				t.Fatalf("err = %q, want the prefix of the use case", err)
			}
			requireNothingConsumed(t, store)
		})
	}
}

func TestARedeliveryIsDisposedOfWithoutWriting(t *testing.T) {
	for _, tc := range []struct {
		name        string
		subject     domain.Subject
		secondHash  string
		want        usecase.Disposition
		wantEntries int
	}{
		{"processed", testSubject, "h1", usecase.R2, 1},
		{"rejected", "", "h1", usecase.R3, 0},
		{"divergent hash", testSubject, "h2", usecase.R4, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := memory.New()
			svc := newConsumeService(store)
			if _, err := svc.ConsumeInvitationProvisioned(withExecution(t, context.Background()), provisioned("m-ext-1", "h1", tc.subject, "expedicao")); err != nil {
				t.Fatalf("first Consume() = %v", err)
			}

			disp, err := svc.ConsumeInvitationProvisioned(withExecution(t, context.Background()), provisioned("m-ext-1", tc.secondHash, tc.subject, "expedicao"))

			if err != nil || disp != tc.want {
				t.Fatalf("second Consume() = (%v, %v), want %v", disp, err, tc.want)
			}
			if got := len(store.Entries()); got != tc.wantEntries {
				t.Fatalf("Entries() = %d, want %d: a redelivery writes nothing", got, tc.wantEntries)
			}
		})
	}
}

func TestAFailedCommitIsR1D4AndPersistsNothing(t *testing.T) {
	store := memory.New()
	store.FailNextCommit(errors.New("commit refused"))
	svc := newConsumeService(store)

	disp, err := svc.ConsumeInvitationProvisioned(withExecution(t, context.Background()), provisioned("m-ext-1", "h1", testSubject, "expedicao"))

	if err == nil || disp != usecase.R1D4 {
		t.Fatalf("Consume() = (%v, %v), want R1D4 with the commit failure (INB-07)", disp, err)
	}
	requireNothingConsumed(t, store)
}

func TestAPendingLeftUncompletedIsR1D4(t *testing.T) {
	store := memory.New()
	svc := newConsumeService(store)
	svc.UoW = memory.NewUnitOfWork(store, func(tx *memory.Tx) application.Resources {
		res := consumeBind(tx)
		res.Inbox = neverCompletingInbox{}
		return res
	})

	disp, err := svc.ConsumeInvitationProvisioned(withExecution(t, context.Background()), provisioned("m-ext-1", "h1", testSubject, "expedicao"))

	if !errors.Is(err, ports.ErrPendingNotCompleted) || disp != usecase.R1D4 {
		t.Fatalf("Consume() = (%v, %v), want R1D4 with ErrPendingNotCompleted", disp, err)
	}
	requireNothingConsumed(t, store)
}

func TestADeniedConsumptionNeverOpensATransaction(t *testing.T) {
	store := memory.New()
	svc := newConsumeService(store)
	var asked application.Operation
	svc.Authorize = func(_ context.Context, op application.Operation) error {
		asked = op
		return usecase.NewFailure(usecase.Forbidden, false, ports.ErrDenied)
	}

	disp, err := svc.ConsumeInvitationProvisioned(withExecution(t, context.Background()), provisioned("m-ext-1", "h1", testSubject, "expedicao"))

	if !errors.Is(err, ports.ErrDenied) || disp != usecase.R1D4 {
		t.Fatalf("Consume() = (%v, %v), want R1D4 with the denial", disp, err)
	}
	if _, ok := asked.(application.ConsumeInvitationProvisioned); !ok {
		t.Fatalf("Authorize saw %T, want the consumption itself", asked)
	}
	if got := store.WithinCalls(); got != 0 {
		t.Fatalf("WithinCalls() = %d, want 0", got)
	}
}

// A consumer continues a chain: the adapter authored the consumed message as
// the cause and the service copies it to the fact it emits.
func TestTheConsumptionCopiesTheMessageContextIntoTheOutboxEntry(t *testing.T) {
	store := memory.New()
	svc := newConsumeService(store)
	want := ports.MessageContext{CorrelationID: "corr-1", CausationID: "m-ext-1", Traceparent: "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01"}

	if _, err := svc.ConsumeInvitationProvisioned(withExecution(t, ports.WithMessageContext(context.Background(), want)), provisioned("m-ext-1", "h1", testSubject, "expedicao")); err != nil {
		t.Fatalf("Consume() = %v", err)
	}

	if entries := store.Entries(); len(entries) != 1 || entries[0].Context != want {
		t.Fatalf("outbox = %+v, want the context %+v", entries, want)
	}
}
