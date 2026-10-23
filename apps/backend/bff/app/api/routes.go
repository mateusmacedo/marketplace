// Package api is the public REST edge: each route is a kernel http.Route that
// references a published OpenAPI operation (RST-04), runs only behind the
// identity the authn verified and the tenant it resolved, and either answers
// from that identity or forwards to one gRPC call of a bounded context.
package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	kernelhttp "github.com/mateusmacedo/dmpf/libs/backend/go/http"
	obsclock "github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/metrics"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/admission"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/deadline"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app/rpc"
)

const (
	IdempotencyHeader = "Idempotency-Key"
	ReplayedHeader    = "Idempotent-Replayed"
	CorrelationHeader = "X-Correlation-ID"

	// ProfilePermission is the standard OIDC scope a client asks for at sign-in:
	// reading one's own identity needs no business role, which the RBAC owns.
	ProfilePermission ports.Permission = "profile"

	TenancyContractPath = "/openapi/tenancy/v1/openapi.yaml"

	contract        = "apps/backend/bff/contract/openapi/v1/openapi.yaml#/paths/"
	tenancyContract = "apps/backend/tenancy/contract/openapi/v1/openapi.yaml#/paths/"
)

// Options is what the composition root decides beyond the clients: the route
// budget the request deadline derives from, the contracts to serve (nil serves
// nothing), the browser origins allowed to call the edge (none by default) and
// the readiness of the contexts (nil serves no readiness route).
type Options struct {
	Budget          deadline.Budget
	Authenticator   ports.Authenticator
	TenancyContract []byte
	CORSOrigins     []string
	Logger          *slog.Logger
	Ready           func(context.Context) error
	Draining        func() bool
	TracerProvider  trace.TracerProvider
	MeterProvider   metric.MeterProvider
	Clock           obsclock.Clock
}

type binding struct {
	route kernelhttp.Route
	build func(handlers) http.HandlerFunc
}

// surface is the REST face of one context: its routes demand the permission
// <context>:read or <context>:write and reference the context's own contract.
type surface struct {
	context  string
	contract string
	budget   deadline.Budget
}

func (s surface) command(name, path string, build func(handlers) http.HandlerFunc) binding {
	route := s.route(name, http.MethodPost, path, "write")
	route.IdempotencyKey = IdempotencyHeader
	return binding{route: route, build: build}
}

func (s surface) query(name, path string, build func(handlers) http.HandlerFunc) binding {
	return binding{route: s.route(name, http.MethodGet, path, "read"), build: build}
}

func (s surface) route(name, method, path, access string) kernelhttp.Route {
	return route(s.contract, name, method, path, s.budget, ports.Permission(s.context+":"+access))
}

func bindings(budget deadline.Budget) []binding {
	tenancy := surface{"tenancy", tenancyContract, budget}
	return []binding{
		{route: route(contract, "me", http.MethodGet, "/v1/me", budget, ProfilePermission), build: func(handlers) http.HandlerFunc { return me }},
		tenancy.command("registerStore", "/tenancy/store", handlers.registerStore),
		tenancy.query("findStoresByStatus", "/tenancy/store", handlers.findStoresByStatus),
		tenancy.query("findStore", "/tenancy/store/{ref}", handlers.findStore),
		tenancy.command("suspendStore", "/tenancy/store/{ref}/suspend", handlers.suspendStore),
		tenancy.command("reactivateStore", "/tenancy/store/{ref}/reactivate", handlers.reactivateStore),
	}
}

func route(contractRoot, name, method, path string, budget deadline.Budget, permission ports.Permission) kernelhttp.Route {
	return kernelhttp.Route{
		Name:        name,
		Method:      method,
		Path:        path,
		ContractRef: contractRoot + strings.ReplaceAll(path, "/", "~1") + "/" + strings.ToLower(method),
		Budget:      budget,
		Requires:    kernelhttp.RequireSubjectAndTenant,
		Permission:  permission,
	}
}

