//go:build integration

package appkit

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	kernelapp "github.com/mateusmacedo/dmpf/libs/backend/go/app"
	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	"github.com/mateusmacedo/dmpf/libs/backend/go/contracts/envelope"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/pg"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/app"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	eventv1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/event/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

// Tables of this context, which the kit cannot know: it resets the kernel
// tables and the ones named here.
var Tables = []string{"roles", "invitations", "memberships", "buyers"}

// PoolOptions is the database this context's suites run in.
var PoolOptions = pg.Options{
	Project:      "iam",
	Capabilities: []postgres.Capability{postgres.Outbox, postgres.Inbox},
	Schemas:      []string{provider.Schema},
	Tables:       Tables,
	Grants:       provider.Grants,
}

func OpenPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	return pg.OpenPool(t, PoolOptions)
}

const (
	// Wait is the INB-17 ceiling the harness declares for the inbox.
	Wait = 2 * time.Second
	// MaxAttempts is the GAR-08 limit the harness declares.
	MaxAttempts = 2
	// Timeout is the consumer's own time policy, which CTX-28 makes mandatory.
	Timeout = 10 * time.Second
)

// Harness is the application service and the consumer adapter composed over
// the Postgres the suite runs against, with the clock and identifiers the test
// injects (KIT-07), observed at the effect edge.
type Harness struct {
	Service  application.Service
	Consumer kernelapp.Consumer
	Pool     *pgxpool.Pool
}

// NewIam composes the service the way the composition root does, minus the
// instrumentation and the tenant check: a harness observes effects, not
// telemetry nor authorization.
func NewIam(t testing.TB, clock ports.Clock, ids ports.IDGenerator) Harness {
	t.Helper()
	pool := OpenPool(t)
	policy, err := kernelapp.IdempotencyPolicy(time.Second, 24*time.Hour)
	if err != nil {
		t.Fatalf("appkit.NewIam: %v", err)
	}
	service := app.NewService(pool, clock, ids, app.Waits{Message: Wait, Command: time.Second}, "")
	service.Authorize = usecase.AllowAll[application.Operation]()
	service.Idempotency = policy
	return Harness{
		Service:  service,
		Consumer: app.NewConsumer(service, pool, clock, Timeout, MaxAttempts, Boundary, app.ConsumerTelemetry{}),
		Pool:     pool,
	}
}

// RelaySource is the source the relay of this context stamps, and Boundary the
// consumer's trust in it, so a harness message crosses the boundary like a
// published one does (CTX-27).
var RelaySource = app.Defaults(app.RoleRelay).Relay.Source

var Boundary = kernelapp.Boundary{Transport: kernelapp.TransportDevelopmentOnly, Sources: []string{RelaySource}}

// Ack records the broker gesture the adapter applied and reads the inbox at
// the moment of the ack: a row already visible to another connection is the
// proof that the commit came first (INB-08).
type Ack struct {
	pool      *pgxpool.Pool
	messageID string

	Acks       int
	Releases   int
	InboxAtAck int64
}

func (a *Ack) Ack(ctx context.Context) error {
	a.Acks++
	return a.pool.QueryRow(ctx,
		"SELECT count(*) FROM inbox WHERE consumer_name = $1 AND message_id = $2",
		app.ConsumerName, a.messageID).Scan(&a.InboxAtAck)
}

func (a *Ack) Release(context.Context) error {
	a.Releases++
	return nil
}

// Deliver hands the raw bytes of one delivery to the adapter and returns what
// it did, with the acknowledger that saw the gesture.
func (h Harness) Deliver(ctx context.Context, messageID string, raw []byte, attempt int) (kernelapp.Outcome, *Ack, error) {
	ack := &Ack{pool: h.Pool, messageID: messageID}
	outcome, err := h.Consumer.Consume(ctx, kernelapp.Delivery{Raw: raw, Attempt: attempt}, ack)
	return outcome, ack, err
}

// Effects is the effect edge of the consumption: how many rows each table
// it touches holds.
type Effects struct {
	Inbox, Memberships, Outbox, Quarantine int64
}

func (h Harness) Effects(t testing.TB) Effects {
	t.Helper()
	var e Effects
	const stmt = `SELECT
		(SELECT count(*) FROM inbox WHERE consumer_name = $1),
		(SELECT count(*) FROM memberships),
		(SELECT count(*) FROM outbox),
		(SELECT count(*) FROM quarantine)`
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	if err := h.Pool.QueryRow(ctx, stmt, app.ConsumerName).Scan(&e.Inbox, &e.Memberships, &e.Outbox, &e.Quarantine); err != nil {
		t.Fatalf("appkit.Effects: %v", err)
	}
	return e
}

// QuarantineReasons lists why each contained delivery left the flow.
func (h Harness) QuarantineReasons(t testing.TB) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	rows, err := h.Pool.Query(ctx, "SELECT reason FROM quarantine ORDER BY reason")
	if err != nil {
		t.Fatalf("appkit.QuarantineReasons: %v", err)
	}
	reasons, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("appkit.QuarantineReasons: %v", err)
	}
	return reasons
}

// queryTimeout bounds each read of the effect edge, so a Postgres that stops
// answering fails the test by name instead of letting it hang.
const queryTimeout = 10 * time.Second

// Tenant is the store the provisioned invitations of the harness belong to:
// the relay puts the tenant on the envelope, and without it persistence refuses.
const Tenant = "loja-acme"

// RawInvitationProvisioned is the CloudEvent of an InvitationProvisioned as the
// relay of this context publishes it: the bytes the harness delivers.
func RawInvitationProvisioned(t testing.TB, messageID, source string, provisioned *eventv1.InvitationProvisioned) []byte {
	t.Helper()
	return RawEnvelope(t, messageID, source, "com.company.iam.invitation-provisioned.v1", provisioned)
}

// RawEnvelope packs any message under any type, which is how a payload that
// is not the contract of its type is delivered.
func RawEnvelope(t testing.TB, messageID, source, eventType string, msg proto.Message) []byte {
	t.Helper()
	payload, typeURL, err := envelope.Pack(msg)
	if err != nil {
		t.Fatalf("appkit.RawEnvelope: Pack: %v", err)
	}
	tenant := Tenant
	ce, err := envelope.Encode(envelope.Envelope{
		ID:              messageID,
		Source:          source,
		SpecVersion:     envelope.SpecVersion,
		Type:            eventType,
		Subject:         messageID,
		Time:            timestamppb.New(time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)),
		DataSchema:      typeURL,
		DataContentType: envelope.ContentType,
		CorrelationID:   "corr-" + messageID,
		CausationID:     messageID,
		PartitionKey:    messageID,
		TraceParent:     "00-0af7651916cd43dd8448eb211c80319c-b7ad6b7169203331-01",
		TenantID:        &tenant,
		Payload:         payload,
	})
	if err != nil {
		t.Fatalf("appkit.RawEnvelope: Encode: %v", err)
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(ce)
	if err != nil {
		t.Fatalf("appkit.RawEnvelope: Marshal: %v", err)
	}
	return raw
}

// Outbox reads what the use case left for the relay, ordered as it was
// enqueued, so a test asserts the sequence and not just the presence.
func (h Harness) Outbox(t testing.TB) []pg.Enqueued {
	t.Helper()
	return pg.Outbox(t, h.Pool)
}
