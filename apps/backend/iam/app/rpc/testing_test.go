package rpc_test

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"testing"
	"time"

	kernelapp "github.com/mateusmacedo/dmpf/libs/backend/go/app"
	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	"github.com/mateusmacedo/dmpf/libs/backend/go/memory"
	obsclock "github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/admission"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/app/rpc"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

var rolesTable = memory.Table[domain.RoleRef, domain.RoleSnapshot]{Name: "roles", Unique: []memory.Unique[domain.RoleSnapshot]{{
	Name:      "roles_tenant_id_name_key",
	Key:       func(s domain.RoleSnapshot) ([]string, bool) { return []string{string(s.Name)}, true },
	Rejection: domain.RejectRoleDuplicateName,
}}}

var invitationsTable = memory.Table[domain.Email, domain.InvitationSnapshot]{Name: "invitations"}

var membershipsTable = memory.Table[domain.Subject, domain.MembershipSnapshot]{Name: "memberships", Unique: []memory.Unique[domain.MembershipSnapshot]{{
	Name:      "memberships_tenant_id_owner_key",
	Key:       func(s domain.MembershipSnapshot) ([]string, bool) { return []string{"owner"}, s.Owns() },
	Rejection: domain.RejectMembershipOwnerClaimed,
}}}

var buyersTable = memory.Table[domain.Subject, domain.BuyerSnapshot]{Name: "buyers"}

const (
	occurred   = ports.Instant(1_755_432_000_000_000_000)
	testTenant = "loja-acme"
)

var unlimited = admission.Limit{PerSecond: 1000, Burst: 1000, Concurrency: 64}

// byStatus answers FindRolesByStatus with what the test planted: the memory
// store has no relation query, and the mapping is what is under test.
type byStatus map[domain.RoleStatus][]domain.RoleSnapshot

func (r byStatus) LoadByStatus(_ context.Context, status domain.RoleStatus) ([]domain.RoleSnapshot, error) {
	return r[status], nil
}

// statusOf plants the invitations and memberships of each status the same way.
type statusOf[St comparable, S any] map[St][]S

func (r statusOf[St, S]) LoadByStatus(_ context.Context, status St) ([]S, error) {
	return r[status], nil
}

var (
	plantedInvitations = statusOf[domain.InvitationStatus, domain.InvitationSnapshot]{}
	plantedMemberships = statusOf[domain.MembershipStatus, domain.MembershipSnapshot]{}
	plantedBuyers      = statusOf[domain.BuyerStatus, domain.BuyerSnapshot]{
		domain.BuyerStatusBlocked: {{Subject: "kc-bloqueado", Name: "Caio", Status: domain.BuyerStatusBlocked, BlockReason: "fraude", AccessVersion: 2}},
	}
)

type harness struct {
	store *memory.Store
	conn  *grpc.ClientConn
}

func newHarness(t *testing.T, planted byStatus) *harness {
	t.Helper()
	return newHarnessAuthorizedBy(t, planted, usecase.AllowAll[application.Operation]())
}

func newHarnessAuthorizedBy(t *testing.T, planted byStatus, authorize usecase.Authorize[application.Operation]) *harness {
	t.Helper()

	store := memory.New()
	policy, err := kernelapp.IdempotencyPolicy(time.Second, time.Hour)
	if err != nil {
		t.Fatalf("IdempotencyPolicy() = %v", err)
	}
	service := application.Service{
		UoW: memory.NewUnitOfWork(store, func(tx *memory.Tx) application.Resources {
			return application.Resources{
				Roles:       rolesTable.Repository(tx),
				Invitations: invitationsTable.Repository(tx),
				Memberships: membershipsTable.Repository(tx),
				Buyers:      buyersTable.Repository(tx),
				Outbox:      tx.Outbox(),
				Commands:    tx.CommandInbox(application.CommandConsumer),
			}
		}),
		Roles:               rolesTable.Reader(store),
		RolesByStatus:       planted,
		Invitations:         invitationsTable.Reader(store),
		InvitationsByStatus: plantedInvitations,
		Memberships:         membershipsTable.Reader(store),
		MembershipsByStatus: plantedMemberships,
		Buyers:              buyersTable.Reader(store),
		BuyersByStatus:      plantedBuyers,
		Clock:               memory.FixedClock{At: occurred},
		IDs:                 &memory.SequenceIDs{Prefix: "m-"},
		Authorize:           authorize,
		Idempotency:         policy,
	}

	ctrl, err := admission.New(admission.Config{Limits: kernelgrpc.MethodLimits(rpc.ServiceName, rpc.Methods(), unlimited), MaxKeys: 16, Clock: obsclock.System()})
	if err != nil {
		t.Fatalf("admission.New() = %v", err)
	}
	server, _, err := kernelgrpc.NewServer(kernelgrpc.ServerConfig{
		InsecureForDevelopmentOnly: true,
		Services:                   []string{rpc.ServiceName},
		UnaryInterceptors:          kernelgrpc.ServerInterceptors(rpc.ServiceName, ctrl, nil, nil, kernelgrpc.WithCommands(rpc.Commands()...)),
	})
	if err != nil {
		t.Fatalf("NewServer() = %v", err)
	}
	server.RegisterService(&rpc.ServiceDesc, rpc.Server{Service: service})

	listener := bufconn.Listen(1 << 20)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("NewClient() = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return &harness{store: store, conn: conn}
}

func (h *harness) invoke(ctx context.Context, method string, req, resp proto.Message, opts ...grpc.CallOption) error {
	return h.conn.Invoke(ctx, rpc.FullMethod(method), req, resp, opts...)
}

var keys atomic.Int64

// withTenant is what the BFF puts on the wire: the deadline GRP-04 requires
// plus the tenant the edge resolved (IDN-15), and a key of its own, which
// every command requires (IDM-01).
func withTenant(t *testing.T) context.Context {
	t.Helper()
	return withKey(t, fmt.Sprintf("k-%d", keys.Add(1)))
}

func withKey(t *testing.T, key string) context.Context {
	t.Helper()
	return metadata.AppendToOutgoingContext(withoutKey(t), kernelgrpc.IdempotencyKey, key)
}

func withoutKey(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return metadata.AppendToOutgoingContext(ctx, kernelgrpc.TenantKey, testTenant)
}
