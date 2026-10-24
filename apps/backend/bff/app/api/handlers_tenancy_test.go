package api_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tenancyv1 "github.com/mateusmacedo/marketplace/apps/backend/tenancy/contract/gen/go/company/tenancy/service/v1"
)

func TestEveryTenancyRouteAnswersItsSuccess(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		method, path, body string
		status             int
		want               string
	}{
		{http.MethodPost, "/tenancy/store", `{"ref":"loja-1","name":"Loja 1"}`, http.StatusCreated, `{"ref":"loja-1"}`},
		{http.MethodPost, "/tenancy/store/loja-1/suspend", `{"reason":"fraude"}`, http.StatusOK, `{"ref":"loja-1"}`},
		{http.MethodPost, "/tenancy/store/loja-1/reactivate", "", http.StatusOK, `{"ref":"loja-1"}`},
		{http.MethodGet, "/tenancy/store/loja-1", "", http.StatusOK, `{"ref":"loja-1","name":"Loja 1","status":"active","suspensionReason":"","registeredAt":1755432000000000000,"statusChangedAt":1755432000000000000}`},
		{http.MethodGet, "/tenancy/store?status=suspended", "", http.StatusOK, `[{"ref":"loja-1","name":"Loja 1","status":"suspended","suspensionReason":"fraude","registeredAt":1755432000000000000,"statusChangedAt":1755518400000000000}]`},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := f.do(t, tc.method, tc.path, strings.NewReader(tc.body), "Idempotency-Key", "k-1")
			if rec.Code != tc.status || strings.TrimSpace(rec.Body.String()) != tc.want {
				t.Fatalf("%d %s, want %d %s", rec.Code, rec.Body.String(), tc.status, tc.want)
			}
		})
	}
}

func TestAnEmptyListIsAnEmptyArray(t *testing.T) {
	f := newFixture(t, withContexts((&fakeContexts{}).on("FindStoresByStatus", func(int) (any, error) {
		return &tenancyv1.FindStoresByStatusResponse{}, nil
	})))

	rec := f.do(t, http.MethodGet, "/tenancy/store?status=active", nil)

	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("%d %s, want 200 []", rec.Code, rec.Body.String())
	}
}

func TestEveryTenancyRouteForwardsTheRequestOfTheContract(t *testing.T) {
	f := newFixture(t)

	f.post(t, "/tenancy/store", `{"ref":"loja-1","name":"Loja 1"}`)
	f.post(t, "/tenancy/store/loja-2/suspend", `{"reason":"fraude"}`)
	f.post(t, "/tenancy/store/loja-3/reactivate", "")
	f.do(t, http.MethodGet, "/tenancy/store/loja-4", nil)
	f.do(t, http.MethodGet, "/tenancy/store?status=active", nil)

	if req, ok := onlyRequest[*tenancyv1.RegisterStoreRequest](t, f, "RegisterStore"); !ok || req.GetRef() != "loja-1" || req.GetName() != "Loja 1" {
		t.Fatalf("RegisterStore request = %v, want loja-1 named Loja 1", req)
	}
	if req, ok := onlyRequest[*tenancyv1.SuspendStoreRequest](t, f, "SuspendStore"); !ok || req.GetRef() != "loja-2" || req.GetReason() != "fraude" {
		t.Fatalf("SuspendStore request = %v, want loja-2 for fraude", req)
	}
	if req, ok := onlyRequest[*tenancyv1.ReactivateStoreRequest](t, f, "ReactivateStore"); !ok || req.GetRef() != "loja-3" {
		t.Fatalf("ReactivateStore request = %v, want loja-3", req)
	}
	if req, ok := onlyRequest[*tenancyv1.FindStoreRequest](t, f, "FindStore"); !ok || req.GetRef() != "loja-4" {
		t.Fatalf("FindStore request = %v, want loja-4", req)
	}
	if req, ok := onlyRequest[*tenancyv1.FindStoresByStatusRequest](t, f, "FindStoresByStatus"); !ok || req.GetStatus() != tenancyv1.StoreStatus_STORE_STATUS_ACTIVE {
		t.Fatalf("FindStoresByStatus request = %v, want the active status", req)
	}
}

