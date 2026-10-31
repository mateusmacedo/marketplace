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

	"github.com/mateusmacedo/marketplace/apps/backend/iam/app"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/app/rpc"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/appkit"
	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/service/v1"
)

const (
	operatorTenant = "marketplace"
	storeTenant    = "loja-acme"
)

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

func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func outboxRows(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	return countRows(t, pool, "outbox")
}

func roleRows(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	return countRows(t, pool, "roles")
}

func defineRequest(ref, name string) *servicev1.DefineRoleRequest {
	return &servicev1.DefineRoleRequest{Ref: ref, Name: name, Permissions: []string{"ordering:read", "fulfillment:write", "ordering:read"}}
}

func TestTheLifecycleOfARoleEndToEndOverGRPC(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)

	var defined servicev1.DefineRoleResponse
	if err := conn.Invoke(call(t, storeTenant), rpc.FullMethod("DefineRole"), defineRequest("expedicao", "Expedição"), &defined); err != nil {
		t.Fatalf("DefineRole() = %v", err)
	}
	if defined.GetDefined().GetRef() != "expedicao" || outboxRows(t, pool) != 1 {
		t.Fatalf("DefineRole() = %v with %d outbox rows, want Defined expedicao and one row", &defined, outboxRows(t, pool))
	}

	var changed servicev1.ChangeRolePermissionsResponse
	if err := conn.Invoke(call(t, storeTenant), rpc.FullMethod("ChangeRolePermissions"), &servicev1.ChangeRolePermissionsRequest{Ref: "expedicao", Permissions: []string{"catalog:write"}}, &changed); err != nil {
		t.Fatalf("ChangeRolePermissions() = %v", err)
	}
	var found servicev1.FindRoleResponse
	if err := conn.Invoke(call(t, storeTenant), rpc.FullMethod("FindRole"), &servicev1.FindRoleRequest{Ref: "expedicao"}, &found); err != nil {
		t.Fatalf("FindRole() = %v", err)
	}
	if role := found.GetRole(); !slices.Equal(role.GetPermissions(), []string{"catalog:write"}) || role.GetAccessVersion() != 2 || role.GetStatus() != servicev1.RoleStatus_ROLE_STATUS_ACTIVE {
		t.Fatalf("FindRole() = %v, want ACTIVE with [catalog:write] at access version 2", role)
	}

	var retired servicev1.RetireRoleResponse
	if err := conn.Invoke(call(t, storeTenant), rpc.FullMethod("RetireRole"), &servicev1.RetireRoleRequest{Ref: "expedicao"}, &retired); err != nil {
		t.Fatalf("RetireRole() = %v", err)
	}
	var byStatus servicev1.FindRolesByStatusResponse
	if err := conn.Invoke(call(t, storeTenant), rpc.FullMethod("FindRolesByStatus"), &servicev1.FindRolesByStatusRequest{Status: servicev1.RoleStatus_ROLE_STATUS_RETIRED}, &byStatus); err != nil {
		t.Fatalf("FindRolesByStatus() = %v", err)
	}
	if roles := byStatus.GetRoles(); len(roles) != 1 || roles[0].GetRef() != "expedicao" || roles[0].GetAccessVersion() != 3 {
		t.Fatalf("FindRolesByStatus(retired) = %v, want expedicao at access version 3", roles)
	}
	if n := outboxRows(t, pool); n != 3 {
		t.Fatalf("outbox rows = %d, want 3: defined, permissions changed and retired", n)
	}
}

// A role lives in the tenant of its store: another store reads nothing of it.
func TestTheRolesOfAStoreAreInvisibleToAnotherStore(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)
	if err := conn.Invoke(call(t, storeTenant), rpc.FullMethod("DefineRole"), defineRequest("expedicao", "Expedição"), &servicev1.DefineRoleResponse{}); err != nil {
		t.Fatalf("DefineRole() = %v", err)
	}

	err := conn.Invoke(call(t, "loja-outra"), rpc.FullMethod("FindRole"), &servicev1.FindRoleRequest{Ref: "expedicao"}, &servicev1.FindRoleResponse{})

	if status.Code(err) != codes.NotFound {
		t.Fatalf("FindRole() from loja-outra = %v, want NotFound", err)
	}
}

