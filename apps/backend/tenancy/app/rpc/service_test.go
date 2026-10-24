package rpc_test

import (
	"reflect"
	"strings"
	"testing"

	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/app"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/app/rpc"
	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/tenancy/contract/gen/go/company/tenancy/service/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

func TestServiceDescCoversEveryMethodOfTheDescriptor(t *testing.T) {
	service := servicev1.File_company_tenancy_service_v1_tenancy_service_proto.Services().ByName("TenancyService")

	if rpc.ServiceDesc.ServiceName != string(service.FullName()) || rpc.ServiceName != "company.tenancy.service.v1.TenancyService" {
		t.Fatalf("ServiceName = %q, want %q", rpc.ServiceDesc.ServiceName, service.FullName())
	}
	if missing := kernelgrpc.Uncovered(&rpc.ServiceDesc, service); len(missing) != 0 || len(rpc.ServiceDesc.Streams) != 0 {
		t.Fatalf("ServiceDesc leaves %v uncovered and declares %d streams, want every unary method of the descriptor",
			missing, len(rpc.ServiceDesc.Streams))
	}
}

func TestTheServerImplementsTheHandlerType(t *testing.T) {
	handlerType := reflect.TypeOf(rpc.ServiceDesc.HandlerType).Elem()
	if handlerType.Kind() != reflect.Interface || !reflect.TypeOf(rpc.Server{}).Implements(handlerType) {
		t.Fatalf("rpc.Server does not implement %v", handlerType)
	}
}

func register(t *testing.T, h *harness, ref, name string) *servicev1.RegisterStoreResponse {
	t.Helper()
	var resp servicev1.RegisterStoreResponse
	if err := h.invoke(withTenant(t), "RegisterStore", &servicev1.RegisterStoreRequest{Ref: ref, Name: name}, &resp); err != nil {
		t.Fatalf("RegisterStore(%s) = %v", ref, err)
	}
	return &resp
}

func find(t *testing.T, h *harness, ref string) *servicev1.Store {
	t.Helper()
	var found servicev1.FindStoreResponse
	if err := h.invoke(withoutKey(t), "FindStore", &servicev1.FindStoreRequest{Ref: ref}, &found); err != nil {
		t.Fatalf("FindStore(%s) = %v", ref, err)
	}
	return found.GetStore()
}

func TestRegisterStoreAnswersTheRegisteredRefAndTheStoreIsActive(t *testing.T) {
	h := newHarness(t, nil)

	resp := register(t, h, "loja-acme", "Loja Acme")

	if got := resp.GetRegistered().GetRef(); got != "loja-acme" {
		t.Fatalf("Registered.Ref = %q, want loja-acme (response %v)", got, resp)
	}
	store := find(t, h, "loja-acme")
	if store.GetName() != "Loja Acme" || store.GetStatus() != servicev1.StoreStatus_STORE_STATUS_ACTIVE || store.GetSuspensionReason() != "" {
		t.Fatalf("FindStore() = %v, want Loja Acme, ACTIVE, no reason", store)
	}
	if store.GetRegisteredAt() != int64(occurred) || store.GetStatusChangedAt() != int64(occurred) {
		t.Fatalf("FindStore() instants = %d/%d, want the domain instant %d in nanoseconds", store.GetRegisteredAt(), store.GetStatusChangedAt(), occurred)
	}
	if n := len(h.store.Entries()); n != 1 {
		t.Fatalf("outbox = %d entries, want the one registration", n)
	}
}

func TestRegisteringAKnownRefAgainChangesNothingAndEmitsNothing(t *testing.T) {
	h := newHarness(t, nil)
	register(t, h, "loja-acme", "Loja Acme")

	resp := register(t, h, "loja-acme", "Outro nome")

	if resp.GetRegistered().GetRef() != "loja-acme" || find(t, h, "loja-acme").GetName() != "Loja Acme" {
		t.Fatalf("second RegisterStore() = %v, want Registered without changing the name", resp)
	}
	if n := len(h.store.Entries()); n != 1 {
		t.Fatalf("outbox = %d entries, want 1: a known ref emits nothing", n)
	}
}

func TestRegisteringARefOutsideTheLabelFormIsADomainRejection(t *testing.T) {
	for _, ref := range []string{"Loja_1", "ab", "-loja", strings.Repeat("a", 64)} {
		t.Run(ref, func(t *testing.T) {
			h := newHarness(t, nil)

			resp := register(t, h, ref, "Loja Acme")

			if got := resp.GetRejection().GetCode(); got != string(domain.CodeStoreRefInvalid) {
				t.Fatalf("RegisterStore(%q) = %v, want the rejection %s", ref, resp, domain.CodeStoreRefInvalid)
			}
			if n := len(h.store.Entries()); n != 0 {
				t.Fatalf("outbox = %d entries, want none", n)
			}
		})
	}
}

func TestRegisteringATakenNameIsADomainRejection(t *testing.T) {
	h := newHarness(t, nil)
	register(t, h, "loja-acme", "Loja Acme")

	resp := register(t, h, "loja-outra", "Loja Acme")

	if got := resp.GetRejection().GetCode(); got != string(domain.CodeStoreDuplicateName) {
		t.Fatalf("RegisterStore(taken name) = %v, want the rejection %s", resp, domain.CodeStoreDuplicateName)
	}
}

func TestSuspendAndReactivateWalkTheLifecycleAndThenReject(t *testing.T) {
	h := newHarness(t, nil)
	register(t, h, "loja-acme", "Loja Acme")

	var suspended, again servicev1.SuspendStoreResponse
	if err := h.invoke(withTenant(t), "SuspendStore", &servicev1.SuspendStoreRequest{Ref: "loja-acme", Reason: "fraude em análise"}, &suspended); err != nil {
		t.Fatalf("SuspendStore() = %v", err)
	}
	if suspended.GetSuspended().GetRef() != "loja-acme" {
		t.Fatalf("SuspendStore() = %v, want Suspended loja-acme", &suspended)
	}
	if store := find(t, h, "loja-acme"); store.GetStatus() != servicev1.StoreStatus_STORE_STATUS_SUSPENDED || store.GetSuspensionReason() != "fraude em análise" {
		t.Fatalf("FindStore() = %v, want SUSPENDED with the reason", store)
	}
	if err := h.invoke(withTenant(t), "SuspendStore", &servicev1.SuspendStoreRequest{Ref: "loja-acme", Reason: "outro motivo"}, &again); err != nil {
		t.Fatalf("second SuspendStore() = %v, want the refusal in the body", err)
	}
	if again.GetRejection().GetCode() != string(domain.CodeStoreNotActive) {
		t.Fatalf("second SuspendStore() = %v, want %s", &again, domain.CodeStoreNotActive)
	}

	var reactivated, twice servicev1.ReactivateStoreResponse
	if err := h.invoke(withTenant(t), "ReactivateStore", &servicev1.ReactivateStoreRequest{Ref: "loja-acme"}, &reactivated); err != nil {
		t.Fatalf("ReactivateStore() = %v", err)
	}
	if reactivated.GetReactivated().GetRef() != "loja-acme" {
		t.Fatalf("ReactivateStore() = %v, want Reactivated loja-acme", &reactivated)
	}
	if store := find(t, h, "loja-acme"); store.GetStatus() != servicev1.StoreStatus_STORE_STATUS_ACTIVE || store.GetSuspensionReason() != "" {
		t.Fatalf("FindStore() = %v, want ACTIVE without a reason", store)
	}
	if err := h.invoke(withTenant(t), "ReactivateStore", &servicev1.ReactivateStoreRequest{Ref: "loja-acme"}, &twice); err != nil {
		t.Fatalf("second ReactivateStore() = %v, want the refusal in the body", err)
	}
	if twice.GetRejection().GetCode() != string(domain.CodeStoreNotSuspended) {
		t.Fatalf("second ReactivateStore() = %v, want %s", &twice, domain.CodeStoreNotSuspended)
	}
}

func TestAnUnknownRefIsNotFound(t *testing.T) {
	h := newHarness(t, nil)

	var suspended servicev1.SuspendStoreResponse
	if err := h.invoke(withTenant(t), "SuspendStore", &servicev1.SuspendStoreRequest{Ref: "loja-nenhuma", Reason: "motivo"}, &suspended); status.Code(err) != codes.NotFound {
		t.Fatalf("SuspendStore(unknown) = %v, want NotFound", err)
	}
	var reactivated servicev1.ReactivateStoreResponse
	if err := h.invoke(withTenant(t), "ReactivateStore", &servicev1.ReactivateStoreRequest{Ref: "loja-nenhuma"}, &reactivated); status.Code(err) != codes.NotFound {
		t.Fatalf("ReactivateStore(unknown) = %v, want NotFound", err)
	}
	var found servicev1.FindStoreResponse
	if err := h.invoke(withoutKey(t), "FindStore", &servicev1.FindStoreRequest{Ref: "loja-nenhuma"}, &found); status.Code(err) != codes.NotFound {
		t.Fatalf("FindStore(unknown) = %v, want NotFound", err)
	}
}

func TestFindStoresByStatusMapsEveryStore(t *testing.T) {
	h := newHarness(t, byStatus{domain.Suspended: {
		{Ref: "loja-a", Name: "Loja A", Status: domain.Suspended, SuspensionReason: "motivo", RegisteredAt: occurred, StatusChangedAt: occurred + 1},
		{Ref: "loja-b", Name: "Loja B", Status: domain.Suspended, SuspensionReason: "outro", RegisteredAt: occurred, StatusChangedAt: occurred + 2},
	}})

	var resp servicev1.FindStoresByStatusResponse
	if err := h.invoke(withoutKey(t), "FindStoresByStatus", &servicev1.FindStoresByStatusRequest{Status: servicev1.StoreStatus_STORE_STATUS_SUSPENDED}, &resp); err != nil {
		t.Fatalf("FindStoresByStatus() = %v", err)
	}
	stores := resp.GetStores()
	if len(stores) != 2 || stores[0].GetRef() != "loja-a" || stores[1].GetSuspensionReason() != "outro" || stores[1].GetStatusChangedAt() != int64(occurred+2) {
		t.Fatalf("FindStoresByStatus() = %v, want loja-a and loja-b as planted", stores)
	}
}

func TestATenantThatIsNotTheOperatorIsPermissionDeniedAndNothingIsWritten(t *testing.T) {
	h := newHarnessAuthorizedBy(t, nil, app.Authorization(testTenant))

	var resp servicev1.RegisterStoreResponse
	err := h.invoke(inTenant(t, "loja-acme"), "RegisterStore", &servicev1.RegisterStoreRequest{Ref: "loja-acme", Name: "Loja Acme"}, &resp)

	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("RegisterStore() from loja-acme = %v, want PermissionDenied", err)
	}
	if n := len(h.store.Entries()); n != 0 {
		t.Fatalf("outbox = %d entries, want none", n)
	}
	var found servicev1.FindStoresByStatusResponse
	if err := h.invoke(inTenant(t, "loja-acme"), "FindStoresByStatus", &servicev1.FindStoresByStatusRequest{Status: servicev1.StoreStatus_STORE_STATUS_ACTIVE}, &found); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("FindStoresByStatus() from loja-acme = %v, want PermissionDenied", err)
	}
}