func onlyRequest[Req any](t *testing.T, f fixture, method string) (Req, bool) {
	t.Helper()
	calls := f.fake.callsTo(method)
	if len(calls) != 1 {
		t.Fatalf("%s reached the context %d times, want 1", method, len(calls))
	}
	req, ok := calls[0].request.(Req)
	return req, ok
}

// WHY: the form of a registered ref is the domain's decision, so the edge
// forwards it and the refusal of the context reaches the client as a 422.
func TestTheRefOfARegistrationIsLeftToTheContext(t *testing.T) {
	f := newFixture(t, withContexts((&fakeContexts{}).on("RegisterStore", func(int) (any, error) {
		return &tenancyv1.RegisterStoreResponse{Result: &tenancyv1.RegisterStoreResponse_Rejection{
			Rejection: &tenancyv1.Rejection{Code: "tenancy/store/ref-invalid", Message: "ref must have 3 to 63 characters"},
		}}, nil
	})))

	rec := f.post(t, "/tenancy/store", `{"ref":"-Loja-","name":"Loja 1"}`)

	requireRejection(t, rec, http.StatusUnprocessableEntity, "tenancy/store/ref-invalid")
	if req, ok := onlyRequest[*tenancyv1.RegisterStoreRequest](t, f, "RegisterStore"); !ok || req.GetRef() != "-Loja-" {
		t.Fatalf("RegisterStore request = %v, want the ref forwarded as sent", req)
	}
}

func TestANameOf200CharactersOutsideASCIIIsAccepted(t *testing.T) {
	f := newFixture(t)
	name := strings.Repeat("á", 200)

	rec := f.post(t, "/tenancy/store", `{"ref":"loja-1","name":"`+name+`"}`)

	calls := f.fake.callsTo("RegisterStore")
	if rec.Code != http.StatusCreated || len(calls) != 1 {
		t.Fatalf("status = %d with %d calls, want 201 with 1 (body %s)", rec.Code, len(calls), rec.Body.String())
	}
	if req, ok := calls[0].request.(*tenancyv1.RegisterStoreRequest); !ok || req.GetName() != name {
		t.Fatalf("RegisterStore request = %v, want the name intact", calls[0].request)
	}
}

func TestTheNameAndTheReasonFollowTheNameRule(t *testing.T) {
	for _, field := range []struct {
		name, path, method string
		body               func(*testing.T, string) string
		text               func(any) (string, bool)
		accepted           int
	}{
		{"name", "/tenancy/store", "RegisterStore", registerBody, func(req any) (string, bool) {
			r, ok := req.(*tenancyv1.RegisterStoreRequest)
			return r.GetName(), ok
		}, http.StatusCreated},
		{"reason", "/tenancy/store/loja-1/suspend", "SuspendStore", suspendBody, func(req any) (string, bool) {
			r, ok := req.(*tenancyv1.SuspendStoreRequest)
			return r.GetReason(), ok
		}, http.StatusOK},
	} {
		for _, tc := range nameRuleCases {
			t.Run(field.name+"/"+tc.desc, func(t *testing.T) {
				f := newFixture(t)

				rec := f.post(t, field.path, field.body(t, tc.name))

				calls := f.fake.callsTo(field.method)
				if !tc.accepted {
					if rec.Code != http.StatusBadRequest || len(calls) != 0 {
						t.Fatalf("status = %d with %d calls, want 400 with none for %q (body %s)", rec.Code, len(calls), tc.name, rec.Body.String())
					}
					return
				}
				if rec.Code != field.accepted || len(calls) != 1 {
					t.Fatalf("status = %d with %d calls, want %d with 1 for %q (body %s)", rec.Code, len(calls), field.accepted, tc.name, rec.Body.String())
				}
				if text, ok := field.text(calls[0].request); !ok || text != tc.name {
					t.Fatalf("%s request = %v, want the %s %q intact", field.method, calls[0].request, field.name, tc.name)
				}
			})
		}
	}
}

func registerBody(t *testing.T, name string) string {
	t.Helper()
	return marshal(t, map[string]string{"ref": "loja-1", "name": name})
}

func suspendBody(t *testing.T, reason string) string {
	t.Helper()
	return marshal(t, map[string]string{"reason": reason})
}

func marshal(t *testing.T, body map[string]string) string {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("json.Marshal() = %v", err)
	}
	return string(encoded)
}

