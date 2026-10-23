//go:build integration

package app_test

import (
	"context"
	"fmt"
	"net"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	kernelapp "github.com/mateusmacedo/dmpf/libs/backend/go/app"
	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	obsclock "github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/admission"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/app"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/app/rpc"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/appkit"
	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/tenancy/contract/gen/go/company/tenancy/service/v1"
)

const operatorTenant = "marketplace"

type fixedClock struct{}

func (fixedClock) Now() ports.Instant { return 1_755_432_000_000_000_000 }

type sequenceIDs struct {
	mu     sync.Mutex
	issued int
}

func (g *sequenceIDs) NewMessageID() ports.MessageID {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.issued++
	return ports.MessageID(fmt.Sprintf("m-%06d", g.issued))
}

// dial serves the use cases over Postgres through the server chain the api
// mounts, with the authorization of the composition root, so the suite crosses
// the same hop the BFF does.
func dial(t *testing.T, pool *pgxpool.Pool) *grpc.ClientConn {
	t.Helper()
	return dialWith(t, pool, nil)
}

func dialWith(t *testing.T, pool *pgxpool.Pool, instrumentation ports.Instrumentation) *grpc.ClientConn {
	t.Helper()
	policy, err := kernelapp.IdempotencyPolicy(time.Second, 24*time.Hour)
	if err != nil {
		t.Fatalf("IdempotencyPolicy() = %v", err)
	}
	service := app.NewService(pool, fixedClock{}, &sequenceIDs{}, app.Waits{Command: time.Second}, operatorTenant)
	service.Idempotency = policy
	service.Instrumentation = instrumentation

	limit := admission.Limit{PerSecond: 1000, Burst: 1000, Concurrency: 64}
	ctrl, err := admission.New(admission.Config{Limits: kernelgrpc.MethodLimits(rpc.ServiceName, rpc.Methods(), limit), MaxKeys: 16, Clock: obsclock.System()})
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
	return conn
}

var keys atomic.Int64

func call(t *testing.T, tenant string) context.Context {
	t.Helper()
	return callWithKey(t, tenant, fmt.Sprintf("e2e-k-%d", keys.Add(1)))
}

func callWithKey(t *testing.T, tenant, key string) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	if key != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, kernelgrpc.IdempotencyKey, key)
	}
	if tenant == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, kernelgrpc.TenantKey, tenant)
}

func outboxRows(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM outbox").Scan(&count); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	return count
}

func storeRows(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM stores").Scan(&count); err != nil {
		t.Fatalf("count stores: %v", err)
	}
	return count
}

func TestTheLifecycleOfAStoreEndToEndOverGRPC(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)

	var registered servicev1.RegisterStoreResponse
	if err := conn.Invoke(call(t, operatorTenant), rpc.FullMethod("RegisterStore"), &servicev1.RegisterStoreRequest{Ref: "loja-acme", Name: "Loja Acme"}, &registered); err != nil {
		t.Fatalf("RegisterStore() = %v", err)
	}
	if registered.GetRegistered().GetRef() != "loja-acme" || outboxRows(t, pool) != 1 {
		t.Fatalf("RegisterStore() = %v with %d outbox rows, want Registered loja-acme and one row", &registered, outboxRows(t, pool))
	}

	var suspended servicev1.SuspendStoreResponse
	if err := conn.Invoke(call(t, operatorTenant), rpc.FullMethod("SuspendStore"), &servicev1.SuspendStoreRequest{Ref: "loja-acme", Reason: "fraude em análise"}, &suspended); err != nil {
		t.Fatalf("SuspendStore() = %v", err)
	}
	var found servicev1.FindStoreResponse
	if err := conn.Invoke(call(t, operatorTenant), rpc.FullMethod("FindStore"), &servicev1.FindStoreRequest{Ref: "loja-acme"}, &found); err != nil {
		t.Fatalf("FindStore() = %v", err)
	}
	if store := found.GetStore(); store.GetStatus() != servicev1.StoreStatus_STORE_STATUS_SUSPENDED || store.GetSuspensionReason() != "fraude em análise" {
		t.Fatalf("FindStore() = %v, want SUSPENDED with the reason", store)
	}

	var reactivated servicev1.ReactivateStoreResponse
	if err := conn.Invoke(call(t, operatorTenant), rpc.FullMethod("ReactivateStore"), &servicev1.ReactivateStoreRequest{Ref: "loja-acme"}, &reactivated); err != nil {
		t.Fatalf("ReactivateStore() = %v", err)
	}
	var active servicev1.FindStoresByStatusResponse
	if err := conn.Invoke(call(t, operatorTenant), rpc.FullMethod("FindStoresByStatus"), &servicev1.FindStoresByStatusRequest{Status: servicev1.StoreStatus_STORE_STATUS_ACTIVE}, &active); err != nil {
		t.Fatalf("FindStoresByStatus() = %v", err)
	}
	if stores := active.GetStores(); len(stores) != 1 || stores[0].GetRef() != "loja-acme" || stores[0].GetSuspensionReason() != "" {
		t.Fatalf("FindStoresByStatus(active) = %v, want loja-acme without a reason", stores)
	}
	if n := outboxRows(t, pool); n != 3 {
		t.Fatalf("outbox rows = %d, want 3: registered, suspended and reactivated", n)
	}
}

