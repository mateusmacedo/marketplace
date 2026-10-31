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

func TestASubjectWithoutABuyerIsRegisteredActive(t *testing.T) {
	h := newHarness(t)

	outcome, err := h.service.RegisterBuyer(withExecution(t, context.Background()), application.RegisterBuyer{Subject: testSubject, Name: "Bia"})

	if err != nil || outcome.Response() != (domain.RegisteredResponse{Subject: testSubject, Status: domain.BuyerStatusActive}) {
		t.Fatalf("RegisterBuyer() = (%+v, %v), want kc-123 active", outcome, err)
	}
	want := domain.BuyerSnapshot{Subject: testSubject, Name: "Bia", Status: domain.BuyerStatusActive, AccessVersion: 1, RegisteredAt: testOccurred, ChangedAt: testOccurred}
	if got := h.storedBuyer(t, testSubject); !got.Equal(want) {
		t.Fatalf("stored = %+v, want %+v", got, want)
	}
}

func TestARegisteredBuyerRegisteringAgainChangesNothing(t *testing.T) {
	for _, status := range []domain.BuyerStatus{domain.BuyerStatusActive, domain.BuyerStatusBlocked} {
		t.Run(status.String(), func(t *testing.T) {
			h := newHarness(t)
			h.seedBuyer(t, status)
			before := h.storedBuyer(t, testSubject)

			outcome, err := h.service.RegisterBuyer(withExecution(t, context.Background()), application.RegisterBuyer{Subject: testSubject, Name: "Outro nome"})

			if err != nil || outcome.Response() != (domain.RegisteredResponse{Subject: testSubject, Status: status}) {
				t.Fatalf("RegisterBuyer() = (%+v, %v), want the status it had", outcome, err)
			}
			if got := h.storedBuyer(t, testSubject); !got.Equal(before) || h.outboxLen() != 0 {
				t.Fatalf("stored = %+v with %d outbox entries, want it unchanged and no event", got, h.outboxLen())
			}
		})
	}
}

func TestBlockUpdateAndUnblockWalkTheLifeOfABuyer(t *testing.T) {
	h := newHarness(t)
	h.seedBuyer(t, domain.BuyerStatusActive)
	ctx := func() context.Context { return withExecution(t, context.Background()) }

	if outcome, err := h.service.UpdateBuyerProfile(ctx(), application.UpdateBuyerProfile{Subject: testSubject, Name: "Bia Souza"}); err != nil || outcome.Response() != (domain.ProfileUpdatedResponse{Subject: testSubject}) {
		t.Fatalf("UpdateBuyerProfile() = (%+v, %v)", outcome, err)
	}
	if outcome, err := h.service.BlockBuyer(ctx(), application.BlockBuyer{Subject: testSubject, Reason: "chargeback recorrente"}); err != nil || outcome.Response() != (domain.BlockedResponse{Subject: testSubject}) {
		t.Fatalf("BlockBuyer() = (%+v, %v)", outcome, err)
	}
	if got := h.storedBuyer(t, testSubject); got.Status != domain.BuyerStatusBlocked || got.Name != "Bia Souza" || got.AccessVersion != 2 {
		t.Fatalf("after block = %+v, want blocked at access version 2 under the new name", got)
	}
	updated, err := h.service.UpdateBuyerProfile(ctx(), application.UpdateBuyerProfile{Subject: testSubject, Name: "Outra"})
	requireCode(t, updated, err, domain.CodeBuyerBlocked)
	blocked, err := h.service.BlockBuyer(ctx(), application.BlockBuyer{Subject: testSubject, Reason: "outro"})
	requireCode(t, blocked, err, domain.CodeBuyerNotActive)
	if outcome, err := h.service.UnblockBuyer(ctx(), application.UnblockBuyer{Subject: testSubject}); err != nil || outcome.Response() != (domain.UnblockedResponse{Subject: testSubject}) {
		t.Fatalf("UnblockBuyer() = (%+v, %v)", outcome, err)
	}
	if got := h.storedBuyer(t, testSubject); got.Status != domain.BuyerStatusActive || got.BlockReason != "" || got.AccessVersion != 3 {
		t.Fatalf("after unblock = %+v, want active without a reason at access version 3", got)
	}

	var events []string
	for _, entry := range h.store.Entries() {
		events = append(events, entry.Event.EventName())
	}
	if want := []string{"iam.buyer-profile-updated", "iam.buyer-blocked", "iam.buyer-unblocked"}; !slices.Equal(events, want) {
		t.Fatalf("outbox = %v, want %v", events, want)
	}
}

