//go:build integration

package app_test

import (
	"context"
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/app/rpc"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/appkit"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/service/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

type rejected interface {
	proto.Message
	GetRejection() *servicev1.Rejection
}

// U+0000 is valid UTF-8 for proto3, but Postgres refuses it in a text key
// (22021): before the fix each of these calls ended Internal.
func TestAnIDWithU0000IsNeverInternalOverPostgres(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)

	for _, tc := range []struct {
		method string
		tenant string
		req    proto.Message
		resp   func() rejected
		code   kernel.Code
	}{
		{"DefineRole", storeTenant, &servicev1.DefineRoleRequest{Ref: "a\u0000b", Name: "Expedição", Permissions: []string{"catalog:read"}}, func() rejected { return &servicev1.DefineRoleResponse{} }, domain.CodeRoleRefInvalid},
		{"InviteMember", storeTenant, &servicev1.InviteMemberRequest{Email: "ana\u0000@loja.com", Roles: []string{"expedicao"}}, func() rejected { return &servicev1.InviteMemberResponse{} }, domain.CodeInvitationEmailInvalid},
		{"ClaimOwnership", storeTenant, &servicev1.ClaimOwnershipRequest{Subject: "a\u0000b"}, func() rejected { return &servicev1.ClaimOwnershipResponse{} }, domain.CodeMembershipSubjectInvalid},
		{"RegisterBuyer", operatorTenant, &servicev1.RegisterBuyerRequest{Subject: "a\u0000b", Name: "Bia"}, func() rejected { return &servicev1.RegisterBuyerResponse{} }, domain.CodeBuyerSubjectInvalid},
	} {
		key := "e2e-form-" + tc.method
		for _, attempt := range []string{"first", "replay"} {
			resp := tc.resp()
			if err := conn.Invoke(callWithKey(t, tc.tenant, key), rpc.FullMethod(tc.method), tc.req, resp); err != nil {
				t.Fatalf("%s(U+0000) %s = %v, want the rejection %s", tc.method, attempt, err, tc.code)
			}
			if got := resp.GetRejection().GetCode(); got != string(tc.code) {
				t.Fatalf("%s(U+0000) %s = %v, want the rejection %s", tc.method, attempt, resp, tc.code)
			}
		}
	}

	for _, tc := range []struct {
		method    string
		tenant    string
		req, resp proto.Message
	}{
		{"GrantRole", storeTenant, &servicev1.GrantRoleRequest{Subject: "a\u0000b", Role: "expedicao"}, &servicev1.GrantRoleResponse{}},
		{"FindMembership", storeTenant, &servicev1.FindMembershipRequest{Subject: "a\u0000b"}, &servicev1.FindMembershipResponse{}},
		{"FindInvitation", storeTenant, &servicev1.FindInvitationRequest{Email: "ana\u0000@loja.com"}, &servicev1.FindInvitationResponse{}},
		{"BlockBuyer", operatorTenant, &servicev1.BlockBuyerRequest{Subject: "a\u0000b", Reason: "fraude"}, &servicev1.BlockBuyerResponse{}},
		{"FindBuyer", operatorTenant, &servicev1.FindBuyerRequest{Subject: "a\u0000b"}, &servicev1.FindBuyerResponse{}},
	} {
		if err := conn.Invoke(call(t, tc.tenant), rpc.FullMethod(tc.method), tc.req, tc.resp); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s(U+0000) = %v, want InvalidArgument", tc.method, err)
		}
	}

	for table, want := range map[string]int{"roles": 0, "invitations": 0, "memberships": 0, "buyers": 0, "outbox": 0} {
		if got := countRows(t, pool, table); got != want {
			t.Errorf("%s rows = %d, want %d", table, got, want)
		}
	}
	var refusals int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM inbox WHERE consumer_name = $1 AND status = 'rejected'", application.CommandConsumer).Scan(&refusals); err != nil || refusals != 4 {
		t.Fatalf("rejected commands in the inbox = %d, %v; want the 4 refusals, each once", refusals, err)
	}
}
