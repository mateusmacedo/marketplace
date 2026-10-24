package rpc_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	obsclock "github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app/rpc"
)

func healthOnly(t *testing.T, status healthpb.HealthCheckResponse_ServingStatus) func(context.Context, string) (net.Conn, error) {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	h := health.NewServer()
	h.SetServingStatus(rpc.TenancyServiceName, status)
	healthpb.RegisterHealthServer(srv, h)
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(func() {
		srv.Stop()
		_ = listener.Close()
	})
	return func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }
}

func refused(context.Context, string) (net.Conn, error) { return nil, errors.New("connection refused") }

func tenancyChannel(t *testing.T, dialer func(context.Context, string) (net.Conn, error)) *grpc.ClientConn {
	t.Helper()
	opts := rpc.Options{Insecure: true, Clock: obsclock.System()}
	conn, err := rpc.Dial("passthrough:///tenancy", rpc.TenancyConfig(opts), grpc.WithContextDialer(dialer))
	if err != nil {
		t.Fatalf("Dial(tenancy) = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestReadinessWithoutContextsIsReady(t *testing.T) {
	if err := (rpc.Readiness{}).Check(context.Background()); err != nil {
		t.Fatalf("Check() = %v, want nil: no context means nothing to wait for", err)
	}
}

func TestReadinessPassesOnceTheContextAnswersServing(t *testing.T) {
	conn := tenancyChannel(t, healthOnly(t, healthpb.HealthCheckResponse_SERVING))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := (rpc.Readiness{"tenancy": conn}).Check(ctx); err != nil {
		t.Fatalf("Check() = %v, want nil", err)
	}
}

func TestReadinessNamesEveryContextThatIsNotServing(t *testing.T) {
	readiness := rpc.Readiness{
		"alpha": tenancyChannel(t, healthOnly(t, healthpb.HealthCheckResponse_NOT_SERVING)),
		"beta":  tenancyChannel(t, refused),
		"gamma": tenancyChannel(t, healthOnly(t, healthpb.HealthCheckResponse_SERVING)),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := readiness.Check(ctx)

	if !errors.Is(err, rpc.ErrNotReady) || !strings.HasSuffix(err.Error(), ": alpha, beta") {
		t.Fatalf("Check() = %v, want ErrNotReady naming alpha and beta", err)
	}
}
