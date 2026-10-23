package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kernelhttp "github.com/mateusmacedo/dmpf/libs/backend/go/http"
	"github.com/mateusmacedo/dmpf/libs/backend/go/transport/deadline"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app/api"
)

func lookup(pairs ...string) func(string) string {
	values := map[string]string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		values[pairs[i]] = pairs[i+1]
	}
	return func(name string) string { return values[name] }
}

const denyList = "redis://revocation:secret@127.0.0.1:1/0"

func TestRunRefusesToStartWithoutConfiguration(t *testing.T) {
	cases := []struct {
		name  string
		env   func(string) string
		named string
	}{
		{"identity", lookup("REDIS_URL", denyList), "AUTH_DEV_MOCK"},
		{"deny-list", lookup("AUTH_DEV_MOCK", "true"), "REDIS_URL"},
		{"public address", lookup("AUTH_DEV_MOCK", "true", "REDIS_URL", denyList, "HTTP_ADDR", "8080"), "HTTP_ADDR"},
		{"public and administration on one port", lookup("AUTH_DEV_MOCK", "true", "REDIS_URL", denyList,
			"HTTP_ADDR", "127.0.0.1:9100", "ADMIN_ADDR", "127.0.0.1:9100"), "HTTP_ADDR"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var errOut bytes.Buffer

			code := run(options{lookup: tc.env}, &errOut)

			if code != exitUsage {
				t.Fatalf("run() = %d, want %d (usage)", code, exitUsage)
			}
			if !strings.Contains(errOut.String(), tc.named) {
				t.Fatalf("stderr = %q, want %s named", errOut.String(), tc.named)
			}
		})
	}
}

// A route demanding a subject without a permission would deny every call it
// serves; ValidateEdge turns that into a start that never happens (IDN-16).
func TestRunRefusesToStartARouteWithASubjectAndNoPermission(t *testing.T) {
	undeclared := func(budget deadline.Budget) []kernelhttp.Route {
		return []kernelhttp.Route{{
			Name: "unguarded", Method: http.MethodGet, Path: "/v1/unguarded",
			ContractRef: "apps/backend/bff/contract/openapi/v1/openapi.yaml#/paths/~1v1~1unguarded/get",
			Budget:      budget, Requires: kernelhttp.RequireSubject,
		}}
	}
	var errOut bytes.Buffer

	code := run(options{lookup: lookup("AUTH_DEV_MOCK", "true", "REDIS_URL", denyList), routes: undeclared}, &errOut)

	if code != exitUsage {
		t.Fatalf("run() = %d, want %d: the start is refused before serving (stderr %q)", code, exitUsage, errOut.String())
	}
	if !strings.Contains(errOut.String(), "unguarded") || !strings.Contains(errOut.String(), "permission") {
		t.Fatalf("stderr = %q, want the route and the missing permission named", errOut.String())
	}
}

func TestTheRoutesTheEdgeServesPassTheStartValidation(t *testing.T) {
	var errOut bytes.Buffer
	served := 0
	counted := func(budget deadline.Budget) []kernelhttp.Route {
		routes := api.Routes(budget)
		served = len(routes)
		return append(routes, kernelhttp.Route{Name: "invalid"})
	}

	code := run(options{lookup: lookup("AUTH_DEV_MOCK", "true", "REDIS_URL", denyList), routes: counted}, &errOut)

	if served == 0 {
		t.Fatal("the edge serves no route, want GET /v1/me at least")
	}
	if code != exitUsage || !strings.Contains(errOut.String(), "invalid") {
		t.Fatalf("run() = %d (stderr %q), want only the appended route refused, after every route the edge serves", code, errOut.String())
	}
}

func TestHealthcheckAnswersOnlyHealthyOrUnhealthy(t *testing.T) {
	edge := func(code int) string {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }))
		t.Cleanup(srv.Close)
		return srv.Listener.Addr().String()
	}
	configured := func(addr string) func(string) string {
		return lookup("AUTH_DEV_MOCK", "true", "ADMIN_ADDR", addr, "REDIS_URL", denyList)
	}
	cases := []struct {
		name string
		env  func(string) string
		want int
	}{
		{"edge ready", configured(edge(http.StatusNoContent)), exitOK},
		{"edge not ready", configured(edge(http.StatusServiceUnavailable)), exitFailure},
		{"no configuration", lookup(), exitFailure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var errOut bytes.Buffer

			if code := run(options{lookup: tc.env, args: []string{"healthcheck"}}, &errOut); code != tc.want {
				t.Fatalf("run(healthcheck) = %d, want %d (stderr %q)", code, tc.want, errOut.String())
			}
		})
	}
}

func TestTheSessionEventsRoleReadsOnlyItsOwnConfiguration(t *testing.T) {
	var errOut bytes.Buffer

	code := run(options{lookup: lookup(), args: []string{"session-events"}}, &errOut)

	if code != exitUsage {
		t.Fatalf("run(session-events) = %d, want %d (usage)", code, exitUsage)
	}
	if !strings.Contains(errOut.String(), "OIDC_ISSUER") || strings.Contains(errOut.String(), "AUTH_DEV_MOCK") {
		t.Fatalf("stderr = %q, want the role's own OIDC_ISSUER named and no variable of the API", errOut.String())
	}
}

func TestTheSessionEventsHealthcheckNeedsOnlyTheAdministrationAddress(t *testing.T) {
	admin := func(code int) string {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }))
		t.Cleanup(srv.Close)
		return srv.Listener.Addr().String()
	}
	cases := []struct {
		name string
		env  func(string) string
		want int
	}{
		{"role ready", lookup("ADMIN_ADDR", admin(http.StatusNoContent)), exitOK},
		{"role not ready", lookup("ADMIN_ADDR", admin(http.StatusServiceUnavailable)), exitFailure},
		{"invalid address", lookup("ADMIN_ADDR", "8092"), exitFailure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var errOut bytes.Buffer

			if code := run(options{lookup: tc.env, args: []string{"session-events", "healthcheck"}}, &errOut); code != tc.want {
				t.Fatalf("run(session-events healthcheck) = %d, want %d (stderr %q)", code, tc.want, errOut.String())
			}
		})
	}
}
