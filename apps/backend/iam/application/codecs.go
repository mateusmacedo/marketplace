package application

import (
	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

var definedCodec = usecase.OutcomeCodec[domain.DefinedResponse]{
	Encode: func(e *usecase.Encoder, r domain.DefinedResponse) { e.String(string(r.Ref)) },
	Decode: func(d *usecase.Decoder) domain.DefinedResponse {
		return domain.DefinedResponse{Ref: domain.RoleRef(d.String())}
	},
}

var permissionsChangedCodec = usecase.OutcomeCodec[domain.PermissionsChangedResponse]{
	Encode: func(e *usecase.Encoder, r domain.PermissionsChangedResponse) { e.String(string(r.Ref)) },
	Decode: func(d *usecase.Decoder) domain.PermissionsChangedResponse {
		return domain.PermissionsChangedResponse{Ref: domain.RoleRef(d.String())}
	},
}

var retiredCodec = usecase.OutcomeCodec[domain.RetiredResponse]{
	Encode: func(e *usecase.Encoder, r domain.RetiredResponse) { e.String(string(r.Ref)) },
	Decode: func(d *usecase.Decoder) domain.RetiredResponse {
		return domain.RetiredResponse{Ref: domain.RoleRef(d.String())}
	},
}

var invitedCodec = usecase.OutcomeCodec[domain.InvitedResponse]{
	Encode: func(e *usecase.Encoder, r domain.InvitedResponse) {
		e.String(string(r.Email))
		e.String(r.Status.String())
	},
	Decode: func(d *usecase.Decoder) domain.InvitedResponse {
		return domain.InvitedResponse{Email: domain.Email(d.String()), Status: invitationStatusNamed(d.String())}
	},
}

func invitationStatusNamed(name string) domain.InvitationStatus {
	for _, status := range []domain.InvitationStatus{domain.InvitationStatusPending, domain.InvitationStatusProvisioned, domain.InvitationStatusFailed} {
		if status.String() == name {
			return status
		}
	}
	return domain.InvitationStatusUndefined
}

var claimedCodec = usecase.OutcomeCodec[domain.EstablishedResponse]{
	Encode: func(e *usecase.Encoder, r domain.EstablishedResponse) { e.String(string(r.Subject)) },
	Decode: func(d *usecase.Decoder) domain.EstablishedResponse {
		return domain.EstablishedResponse{Subject: domain.Subject(d.String())}
	},
}

var grantedCodec = usecase.OutcomeCodec[domain.GrantedResponse]{
	Encode: func(e *usecase.Encoder, r domain.GrantedResponse) { e.String(string(r.Subject)) },
	Decode: func(d *usecase.Decoder) domain.GrantedResponse {
		return domain.GrantedResponse{Subject: domain.Subject(d.String())}
	},
}

var revokedCodec = usecase.OutcomeCodec[domain.RevokedResponse]{
	Encode: func(e *usecase.Encoder, r domain.RevokedResponse) { e.String(string(r.Subject)) },
	Decode: func(d *usecase.Decoder) domain.RevokedResponse {
		return domain.RevokedResponse{Subject: domain.Subject(d.String())}
	},
}

var removedCodec = usecase.OutcomeCodec[domain.RemovedResponse]{
	Encode: func(e *usecase.Encoder, r domain.RemovedResponse) { e.String(string(r.Subject)) },
	Decode: func(d *usecase.Decoder) domain.RemovedResponse {
		return domain.RemovedResponse{Subject: domain.Subject(d.String())}
	},
}

var registeredCodec = usecase.OutcomeCodec[domain.RegisteredResponse]{
	Encode: func(e *usecase.Encoder, r domain.RegisteredResponse) {
		e.String(string(r.Subject))
		e.String(r.Status.String())
	},
	Decode: func(d *usecase.Decoder) domain.RegisteredResponse {
		return domain.RegisteredResponse{Subject: domain.Subject(d.String()), Status: buyerStatusNamed(d.String())}
	},
}

func buyerStatusNamed(name string) domain.BuyerStatus {
	for _, status := range []domain.BuyerStatus{domain.BuyerStatusActive, domain.BuyerStatusBlocked} {
		if status.String() == name {
			return status
		}
	}
	return domain.BuyerStatusUndefined
}

var profileUpdatedCodec = usecase.OutcomeCodec[domain.ProfileUpdatedResponse]{
	Encode: func(e *usecase.Encoder, r domain.ProfileUpdatedResponse) { e.String(string(r.Subject)) },
	Decode: func(d *usecase.Decoder) domain.ProfileUpdatedResponse {
		return domain.ProfileUpdatedResponse{Subject: domain.Subject(d.String())}
	},
}

var blockedCodec = usecase.OutcomeCodec[domain.BlockedResponse]{
	Encode: func(e *usecase.Encoder, r domain.BlockedResponse) { e.String(string(r.Subject)) },
	Decode: func(d *usecase.Decoder) domain.BlockedResponse {
		return domain.BlockedResponse{Subject: domain.Subject(d.String())}
	},
}

var unblockedCodec = usecase.OutcomeCodec[domain.UnblockedResponse]{
	Encode: func(e *usecase.Encoder, r domain.UnblockedResponse) { e.String(string(r.Subject)) },
	Decode: func(d *usecase.Decoder) domain.UnblockedResponse {
		return domain.UnblockedResponse{Subject: domain.Subject(d.String())}
	},
}