func TestABuyerCommandOverAnUnknownSubjectIsNotFound(t *testing.T) {
	h := newHarness(t)
	ctx := func() context.Context { return withExecution(t, context.Background()) }

	for name, err := range map[string]error{
		"UpdateBuyerProfile": second(h.service.UpdateBuyerProfile(ctx(), application.UpdateBuyerProfile{Subject: testSubject, Name: "Bia"})),
		"BlockBuyer":         second(h.service.BlockBuyer(ctx(), application.BlockBuyer{Subject: testSubject, Reason: "motivo"})),
		"UnblockBuyer":       second(h.service.UnblockBuyer(ctx(), application.UnblockBuyer{Subject: testSubject})),
	} {
		if !errors.Is(err, ports.ErrNotFound) {
			t.Errorf("%s(unknown) = %v, want ErrNotFound", name, err)
		}
	}
}

var buyerCommands = []accessCommand{
	{"RegisterBuyer", "buyers", nil, func(t *testing.T, h *harness) error {
		_, err := h.service.RegisterBuyer(withExecution(t, context.Background()), application.RegisterBuyer{Subject: testSubject, Name: "Bia"})
		return err
	}, "application: register buyer kc-123: ", application.OperationRegisterBuyer, string(testSubject), application.AggregateTypeBuyer},
	{"UpdateBuyerProfile", "buyers", func(h *harness, t *testing.T) { h.seedBuyer(t, domain.BuyerStatusActive) }, func(t *testing.T, h *harness) error {
		_, err := h.service.UpdateBuyerProfile(withExecution(t, context.Background()), application.UpdateBuyerProfile{Subject: testSubject, Name: "Bia Souza"})
		return err
	}, "application: update buyer profile kc-123: ", application.OperationUpdateBuyerProfile, string(testSubject), application.AggregateTypeBuyer},
	{"BlockBuyer", "buyers", func(h *harness, t *testing.T) { h.seedBuyer(t, domain.BuyerStatusActive) }, func(t *testing.T, h *harness) error {
		_, err := h.service.BlockBuyer(withExecution(t, context.Background()), application.BlockBuyer{Subject: testSubject, Reason: "chargeback recorrente"})
		return err
	}, "application: block buyer kc-123: ", application.OperationBlockBuyer, string(testSubject), application.AggregateTypeBuyer},
	{"UnblockBuyer", "buyers", func(h *harness, t *testing.T) { h.seedBuyer(t, domain.BuyerStatusBlocked) }, func(t *testing.T, h *harness) error {
		_, err := h.service.UnblockBuyer(withExecution(t, context.Background()), application.UnblockBuyer{Subject: testSubject})
		return err
	}, "application: unblock buyer kc-123: ", application.OperationUnblockBuyer, string(testSubject), application.AggregateTypeBuyer},
}

func TestEachBuyerCommandWalksTheNineStepsAuditsOnceAndCarriesItsPrefix(t *testing.T) {
	for _, c := range buyerCommands {
		t.Run(c.name+" steps", func(t *testing.T) {
			h := newHarness(t)
			c.withSeed(h, t)
			if err := c.run(t, h); err != nil {
				t.Fatalf("%s() = %v", c.name, err)
			}
			if want := stepsOver(c.table); !slices.Equal(h.rec.Observed(), want) {
				t.Fatalf("sequence mismatch (FND-04 §3.2)\ngot:  %v\nwant: %v", h.rec.Observed(), want)
			}
			if entries := h.store.Entries(); len(entries) != 1 || entries[0].AggregateType != c.aggType || entries[0].AggregateID != c.object {
				t.Fatalf("outbox = %+v, want one entry of %s %s", entries, c.aggType, c.object)
			}
		})
		t.Run(c.name+" audit", func(t *testing.T) {
			h, instr := newInstrumentedHarness(t)
			c.withSeed(h, t)
			if err := c.run(t, h); err != nil {
				t.Fatalf("%s() = %v", c.name, err)
			}
			instr.requireOnlyAudit(t, ports.AuditEvent{Object: c.object, Action: c.action, Outcome: ports.OutcomeAccepted, At: testOccurred})
		})
		for name, fault := range map[string]option{"load": withLoadError(errBoom), "save": withSaveError(errBoom), "enqueue": withEnqueueError(errBoom)} {
			t.Run(c.name+" "+name, func(t *testing.T) {
				h := newHarness(t, fault)
				c.withSeed(h, t)
				want := c.prefix + "boom"
				if name == "enqueue" {
					want = c.prefix + "enqueue: boom"
				}
				if err := c.run(t, h); !errors.Is(err, errBoom) || err.Error() != want {
					t.Fatalf("err = %v, want %q", err, want)
				}
			})
		}
	}
}