func TestMalformedTenancyRequestsNeverCallTheContext(t *testing.T) {
	cases := map[string]struct {
		method, path, body string
	}{
		"register without name":          {http.MethodPost, "/tenancy/store", `{"ref":"loja-1"}`},
		"register with an unknown field": {http.MethodPost, "/tenancy/store", `{"ref":"loja-1","name":"Loja 1","status":"active"}`},
		"name above 200 characters":      {http.MethodPost, "/tenancy/store", `{"ref":"loja-1","name":"` + strings.Repeat("á", 201) + `"}`},
		"name with a line break":         {http.MethodPost, "/tenancy/store", `{"ref":"loja-1","name":"Loja\n1"}`},
		"suspend without reason":         {http.MethodPost, "/tenancy/store/loja-1/suspend", `{}`},
		"suspend without body":           {http.MethodPost, "/tenancy/store/loja-1/suspend", ""},
		"ref of two characters":          {http.MethodPost, "/tenancy/store/lj/suspend", `{"reason":"fraude"}`},
		"ref of 64 characters":           {http.MethodPost, "/tenancy/store/" + strings.Repeat("a", 64) + "/reactivate", ""},
		"ref with a leading hyphen":      {http.MethodPost, "/tenancy/store/-loja/reactivate", ""},
		"ref with a trailing hyphen":     {http.MethodGet, "/tenancy/store/loja-", ""},
		"ref in upper case":              {http.MethodGet, "/tenancy/store/LOJA", ""},
		"ref with an underscore":         {http.MethodGet, "/tenancy/store/loja_1", ""},
		"missing status filter":          {http.MethodGet, "/tenancy/store", ""},
		"unknown status filter":          {http.MethodGet, "/tenancy/store?status=closed", ""},
		"status filter in upper case":    {http.MethodGet, "/tenancy/store?status=ACTIVE", ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)

			rec := f.do(t, tc.method, tc.path, strings.NewReader(tc.body), "Idempotency-Key", "k-1", "Content-Type", "application/json")

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			if n := f.fake.total(); n != 0 {
				t.Fatalf("calls = %d, want 0", n)
			}
		})
	}
}