func TestSuspendingAnUnknownRefIsNotFound(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)

	var resp servicev1.SuspendStoreResponse
	err := conn.Invoke(call(t, operatorTenant), rpc.FullMethod("SuspendStore"), &servicev1.SuspendStoreRequest{Ref: "loja-nenhuma", Reason: "motivo"}, &resp)

	if status.Code(err) != codes.NotFound {
		t.Fatalf("SuspendStore(unknown) = %v, want NotFound", err)
	}
}

func TestATenantThatIsNotTheOperatorIsPermissionDeniedAndNothingIsWritten(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)

	var resp servicev1.RegisterStoreResponse
	err := conn.Invoke(call(t, "loja-acme"), rpc.FullMethod("RegisterStore"), &servicev1.RegisterStoreRequest{Ref: "loja-acme", Name: "Loja Acme"}, &resp)

	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("RegisterStore() from loja-acme = %v, want PermissionDenied", err)
	}
	if outboxRows(t, pool) != 0 || storeRows(t, pool) != 0 {
		t.Fatalf("outbox %d and stores %d rows, want none", outboxRows(t, pool), storeRows(t, pool))
	}
}

// IDN-15 over the hop: a call the edge sent without a tenant resolves no scope,
// and the context refuses it rather than writing under an invented one.
func TestACallWithoutATenantIsRefusedAndNothingIsWritten(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)

	var resp servicev1.RegisterStoreResponse
	err := conn.Invoke(call(t, ""), rpc.FullMethod("RegisterStore"), &servicev1.RegisterStoreRequest{Ref: "loja-acme", Name: "Loja Acme"}, &resp)

	if err == nil {
		t.Fatalf("RegisterStore() without a tenant = %v, want the call refused", &resp)
	}
	if outboxRows(t, pool) != 0 || storeRows(t, pool) != 0 {
		t.Fatalf("outbox %d and stores %d rows, want none", outboxRows(t, pool), storeRows(t, pool))
	}
}

func TestARepeatedRegisterOverGRPCReplays(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)
	req := &servicev1.RegisterStoreRequest{Ref: "loja-replay", Name: "Loja Replay"}

	var first, again servicev1.RegisterStoreResponse
	var header metadata.MD
	if err := conn.Invoke(callWithKey(t, operatorTenant, "e2e-replay"), rpc.FullMethod("RegisterStore"), req, &first); err != nil {
		t.Fatalf("RegisterStore() = %v", err)
	}
	if err := conn.Invoke(callWithKey(t, operatorTenant, "e2e-replay"), rpc.FullMethod("RegisterStore"), req, &again, grpc.Header(&header)); err != nil {
		t.Fatalf("repeated RegisterStore() = %v, want the stored response", err)
	}
	if again.GetRegistered().GetRef() != "loja-replay" || !slices.Equal(header.Get(kernelgrpc.ReplayedHeader), []string{"true"}) {
		t.Fatalf("replay = %v with header %v, want Registered loja-replay marked as replay", &again, header)
	}
	if n := outboxRows(t, pool); n != 1 {
		t.Fatalf("outbox rows = %d, want 1: one effect per key", n)
	}
}

func TestACommandWithoutAKeyIsRefusedAndNothingIsWritten(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)

	var resp servicev1.RegisterStoreResponse
	err := conn.Invoke(callWithKey(t, operatorTenant, ""), rpc.FullMethod("RegisterStore"), &servicev1.RegisterStoreRequest{Ref: "loja-acme", Name: "Loja Acme"}, &resp)

	if status.Code(err) != codes.InvalidArgument || kernelgrpc.ReasonOf(err) != kernelgrpc.ReasonMissingIdempotencyKey {
		t.Fatalf("RegisterStore() without a key = %v, want InvalidArgument with %s", err, kernelgrpc.ReasonMissingIdempotencyKey)
	}
	if n := outboxRows(t, pool); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
}
