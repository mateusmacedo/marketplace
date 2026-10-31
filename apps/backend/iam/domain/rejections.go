package domain

import kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

const (
	CodeRoleRefInvalid        kernel.Code = "iam/role/ref-invalid"
	CodeRoleReserved          kernel.Code = "iam/role/reserved"
	CodeRoleNameEmpty         kernel.Code = "iam/role/name-empty"
	CodeRolePermissionsEmpty  kernel.Code = "iam/role/permissions-empty"
	CodeRolePermissionUnknown kernel.Code = "iam/role/permission-unknown"
	CodeRoleDuplicateName     kernel.Code = "iam/role/duplicate-name"
	CodeRoleRetired           kernel.Code = "iam/role/retired"
	CodeRoleVersionAhead      kernel.Code = "iam/role/version-ahead"
)

var RejectRoleDuplicateName = kernel.Reject(CodeRoleDuplicateName, "a role with this name already exists in the store")

const (
	CodeInvitationEmailInvalid   kernel.Code = "iam/invitation/email-invalid"
	CodeInvitationRolesEmpty     kernel.Code = "iam/invitation/roles-empty"
	CodeInvitationRoleInvalid    kernel.Code = "iam/invitation/role-invalid"
	CodeInvitationRoleReserved   kernel.Code = "iam/invitation/role-reserved"
	CodeInvitationNotPending     kernel.Code = "iam/invitation/not-pending"
	CodeInvitationSubjectInvalid kernel.Code = "iam/invitation/subject-invalid"
	CodeInvitationReasonEmpty    kernel.Code = "iam/invitation/reason-empty"
)

const (
	CodeMembershipSubjectInvalid kernel.Code = "iam/membership/subject-invalid"
	CodeMembershipLastRole       kernel.Code = "iam/membership/last-role"
	CodeMembershipRoleInvalid    kernel.Code = "iam/membership/role-invalid"
	CodeMembershipOwnerClaimed   kernel.Code = "iam/membership/owner-claimed"
	CodeMembershipRemoved        kernel.Code = "iam/membership/removed"
	CodeMembershipOwnerReserved  kernel.Code = "iam/membership/owner-reserved"
	CodeMembershipOwnerProtected kernel.Code = "iam/membership/owner-protected"
	CodeMembershipRoleNotHeld    kernel.Code = "iam/membership/role-not-held"
	CodeMembershipVersionAhead   kernel.Code = "iam/membership/version-ahead"
)

// RejectMembershipOwnerClaimed is the refusal of the one active owner per
// store, which the provider declares and no decision produces (ADR-066).
var RejectMembershipOwnerClaimed = kernel.Reject(CodeMembershipOwnerClaimed, "the store already has an owner")

const (
	CodeBuyerSubjectInvalid kernel.Code = "iam/buyer/subject-invalid"
	CodeBuyerNameEmpty      kernel.Code = "iam/buyer/name-empty"
	CodeBuyerReasonEmpty    kernel.Code = "iam/buyer/reason-empty"
	CodeBuyerBlocked        kernel.Code = "iam/buyer/blocked"
	CodeBuyerNotActive      kernel.Code = "iam/buyer/not-active"
	CodeBuyerNotBlocked     kernel.Code = "iam/buyer/not-blocked"
	CodeBuyerVersionAhead   kernel.Code = "iam/buyer/version-ahead"
)
