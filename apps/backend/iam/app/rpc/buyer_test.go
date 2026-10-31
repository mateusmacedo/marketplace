package rpc_test

import (
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/service/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func register(t *testing.T, h *harness, subject, name string) *servicev1.RegisterBuyerResponse {
	t.Helper()
	var resp servicev1.RegisterBuyerResponse
	if err := h.invoke(withTenant(t), "RegisterBuyer", &servicev1.RegisterBuyerRequest{Subject: subject, Name: name}, &resp); err != nil {
		t.Fatalf("RegisterBuyer(%s) = %v", subject, err)
	}
	return &resp
}

func block(t *testing.T, h *harness, subject, reason string) *servicev1.BlockBuyerResponse {
	t.Helper()
	var resp servicev1.BlockBuyerResponse
	if err := h.invoke(withTenant(t), "BlockBuyer", &servicev1.BlockBuyerRequest{Subject: subject, Reason: reason}, &resp); err != nil {
		t.Fatalf("BlockBuyer(%s) = %v", subject, err)
	}
	return &resp
}

func buyer(t *testing.T, h *harness, subject string) *servicev1.Buyer {
	t.Helper()
	var found servicev1.FindBuyerResponse
	if err := h.invoke(withoutKey(t), "FindBuyer", &servicev1.FindBuyerRequest{Subject: subject}, &found); err != nil {
		t.Fatalf("FindBuyer(%s) = %v", subject, err)
	}
	return found.GetBuyer()
}

func TestRegisterBuyerAnswersTheSubjectActiveAndTheBuyerIsReadBack(t *testing.T) {
	h := newHarness(t, nil)

	registered := register(t, h, "kc-123", "Bia").GetRegistered()

	if registered.GetSubject() != "kc-123" || registered.GetStatus() != servicev1.BuyerStatus_BUYER_STATUS_ACTIVE {
		t.Fatalf("RegisterBuyer() = %v, want kc-123 ACTIVE", registered)
	}
	got := buyer(t, h, "kc-123")
	if got.GetName() != "Bia" || got.GetStatus() != servicev1.BuyerStatus_BUYER_STATUS_ACTIVE || got.GetBlockReason() != "" ||
		got.GetAccessVersion() != 1 || got.GetSyncedVersion() != 0 || got.GetRegisteredAt() != int64(occurred) || got.GetChangedAt() != int64(occurred) {
		t.Fatalf("FindBuyer() = %v, want Bia ACTIVE at access version 1 and the domain instant", got)
	}
	if n := len(h.store.Entries()); n != 1 {
		t.Fatalf("outbox = %d entries, want the one registration", n)
	}
}

// A known buyer answers its current status, blocked included, and emits
// nothing: registering again never unblocks.
func TestRegisteringAKnownBuyerAnswersItsStatusAndEmitsNothing(t *testing.T) {
	h := newHarness(t, nil)
	register(t, h, "kc-123", "Bia")
	block(t, h, "kc-123", "chargeback recorrente")

	again := register(t, h, "kc-123", "Outro nome").GetRegistered()

	if again.GetStatus() != servicev1.BuyerStatus_BUYER_STATUS_BLOCKED || buyer(t, h, "kc-123").GetName() != "Bia" {
		t.Fatalf("RegisterBuyer(known) = %v, want BLOCKED without changing the name", again)
	}
	if n := len(h.store.Entries()); n != 2 {
		t.Fatalf("outbox = %d entries, want the registration and the block only", n)
	}
}

func TestTheBuyerWalksItsLifecycleOverTheEdge(t *testing.T) {
	h := newHarness(t, nil)
	register(t, h, "kc-123", "Bia")

	var updated servicev1.UpdateBuyerProfileResponse
	if err := h.invoke(withTenant(t), "UpdateBuyerProfile", &servicev1.UpdateBuyerProfileRequest{Subject: "kc-123", Name: "Bia Souza"}, &updated); err != nil {
		t.Fatalf("UpdateBuyerProfile() = %v", err)
	}
	if updated.GetProfileUpdated().GetSubject() != "kc-123" || buyer(t, h, "kc-123").GetName() != "Bia Souza" {
		t.Fatalf("UpdateBuyerProfile() = %v, want ProfileUpdated and the new name", &updated)
	}

	if got := block(t, h, "kc-123", "chargeback recorrente").GetBlocked().GetSubject(); got != "kc-123" {
		t.Fatalf("BlockBuyer() = %q, want Blocked kc-123", got)
	}
	if b := buyer(t, h, "kc-123"); b.GetStatus() != servicev1.BuyerStatus_BUYER_STATUS_BLOCKED || b.GetBlockReason() != "chargeback recorrente" || b.GetAccessVersion() != 2 {
		t.Fatalf("FindBuyer() = %v, want BLOCKED with the reason at access version 2", b)
	}

	var unblocked servicev1.UnblockBuyerResponse
	if err := h.invoke(withTenant(t), "UnblockBuyer", &servicev1.UnblockBuyerRequest{Subject: "kc-123"}, &unblocked); err != nil {
		t.Fatalf("UnblockBuyer() = %v", err)
	}
	if b := buyer(t, h, "kc-123"); unblocked.GetUnblocked().GetSubject() != "kc-123" || b.GetStatus() != servicev1.BuyerStatus_BUYER_STATUS_ACTIVE || b.GetBlockReason() != "" || b.GetAccessVersion() != 3 {
		t.Fatalf("UnblockBuyer() = %v and FindBuyer() = %v, want Unblocked and ACTIVE without reason at access version 3", &unblocked, b)
	}
	if n := len(h.store.Entries()); n != 4 {
		t.Fatalf("outbox = %d entries, want registered, profile updated, blocked and unblocked", n)
	}
}

func TestABlockedBuyerRefusesTheProfileAndASecondBlock(t *testing.T) {
	h := newHarness(t, nil)
	register(t, h, "kc-123", "Bia")
	block(t, h, "kc-123", "chargeback recorrente")

	var updated servicev1.UpdateBuyerProfileResponse
	if err := h.invoke(withTenant(t), "UpdateBuyerProfile", &servicev1.UpdateBuyerProfileRequest{Subject: "kc-123", Name: "Bia Souza"}, &updated); err != nil {
		t.Fatalf("UpdateBuyerProfile(blocked) = %v", err)
	}
	if got := updated.GetRejection().GetCode(); got != string(domain.CodeBuyerBlocked) {
		t.Fatalf("UpdateBuyerProfile(blocked) = %v, want %s", &updated, domain.CodeBuyerBlocked)
	}
	if got := block(t, h, "kc-123", "outro").GetRejection().GetCode(); got != string(domain.CodeBuyerNotActive) {
		t.Fatalf("BlockBuyer(blocked) = %q, want %s", got, domain.CodeBuyerNotActive)
	}
	var unblocked servicev1.UnblockBuyerResponse
	if err := h.invoke(withTenant(t), "UnblockBuyer", &servicev1.UnblockBuyerRequest{Subject: "kc-123"}, &unblocked); err != nil {
		t.Fatalf("UnblockBuyer() = %v", err)
	}
	if err := h.invoke(withTenant(t), "UnblockBuyer", &servicev1.UnblockBuyerRequest{Subject: "kc-123"}, &unblocked); err != nil {
		t.Fatalf("UnblockBuyer(active) = %v", err)
	}
	if got := unblocked.GetRejection().GetCode(); got != string(domain.CodeBuyerNotBlocked) {
		t.Fatalf("UnblockBuyer(active) = %v, want %s", &unblocked, domain.CodeBuyerNotBlocked)
	}
}

// The subject the bff filled is judged by the domain, whose rejection travels
// in the response.
func TestAnEmptySubjectIsADomainRejection(t *testing.T) {
	h := newHarness(t, nil)

	if got := register(t, h, "", "Bia").GetRejection().GetCode(); got != string(domain.CodeBuyerSubjectInvalid) {
		t.Fatalf("RegisterBuyer(empty subject) = %q, want %s", got, domain.CodeBuyerSubjectInvalid)
	}
	if n := len(h.store.Entries()); n != 0 {
		t.Fatalf("outbox = %d entries, want none", n)
	}
}

func TestTheNameAndTheReasonOutsideTheTextRuleAreInvalidArgumentBeforeTheUseCase(t *testing.T) {
	for _, text := range []string{"", " ", strings.Repeat("a", 201), "a\u200bb"} {
		h := newHarness(t, nil)
		calls := map[string]func() error{
			"RegisterBuyer": func() error {
				return h.invoke(withTenant(t), "RegisterBuyer", &servicev1.RegisterBuyerRequest{Subject: "kc-123", Name: text}, &servicev1.RegisterBuyerResponse{})
			},
			"UpdateBuyerProfile": func() error {
				return h.invoke(withTenant(t), "UpdateBuyerProfile", &servicev1.UpdateBuyerProfileRequest{Subject: "kc-123", Name: text}, &servicev1.UpdateBuyerProfileResponse{})
			},
			"BlockBuyer": func() error {
				return h.invoke(withTenant(t), "BlockBuyer", &servicev1.BlockBuyerRequest{Subject: "kc-123", Reason: text}, &servicev1.BlockBuyerResponse{})
			},
		}
		for method, call := range calls {
			if err := call(); status.Code(err) != codes.InvalidArgument {
				t.Errorf("%s(%q) = %v, want InvalidArgument", method, text, err)
			}
		}
		if n := len(h.store.Entries()); n != 0 {
			t.Fatalf("outbox = %d entries, want none", n)
		}
	}
}

func TestACommandOverAnUnknownBuyerIsNotFound(t *testing.T) {
	h := newHarness(t, nil)

	if err := h.invoke(withTenant(t), "UnblockBuyer", &servicev1.UnblockBuyerRequest{Subject: "kc-nenhum"}, &servicev1.UnblockBuyerResponse{}); status.Code(err) != codes.NotFound {
		t.Fatalf("UnblockBuyer(unknown) = %v, want NotFound", err)
	}
	if err := h.invoke(withoutKey(t), "FindBuyer", &servicev1.FindBuyerRequest{Subject: "kc-nenhum"}, &servicev1.FindBuyerResponse{}); status.Code(err) != codes.NotFound {
		t.Fatalf("FindBuyer(unknown) = %v, want NotFound", err)
	}
}

func TestFindBuyersByStatusAnswersThePlantedBuyersAndRefusesAnUnspecifiedStatus(t *testing.T) {
	h := newHarness(t, nil)

	var resp servicev1.FindBuyersByStatusResponse
	if err := h.invoke(withoutKey(t), "FindBuyersByStatus", &servicev1.FindBuyersByStatusRequest{Status: servicev1.BuyerStatus_BUYER_STATUS_BLOCKED}, &resp); err != nil {
		t.Fatalf("FindBuyersByStatus(blocked) = %v", err)
	}
	if buyers := resp.GetBuyers(); len(buyers) != 1 || buyers[0].GetSubject() != "kc-bloqueado" || buyers[0].GetBlockReason() != "fraude" ||
		buyers[0].GetStatus() != servicev1.BuyerStatus_BUYER_STATUS_BLOCKED {
		t.Fatalf("FindBuyersByStatus(blocked) = %v, want kc-bloqueado BLOCKED by fraude", buyers)
	}
	if err := h.invoke(withoutKey(t), "FindBuyersByStatus", &servicev1.FindBuyersByStatusRequest{}, &servicev1.FindBuyersByStatusResponse{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("FindBuyersByStatus(unspecified) = %v, want InvalidArgument", err)
	}
}
