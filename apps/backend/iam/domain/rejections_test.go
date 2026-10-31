package domain_test

import (
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestDeclaredCodesAreValid(t *testing.T) {
	codes := map[string]kernel.Code{
		"CodeRoleRefInvalid":        domain.CodeRoleRefInvalid,
		"CodeRoleReserved":          domain.CodeRoleReserved,
		"CodeRoleNameEmpty":         domain.CodeRoleNameEmpty,
		"CodeRolePermissionsEmpty":  domain.CodeRolePermissionsEmpty,
		"CodeRolePermissionUnknown": domain.CodeRolePermissionUnknown,
		"CodeRoleDuplicateName":     domain.CodeRoleDuplicateName,
		"CodeRoleRetired":           domain.CodeRoleRetired,
		"CodeRoleVersionAhead":      domain.CodeRoleVersionAhead,

		"CodeInvitationEmailInvalid":   domain.CodeInvitationEmailInvalid,
		"CodeInvitationRolesEmpty":     domain.CodeInvitationRolesEmpty,
		"CodeInvitationRoleInvalid":    domain.CodeInvitationRoleInvalid,
		"CodeInvitationRoleReserved":   domain.CodeInvitationRoleReserved,
		"CodeInvitationNotPending":     domain.CodeInvitationNotPending,
		"CodeInvitationSubjectInvalid": domain.CodeInvitationSubjectInvalid,
		"CodeInvitationReasonEmpty":    domain.CodeInvitationReasonEmpty,

		"CodeMembershipSubjectInvalid": domain.CodeMembershipSubjectInvalid,
		"CodeMembershipLastRole":       domain.CodeMembershipLastRole,
		"CodeMembershipRoleInvalid":    domain.CodeMembershipRoleInvalid,
		"CodeMembershipOwnerClaimed":   domain.CodeMembershipOwnerClaimed,
		"CodeMembershipRemoved":        domain.CodeMembershipRemoved,
		"CodeMembershipOwnerReserved":  domain.CodeMembershipOwnerReserved,
		"CodeMembershipOwnerProtected": domain.CodeMembershipOwnerProtected,
		"CodeMembershipRoleNotHeld":    domain.CodeMembershipRoleNotHeld,
		"CodeMembershipVersionAhead":   domain.CodeMembershipVersionAhead,

		"CodeBuyerSubjectInvalid": domain.CodeBuyerSubjectInvalid,
		"CodeBuyerNameEmpty":      domain.CodeBuyerNameEmpty,
		"CodeBuyerReasonEmpty":    domain.CodeBuyerReasonEmpty,
		"CodeBuyerBlocked":        domain.CodeBuyerBlocked,
		"CodeBuyerNotActive":      domain.CodeBuyerNotActive,
		"CodeBuyerNotBlocked":     domain.CodeBuyerNotBlocked,
		"CodeBuyerVersionAhead":   domain.CodeBuyerVersionAhead,
	}
	for name, code := range codes {
		t.Run(name, func(t *testing.T) {
			if !code.Valid() {
				t.Fatalf("%s = %q is not a valid context/aggregate/reason code", name, code)
			}
		})
	}
}