func TestTheBuyerIsFoundOutsideAnyUnitOfWorkAndByStatusNeverNil(t *testing.T) {
	h := newHarness(t)
	h.seedBuyer(t, domain.BuyerStatusBlocked)
	h.service.BuyersByStatus = plantedBuyers{domain.BuyerStatusBlocked: {{Subject: testSubject, Status: domain.BuyerStatusBlocked}}}
	h.rec.Reset()

	buyer, err := h.service.FindBuyer(withExecution(t, context.Background()), testSubject)
	if err != nil || buyer.Status != domain.BuyerStatusBlocked || buyer.BlockReason != "chargeback recorrente" {
		t.Fatalf("FindBuyer() = %+v, %v; want the blocked buyer", buyer, err)
	}
	blocked, err := h.service.FindBuyersByStatus(withExecution(t, context.Background()), domain.BuyerStatusBlocked)
	if err != nil || len(blocked) != 1 {
		t.Fatalf("FindBuyersByStatus(blocked) = %+v, %v", blocked, err)
	}
	active, err := h.service.FindBuyersByStatus(withExecution(t, context.Background()), domain.BuyerStatusActive)
	if err != nil || active == nil || len(active) != 0 {
		t.Fatalf("FindBuyersByStatus(active) = %#v, %v; want an empty, non-nil collection", active, err)
	}
	if slices.Contains(h.rec.Observed(), "within") {
		t.Fatalf("sequence = %v: a query must not open a unit of work (UOW-11)", h.rec.Observed())
	}
	if _, err := h.service.FindBuyer(withExecution(t, context.Background()), "kc-nenhum"); !errors.Is(err, ports.ErrNotFound) ||
		err.Error() != "application: find buyer kc-nenhum: ports: aggregate not found" {
		t.Fatalf("FindBuyer(unknown) = %v, want the prefixed ErrNotFound", err)
	}
}

func TestTheSameKeyWithAnotherReasonToBlockIsAMismatch(t *testing.T) {
	h := newHarness(t)
	h.seedBuyer(t, domain.BuyerStatusActive)
	if _, err := h.service.BlockBuyer(withKey(t, context.Background(), "k-block"), application.BlockBuyer{Subject: testSubject, Reason: "motivo"}); err != nil {
		t.Fatalf("first BlockBuyer() = %v", err)
	}

	_, err := h.service.BlockBuyer(withKey(t, context.Background(), "k-block"), application.BlockBuyer{Subject: testSubject, Reason: "outro motivo"})

	if !errors.Is(err, ports.ErrIdempotencyMismatch) {
		t.Fatalf("second BlockBuyer() = %v, want ErrIdempotencyMismatch", err)
	}
}

// The stored format is frozen: an inbox entry written by one release must
// still decode in the next.
func TestTheBuyerResponsesKeepTheirStoredFormat(t *testing.T) {
	subject := []byte{0x01, 'a', 0x06, 'k', 'c', '-', '1', '2', '3'}
	for name, stored := range map[string][]byte{
		"profile-updated": usecase.EncodeOutcome(application.ProfileUpdatedCodec, usecase.Accepted(domain.ProfileUpdatedResponse{Subject: testSubject})),
		"blocked":         usecase.EncodeOutcome(application.BlockedCodec, usecase.Accepted(domain.BlockedResponse{Subject: testSubject})),
		"unblocked":       usecase.EncodeOutcome(application.UnblockedCodec, usecase.Accepted(domain.UnblockedResponse{Subject: testSubject})),
	} {
		if !bytes.Equal(stored, subject) {
			t.Errorf("%s: stored = %x, want %x", name, stored, subject)
		}
	}
	registered := append(append([]byte(nil), subject...), append([]byte{0x07}, []byte("blocked")...)...)
	if stored := usecase.EncodeOutcome(application.RegisteredCodec, usecase.Accepted(domain.RegisteredResponse{Subject: testSubject, Status: domain.BuyerStatusBlocked})); !bytes.Equal(stored, registered) {
		t.Errorf("registered: stored = %x, want %x", stored, registered)
	}
	for _, status := range []domain.BuyerStatus{domain.BuyerStatusActive, domain.BuyerStatusBlocked} {
		want := domain.RegisteredResponse{Subject: testSubject, Status: status}
		if got, err := usecase.DecodeOutcome(application.RegisteredCodec, usecase.EncodeOutcome(application.RegisteredCodec, usecase.Accepted(want))); err != nil || got.Response() != want {
			t.Errorf("round trip of %v = (%+v, %v)", status, got.Response(), err)
		}
	}
}
