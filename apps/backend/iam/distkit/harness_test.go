//go:build integration && distributed

package distkit_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/kafka"
	obsclock "github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/resilience"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/ids"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/dist"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/execctx"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/pg"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/channel"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/app"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/appkit"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	eventv1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/event/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/distkit"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

// TestDistkitRole is the entry point of a re-executed child; it is inert in a
// normal run.
func TestDistkitRole(t *testing.T) { distkit.RunRole(t) }

const (
	settle      = 90 * time.Second
	definitions = 6
)

func TestTwoDrainsPublishEveryRecordExactlyOnce(t *testing.T) {
	h := dist.New(t, distkit.Spec)
	service := appkit.NewIam(t, clock.New(ports.Instant(1)), &ids.Sequence{Prefix: "m-"}).Service
	for i := range definitions {
		ref := domain.RoleRef(fmt.Sprintf("papel-distkit-%d", i))
		if _, err := service.DefineRole(withMessage(t), application.DefineRole{Ref: ref, Name: domain.RoleName("Papel " + ref), Permissions: []domain.Permission{"catalog:read"}}); err != nil {
			t.Fatalf("DefineRole(%s) = %v", ref, err)
		}
	}

	tb.Require(t, dist.RelayVector(t, h, definitions))
}

// The provisioned invitation travels the production path: the invitation is
// confirmed in the domain and saved with its event, the relay publishes it on
// iam.events, and the consumer establishes the membership from it.
func TestAProvisionedInvitationBecomesAMembershipThroughTheRelayAndTheConsumer(t *testing.T) {
	h := dist.New(t, distkit.Spec)
	invite(t, newService(t), "ana@loja.com", "expedicao", "catalogo")
	confirm(t, h.Pool, "ana@loja.com", "kc-123")
	relay := h.Start(t, dist.RoleRelay)
	consumer := distkit.StartConsumer(t, h)

	dist.WaitFor(t, "the consumer to establish kc-123", settle, func() bool { return membershipVersion(t, h.Pool, "kc-123") > 0 })
	dist.WaitFor(t, "the relay to publish the membership established", settle, func() bool { return published(t, h.Pool) == 3 })
	consumer.Stop(t, settle)
	relay.Stop(t, settle)

	member := membership(t, h.Pool, "kc-123")
	if member.Status != domain.MembershipStatusActive || member.Email != "ana@loja.com" || !slices.Equal(member.Roles, []domain.RoleRef{"catalogo", "expedicao"}) {
		t.Fatalf("membership = %+v, want active with the email and the roles of the invitation\nconsumer:\n%s", member, consumer.Output())
	}
	if n := consumed(t, h.Pool); n != 1 {
		t.Fatalf("inbox of %s = %d rows, want 1: every other event of iam.events is acknowledged untouched\nconsumer:\n%s", app.ConsumerName, n, consumer.Output())
	}
}

func TestARemovedMemberComesBackActiveThroughANewInvitation(t *testing.T) {
	h := dist.New(t, distkit.Spec)
	service := newService(t)
	pg.Seed(t, withMessage(t), h.Pool, provider.NewMembershipRepository, "kc-123", domain.MembershipSnapshot{
		Subject: "kc-123", Email: "ana@loja.com", Roles: []domain.RoleRef{"expedicao"}, Status: domain.MembershipStatusRemoved,
		AccessVersion: 2, JoinedAt: 1, ChangedAt: 2,
	})
	invite(t, service, "ana@loja.com", "vendas")
	confirm(t, h.Pool, "ana@loja.com", "kc-123")
	relay := h.Start(t, dist.RoleRelay)
	consumer := distkit.StartConsumer(t, h)

	dist.WaitFor(t, "the consumer to bring kc-123 back", settle, func() bool { return membershipVersion(t, h.Pool, "kc-123") > 1 })
	consumer.Stop(t, settle)
	relay.Stop(t, settle)

	member := membership(t, h.Pool, "kc-123")
	if member.Status != domain.MembershipStatusActive || !slices.Equal(member.Roles, []domain.RoleRef{"vendas"}) || member.AccessVersion != 3 {
		t.Fatalf("membership = %+v, want active again with [vendas] at access version 3\nconsumer:\n%s", member, consumer.Output())
	}
}

// The same envelope delivered twice is disposed of once: the second delivery
// meets the inbox row of the first and writes nothing (R2). A third message
// published after both marks when the partition was read past them.
func TestTheSameProvisionedInvitationDeliveredTwiceIsProcessedOnce(t *testing.T) {
	h := dist.New(t, distkit.Spec)
	consumer := distkit.StartConsumer(t, h)
	twice := appkit.RawInvitationProvisioned(t, "evt-twice", appkit.RelaySource, &eventv1.InvitationProvisioned{Email: "ana@loja.com", Subject: "kc-123", Roles: []string{"expedicao"}})
	marker := appkit.RawInvitationProvisioned(t, "evt-marker", appkit.RelaySource, &eventv1.InvitationProvisioned{Email: "bia@loja.com", Subject: "kc-456", Roles: []string{"catalogo"}})
	publish(t, h, twice, twice, marker)

	dist.WaitFor(t, "the consumer to read past the duplicate", settle, func() bool { return membershipVersion(t, h.Pool, "kc-456") > 0 })
	consumer.Stop(t, settle)

	if v := membershipVersion(t, h.Pool, "kc-123"); v != 1 {
		t.Fatalf("membership kc-123 at version %d, want 1: the redelivery must not apply the effect again\nconsumer:\n%s", v, consumer.Output())
	}
	if n := consumed(t, h.Pool); n != 2 {
		t.Fatalf("inbox of %s = %d rows, want 2: one per message identity", app.ConsumerName, n)
	}
	if n := countWhere(t, h.Pool, "SELECT count(*) FROM outbox WHERE message_type = 'com.company.iam.membership-established.v1'"); n != 2 {
		t.Fatalf("memberships established = %d, want 2: one per message identity", n)
	}
}

