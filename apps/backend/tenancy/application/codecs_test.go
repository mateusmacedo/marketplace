package application_test

import (
	"bytes"
	"testing"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

// The stored format is frozen: an inbox entry written by one release must
// still decode in the next.
func TestTheResponsesKeepTheirStoredFormat(t *testing.T) {
	golden := []byte{0x01, 'a', 0x09, 'l', 'o', 'j', 'a', '-', 'a', 'c', 'm', 'e'}
	for name, stored := range map[string][]byte{
		"registered":  usecase.EncodeOutcome(application.RegisteredCodec, usecase.Accepted(domain.RegisteredResponse{Ref: "loja-acme"})),
		"suspended":   usecase.EncodeOutcome(application.SuspendedCodec, usecase.Accepted(domain.SuspendedResponse{Ref: "loja-acme"})),
		"reactivated": usecase.EncodeOutcome(application.ReactivatedCodec, usecase.Accepted(domain.ReactivatedResponse{Ref: "loja-acme"})),
	} {
		if !bytes.Equal(stored, golden) {
			t.Errorf("%s: stored = %x, want %x", name, stored, golden)
		}
	}
}

func TestTheResponsesRoundTrip(t *testing.T) {
	registered, err := usecase.DecodeOutcome(application.RegisteredCodec, usecase.EncodeOutcome(application.RegisteredCodec, usecase.Accepted(domain.RegisteredResponse{Ref: "loja-1"})))
	if err != nil || registered.Response() != (domain.RegisteredResponse{Ref: "loja-1"}) {
		t.Errorf("registered = (%+v, %v), want {loja-1}", registered.Response(), err)
	}
	suspended, err := usecase.DecodeOutcome(application.SuspendedCodec, usecase.EncodeOutcome(application.SuspendedCodec, usecase.Accepted(domain.SuspendedResponse{Ref: "loja-2"})))
	if err != nil || suspended.Response() != (domain.SuspendedResponse{Ref: "loja-2"}) {
		t.Errorf("suspended = (%+v, %v), want {loja-2}", suspended.Response(), err)
	}
	reactivated, err := usecase.DecodeOutcome(application.ReactivatedCodec, usecase.EncodeOutcome(application.ReactivatedCodec, usecase.Accepted(domain.ReactivatedResponse{Ref: "loja-3"})))
	if err != nil || reactivated.Response() != (domain.ReactivatedResponse{Ref: "loja-3"}) {
		t.Errorf("reactivated = (%+v, %v), want {loja-3}", reactivated.Response(), err)
	}
}
