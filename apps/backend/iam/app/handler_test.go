package app_test

import (
	"context"
	"errors"
	"testing"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	"github.com/mateusmacedo/dmpf/libs/backend/go/contracts/envelope"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/app"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	eventv1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/event/v1"
)

var errReached = errors.New("the use case was reached")

func provisionedEnvelope(t *testing.T, provisioned *eventv1.InvitationProvisioned) envelope.Envelope {
	t.Helper()
	payload, typeURL, err := envelope.Pack(provisioned)
	if err != nil {
		t.Fatalf("Pack() = %v", err)
	}
	return envelope.Envelope{Type: "com.company.iam.invitation-provisioned.v1", DataSchema: typeURL, Payload: payload}
}

// reachable reports whether the handler handed the message to the use case:
// the first step of the consumption is the authorization.
func reachable() (application.Service, *bool) {
	reached := false
	return application.Service{Authorize: func(context.Context, application.Operation) error {
		reached = true
		return errReached
	}}, &reached
}

// An address outside the form is not this consumer's contract: it is terminal
// before any unit of work, as Postgres would refuse U+0000 in the snapshot.
func TestAProvisionedEmailOutsideTheFormIsTerminalValidationWithoutTheUseCase(t *testing.T) {
	for _, email := range []string{"ana\u0000@loja.com", "ana@lo\u0007ja.com", "ana"} {
		service, reached := reachable()

		disp, err := app.Handler(service)(context.Background(), ports.Receipt{MessageID: "evt-1"},
			provisionedEnvelope(t, &eventv1.InvitationProvisioned{Email: email, Subject: "kc-123", Roles: []string{"expedicao"}}))

		var failure *usecase.Failure
		if disp != usecase.R1D4 || !errors.As(err, &failure) || failure.Category() != usecase.Validation || failure.Retryable() {
			t.Errorf("Handler(%q) = (%v, %v), want R1D4 with a terminal Validation", email, disp, err)
		}
		if *reached {
			t.Errorf("Handler(%q) reached the use case, want it refused before", email)
		}
	}
}

// The address the domain accepts, and the empty one of an establishment
// without an email, reach the use case unchanged.
func TestAProvisionedEmailInTheFormOrEmptyReachesTheUseCase(t *testing.T) {
	for _, email := range []string{"ana@loja.com", ""} {
		service, reached := reachable()

		_, err := app.Handler(service)(context.Background(), ports.Receipt{MessageID: "evt-1"},
			provisionedEnvelope(t, &eventv1.InvitationProvisioned{Email: email, Subject: "kc-123", Roles: []string{"expedicao"}}))

		if !*reached || !errors.Is(err, errReached) {
			t.Errorf("Handler(%q) = %v, want the use case reached", email, err)
		}
	}
}
