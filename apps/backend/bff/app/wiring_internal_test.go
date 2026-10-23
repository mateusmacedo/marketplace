package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	obsclock "github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/otelboot"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app/rpc"
)

// threeBackends stands for an edge with more contexts than the one it dials
// today, so a dial that fails midway has backends already open to close.
func threeBackends() []backend {
	return []backend{
		{"alpha", "passthrough:///alpha", rpc.TenancyConfig},
		{"beta", "passthrough:///beta", rpc.TenancyConfig},
		{"gamma", "passthrough:///gamma", rpc.TenancyConfig},
	}
}

func runtimeOf(t *testing.T) *otelboot.Runtime {
	t.Helper()
	rt, err := otelboot.Start(context.Background(), otelboot.Config{
		Disabled:   true,
		Propagator: propagation.TraceContext{},
		Resource:   otelboot.Resource{ServiceName: "bff", ServiceVersion: "test", ServiceInstanceID: "bff-1", Role: processRole},
	})
	if err != nil {
		t.Fatalf("otelboot.Start() = %v", err)
	}
	t.Cleanup(func() { _ = rt.Shutdown(context.Background()) })
	return rt
}

func TestTheEdgeDialsTheTenancyContext(t *testing.T) {
	got := backendsOf(Config{TenancyTarget: "dns:///tenancy-api:9090"})

	if len(got) != 1 || got[0].name != tenancyBackend || got[0].target != "dns:///tenancy-api:9090" {
		t.Fatalf("backendsOf() = %v, want the tenancy context at its target", got)
	}
	if cfg := got[0].config(rpc.Options{Insecure: true}); cfg.HealthServiceName != rpc.TenancyServiceName {
		t.Fatalf("config = %+v, want the readiness of the tenancy service", cfg)
	}
}

func TestABackendIsNeverDialedInTheClearUndeclared(t *testing.T) {
	_, err := ClientOptions(context.Background(), Config{}, runtimeOf(t))

	if !errors.Is(err, ErrInsecureNotDeclared) {
		t.Fatalf("ClientOptions() = %v, want ErrInsecureNotDeclared", err)
	}
}

func TestClientOptionsResolveTheDeclaredTransport(t *testing.T) {
	rt := runtimeOf(t)

	insecure, err := ClientOptions(context.Background(), Config{GRPCInsecure: true}, rt)
	if err != nil || !insecure.Insecure || insecure.TLS != nil {
		t.Fatalf("ClientOptions(GRPC_INSECURE) = %+v, %v; want the development opt-out", insecure, err)
	}

	p := tb.NewPKI(t)
	cert, key := p.Client(t, "bff")
	trusted, err := ClientOptions(context.Background(), Config{CAFile: p.CAFile, ServerName: "localhost", ClientCertFile: cert, ClientKeyFile: key}, rt)
	if err != nil || trusted.Insecure || trusted.TLS == nil || len(trusted.TLS.Certificates) != 1 {
		t.Fatalf("ClientOptions(GRPC_CA_FILE) = %+v, %v; want mutual TLS presenting the edge's certificate", trusted, err)
	}

	if _, err := ClientOptions(context.Background(), Config{CAFile: filepath.Join(t.TempDir(), "absent.pem")}, rt); err == nil {
		t.Fatal("ClientOptions() with an unreadable authority = nil, want an error")
	}
}

func TestEveryBackendIsDialedUnderItsName(t *testing.T) {
	backends := backendsOf(Config{TenancyTarget: "passthrough:///tenancy"})

	conns, err := dialBackends(rpc.Dial, rpc.Options{Insecure: true, Clock: obsclock.System()}, backends)
	if err != nil {
		t.Fatalf("dialBackends() = %v, want nil", err)
	}
	t.Cleanup(func() { closeBackends(backends, conns) })

	for _, b := range backends {
		if conns[b.name] == nil {
			t.Fatalf("conns[%q] = nil, want the dialed connection", b.name)
		}
	}
}

func TestAFailedDialClosesTheBackendsAlreadyOpen(t *testing.T) {
	refused := errors.New("dial refused")
	var opened []*grpc.ClientConn
	dial := func(target string, cfg kernelgrpc.Config, extra ...grpc.DialOption) (*grpc.ClientConn, error) {
		if len(opened) == 1 {
			return nil, refused
		}
		conn, err := rpc.Dial(target, cfg, extra...)
		if err == nil {
			opened = append(opened, conn)
		}
		return conn, err
	}

	conns, err := dialBackends(dial, rpc.Options{Insecure: true, Clock: obsclock.System()}, threeBackends())

	if !errors.Is(err, refused) || conns != nil {
		t.Fatalf("dialBackends() = (%v, %v), want (nil, the dial error)", conns, err)
	}
	if len(opened) != 1 || opened[0].GetState() != connectivity.Shutdown {
		t.Fatalf("opened = %v, want the first backend closed after the second dial failed", opened)
	}
}

func TestReadContractServesNothingUndeclaredAndRefusesAnUnreadableFile(t *testing.T) {
	if document, err := readContract(""); err != nil || document != nil {
		t.Fatalf("readContract(\"\") = (%q, %v), want nothing to serve", document, err)
	}
	path := filepath.Join(t.TempDir(), "openapi.yaml")
	if err := os.WriteFile(path, []byte("openapi: 3.1.0\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}
	if document, err := readContract(path); err != nil || string(document) != "openapi: 3.1.0\n" {
		t.Fatalf("readContract() = (%q, %v), want the document", document, err)
	}
	if _, err := readContract(filepath.Join(t.TempDir(), "absent.yaml")); err == nil || !strings.HasPrefix(err.Error(), "openapi: ") {
		t.Fatalf("readContract(absent) = %v, want an openapi error", err)
	}
}
