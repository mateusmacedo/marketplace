package rpc_test

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/service/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

const (
	nulSubject = "a\u0000b"
	nulEmail   = "ana\u0000@loja.com"
)

// The subject of a path segment of the bff, or of the token in /iam/buyer/me,
// has no refusal of its own in the spec: outside the form it is refused at
// the edge, before any use case, as the ref and the email of a path are.
func TestASubjectOrEmailOfThePathOutsideTheFormIsInvalidArgument(t *testing.T) {
	for method, req := range map[string][2]proto.Message{
		"GrantRole":          {&servicev1.GrantRoleRequest{Subject: nulSubject, Role: "expedicao"}, &servicev1.GrantRoleResponse{}},
		"RevokeRole":         {&servicev1.RevokeRoleRequest{Subject: nulSubject, Role: "expedicao"}, &servicev1.RevokeRoleResponse{}},
		"RemoveMember":       {&servicev1.RemoveMemberRequest{Subject: nulSubject}, &servicev1.RemoveMemberResponse{}},
		"FindMembership":     {&servicev1.FindMembershipRequest{Subject: nulSubject}, &servicev1.FindMembershipResponse{}},
		"UpdateBuyerProfile": {&servicev1.UpdateBuyerProfileRequest{Subject: nulSubject, Name: "Bia"}, &servicev1.UpdateBuyerProfileResponse{}},
		"BlockBuyer":         {&servicev1.BlockBuyerRequest{Subject: nulSubject, Reason: "fraude"}, &servicev1.BlockBuyerResponse{}},
		"UnblockBuyer":       {&servicev1.UnblockBuyerRequest{Subject: nulSubject}, &servicev1.UnblockBuyerResponse{}},
		"FindBuyer":          {&servicev1.FindBuyerRequest{Subject: nulSubject}, &servicev1.FindBuyerResponse{}},
		"FindInvitation":     {&servicev1.FindInvitationRequest{Email: nulEmail}, &servicev1.FindInvitationResponse{}},
	} {
		h := newHarness(t, nil)

		err := h.invoke(withTenant(t), method, req[0], req[1])

		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s(U+0000) = %v, want InvalidArgument", method, err)
		}
		if n := len(h.store.Entries()); n != 0 {
			t.Errorf("%s(U+0000): outbox = %d entries, want none", method, n)
		}
	}
	h := newHarness(t, nil)
	if err := h.invoke(withTenant(t), "RemoveMember", &servicev1.RemoveMemberRequest{}, &servicev1.RemoveMemberResponse{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("RemoveMember(empty subject) = %v, want InvalidArgument", err)
	}
}

func rejectionCode(resp interface{ GetRejection() *servicev1.Rejection }) string {
	return resp.GetRejection().GetCode()
}

// An id of the body is judged by the domain, whose rejection travels in the
// response: never a status, never Internal.
func TestAnIDOfTheBodyOutsideTheFormIsTheRejectionOfTheSpec(t *testing.T) {
	h := newHarness(t, nil)

	var defined servicev1.DefineRoleResponse
	if err := h.invoke(withTenant(t), "DefineRole", &servicev1.DefineRoleRequest{Ref: nulSubject, Name: "Expedição", Permissions: []string{"catalog:read"}}, &defined); err != nil || rejectionCode(&defined) != string(domain.CodeRoleRefInvalid) {
		t.Errorf("DefineRole(U+0000) = %v, %v; want %s", &defined, err, domain.CodeRoleRefInvalid)
	}
	var invited servicev1.InviteMemberResponse
	if err := h.invoke(withTenant(t), "InviteMember", &servicev1.InviteMemberRequest{Email: nulEmail, Roles: []string{"expedicao"}}, &invited); err != nil || rejectionCode(&invited) != string(domain.CodeInvitationEmailInvalid) {
		t.Errorf("InviteMember(U+0000) = %v, %v; want %s", &invited, err, domain.CodeInvitationEmailInvalid)
	}
	var claimed servicev1.ClaimOwnershipResponse
	if err := h.invoke(withTenant(t), "ClaimOwnership", &servicev1.ClaimOwnershipRequest{Subject: nulSubject}, &claimed); err != nil || rejectionCode(&claimed) != string(domain.CodeMembershipSubjectInvalid) {
		t.Errorf("ClaimOwnership(U+0000) = %v, %v; want %s", &claimed, err, domain.CodeMembershipSubjectInvalid)
	}
	var registered servicev1.RegisterBuyerResponse
	if err := h.invoke(withTenant(t), "RegisterBuyer", &servicev1.RegisterBuyerRequest{Subject: nulSubject, Name: "Bia"}, &registered); err != nil || rejectionCode(&registered) != string(domain.CodeBuyerSubjectInvalid) {
		t.Errorf("RegisterBuyer(U+0000) = %v, %v; want %s", &registered, err, domain.CodeBuyerSubjectInvalid)
	}
	if n := len(h.store.Entries()); n != 0 {
		t.Fatalf("outbox = %d entries, want none", n)
	}
}