func Routes(budget deadline.Budget) []kernelhttp.Route {
	bound := bindings(budget)
	routes := make([]kernelhttp.Route, len(bound))
	for i, b := range bound {
		routes[i] = b.route
	}
	return routes
}

// ValidateRoutes refuses the first route an inbound edge cannot serve, such as
// one that demands a subject without declaring the permission it needs: the
// process refuses to start instead of denying every call at the first request.
func ValidateRoutes(routes []kernelhttp.Route) error {
	for _, route := range routes {
		if err := route.ValidateEdge(); err != nil {
			return err
		}
	}
	return nil
}

func Limits(limit admission.Limit) map[string]admission.Limit {
	routes := Routes(deadline.Budget{})
	limits := make(map[string]admission.Limit, len(routes))
	for _, route := range routes {
		limits[pattern(route)] = limit
	}
	return limits
}

func pattern(route kernelhttp.Route) string { return route.Method + " " + route.Path }

// ErrAuthenticatorRequired refuses a handler that could not authenticate: every
// route here demands a subject, and a nil verifier would deny all of them at
// the first request instead of at the start.
var ErrAuthenticatorRequired = errors.New("api: no authenticator provided")

// WHY: the deadline wraps the execution context, not the reverse, because
// CTX-01 makes deadline mandatory and the context cannot resolve what the
// timeout has not yet set. Admission stays inside, still ahead of the body.
func NewHandler(tenancy rpc.Tenancy, ctrl *admission.Controller, instruments *metrics.Instruments, opts Options) (http.Handler, error) {
	if opts.Authenticator == nil {
		return nil, ErrAuthenticatorRequired
	}
	bound := bindings(opts.Budget)
	if err := ValidateRoutes(Routes(opts.Budget)); err != nil {
		return nil, err
	}
	h := handlers{tenancy: tenancy}

	logger := loggerOf(opts)
	admit := kernelhttp.Admission(ctrl, routeOf, tenantOf, instruments, refuseAsRejection)
	mux := http.NewServeMux()
	for _, b := range bound {
		handler := requireIdempotencyKey(b.route, b.build(h))
		mounted := withExecutionContext(logger, opts.Authenticator, b.route, admit(handler))
		mux.Handle(pattern(b.route), withRecover(withRouteDeadline(b.route.Budget, mounted)))
	}
	for _, contract := range []struct {
		path     string
		document []byte
	}{
		{TenancyContractPath, opts.TenancyContract},
	} {
		if len(contract.document) > 0 {
			mux.Handle(http.MethodGet+" "+contract.path, serveContract(contract.document))
		}
	}
	return otelhttp.NewHandler(withRoute(withAccessLog(logger, withCORS(opts.CORSOrigins, mux))), "bff", instrumentation(opts)...), nil
}

func instrumentation(opts Options) []otelhttp.Option {
	return []otelhttp.Option{
		otelhttp.WithTracerProvider(opts.TracerProvider),
		otelhttp.WithMeterProvider(opts.MeterProvider),
		otelhttp.WithPropagators(traceparentOnly{}),
	}
}

func loggerOf(opts Options) *slog.Logger {
	if opts.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return opts.Logger
}

func clockOf(opts Options) obsclock.Clock {
	if opts.Clock == nil {
		return obsclock.System()
	}
	return opts.Clock
}

func withRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if _, route, routed := strings.Cut(r.Pattern, " "); routed {
			trace.SpanFromContext(r.Context()).SetAttributes(semconv.HTTPRoute(route))
		}
	})
}

func routeOf(r *http.Request) string { return r.Pattern }

// tenantOf keys the admission bucket by the tenant the edge authenticated
// (RES-16); admission runs inside withExecutionContext, so the context is there.
func tenantOf(r *http.Request) string {
	execution, ok := ports.ExecutionContextFrom(r.Context())
	if !ok {
		return ""
	}
	tenant, _ := execution.Tenant()
	return string(tenant)
}
