package application

import "github.com/mateusmacedo/marketplace/apps/backend/iam/domain"

var (
	DefinedCodec            = definedCodec
	PermissionsChangedCodec = permissionsChangedCodec
	RetiredCodec            = retiredCodec
	InvitedCodec            = invitedCodec
	ClaimedCodec            = claimedCodec
	GrantedCodec            = grantedCodec
	RevokedCodec            = revokedCodec
	RemovedCodec            = removedCodec
)

var (
	RegisteredCodec     = registeredCodec
	ProfileUpdatedCodec = profileUpdatedCodec
	BlockedCodec        = blockedCodec
	UnblockedCodec      = unblockedCodec
)

func EmailDigest(s Service, email domain.Email) string { return s.auditedEmail(email) }
