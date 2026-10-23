package api

import (
	"net/http"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
)

type profile struct {
	Subject string `json:"subject"`
	Tenant  string `json:"tenant"`
}

// me answers the identity the edge resolved from the verified token, never one
// the request asserts: withExecutionContext already refused any divergence.
func me(w http.ResponseWriter, r *http.Request) {
	execution, mounted := ports.ExecutionContextFrom(r.Context())
	subject, hasSubject := execution.Subject()
	tenant, hasTenant := execution.Tenant()
	if !mounted || !hasSubject || !hasTenant {
		writeInternal(w, r)
		return
	}
	writeJSON(r, w, http.StatusOK, profile{Subject: string(subject), Tenant: string(tenant)})
}
