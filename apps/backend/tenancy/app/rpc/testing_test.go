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

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/app/rpc"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

var storesTable = memory.Table[domain.StoreRef, domain.StoreSnapshot]{Name: "stores", Unique: []memory.Unique[domain.StoreSnapshot]{{
	Name:      "stores_tenant_id_name_key",
	Key:       func(s domain.StoreSnapshot) ([]string, bool) { return []string{string(s.Name)}, true },
	Rejection: domain.RejectStoreDuplicateName,
}}}

const (
	occurred   = ports.Instant(1_755_432_000_000_000_000)
	testTenant = "marketplace"
)

var unlimited = admission.Limit{PerSecond: 1000, Burst: 1000, Concurrency: 64}

// byStatus answers FindStoresByStatus with what the test planted: the memory
// store has no relation query, and the mapping is what is under test.
type byStatus map[domain.StoreStatus][]domain.StoreSnapshot

func (r byStatus) LoadByStatus(_ context.Context, status domain.StoreStatus) ([]domain.StoreSnapshot, error) {
	return r[status], nil
}

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
				Stores:   storesTable.Repository(tx),
				Outbox:   tx.Outbox(),
				Commands: tx.CommandInbox(application.CommandConsumer),
			}
		}),
		Reader:       storesTable.Reader(store),
		StatusReader: planted,
		Clock:        memory.FixedClock{At: occurred},
		IDs:          &memory.SequenceIDs{Prefix: "m-"},
		Authorize:    authorize,
		Idempotency:  policy,
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
	return tenantOnly(t, testTenant)
}

// inTenant is a keyed call that the edge resolved to another tenant.
func inTenant(t *testing.T, tenant string) context.Context {
	t.Helper()
	return metadata.AppendToOutgoingContext(tenantOnly(t, tenant), kernelgrpc.IdempotencyKey, fmt.Sprintf("k-%d", keys.Add(1)))
}

func tenantOnly(t *testing.T, tenant string) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return metadata.AppendToOutgoingContext(ctx, kernelgrpc.TenantKey, tenant)
}
