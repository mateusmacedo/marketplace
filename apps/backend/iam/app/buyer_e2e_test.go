//go:build integration

package app_test

import (
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/app/rpc"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/appkit"
	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/service/v1"
)

func buyerOverGRPC(t *testing.T, conn *grpc.ClientConn, subject string) *servicev1.Buyer {
	t.Helper()
	var found servicev1.FindBuyerResponse
	if err := conn.Invoke(call(t, operatorTenant), rpc.FullMethod("FindBuyer"), &servicev1.FindBuyerRequest{Subject: subject}, &found); err != nil {
		t.Fatalf("FindBuyer(%s) = %v", subject, err)
	}
	return found.GetBuyer()
}

func invokeInOperator(t *testing.T, conn *grpc.ClientConn, method string, req, resp proto.Message) {
	t.Helper()
	if err := conn.Invoke(call(t, operatorTenant), rpc.FullMethod(method), req, resp); err != nil {
		t.Fatalf("%s() = %v", method, err)
	}
}

func TestTheLifecycleOfABuyerEndToEndInTheOperatorsTenant(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)

	var registered servicev1.RegisterBuyerResponse
	invokeInOperator(t, conn, "RegisterBuyer", &servicev1.RegisterBuyerRequest{Subject: "kc-123", Name: "Bia"}, &registered)
	if r := registered.GetRegistered(); r.GetSubject() != "kc-123" || r.GetStatus() != servicev1.BuyerStatus_BUYER_STATUS_ACTIVE {
		t.Fatalf("RegisterBuyer() = %v, want kc-123 ACTIVE", &registered)
	}
	invokeInOperator(t, conn, "UpdateBuyerProfile", &servicev1.UpdateBuyerProfileRequest{Subject: "kc-123", Name: "Bia Souza"}, &servicev1.UpdateBuyerProfileResponse{})
	invokeInOperator(t, conn, "BlockBuyer", &servicev1.BlockBuyerRequest{Subject: "kc-123", Reason: "chargeback recorrente"}, &servicev1.BlockBuyerResponse{})

	var blocked servicev1.FindBuyersByStatusResponse
	invokeInOperator(t, conn, "FindBuyersByStatus", &servicev1.FindBuyersByStatusRequest{Status: servicev1.BuyerStatus_BUYER_STATUS_BLOCKED}, &blocked)
	if buyers := blocked.GetBuyers(); len(buyers) != 1 || buyers[0].GetName() != "Bia Souza" || buyers[0].GetBlockReason() != "chargeback recorrente" || buyers[0].GetAccessVersion() != 2 {
		t.Fatalf("FindBuyersByStatus(blocked) = %v, want Bia Souza blocked by chargeback at access version 2", buyers)
	}

	var again servicev1.RegisterBuyerResponse
	invokeInOperator(t, conn, "RegisterBuyer", &servicev1.RegisterBuyerRequest{Subject: "kc-123", Name: "Bia"}, &again)
	if again.GetRegistered().GetStatus() != servicev1.BuyerStatus_BUYER_STATUS_BLOCKED {
		t.Fatalf("RegisterBuyer(known) = %v, want the blocked status kept", &again)
	}

	invokeInOperator(t, conn, "UnblockBuyer", &servicev1.UnblockBuyerRequest{Subject: "kc-123"}, &servicev1.UnblockBuyerResponse{})
	if b := buyerOverGRPC(t, conn, "kc-123"); b.GetStatus() != servicev1.BuyerStatus_BUYER_STATUS_ACTIVE || b.GetBlockReason() != "" || b.GetAccessVersion() != 3 {
		t.Fatalf("FindBuyer() = %v, want ACTIVE without reason at access version 3", b)
	}
	if rows, outbox := countRows(t, pool, "buyers"), outboxRows(t, pool); rows != 1 || outbox != 4 {
		t.Fatalf("buyers rows = %d and outbox rows = %d, want 1 and 4: registered, profile updated, blocked and unblocked", rows, outbox)
	}
}

// The store's iam:write never reaches a buyer, and the operator's never
// reaches a store: each side is refused before any write.
func TestAStoreOnTheBuyersAndTheOperatorOnAStoreArePermissionDeniedWithoutWriting(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)
	invokeInOperator(t, conn, "RegisterBuyer", &servicev1.RegisterBuyerRequest{Subject: "kc-123", Name: "Bia"}, &servicev1.RegisterBuyerResponse{})

	for method, req := range map[string][2]proto.Message{
		"BlockBuyer":         {&servicev1.BlockBuyerRequest{Subject: "kc-123", Reason: "fraude"}, &servicev1.BlockBuyerResponse{}},
		"UnblockBuyer":       {&servicev1.UnblockBuyerRequest{Subject: "kc-123"}, &servicev1.UnblockBuyerResponse{}},
		"RegisterBuyer":      {&servicev1.RegisterBuyerRequest{Subject: "kc-456", Name: "Caio"}, &servicev1.RegisterBuyerResponse{}},
		"UpdateBuyerProfile": {&servicev1.UpdateBuyerProfileRequest{Subject: "kc-123", Name: "Outro"}, &servicev1.UpdateBuyerProfileResponse{}},
		"FindBuyer":          {&servicev1.FindBuyerRequest{Subject: "kc-123"}, &servicev1.FindBuyerResponse{}},
		"FindBuyersByStatus": {&servicev1.FindBuyersByStatusRequest{Status: servicev1.BuyerStatus_BUYER_STATUS_ACTIVE}, &servicev1.FindBuyersByStatusResponse{}},
	} {
		if err := conn.Invoke(call(t, storeTenant), rpc.FullMethod(method), req[0], req[1]); status.Code(err) != codes.PermissionDenied {
			t.Errorf("%s() from %s = %v, want PermissionDenied", method, storeTenant, err)
		}
	}
	for method, req := range map[string][2]proto.Message{
		"DefineRole":     {defineRequest("expedicao", "Expedição"), &servicev1.DefineRoleResponse{}},
		"ClaimOwnership": {&servicev1.ClaimOwnershipRequest{Subject: "kc-123"}, &servicev1.ClaimOwnershipResponse{}},
		"InviteMember":   {&servicev1.InviteMemberRequest{Email: "ana@loja.com", Roles: []string{"expedicao"}}, &servicev1.InviteMemberResponse{}},
	} {
		if err := conn.Invoke(call(t, operatorTenant), rpc.FullMethod(method), req[0], req[1]); status.Code(err) != codes.PermissionDenied {
			t.Errorf("%s() from the operator = %v, want PermissionDenied", method, err)
		}
	}

	if b := buyerOverGRPC(t, conn, "kc-123"); b.GetStatus() != servicev1.BuyerStatus_BUYER_STATUS_ACTIVE || b.GetName() != "Bia" || b.GetAccessVersion() != 1 {
		t.Fatalf("FindBuyer() = %v, want Bia ACTIVE at access version 1, untouched", b)
	}
	for table, want := range map[string]int{"buyers": 1, "outbox": 1, "roles": 0, "invitations": 0, "memberships": 0} {
		if got := countRows(t, pool, table); got != want {
			t.Errorf("%s rows = %d, want %d", table, got, want)
		}
	}
}