func TestRetiringAnUnknownRefIsNotFound(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)

	err := conn.Invoke(call(t, storeTenant), rpc.FullMethod("RetireRole"), &servicev1.RetireRoleRequest{Ref: "nenhum"}, &servicev1.RetireRoleResponse{})

	if status.Code(err) != codes.NotFound {
		t.Fatalf("RetireRole(unknown) = %v, want NotFound", err)
	}
}

// The operator holds iam:write as a realm role; its tenant is refused before
// the transaction, so nothing reaches the table nor the outbox.
func TestTheOperatorTenantIsPermissionDeniedAndNothingIsWritten(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)

	err := conn.Invoke(call(t, operatorTenant), rpc.FullMethod("DefineRole"), defineRequest("expedicao", "Expedição"), &servicev1.DefineRoleResponse{})

	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("DefineRole() from the operator = %v, want PermissionDenied", err)
	}
	if outboxRows(t, pool) != 0 || roleRows(t, pool) != 0 {
		t.Fatalf("outbox %d and roles %d rows, want none", outboxRows(t, pool), roleRows(t, pool))
	}
	if err := conn.Invoke(call(t, operatorTenant), rpc.FullMethod("FindRole"), &servicev1.FindRoleRequest{Ref: "expedicao"}, &servicev1.FindRoleResponse{}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("FindRole() from the operator = %v, want PermissionDenied", err)
	}
	byStatus := &servicev1.FindRolesByStatusRequest{Status: servicev1.RoleStatus_ROLE_STATUS_ACTIVE}
	if err := conn.Invoke(call(t, operatorTenant), rpc.FullMethod("FindRolesByStatus"), byStatus, &servicev1.FindRolesByStatusResponse{}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("FindRolesByStatus() from the operator = %v, want PermissionDenied", err)
	}
}

// IDN-15 over the hop: a call the edge sent without a tenant resolves no scope,
// and the context refuses it rather than writing under an invented one.
func TestACallWithoutATenantIsRefusedAndNothingIsWritten(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)

	err := conn.Invoke(call(t, ""), rpc.FullMethod("DefineRole"), defineRequest("expedicao", "Expedição"), &servicev1.DefineRoleResponse{})

	if err == nil {
		t.Fatal("DefineRole() without a tenant = nil, want the call refused")
	}
	if outboxRows(t, pool) != 0 || roleRows(t, pool) != 0 {
		t.Fatalf("outbox %d and roles %d rows, want none", outboxRows(t, pool), roleRows(t, pool))
	}
}

func TestARepeatedDefineOverGRPCReplays(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)
	req := defineRequest("papel-replay", "Papel Replay")

	var first, again servicev1.DefineRoleResponse
	var header metadata.MD
	if err := conn.Invoke(callWithKey(t, storeTenant, "e2e-replay"), rpc.FullMethod("DefineRole"), req, &first); err != nil {
		t.Fatalf("DefineRole() = %v", err)
	}
	if err := conn.Invoke(callWithKey(t, storeTenant, "e2e-replay"), rpc.FullMethod("DefineRole"), req, &again, grpc.Header(&header)); err != nil {
		t.Fatalf("repeated DefineRole() = %v, want the stored response", err)
	}
	if again.GetDefined().GetRef() != "papel-replay" || !slices.Equal(header.Get(kernelgrpc.ReplayedHeader), []string{"true"}) {
		t.Fatalf("replay = %v with header %v, want Defined papel-replay marked as replay", &again, header)
	}
	if n := outboxRows(t, pool); n != 1 {
		t.Fatalf("outbox rows = %d, want 1: one effect per key", n)
	}
}

func TestACommandWithoutAKeyIsRefusedAndNothingIsWritten(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)

	err := conn.Invoke(callWithKey(t, storeTenant, ""), rpc.FullMethod("DefineRole"), defineRequest("expedicao", "Expedição"), &servicev1.DefineRoleResponse{})

	if status.Code(err) != codes.InvalidArgument || kernelgrpc.ReasonOf(err) != kernelgrpc.ReasonMissingIdempotencyKey {
		t.Fatalf("DefineRole() without a key = %v, want InvalidArgument with %s", err, kernelgrpc.ReasonMissingIdempotencyKey)
	}
	if n := outboxRows(t, pool); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
}
