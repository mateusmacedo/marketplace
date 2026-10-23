package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/tracing"
	"go.opentelemetry.io/otel/trace"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app/rpc"
)

const maxBodyBytes = 1 << 16

var (
	errMissingResult = errors.New("api: the context answered without a result")
	errUnknownStatus = errors.New("api: the context answered an unknown status")
)

// handlers holds the client of every context the edge forwards to; each
// context adds its own field, and its routes build their handler from it.
type handlers struct {
	tenancy rpc.Tenancy
}

type rejection struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type refusal interface {
	GetCode() string
	GetMessage() string
}

// form is the shape a path or query value must have: a value outside it is
// refused with a 400 that states the shape, before any call to a context.
type form struct {
	valid func(string) bool
	shape string
}

func (f form) accept(w http.ResponseWriter, r *http.Request, name, value string) bool {
	if f.valid(value) {
		return true
	}
	writeRejection(r, w, http.StatusBadRequest, "invalid-request", name+" must "+f.shape)
	return false
}

// decodeBody reads exactly one JSON object of the contract: unknown fields and
// trailing data are refused, so nothing half-parsed reaches a context.
func decodeBody(w http.ResponseWriter, r *http.Request, into any) error {
	stream := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	stream.DisallowUnknownFields()
	if err := stream.Decode(into); err != nil {
		return err
	}
	if err := stream.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("api: trailing data after the request body")
	}
	return nil
}

func writeFailure(w http.ResponseWriter, r *http.Request, err error) {
	failure := rpc.Classify(err)
	if failure.Status >= http.StatusInternalServerError {
		tracing.RecordError(trace.SpanFromContext(r.Context()), rpc.Category(err))
	}
	writeRejection(r, w, failure.Status, failure.Code, failure.Message)
}

func writeRefusal(r *http.Request, w http.ResponseWriter, refused refusal) {
	writeRejection(r, w, http.StatusUnprocessableEntity, refused.GetCode(), refused.GetMessage())
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
	if status < http.StatusInternalServerError && rpc.Replayed(r.Context()) {
		w.Header().Set(ReplayedHeader, "true")
	}
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		tracing.RecordError(trace.SpanFromContext(r.Context()), rpc.Category(err))
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

type decoder[Req any] func(w http.ResponseWriter, r *http.Request) (Req, bool)

func endpoint[Req, Resp any](decode decoder[Req], call func(context.Context, Req) (Resp, error), present func(http.ResponseWriter, *http.Request, Resp)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, ok := decode(w, r)
		if !ok {
			return
		}
		resp, err := call(r.Context(), req)
		if err != nil {
			writeFailure(w, r, err)
			return
		}
		present(w, r, resp)
	}
}

func fromPath[Req any](name string, shape form, build func(id string) Req) decoder[Req] {
	return func(w http.ResponseWriter, r *http.Request) (Req, bool) {
		id := r.PathValue(name)
		if !shape.accept(w, r, name, id) {
			var none Req
			return none, false
		}
		return build(id), true
	}
}

func fromQuery[Req any](name string, shape form, build func(value string) Req) decoder[Req] {
	return func(w http.ResponseWriter, r *http.Request) (Req, bool) {
		value := r.URL.Query().Get(name)
		if !shape.accept(w, r, name, value) {
			var none Req
			return none, false
		}
		return build(value), true
	}
}

func fromBody[Body, Req any](contract, invalid string, valid func(Body) bool, build func(Body) Req) decoder[Req] {
	return func(w http.ResponseWriter, r *http.Request) (Req, bool) {
		var none Req
		var body Body
		if err := decodeBody(w, r, &body); err != nil {
			writeRejection(r, w, http.StatusBadRequest, "malformed-body", "the body is not the "+contract+" of the contract")
			return none, false
		}
		if !valid(body) {
			writeRejection(r, w, http.StatusBadRequest, "invalid-request", invalid)
			return none, false
		}
		return build(body), true
	}
}

func outcome[Resp any](status int, present func(Resp) (any, refusal)) func(http.ResponseWriter, *http.Request, Resp) {
	return func(w http.ResponseWriter, r *http.Request, resp Resp) {
		body, refused := present(resp)
		switch {
		case refused != nil:
			writeRefusal(r, w, refused)
		case body == nil:
			writeFailure(w, r, errMissingResult)
		default:
			writeJSON(r, w, status, body)
		}
	}
}

func view[Resp any](present func(Resp) (any, bool)) func(http.ResponseWriter, *http.Request, Resp) {
	return func(w http.ResponseWriter, r *http.Request, resp Resp) {
		body, known := present(resp)
		if !known {
			writeFailure(w, r, errUnknownStatus)
			return
		}
		writeJSON(r, w, http.StatusOK, body)
	}
}

func fromPathAndBody[Body, Req any](name string, shape form, contract, invalid string, valid func(Body) bool, build func(id string, body Body) Req) decoder[Req] {
	body := fromBody(contract, invalid, valid, func(b Body) Body { return b })
	return func(w http.ResponseWriter, r *http.Request) (Req, bool) {
		var none Req
		id := r.PathValue(name)
		if !shape.accept(w, r, name, id) {
			return none, false
		}
		decoded, ok := body(w, r)
		if !ok {
			return none, false
		}
		return build(id, decoded), true
	}
}