func TestTheOperatorTenantIsServedByTheContextAuthorization(t *testing.T) {
	h := newHarnessAuthorizedBy(t, nil, app.Authorization(testTenant))

	if resp := register(t, h, "loja-acme", "Loja Acme"); resp.GetRegistered().GetRef() != "loja-acme" {
		t.Fatalf("RegisterStore() from the operator = %v, want Registered", resp)
	}
}

func TestMalformedRequestsAreRefusedBeforeTheUseCase(t *testing.T) {
	h := newHarness(t, nil)

	calls := map[string]func() error{
		"empty name to register": func() error {
			var r servicev1.RegisterStoreResponse
			return h.invoke(withTenant(t), "RegisterStore", &servicev1.RegisterStoreRequest{Ref: "loja-acme"}, &r)
		},
		"name above the ceiling": func() error {
			var r servicev1.RegisterStoreResponse
			return h.invoke(withTenant(t), "RegisterStore", &servicev1.RegisterStoreRequest{Ref: "loja-acme", Name: strings.Repeat("é", 201)}, &r)
		},
		"name with a control character": func() error {
			var r servicev1.RegisterStoreResponse
			return h.invoke(withTenant(t), "RegisterStore", &servicev1.RegisterStoreRequest{Ref: "loja-acme", Name: "Loja\n1"}, &r)
		},
		"empty reason to suspend": func() error {
			var r servicev1.SuspendStoreResponse
			return h.invoke(withTenant(t), "SuspendStore", &servicev1.SuspendStoreRequest{Ref: "loja-acme"}, &r)
		},
		"reason above the ceiling": func() error {
			var r servicev1.SuspendStoreResponse
			return h.invoke(withTenant(t), "SuspendStore", &servicev1.SuspendStoreRequest{Ref: "loja-acme", Reason: strings.Repeat("a", 201)}, &r)
		},
		"ref to suspend outside the label form": func() error {
			var r servicev1.SuspendStoreResponse
			return h.invoke(withTenant(t), "SuspendStore", &servicev1.SuspendStoreRequest{Ref: "Loja_1", Reason: "motivo"}, &r)
		},
		"ref to reactivate outside the label form": func() error {
			var r servicev1.ReactivateStoreResponse
			return h.invoke(withTenant(t), "ReactivateStore", &servicev1.ReactivateStoreRequest{Ref: "loja acme"}, &r)
		},
		"ref to find outside the label form": func() error {
			var r servicev1.FindStoreResponse
			return h.invoke(withoutKey(t), "FindStore", &servicev1.FindStoreRequest{Ref: ""}, &r)
		},
		"unspecified status to find": func() error {
			var r servicev1.FindStoresByStatusResponse
			return h.invoke(withoutKey(t), "FindStoresByStatus", &servicev1.FindStoresByStatusRequest{}, &r)
		},
		"unknown status to find": func() error {
			var r servicev1.FindStoresByStatusResponse
			return h.invoke(withoutKey(t), "FindStoresByStatus", &servicev1.FindStoresByStatusRequest{Status: 9}, &r)
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); status.Code(err) != codes.InvalidArgument {
				t.Fatalf("call = %v, want InvalidArgument", err)
			}
		})
	}
	if len(h.store.Entries()) != 0 {
		t.Fatalf("outbox holds %d entries, want none: no malformed call reached the use case", len(h.store.Entries()))
	}
}

