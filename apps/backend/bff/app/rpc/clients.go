package rpc

import (
	"context"
	"crypto/tls"
	"time"

	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/metrics"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/resilience"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/deadline"
	"go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"

	tenancyv1 "github.com/mateusmacedo/marketplace/apps/backend/tenancy/contract/gen/go/company/tenancy/service/v1"
)

const (
	methodLimit     = 1500 * time.Millisecond
	methodSlack     = 100 * time.Millisecond
	methodEstimated = 200 * time.Millisecond
)

var (
	tenancyService = tenancyv1.File_company_tenancy_service_v1_tenancy_service_proto.Services().ByName("TenancyService")

	TenancyServiceName = string(tenancyService.FullName())

	MethodRegisterStore      = fullMethod(tenancyService, "RegisterStore")
	MethodSuspendStore       = fullMethod(tenancyService, "SuspendStore")
	MethodReactivateStore    = fullMethod(tenancyService, "ReactivateStore")
	MethodFindStore          = fullMethod(tenancyService, "FindStore")
	MethodFindStoresByStatus = fullMethod(tenancyService, "FindStoresByStatus")
)

// fullMethod reads the wire name off the generated descriptor, so a method the
// contract no longer declares fails at start instead of at the first call.
func fullMethod(service protoreflect.ServiceDescriptor, name protoreflect.Name) string {
	method := service.Methods().ByName(name)
	if method == nil {
		panic("rpc: " + string(service.FullName()) + " declares no method " + string(name))
	}
	return "/" + string(service.FullName()) + "/" + string(method.Name())
}

// Options is what the composition root decides for every client: transport
// security (TLS or the development opt-out) and where the calls record.
type Options struct {
	TLS            *tls.Config
	Insecure       bool
	Clock          clock.Clock
	Tracer         trace.Tracer
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	Propagator     propagation.TextMapPropagator
	Instruments    *metrics.Instruments
	LoggerProvider log.LoggerProvider
}

func TenancyConfig(opts Options) kernelgrpc.Config {
	return config("tenancy", TenancyServiceName, opts, map[string]kernelgrpc.MethodPolicy{
		MethodRegisterStore:      policy("tenancy", MethodRegisterStore),
		MethodSuspendStore:       policy("tenancy", MethodSuspendStore),
		MethodReactivateStore:    policy("tenancy", MethodReactivateStore),
		MethodFindStore:          policy("tenancy", MethodFindStore),
		MethodFindStoresByStatus: policy("tenancy", MethodFindStoresByStatus),
	})
}

// policy retries every method on Unavailable: a read has no effect, and a
// command carries the key the context deduplicates by (GRP-09, IDM-01).
func policy(dependency, method string) kernelgrpc.MethodPolicy {
	return kernelgrpc.MethodPolicy{
		Budget: deadline.Budget{
			Dependency:        dependency,
			Method:            method,
			Limit:             methodLimit,
			Slack:             methodSlack,
			EstimatedDuration: methodEstimated,
		},
		Idempotent:     true,
		RetryableCodes: []codes.Code{codes.Unavailable},
	}
}

func config(dependency, healthService string, opts Options, methods map[string]kernelgrpc.MethodPolicy) kernelgrpc.Config {
	return kernelgrpc.Config{
		TLS:                        opts.TLS,
		InsecureForDevelopmentOnly: opts.Insecure,
		Sheet:                      resilience.Defaults(dependency),
		Methods:                    methods,
		HealthServiceName:          healthService,
		Clock:                      opts.Clock,
		Tracer:                     opts.Tracer,
		TracerProvider:             opts.TracerProvider,
		MeterProvider:              opts.MeterProvider,
		Propagator:                 opts.Propagator,
		Instruments:                opts.Instruments,
		LoggerProvider:             opts.LoggerProvider,
	}
}

// Dial opens a client with the edge's context interceptor innermost, so the
// traceparent written to the metadata is the one of the client span.
func Dial(target string, cfg kernelgrpc.Config, extra ...grpc.DialOption) (*grpc.ClientConn, error) {
	options := append([]grpc.DialOption{grpc.WithChainUnaryInterceptor(contextInterceptor)}, extra...)
	return kernelgrpc.Dial(target, cfg, options...)
}

// Unary is one method of a context as the handlers see it.
type Unary[Req, Resp any] func(context.Context, *Req) (*Resp, error)

type Tenancy struct {
	RegisterStore      Unary[tenancyv1.RegisterStoreRequest, tenancyv1.RegisterStoreResponse]
	SuspendStore       Unary[tenancyv1.SuspendStoreRequest, tenancyv1.SuspendStoreResponse]
	ReactivateStore    Unary[tenancyv1.ReactivateStoreRequest, tenancyv1.ReactivateStoreResponse]
	FindStore          Unary[tenancyv1.FindStoreRequest, tenancyv1.FindStoreResponse]
	FindStoresByStatus Unary[tenancyv1.FindStoresByStatusRequest, tenancyv1.FindStoresByStatusResponse]
}

func NewTenancy(conn grpc.ClientConnInterface) Tenancy {
	return Tenancy{
		RegisterStore:      unaryOf[tenancyv1.RegisterStoreRequest, tenancyv1.RegisterStoreResponse](conn, MethodRegisterStore),
		SuspendStore:       unaryOf[tenancyv1.SuspendStoreRequest, tenancyv1.SuspendStoreResponse](conn, MethodSuspendStore),
		ReactivateStore:    unaryOf[tenancyv1.ReactivateStoreRequest, tenancyv1.ReactivateStoreResponse](conn, MethodReactivateStore),
		FindStore:          unaryOf[tenancyv1.FindStoreRequest, tenancyv1.FindStoreResponse](conn, MethodFindStore),
		FindStoresByStatus: unaryOf[tenancyv1.FindStoresByStatusRequest, tenancyv1.FindStoresByStatusResponse](conn, MethodFindStoresByStatus),
	}
}

// unaryOf binds one method of conn and marks the request replayed when the
// context answers a command from its record (IDM-08).
func unaryOf[Req, Resp any](conn grpc.ClientConnInterface, method string) Unary[Req, Resp] {
	return func(ctx context.Context, req *Req) (*Resp, error) {
		resp := new(Resp)
		var header metadata.MD
		if err := conn.Invoke(ctx, method, req, resp, grpc.Header(&header)); err != nil {
			return nil, err
		}
		markReplayed(ctx, header)
		return resp, nil
	}
}