func TestARefOfThreeAndOfSixtyThreeCharactersIsRouted(t *testing.T) {
	for _, ref := range []string{"a1b", strings.Repeat("a", 63), "0-0"} {
		t.Run(ref, func(t *testing.T) {
			rec := newFixture(t).do(t, http.MethodGet, "/tenancy/store/"+ref, nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET /tenancy/store/%s = %d, want 200 (body %s)", ref, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestADomainRefusalOfTheTenancyContextIs422(t *testing.T) {
	cases := []struct {
		method, path, body, code string
		fake                     *fakeContexts
	}{
		{"SuspendStore", "/tenancy/store/loja-1/suspend", `{"reason":"fraude"}`, "tenancy/store/not-active", (&fakeContexts{}).on("SuspendStore", func(int) (any, error) {
			return &tenancyv1.SuspendStoreResponse{Result: &tenancyv1.SuspendStoreResponse_Rejection{
				Rejection: &tenancyv1.Rejection{Code: "tenancy/store/not-active", Message: "store is not active"},
			}}, nil
		})},
		{"ReactivateStore", "/tenancy/store/loja-1/reactivate", "", "tenancy/store/not-suspended", (&fakeContexts{}).on("ReactivateStore", func(int) (any, error) {
			return &tenancyv1.ReactivateStoreResponse{Result: &tenancyv1.ReactivateStoreResponse_Rejection{
				Rejection: &tenancyv1.Rejection{Code: "tenancy/store/not-suspended", Message: "store is not suspended"},
			}}, nil
		})},
		{"RegisterStore", "/tenancy/store", `{"ref":"loja-2","name":"Loja 1"}`, "tenancy/store/duplicate-name", (&fakeContexts{}).on("RegisterStore", func(int) (any, error) {
			return &tenancyv1.RegisterStoreResponse{Result: &tenancyv1.RegisterStoreResponse_Rejection{
				Rejection: &tenancyv1.Rejection{Code: "tenancy/store/duplicate-name", Message: "another store has this name"},
			}}, nil
		})},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			rec := newFixture(t, withContexts(tc.fake)).post(t, tc.path, tc.body)

			requireRejection(t, rec, http.StatusUnprocessableEntity, tc.code)
			if got := rec.Header().Get("Idempotent-Replayed"); got != "" {
				t.Fatalf("Idempotent-Replayed = %q on a first refusal, want none", got)
			}
		})
	}
}

func TestAnUnknownStoreIs404(t *testing.T) {
	for _, route := range []struct{ method, rpc, path, body string }{
		{http.MethodGet, "FindStore", "/tenancy/store/loja-none", ""},
		{http.MethodPost, "SuspendStore", "/tenancy/store/loja-none/suspend", `{"reason":"fraude"}`},
		{http.MethodPost, "ReactivateStore", "/tenancy/store/loja-none/reactivate", ""},
	} {
		t.Run(route.rpc, func(t *testing.T) {
			f := newFixture(t, withContexts((&fakeContexts{}).on(route.rpc, func(int) (any, error) {
				return nil, status.Error(codes.NotFound, "not found")
			})))

			rec := f.do(t, route.method, route.path, strings.NewReader(route.body), "Idempotency-Key", "k-1", "Content-Type", "application/json")

			requireRejection(t, rec, http.StatusNotFound, "not-found")
		})
	}
}

// tenancyRoutes are the five tenancy routes with a body that passes the edge
// validation and the permission each one demands.
var tenancyRoutes = []struct{ method, rpc, path, body, permission string }{
	{http.MethodPost, "RegisterStore", "/tenancy/store", `{"ref":"loja-1","name":"Loja 1"}`, "tenancy:write"},
	{http.MethodGet, "FindStoresByStatus", "/tenancy/store?status=active", "", "tenancy:read"},
	{http.MethodGet, "FindStore", "/tenancy/store/loja-1", "", "tenancy:read"},
	{http.MethodPost, "SuspendStore", "/tenancy/store/loja-1/suspend", `{"reason":"fraude"}`, "tenancy:write"},
	{http.MethodPost, "ReactivateStore", "/tenancy/store/loja-1/reactivate", "", "tenancy:write"},
}

func credentialHolding(permission string) string {
	return `Bearer {"sub":"tester","tenant":"acme","permissions":["` + permission + `"]}`
}

func TestATenancyRouteWithoutItsPermissionIs403WithoutReachingTheContext(t *testing.T) {
	other := map[string]string{"tenancy:write": "tenancy:read", "tenancy:read": "tenancy:write"}
	for _, route := range tenancyRoutes {
		t.Run(route.rpc, func(t *testing.T) {
			f := newFixture(t)

			rec := f.do(t, route.method, route.path, strings.NewReader(route.body),
				"Idempotency-Key", "k-1", "Content-Type", "application/json", "Authorization", credentialHolding(other[route.permission]))

			requireRejection(t, rec, http.StatusForbidden, "permission-denied")
			if n := f.fake.total(); n != 0 {
				t.Fatalf("calls = %d, want 0: the edge refuses a caller without %s", n, route.permission)
			}
		})
	}
}

// WHY: the operator check lives in the context, so a caller the edge lets
// through — holding the route's permission in a tenant that is not the
// operator — is still refused there, and the 403 reaches the client without the
// detail the context gave.
func TestAContextThatRefusesTheCallerIs403(t *testing.T) {
	for _, route := range tenancyRoutes {
		t.Run(route.rpc, func(t *testing.T) {
			f := newFixture(t, withContexts((&fakeContexts{}).on(route.rpc, func(int) (any, error) {
				return nil, status.Error(codes.PermissionDenied, "tenant acme is not the operator")
			})))

			rec := f.do(t, route.method, route.path, strings.NewReader(route.body),
				"Idempotency-Key", "k-1", "Content-Type", "application/json", "Authorization", credentialHolding(route.permission))

			requireRejection(t, rec, http.StatusForbidden, "permission-denied")
			if strings.Contains(rec.Body.String(), "operator") {
				t.Fatalf("body %s leaked the context detail", rec.Body.String())
			}
			if n := len(f.fake.callsTo(route.rpc)); n != 1 {
				t.Fatalf("%s reached the context %d times, want 1", route.rpc, n)
			}
		})
	}
}
