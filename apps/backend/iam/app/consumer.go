package app

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	kernelapp "github.com/mateusmacedo/dmpf/libs/backend/go/app"
	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	"github.com/mateusmacedo/dmpf/libs/backend/go/contracts/envelope"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	eventv1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/event/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

// ConsumerName is the logical, stable identity of INB-01, shared by every
// replica: the inbox of the memberships established from invitations.
const ConsumerName = "iam-membership"

var errProvisionedEmail = errors.New("app: the provisioned email is outside the form of an address")

// consumerLocale answers a mandatory field of CTX-01 that a consumption has no
// caller to state.
const consumerLocale = "en"

// Handler unpacks InvitationProvisioned and hands it to the use case. A payload
// that is not this consumer's contract is terminal (Validation, ERR-11): no
// unit of work is opened for it.
func Handler(service application.Service) kernelapp.Handler {
	return func(ctx context.Context, receipt ports.Receipt, env envelope.Envelope) (usecase.Disposition, error) {
		var provisioned eventv1.InvitationProvisioned
		if err := envelope.Unpack(env, &provisioned); err != nil {
			return usecase.R1D4, usecase.NewFailure(usecase.Validation, false, err)
		}
		// EstablishMembership does not judge the email, so an address outside the
		// form, U+0000 included, would only fail in the snapshot (22P05).
		if email := domain.Email(provisioned.GetEmail()); email != "" && !domain.ValidEmail(email) {
			return usecase.R1D4, usecase.NewFailure(usecase.Validation, false, errProvisionedEmail)
		}
		roles := make([]domain.RoleRef, 0, len(provisioned.GetRoles()))
		for _, role := range provisioned.GetRoles() {
			roles = append(roles, domain.RoleRef(role))
		}
		return service.ConsumeInvitationProvisioned(ctx, application.ConsumeInvitationProvisioned{
			MessageID:   receipt.MessageID,
			MessageType: receipt.MessageType,
			PayloadHash: receipt.PayloadHash,
			ReceivedAt:  receipt.ReceivedAt,
			Subject:     domain.Subject(provisioned.GetSubject()),
			Email:       domain.Email(provisioned.GetEmail()),
			Roles:       roles,
		})
	}
}

type ConsumerTelemetry struct {
	Tracer         trace.Tracer
	MeterProvider  metric.MeterProvider
	LoggerProvider log.LoggerProvider
	System         string
	Channel        kernelapp.Channel
}

// NewConsumer is the whole consumer: adapter, service and Postgres quarantine.
// maxAttempts <= 0 disables the attempt limit (GAR-08 fixes that one exists).
func NewConsumer(service application.Service, pool *pgxpool.Pool, clock ports.Clock, timeout time.Duration, maxAttempts int, boundary kernelapp.Boundary, telemetry ConsumerTelemetry) kernelapp.Consumer {
	return kernelapp.Consumer{
		Name:           ConsumerName,
		MaxAttempts:    maxAttempts,
		Handle:         Handler(service),
		Containment:    postgres.NewQuarantine(pool, postgres.WithQuarantineLoggerProvider(telemetry.LoggerProvider)),
		Clock:          clock,
		Timeout:        timeout,
		Boundary:       boundary,
		Locale:         consumerLocale,
		Tracer:         telemetry.Tracer,
		MeterProvider:  telemetry.MeterProvider,
		LoggerProvider: telemetry.LoggerProvider,
		System:         telemetry.System,
		Channel:        telemetry.Channel,
	}
}

// MembershipBoundary is the one place the consumer's trust is declared: the
// relay of this very context, by the source it stamps, and a transport called
// verified only when TLS checks the broker and this client authenticates to it.
func MembershipBoundary(cfg Config, authenticated bool) kernelapp.Boundary {
	transport := kernelapp.TransportDevelopmentOnly
	if authenticated {
		transport = kernelapp.TransportVerified
	}
	return kernelapp.Boundary{Transport: transport, Sources: []string{cfg.Relay.Source}}
}
