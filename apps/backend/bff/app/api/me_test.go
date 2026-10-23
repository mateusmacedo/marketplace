package api_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	kernelhttp "github.com/mateusmacedo/dmpf/libs/backend/go/http"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app/api"
)

const (
	mePath = "/v1/me"

	forgedCredential   = "Bearer not-a-declaration"
	tenantless         = `Bearer {"sub":"tester","permissions":["profile"]}`
	withoutProfile     = `Bearer {"sub":"tester","tenant":"acme","permissions":["openid","email"]}`
	contractOfTheRoute = "apps/backend/bff/contract/openapi/v1/openapi.yaml#/paths/~1v1~1me/get"
)

type profile struct {
	Subject string `json:"subject"`
	Tenant  string `json:"tenant"`
}

func TestMeIsDeclaredForASubjectAndATenantWithTheProfilePermission(t *testing.T) {
	routes := api.Routes(routeBudget)

	i := slices.IndexFunc(routes, func(r kernelhttp.Route) bool { return r.Method == http.MethodGet && r.Path == mePath })
	if i < 0 {
		t.Fatalf("Routes() = %v, want GET %s", routes, mePath)
	}
	me := routes[i]
	if me.Requires != kernelhttp.RequireSubjectAndTenant || me.Permission != "profile" || me.ContractRef != contractOfTheRoute {
		t.Fatalf("route = %+v, want RequireSubjectAndTenant, permission profile and the bff contract", me)
	}
	if err := me.ValidateEdge(); err != nil {
		t.Fatalf("ValidateEdge() = %v, want an inbound route the edge can serve", err)
	}
}

func TestMeAnswersTheSubjectAndTheTenantTheCredentialResolved(t *testing.T) {
	f := newFixture(t)

	rec := f.do(t, http.MethodGet, mePath, nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var body profile
	decode(t, rec, &body)
	if body != (profile{Subject: "tester", Tenant: testTenant}) {
		t.Fatalf("body = %+v, want the subject and the tenant of the credential", body)
	}
}

func TestMeRefusesWhatTheCredentialDoesNotResolve(t *testing.T) {
	cases := []struct {
		name, credential string
		status           int
		code             string
	}{
		{"no credential", "", http.StatusUnauthorized, "unauthenticated"},
		{"forged credential", forgedCredential, http.StatusUnauthorized, "unauthenticated"},
		{"no tenant", tenantless, http.StatusForbidden, "tenant-unresolved"},
		{"no profile permission", withoutProfile, http.StatusForbidden, "permission-denied"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)

			rec := f.do(t, http.MethodGet, mePath, nil, "Authorization", tc.credential)

			requireRejection(t, rec, tc.status, tc.code)
			requireNoProfile(t, rec.Body.String())
		})
	}
}

// CTX-06 at the edge: the client asserting another identity than the credential
// resolved is an elevation attempt, refused before the handler runs.
func TestMeRefusesAnIdentityTheClientAssertsAgainstTheCredential(t *testing.T) {
	cases := []struct {
		name, path string
		headers    []string
	}{
		{"X-Tenant-ID of another tenant", mePath, []string{"X-Tenant-ID", "globex"}},
		{"X-Subject-ID of another subject", mePath, []string{"X-Subject-ID", "mallory"}},
		{"tenant_id query of another tenant", mePath + "?tenant_id=globex", nil},
		{"empty X-Tenant-ID", mePath, []string{"X-Tenant-ID", ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)

			rec := f.do(t, http.MethodGet, tc.path, nil, tc.headers...)

			requireRejection(t, rec, http.StatusForbidden, "identity-mismatch")
			requireNoProfile(t, rec.Body.String())
		})
	}
}

func TestMeTakesAnIdentityEqualToTheCredentialAsRedundant(t *testing.T) {
	f := newFixture(t)

	rec := f.do(t, http.MethodGet, mePath+"?tenant_id="+testTenant, nil, "X-Tenant-ID", testTenant, "X-Subject-ID", "tester")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: an equal value is redundant, and still never the source (body %s)", rec.Code, rec.Body.String())
	}
}

func requireNoProfile(t *testing.T, body string) {
	t.Helper()
	if strings.Contains(body, `"subject"`) || strings.Contains(body, `"tenant"`) {
		t.Fatalf("body = %s, want the refusal only: the handler must not run", body)
	}
}
