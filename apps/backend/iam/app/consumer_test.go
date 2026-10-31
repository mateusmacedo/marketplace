package app

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	kernelapp "github.com/mateusmacedo/dmpf/libs/backend/go/app"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/channel"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type identifiedMeters struct{ metric.MeterProvider }

func TestTheMembershipConsumerProcessesOnTheIamTopicAndItsGroup(t *testing.T) {
	cfg := Defaults(RoleConsumer)
	cfg.IamTopic, cfg.IamDLQ, cfg.Group = "dev.iam.events", "dev.iam.events.dlq", "iam-membership-dev"
	provider := sdktrace.NewTracerProvider()
	t.Cleanup(func() { _ = provider.Shutdown(t.Context()) })
	tracer := provider.Tracer("consumer-test")
	meters := &identifiedMeters{MeterProvider: metricnoop.NewMeterProvider()}
	pool, err := pgxpool.New(t.Context(), "postgres://iam@127.0.0.1:1/iam")
	if err != nil {
		t.Fatalf("pgxpool.New() = %v, want a lazy pool", err)
	}
	t.Cleanup(pool.Close)
	logs := sdklog.NewLoggerProvider()

	consumer := NewIamConsumer(pool, cfg, MembershipChannel(cfg), false, tracer, meters, logs)

	if consumer.Name != ConsumerName || ConsumerName != "iam-membership" {
		t.Fatalf("Name = %q, want the inbox of the membership consumer", consumer.Name)
	}
	if consumer.Tracer != tracer || consumer.MeterProvider != meters || consumer.LoggerProvider != logs || consumer.System != "kafka" {
		t.Fatalf("consumer = %+v, want the runtime's telemetry over kafka", consumer)
	}
	if want := (kernelapp.Channel{Address: "dev.iam.events", Group: "iam-membership-dev"}); consumer.Channel != want {
		t.Fatalf("Channel = %+v, want %+v: the physical topic and the transport's group (RF-B9)", consumer.Channel, want)
	}
	if consumer.MaxAttempts != channel.InlineAttempts || consumer.Timeout != cfg.ConsumerTimeout {
		t.Fatalf("MaxAttempts %d and Timeout %v, want the channel's %d and the configured %v", consumer.MaxAttempts, consumer.Timeout, channel.InlineAttempts, cfg.ConsumerTimeout)
	}
}

// The consumer reads the channel the relay of this very context publishes, and
// subscribes to the provisioned invitations alone.
func TestTheMembershipChannelIsTheTopicOfTheRelayForTheProvisionedInvitations(t *testing.T) {
	cfg := Defaults(RoleConsumer)
	cfg.IamTopic, cfg.IamDLQ, cfg.Group = "iam.events", "iam.events.dlq", "iam-membership"

	consumed, published := MembershipChannel(cfg), IamChannel(cfg)

	if consumed.Name != published.Name || consumed.Address != published.Address || consumed.Containment != published.Containment {
		t.Fatalf("consumed %s on %s, published %s on %s: want the same channel", consumed.Name, consumed.Address, published.Name, published.Address)
	}
	if got := channel.EventTypeOf(consumed); got != "com.company.iam.invitation-provisioned.v1" {
		t.Fatalf("EventTypeOf() = %q, want the provisioned invitation", got)
	}
}

func TestTheBoundaryAdmitsTheSourceTheRelayOfThisContextStamps(t *testing.T) {
	relay := Defaults(RoleRelay).Relay.Source

	boundary := MembershipBoundary(Defaults(RoleConsumer), true)

	if !slices.Equal(boundary.Sources, []string{relay}) || relay != "urn:dmpf:reference-iam" {
		t.Fatalf("Sources = %v, want only %q, the source the relay stamps", boundary.Sources, relay)
	}
	if boundary.Transport != kernelapp.TransportVerified {
		t.Fatalf("Transport = %q, want verified: TLS with an authenticated client", boundary.Transport)
	}
}

// IDN-04: a boundary is verified only on what the transport proved.
func TestTheBoundaryWithoutAnAuthenticatedClientIsDevelopmentOnly(t *testing.T) {
	if got := MembershipBoundary(Defaults(RoleConsumer), false).Transport; got != kernelapp.TransportDevelopmentOnly {
		t.Fatalf("Transport = %q, want development-only", got)
	}
}