// withMessage is the execution of the harness's store under a message context:
// the claim of OBX-08 only sees records whose metadata names one.
func withMessage(t *testing.T) context.Context {
	t.Helper()
	ctx := ports.WithMessageContext(context.Background(), ports.MessageContext{
		CorrelationID: "corr-distkit",
		CausationID:   "caus-distkit",
		Traceparent:   "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
	})
	return execctx.WithExecution(t, ctx, execctx.WithTenant(appkit.Tenant), execctx.AutoKey())
}

// newService composes the use cases over the database of the harness; the kit
// resets the tables when it opens it, so a test composes it before seeding.
func newService(t *testing.T) application.Service {
	t.Helper()
	return appkit.NewIam(t, clock.New(ports.Instant(1)), &ids.Sequence{Prefix: "m-invite-"}).Service
}

func invite(t *testing.T, service application.Service, email string, roles ...domain.RoleRef) {
	t.Helper()
	outcome, err := service.InviteMember(withMessage(t), application.InviteMember{Email: email, Roles: roles})
	if rejection, refused := outcome.Rejection(); err != nil || refused {
		t.Fatalf("InviteMember(%s) = %v, %v", email, rejection, err)
	}
}

// confirm applies the external step of the invitation the way the cycle of
// ADR-070 will: Confirm in the domain, the state and its event in one unit of
// work. No use case wraps it yet, so the harness composes the same steps.
func confirm(t *testing.T, pool *pgxpool.Pool, email domain.Email, subject domain.Subject) {
	t.Helper()
	uow := postgres.NewUnitOfWork(pool, func(tx *postgres.Tx) application.Resources {
		return application.Resources{Invitations: provider.NewInvitationRepository(tx), Outbox: tx.Outbox(provider.Mapper{})}
	})
	identity := usecase.ResolveIdentity(clock.New(ports.Instant(2)), &ids.Sequence{Prefix: "m-confirm-"}, 1)
	origin := usecase.Origin{Destination: application.Destination, AggregateType: application.AggregateTypeInvitation, AggregateID: string(email)}
	err := uow.Within(withMessage(t), func(ctx context.Context, res application.Resources) error {
		outcome, err := usecase.Decide(ctx, res.Invitations, res.Outbox, origin, email, identity,
			usecase.Existing[domain.Email](domain.FromInvitationSnapshot),
			func(i *domain.Invitation) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
				return i.Confirm(domain.ConfirmInvitation{Subject: subject, At: identity.OccurredAt})
			})
		if rejection, refused := outcome.Rejection(); refused {
			return rejection
		}
		return err
	})
	if err != nil {
		t.Fatalf("confirm(%s) = %v", email, err)
	}
}

// publish writes envelopes to the topic of the harness as the relay would,
// which is how a redelivery of the same identity is injected.
func publish(t *testing.T, h *dist.Harness, raws ...[]byte) {
	t.Helper()
	cfg := app.Defaults(app.RoleConsumer)
	cfg.IamTopic, cfg.IamDLQ, cfg.Group = h.Topic, h.DLQ, h.Group
	ch := app.MembershipChannel(cfg)
	pub, err := kafka.NewPublisher(kafka.Config{
		Brokers:                    h.Brokers,
		InsecureForDevelopmentOnly: true,
		Catalog:                    channel.Catalog{ch.Name: ch},
		Sheet:                      resilience.Defaults("kafka"),
		Clock:                      obsclock.System(),
		RevokeGrace:                10 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("NewPublisher() = %v", err)
	}
	defer pub.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for _, raw := range raws {
		if err := pub.Publish(ctx, ch.Name, raw); err != nil {
			t.Fatalf("Publish() = %v", err)
		}
	}
}

func countWhere(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var n int
	if err := pool.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		t.Fatalf("%s = %v", strings.Fields(query)[0], err)
	}
	return n
}

func consumed(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	return countWhere(t, pool, "SELECT count(*) FROM inbox WHERE consumer_name = $1", app.ConsumerName)
}

func published(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	return countWhere(t, pool, "SELECT count(*) FROM outbox WHERE status <> 'pending'")
}

func membershipVersion(t *testing.T, pool *pgxpool.Pool, subject string) int {
	t.Helper()
	return countWhere(t, pool, "SELECT COALESCE(max(version), 0) FROM memberships WHERE membership_id = $1", subject)
}

func membership(t *testing.T, pool *pgxpool.Pool, subject domain.Subject) domain.MembershipSnapshot {
	t.Helper()
	ctx := execctx.WithExecution(t, context.Background(), execctx.WithTenant(appkit.Tenant))
	snap, _, err := provider.NewMembershipReader(postgres.NewReadPool(pool)).Load(ctx, subject)
	if err != nil {
		t.Fatalf("Load(%s) = %v", subject, err)
	}
	return snap
}
