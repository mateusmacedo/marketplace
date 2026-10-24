package application

import (
	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

var registeredCodec = usecase.OutcomeCodec[domain.RegisteredResponse]{
	Encode: func(e *usecase.Encoder, r domain.RegisteredResponse) { e.String(string(r.Ref)) },
	Decode: func(d *usecase.Decoder) domain.RegisteredResponse {
		return domain.RegisteredResponse{Ref: domain.StoreRef(d.String())}
	},
}

var suspendedCodec = usecase.OutcomeCodec[domain.SuspendedResponse]{
	Encode: func(e *usecase.Encoder, r domain.SuspendedResponse) { e.String(string(r.Ref)) },
	Decode: func(d *usecase.Decoder) domain.SuspendedResponse {
		return domain.SuspendedResponse{Ref: domain.StoreRef(d.String())}
	},
}

var reactivatedCodec = usecase.OutcomeCodec[domain.ReactivatedResponse]{
	Encode: func(e *usecase.Encoder, r domain.ReactivatedResponse) { e.String(string(r.Ref)) },
	Decode: func(d *usecase.Decoder) domain.ReactivatedResponse {
		return domain.ReactivatedResponse{Ref: domain.StoreRef(d.String())}
	},
}
