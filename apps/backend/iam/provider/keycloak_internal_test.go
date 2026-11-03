package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	kernelhttp "github.com/mateusmacedo/dmpf/libs/backend/go/http"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/resilience"
	kernelports "github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/ports"
)

func TestTheKeycloakSheetIsThePlatformDefaultOfItsDependency(t *testing.T) {
	sheet := keycloakSheet()

	if err := sheet.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	if !reflect.DeepEqual(sheet, resilience.Defaults("keycloak")) {
		t.Fatalf("sheet = %+v, want resilience.Defaults(keycloak): a departure needs an Override (RES-40)", sheet)
	}
}

func TestEveryKeycloakRouteIsDeclaredForTheTenantAndRetriesOnlyWhatIsIdempotent(t *testing.T) {
	routes := keycloakRoutes()
	if len(routes) == 0 {
		t.Fatal("no route is declared")
	}
	for name, route := range routes {
		t.Run(name, func(t *testing.T) {
			if err := route.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if route.Name != name || route.Budget.Dependency != keycloakDependency || route.Budget.Method != name {
				t.Fatalf("route %+v, want it named %s under the keycloak dependency", route, name)
			}
			if route.Requires != kernelhttp.RequireTenant {
				t.Fatalf("Requires = %v, want RequireTenant: the narrowest declaration Route.Validate accepts without a platform reach", route.Requires)
			}
			if !strings.HasPrefix(route.ContractRef, "https://www.keycloak.org/") {
				t.Fatalf("ContractRef = %q, want the published Keycloak contract", route.ContractRef)
			}
			if route.IdempotencyKey != "" {
				t.Fatalf("IdempotencyKey = %q, want none: Keycloak deduplicates on no key", route.IdempotencyKey)
			}
			want := []int{http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout}
			if route.Method == http.MethodPost {
				want = nil
			}
			if !slices.Equal(route.RetryableStatus, want) {
				t.Fatalf("%s RetryableStatus = %v, want %v", route.Method, route.RetryableStatus, want)
			}
		})
	}
}

func TestKeycloakCallRefusesAnEmptyOrDotSegmentBeforeAnyRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	k, err := NewKeycloak(KeycloakConfig{
		BaseURL: server.URL, Realm: "dmpf", ClientID: "iam-admin", Secret: NewKeycloakSecret("s"), OperatorTenant: "marketplace",
	}, KeycloakDeps{Clock: clock.System()})
	if err != nil {
		t.Fatalf("NewKeycloak() = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	for _, segment := range []string{"", ".", ".."} {
		t.Run(strconv.Quote(segment), func(t *testing.T) {
			_, err := k.call(ctx, routeRemoveMember, k.admin("organizations", "org-1", "members", segment), nil)

			if class := classOf(err); class != kernelports.ClassFinalValidation {
				t.Fatalf("call() = %v (%v), want final_validation", class, err)
			}
			if strings.Contains(err.Error(), "org-1") {
				t.Fatalf("err = %v, want a constant detail without the path", err)
			}
		})
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("requests = %d, want none: not even the token", n)
	}
}

func TestKeycloakPayloadsRoundTripThroughTheDecoders(t *testing.T) {
	codec := KeycloakPayloads{}

	role := ports.RoleAccess{Ref: "expedicao", Status: domain.RoleStatusActive, Permissions: []domain.Permission{"fulfillment:read"}}
	if got := roundTrip(t, codec.RoleAccess, decodeRoleAccess, role); !reflect.DeepEqual(got, role) {
		t.Errorf("RoleAccess round trip = %+v, want %+v", got, role)
	}
	provision := ports.MemberProvision{Email: "ana@loja.example"}
	if got := roundTrip(t, codec.MemberProvision, decodeMemberProvision, provision); got != provision {
		t.Errorf("MemberProvision round trip = %+v, want %+v", got, provision)
	}
	member := ports.MemberAccess{Subject: "sub-1", Status: domain.MembershipStatusRemoved, Roles: []domain.RoleRef{}}
	if got := roundTrip(t, codec.MemberAccess, decodeMemberAccess, member); !reflect.DeepEqual(got, member) {
		t.Errorf("MemberAccess round trip = %+v, want %+v", got, member)
	}
	buyer := ports.BuyerAccess{Subject: "sub-2", Status: domain.BuyerStatusBlocked}
	if got := roundTrip(t, codec.BuyerAccess, decodeBuyerAccess, buyer); got != buyer {
		t.Errorf("BuyerAccess round trip = %+v, want %+v", got, buyer)
	}
}

func roundTrip[T any](t *testing.T, encode func(T) ([]byte, error), decode func([]byte) (T, error), value T) T {
	t.Helper()
	raw, err := encode(value)
	if err != nil {
		t.Fatalf("encode(%+v) = %v", value, err)
	}
	got, err := decode(raw)
	if err != nil {
		t.Fatalf("decode(%s) = %v", raw, err)
	}
	return got
}

func TestKeycloakPayloadDecodersRefuseAnInvalidForm(t *testing.T) {
	tests := []struct {
		name   string
		decode func() error
	}{
		{"role not JSON", func() error { _, err := decodeRoleAccess([]byte("{")); return err }},
		{"role without ref", func() error { _, err := decodeRoleAccess([]byte(`{"status":"active"}`)); return err }},
		{"role with an invalid ref", func() error {
			_, err := decodeRoleAccess([]byte(`{"ref":"Expedição","status":"active"}`))
			return err
		}},
		{"role with an unknown status", func() error { _, err := decodeRoleAccess([]byte(`{"ref":"expedicao","status":"paused"}`)); return err }},
		{"role with an empty permission", func() error {
			_, err := decodeRoleAccess([]byte(`{"ref":"expedicao","status":"active","permissions":[""]}`))
			return err
		}},
		{"provision without email", func() error { _, err := decodeMemberProvision([]byte(`{}`)); return err }},
		{"provision with an invalid email", func() error { _, err := decodeMemberProvision([]byte(`{"email":"ana"}`)); return err }},
		{"provision with an email not normalized", func() error {
			_, err := decodeMemberProvision([]byte(`{"email":"Ana@Loja.example"}`))
			return err
		}},
		{"member without subject", func() error { _, err := decodeMemberAccess([]byte(`{"status":"active"}`)); return err }},
		{"member with an unknown status", func() error { _, err := decodeMemberAccess([]byte(`{"subject":"sub-1","status":"gone"}`)); return err }},
		{"member with an invalid role", func() error {
			_, err := decodeMemberAccess([]byte(`{"subject":"sub-1","status":"active","roles":["X"]}`))
			return err
		}},
		{"buyer without subject", func() error { _, err := decodeBuyerAccess([]byte(`{"status":"active"}`)); return err }},
		{"buyer with an unknown status", func() error {
			_, err := decodeBuyerAccess([]byte(`{"subject":"sub-2","status":"retired"}`))
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.decode(); !errors.Is(err, ErrKeycloakPayload) {
				t.Fatalf("decode = %v, want ErrKeycloakPayload", err)
			}
		})
	}
}
