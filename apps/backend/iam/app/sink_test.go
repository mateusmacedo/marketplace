package app_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	kernelapp "github.com/mateusmacedo/dmpf/libs/backend/go/app"
	"github.com/mateusmacedo/dmpf/libs/backend/go/application"
	"github.com/mateusmacedo/dmpf/libs/backend/go/contracts/envelope"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/app"
	eventv1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/event/v1"
)

const (
	invitationProvisionedV1 = "com.company.iam.invitation-provisioned.v1"
	iamTopic                = "iam.events"
)

type fakeHandler struct{ calls int }

func (h *fakeHandler) handle(context.Context, ports.Receipt, envelope.Envelope) (application.Disposition, error) {
	h.calls++
	return application.R1D1, nil
}

type fakeAck struct{ acks, releases int }

func (a *fakeAck) Ack(context.Context) error     { a.acks++; return nil }
func (a *fakeAck) Release(context.Context) error { a.releases++; return nil }

type fakeContainment struct{ contained []ports.Contained }

func (c *fakeContainment) Quarantine(_ context.Context, contained ports.Contained) error {
	c.contained = append(c.contained, contained)
	return nil
}

type sinkClock struct{}

func (sinkClock) Now() ports.Instant { return ports.Instant(1_756_000_000_000_000_000) }

func rawEnvelope(t *testing.T, eventType, source string, msg proto.Message) []byte {
	t.Helper()
	payload, typeURL, err := envelope.Pack(msg)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	tenant := "loja-acme"
	ce, err := envelope.Encode(envelope.Envelope{
		ID:              "evt-1",
		Source:          source,
		SpecVersion:     envelope.SpecVersion,
		Type:            eventType,
		Subject:         "kc-123",
		Time:            timestamppb.New(time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)),
		DataSchema:      typeURL,
		DataContentType: envelope.ContentType,
		CorrelationID:   "corr-1",
		CausationID:     "evt-1",
		PartitionKey:    "kc-123",
		TraceParent:     "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
		TenantID:        &tenant,
		Payload:         payload,
	})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(ce)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return raw
}

var relaySource = app.Defaults(app.RoleRelay).Relay.Source

func newSink(handler *fakeHandler, containment *fakeContainment) app.Sink {
	return app.Sink{
		Consumer: kernelapp.Consumer{
			Name:        app.ConsumerName,
			MaxAttempts: 2,
			Handle:      handler.handle,
			Containment: containment,
			Clock:       sinkClock{},
			Timeout:     10 * time.Second,
			Boundary:    app.MembershipBoundary(app.Defaults(app.RoleConsumer), false),
			Locale:      "en",
			Channel:     kernelapp.Channel{Address: iamTopic, Group: app.ConsumerName},
		},
		EventType: invitationProvisionedV1,
	}
}

func provisioned() *eventv1.InvitationProvisioned {
	return &eventv1.InvitationProvisioned{Email: "ana@loja.com", Subject: "kc-123", Roles: []string{"expedicao"}}
}

func TestTheSinkHandsTheProvisionedInvitationToTheAdapter(t *testing.T) {
	handler, containment, ack := &fakeHandler{}, &fakeContainment{}, &fakeAck{}

	if err := newSink(handler, containment).Handle(context.Background(), rawEnvelope(t, invitationProvisionedV1, relaySource, provisioned()), 1, ack); err != nil {
		t.Fatalf("Handle() = %v, want nil", err)
	}

	if handler.calls != 1 || ack.acks != 1 || len(containment.contained) != 0 {
		t.Fatalf("handler=%d acks=%d contained=%d, want the adapter to run and ack once", handler.calls, ack.acks, len(containment.contained))
	}
}

// Every other event of iam.events is not addressed to this consumer: it is
// acknowledged untouched, so the database is never reached for it.
func TestTheSinkAcknowledgesEveryOtherEventOfTheContextWithoutTheAdapter(t *testing.T) {
	for eventType, msg := range map[string]proto.Message{
		"com.company.iam.member-invited.v1":         &eventv1.MemberInvited{Email: "ana@loja.com", Roles: []string{"expedicao"}, Round: 1},
		"com.company.iam.role-defined.v1":           &eventv1.RoleDefined{Ref: "expedicao", Name: "Expedição", Permissions: []string{"catalog:read"}},
		"com.company.iam.membership-established.v1": &eventv1.MembershipEstablished{Subject: "kc-123", Roles: []string{"expedicao"}},
	} {
		t.Run(eventType, func(t *testing.T) {
			handler, containment, ack := &fakeHandler{}, &fakeContainment{}, &fakeAck{}

			if err := newSink(handler, containment).Handle(context.Background(), rawEnvelope(t, eventType, relaySource, msg), 1, ack); err != nil {
				t.Fatalf("Handle() = %v, want nil", err)
			}

			if handler.calls != 0 || len(containment.contained) != 0 || ack.acks != 1 || ack.releases != 0 {
				t.Fatalf("handler=%d contained=%d acks=%d releases=%d, want only the ack", handler.calls, len(containment.contained), ack.acks, ack.releases)
			}
		})
	}
}

func TestTheSinkLeavesAnInvalidEnvelopeToTheAdapterWhichQuarantinesIt(t *testing.T) {
	handler, containment, ack := &fakeHandler{}, &fakeContainment{}, &fakeAck{}

	if err := newSink(handler, containment).Handle(context.Background(), []byte("not a cloudevent"), 1, ack); err != nil {
		t.Fatalf("Handle() = %v, want nil", err)
	}

	if handler.calls != 0 || len(containment.contained) != 1 || ack.acks != 1 {
		t.Fatalf("handler=%d contained=%d acks=%d, want the adapter to quarantine it (INB-10)", handler.calls, len(containment.contained), ack.acks)
	}
}

// CTX-27: only the relay of this context produces what the consumer admits; a
// provisioned invitation from any other source never reaches the use case.
func TestAProvisionedInvitationFromAnotherSourceIsQuarantinedAsUntrusted(t *testing.T) {
	handler, containment, ack := &fakeHandler{}, &fakeContainment{}, &fakeAck{}

	if err := newSink(handler, containment).Handle(context.Background(), rawEnvelope(t, invitationProvisionedV1, "urn:dmpf:intruder", provisioned()), 1, ack); err != nil {
		t.Fatalf("Handle() = %v, want nil", err)
	}

	if handler.calls != 0 || len(containment.contained) != 1 || containment.contained[0].Reason != ports.ReasonUntrustedBoundary {
		t.Fatalf("handler=%d contained=%+v, want the delivery quarantined as %s", handler.calls, containment.contained, ports.ReasonUntrustedBoundary)
	}
}

func TestTheSinkLogsAnotherTypeAtDebug(t *testing.T) {
	var out bytes.Buffer
	sink := newSink(&fakeHandler{}, &fakeContainment{})
	sink.Logger = slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug}))

	if err := sink.Handle(context.Background(), rawEnvelope(t, "com.company.iam.member-invited.v1", relaySource, &eventv1.MemberInvited{}), 1, &fakeAck{}); err != nil {
		t.Fatalf("Handle() = %v, want nil", err)
	}

	if got := out.String(); strings.Count(got, "\n") != 1 || !strings.Contains(got, "level=DEBUG") || !strings.Contains(got, "delivery of another event type acknowledged") {
		t.Fatalf("log = %q, want the one discard record at debug (RF-A5)", got)
	}
}
