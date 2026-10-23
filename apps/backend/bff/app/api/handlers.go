package api

import (
	"encoding/json"
	"net/http"

	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/tracing"
	"go.opentelemetry.io/otel/trace"
)

// categoryUnexpected is the FND-07 category of a failure the edge did not
// anticipate: a panic or an execution context it could not mount.
const categoryUnexpected = "Unexpected"

type rejection struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeRejection(r *http.Request, w http.ResponseWriter, status int, code, message string) {
	writeJSON(r, w, status, rejection{Code: code, Message: message})
}

// WHY: the status line is already on the wire when the body fails, so the only
// thing left is to say so on the span; silence makes a truncated answer look
// like a success in every dashboard.
func writeJSON(r *http.Request, w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		tracing.RecordError(trace.SpanFromContext(r.Context()), categoryUnexpected)
	}
}

// WHY: the published contract fixes Rejection as the body of every 4xx, and
// the kernel's default refusal is plain text, which no generated client parses.
func refuseAsRejection(w http.ResponseWriter, r *http.Request, status int, reason string) {
	code := "admission-refused"
	if status == http.StatusNotFound {
		code = "not-found"
	}
	writeRejection(r, w, status, code, reason)
}

func writeInternal(w http.ResponseWriter, r *http.Request) {
	writeRejection(r, w, http.StatusInternalServerError, "internal-failure", "the request could not be completed")
}
