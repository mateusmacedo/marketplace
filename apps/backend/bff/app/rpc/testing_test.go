package rpc_test

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app/rpc"
	tenancyv1 "github.com/mateusmacedo/marketplace/apps/backend/tenancy/contract/gen/go/company/tenancy/service/v1"
)

type received struct {
	method      string
	md          metadata.MD
	deadline    time.Time
	hasDeadline bool
}

type fakeContexts struct {
	mu    sync.Mutex
	calls []received

	findStore      func(n int) (*tenancyv1.FindStoreResponse, error)
	registerStore  func(n int) (*tenancyv1.RegisterStoreResponse, error)
	replayRegister bool
}

func (f *fakeContexts) record(ctx context.Context, method string) int {
	md, _ := metadata.FromIncomingContext(ctx)
	d, ok := ctx.Deadline()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, received{method: method, md: md.Copy(), deadline: d, hasDeadline: ok})
	n := 0
	for _, c := range f.calls {
		if c.method == method {
			n++
		}
	}
	return n
}

func (f *fakeContexts) callsTo(method string) []received {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []received
	for _, c := range f.calls {
		if c.method == method {
			out = append(out, c)
		}
	}
	return out
}

var tenancyService = tenancyv1.File_company_tenancy_service_v1_tenancy_service_proto.Services().ByName("TenancyService")

func unary[Req any, PReq interface {
	*Req
	proto.Message
}](f *fakeContexts, service protoreflect.ServiceDescriptor, name protoreflect.Name, respond func(ctx context.Context, n int) (any, error)) grpc.MethodDesc {
	return kernelgrpc.Unary[any, Req, PReq, proto.Message](string(service.FullName()), kernelgrpc.Method(service, name),
		func(_ any, ctx context.Context, _ PReq) (proto.Message, error) {
			resp, err := respond(ctx, f.record(ctx, string(name)))
			msg, _ := resp.(proto.Message)
			return msg, err
		})
}

func (f *fakeContexts) register(srv *grpc.Server) {
	srv.RegisterService(&grpc.ServiceDesc{
		ServiceName: rpc.TenancyServiceName,
		HandlerType: (*any)(nil),
		Methods: []grpc.MethodDesc{
			unary[tenancyv1.RegisterStoreRequest](f, tenancyService, "RegisterStore", func(ctx context.Context, n int) (any, error) {
				if f.replayRegister {
					_ = grpc.SetHeader(ctx, metadata.Pairs(kernelgrpc.ReplayedHeader, "true"))
				}
				if f.registerStore != nil {
					return f.registerStore(n)
				}
				return &tenancyv1.RegisterStoreResponse{}, nil
			}),
			unary[tenancyv1.SuspendStoreRequest](f, tenancyService, "SuspendStore", func(context.Context, int) (any, error) {
				return &tenancyv1.SuspendStoreResponse{}, nil
			}),
			unary[tenancyv1.ReactivateStoreRequest](f, tenancyService, "ReactivateStore", func(context.Context, int) (any, error) {
				return &tenancyv1.ReactivateStoreResponse{}, nil
			}),
			unary[tenancyv1.FindStoreRequest](f, tenancyService, "FindStore", func(_ context.Context, n int) (any, error) {
				if f.findStore != nil {
					return f.findStore(n)
				}
				return &tenancyv1.FindStoreResponse{Store: &tenancyv1.Store{Ref: "loja-1", Status: tenancyv1.StoreStatus_STORE_STATUS_ACTIVE}}, nil
			}),
			unary[tenancyv1.FindStoresByStatusRequest](f, tenancyService, "FindStoresByStatus", func(context.Context, int) (any, error) {
				return &tenancyv1.FindStoresByStatusResponse{}, nil
			}),
		},
	}, f)
	h := health.NewServer()
	h.SetServingStatus(rpc.TenancyServiceName, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(srv, h)
}

func (f *fakeContexts) serveBuffered(t *testing.T) func(context.Context, string) (net.Conn, error) {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	f.register(srv)
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(func() {
		srv.Stop()
		_ = listener.Close()
	})
	return func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }
}

func (f *fakeContexts) serveTCP(t *testing.T, opts ...grpc.ServerOption) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() = %v", err)
	}
	srv := grpc.NewServer(opts...)
	f.register(srv)
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(srv.Stop)
	return listener.Addr().String()
}
