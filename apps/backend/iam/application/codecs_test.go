package application_test

import (
	"bytes"
	"testing"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

// The stored format is frozen: an inbox entry written by one release must
// still decode in the next.
func TestTheResponsesKeepTheirStoredFormat(t *testing.T) {
	golden := []byte{0x01, 'a', 0x09, 'e', 'x', 'p', 'e', 'd', 'i', 'c', 'a', 'o'}
	for name, stored := range map[string][]byte{
		"defined":             usecase.EncodeOutcome(application.DefinedCodec, usecase.Accepted(domain.DefinedResponse{Ref: "expedicao"})),
		"permissions-changed": usecase.EncodeOutcome(application.PermissionsChangedCodec, usecase.Accepted(domain.PermissionsChangedResponse{Ref: "expedicao"})),
		"retired":             usecase.EncodeOutcome(application.RetiredCodec, usecase.Accepted(domain.RetiredResponse{Ref: "expedicao"})),
	} {
		if !bytes.Equal(stored, golden) {
			t.Errorf("%s: stored = %x, want %x", name, stored, golden)
		}
	}
}

func TestTheResponsesRoundTrip(t *testing.T) {
	defined, err := usecase.DecodeOutcome(application.DefinedCodec, usecase.EncodeOutcome(application.DefinedCodec, usecase.Accepted(domain.DefinedResponse{Ref: "papel-1"})))
	if err != nil || defined.Response() != (domain.DefinedResponse{Ref: "papel-1"}) {
		t.Errorf("defined = (%+v, %v), want {papel-1}", defined.Response(), err)
	}
	changed, err := usecase.DecodeOutcome(application.PermissionsChangedCodec, usecase.EncodeOutcome(application.PermissionsChangedCodec, usecase.Accepted(domain.PermissionsChangedResponse{Ref: "papel-2"})))
	if err != nil || changed.Response() != (domain.PermissionsChangedResponse{Ref: "papel-2"}) {
		t.Errorf("changed = (%+v, %v), want {papel-2}", changed.Response(), err)
	}
	retired, err := usecase.DecodeOutcome(application.RetiredCodec, usecase.EncodeOutcome(application.RetiredCodec, usecase.Accepted(domain.RetiredResponse{Ref: "papel-3"})))
	if err != nil || retired.Response() != (domain.RetiredResponse{Ref: "papel-3"}) {
		t.Errorf("retired = (%+v, %v), want {papel-3}", retired.Response(), err)
	}
}
