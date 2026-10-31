package domain_test

import (
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

// The codes, the event names and the reserved ref are published: a rename in
// Go must not move the value on the wire that the bff and the consumers read.
func TestRejectionCodesKeepTheirPublishedValues(t *testing.T) {
	codes := map[kernel.Code]string{
		domain.CodeRoleRefInvalid:        "iam/role/ref-invalid",
		domain.CodeRoleReserved:          "iam/role/reserved",
		domain.CodeRoleNameEmpty:         "iam/role/name-empty",
		domain.CodeRolePermissionsEmpty:  "iam/role/permissions-empty",
		domain.CodeRolePermissionUnknown: "iam/role/permission-unknown",
		domain.CodeRoleDuplicateName:     "iam/role/duplicate-name",
		domain.CodeRoleRetired:           "iam/role/retired",
		domain.CodeRoleVersionAhead:      "iam/role/version-ahead",

		domain.CodeInvitationEmailInvalid:   "iam/invitation/email-invalid",
		domain.CodeInvitationRolesEmpty:     "iam/invitation/roles-empty",
		domain.CodeInvitationRoleInvalid:    "iam/invitation/role-invalid",
		domain.CodeInvitationRoleReserved:   "iam/invitation/role-reserved",
		domain.CodeInvitationNotPending:     "iam/invitation/not-pending",
		domain.CodeInvitationSubjectInvalid: "iam/invitation/subject-invalid",
		domain.CodeInvitationReasonEmpty:    "iam/invitation/reason-empty",

		domain.CodeMembershipSubjectInvalid: "iam/membership/subject-invalid",
		domain.CodeMembershipLastRole:       "iam/membership/last-role",
		domain.CodeMembershipRoleInvalid:    "iam/membership/role-invalid",
		domain.CodeMembershipOwnerClaimed:   "iam/membership/owner-claimed",
		domain.CodeMembershipRemoved:        "iam/membership/removed",
		domain.CodeMembershipOwnerReserved:  "iam/membership/owner-reserved",
		domain.CodeMembershipOwnerProtected: "iam/membership/owner-protected",
		domain.CodeMembershipRoleNotHeld:    "iam/membership/role-not-held",
		domain.CodeMembershipVersionAhead:   "iam/membership/version-ahead",

		domain.CodeBuyerSubjectInvalid: "iam/buyer/subject-invalid",
		domain.CodeBuyerNameEmpty:      "iam/buyer/name-empty",
		domain.CodeBuyerReasonEmpty:    "iam/buyer/reason-empty",
		domain.CodeBuyerBlocked:        "iam/buyer/blocked",
		domain.CodeBuyerNotActive:      "iam/buyer/not-active",
		domain.CodeBuyerNotBlocked:     "iam/buyer/not-blocked",
		domain.CodeBuyerVersionAhead:   "iam/buyer/version-ahead",
	}
	for code, want := range codes {
		if string(code) != want {
			t.Errorf("code = %q, want the published value %q", code, want)
		}
	}
}

func TestEventNamesKeepTheirPublishedValues(t *testing.T) {
	names := map[string]kernel.DomainEvent{
		"iam.role-defined":             domain.RoleDefined{},
		"iam.role-permissions-changed": domain.RolePermissionsChanged{},
		"iam.role-retired":             domain.RoleRetired{},
		"iam.member-invited":           domain.MemberInvited{},
		"iam.invitation-provisioned":   domain.InvitationProvisioned{},
		"iam.invitation-failed":        domain.InvitationFailed{},
		"iam.membership-established":   domain.MembershipEstablished{},
		"iam.member-access-changed":    domain.MemberAccessChanged{},
		"iam.member-removed":           domain.MemberRemoved{},
		"iam.buyer-registered":         domain.BuyerRegistered{},
		"iam.buyer-profile-updated":    domain.BuyerProfileUpdated{},
		"iam.buyer-blocked":            domain.BuyerBlocked{},
		"iam.buyer-unblocked":          domain.BuyerUnblocked{},
	}
	for want, event := range names {
		if got := event.EventName(); got != want {
			t.Errorf("EventName() = %q, want the published value %q", got, want)
		}
	}
}

func TestTheReservedRoleIsOwner(t *testing.T) {
	if domain.OwnerRole != "owner" {
		t.Fatalf("OwnerRole = %q, want owner: it names the group seeded with every Organization", domain.OwnerRole)
	}
}

func TestRoleStatusNamesReadAsTheSpecWritesThem(t *testing.T) {
	names := map[domain.RoleStatus]string{
		domain.RoleStatusUndefined: "undefined",
		domain.RoleStatusActive:    "active",
		domain.RoleStatusRetired:   "retired",
	}
	for status, want := range names {
		if got := status.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}

func TestInvitationAndMembershipStatusNamesReadAsTheSpecWritesThem(t *testing.T) {
	invitations := map[domain.InvitationStatus]string{
		domain.InvitationStatusUndefined:   "undefined",
		domain.InvitationStatusPending:     "pending",
		domain.InvitationStatusProvisioned: "provisioned",
		domain.InvitationStatusFailed:      "failed",
	}
	for status, want := range invitations {
		if got := status.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
	memberships := map[domain.MembershipStatus]string{
		domain.MembershipStatusUndefined: "undefined",
		domain.MembershipStatusActive:    "active",
		domain.MembershipStatusRemoved:   "removed",
	}
	for status, want := range memberships {
		if got := status.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}

func TestBuyerStatusNamesReadAsTheSpecWritesThem(t *testing.T) {
	names := map[domain.BuyerStatus]string{
		domain.BuyerStatusUndefined: "undefined",
		domain.BuyerStatusActive:    "active",
		domain.BuyerStatusBlocked:   "blocked",
	}
	for status, want := range names {
		if got := status.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}