func TestRegisterStoreAppliesTheNameRule(t *testing.T) {
	for _, tc := range nameRuleCases {
		t.Run(tc.desc, func(t *testing.T) {
			h := newHarness(t, nil)

			var r servicev1.RegisterStoreResponse
			err := h.invoke(withTenant(t), "RegisterStore", &servicev1.RegisterStoreRequest{Ref: "loja-acme", Name: tc.name}, &r)

			if tc.accepted {
				if err != nil || r.GetRegistered().GetRef() != "loja-acme" {
					t.Fatalf("RegisterStore(%q) = %v (%v), want it registered", tc.name, err, &r)
				}
				return
			}
			if status.Code(err) != codes.InvalidArgument || len(h.store.Entries()) != 0 {
				t.Fatalf("RegisterStore(%q) = %v with %d outbox entries, want InvalidArgument and none", tc.name, err, len(h.store.Entries()))
			}
		})
	}
}

func TestSuspendStoreAppliesTheNameRuleToTheReason(t *testing.T) {
	for _, tc := range nameRuleCases {
		t.Run(tc.desc, func(t *testing.T) {
			h := newHarness(t, nil)
			register(t, h, "loja-acme", "Loja Acme")

			var r servicev1.SuspendStoreResponse
			err := h.invoke(withTenant(t), "SuspendStore", &servicev1.SuspendStoreRequest{Ref: "loja-acme", Reason: tc.name}, &r)

			if tc.accepted {
				if err != nil || r.GetSuspended().GetRef() != "loja-acme" {
					t.Fatalf("SuspendStore(%q) = %v (%v), want it suspended", tc.name, err, &r)
				}
				return
			}
			if status.Code(err) != codes.InvalidArgument || len(h.store.Entries()) != 1 {
				t.Fatalf("SuspendStore(%q) = %v with %d outbox entries, want InvalidArgument and only the registration", tc.name, err, len(h.store.Entries()))
			}
		})
	}
}

func TestRegisterStoreAcceptsANameAtTheCeilingInCharacters(t *testing.T) {
	h := newHarness(t, nil)

	if resp := register(t, h, "loja-acme", strings.Repeat("é", 200)); resp.GetRegistered().GetRef() != "loja-acme" {
		t.Fatalf("RegisterStore() = %v, want the 200-character name accepted: the ceiling counts characters, not bytes", resp)
	}
}
