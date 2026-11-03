//go:build integration && keycloak

package provider_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

const (
	realmPath          = "/admin/realms/dmpf"
	realmOperator      = "marketplace"
	realmOwnerGroup    = "owner"
	realmStoreClient   = "marketplace-store"
	realmBuyerRole     = "iam:buyer"
	realmOperatorUser  = "operator"
	realmSessionEvents = "service-account-dmpf-session-events"
	realmTimeout       = 15 * time.Second
	realmHold          = 3 * time.Second
	realmKey           = "0123456789abcdef0123456789abcdef"
)

var realmHTTP = &http.Client{Timeout: realmTimeout}

func keycloakBase(t *testing.T) string {
	t.Helper()
	return strings.TrimSuffix(tb.Env(t, "KEYCLOAK_URL"), "/")
}

func iamSecret(t *testing.T) string {
	t.Helper()
	return tb.Env(t, "KEYCLOAK_IAM_ADMIN_SECRET")
}

func realmSuffix() string { return strings.ToLower(rand.Text()[:8]) }

func grantToken(t *testing.T, tokenURL string, form url.Values) string {
	t.Helper()
	resp, err := realmHTTP.PostForm(tokenURL, form)
	if err != nil {
		t.Fatalf("token endpoint err = %v: Keycloak must answer the grant", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var tokens struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil || resp.StatusCode != http.StatusOK || tokens.AccessToken == "" {
		t.Fatalf("grant of %s = HTTP %d %s, want 200 with an access token", form.Get("client_id"), resp.StatusCode, tokens.Error)
	}
	return tokens.AccessToken
}

// realmClient calls the Admin API of the dmpf realm as one identity: the admin
// of the master realm, which builds and reads the fixtures, or the iam service
// account, whose answers are the ones the adapter receives.
type realmClient struct {
	t     *testing.T
	grant func(t *testing.T) string
	token string
}

func newRealmAdmin(t *testing.T) *realmClient {
	t.Helper()
	base, user, password := keycloakBase(t), tb.Env(t, "KEYCLOAK_ADMIN_USER"), tb.Env(t, "KEYCLOAK_ADMIN_PASSWORD")
	c := &realmClient{t: t, grant: func(t *testing.T) string {
		return grantToken(t, base+"/realms/master/protocol/openid-connect/token", url.Values{
			"grant_type": {"password"}, "client_id": {"admin-cli"}, "username": {user}, "password": {password},
		})
	}}
	c.token = c.grant(t)
	return c
}

func newRealmServiceAccount(t *testing.T) *realmClient {
	t.Helper()
	base, secret := keycloakBase(t), iamSecret(t)
	c := &realmClient{t: t, grant: func(t *testing.T) string {
		return grantToken(t, base+"/realms/dmpf/protocol/openid-connect/token", url.Values{
			"grant_type": {"client_credentials"}, "client_id": {"iam-admin"}, "client_secret": {secret},
		})
	}}
	c.token = c.grant(t)
	return c
}

// newRealm is what every test starts with: the admin, with the guard of the
// operator Organization already armed.
func newRealm(t *testing.T) *realmClient {
	t.Helper()
	admin := newRealmAdmin(t)
	admin.guardOperator()
	return admin
}

func (c *realmClient) on(t *testing.T) *realmClient {
	scoped := *c
	scoped.t = t
	return &scoped
}

func (c *realmClient) send(method, resource string, body any) (int, string, []byte) {
	c.t.Helper()
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("json.Marshal(%T) err = %v", body, err)
		}
		payload = bytes.NewReader(encoded)
	}
	// WHY: cleanups run after t.Context() is canceled and must still reach the Admin API.
	ctx := context.WithoutCancel(c.t.Context())
	req, err := http.NewRequestWithContext(ctx, method, keycloakBase(c.t)+realmPath+resource, payload)
	if err != nil {
		c.t.Fatalf("http.NewRequest(%s %s) err = %v", method, resource, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := realmHTTP.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s err = %v: the Admin API must answer", method, resource, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("reading %s %s err = %v", method, resource, err)
	}
	return resp.StatusCode, resp.Header.Get("Location"), raw
}

// do renews the token once on a 401: the token of admin-cli lives one minute.
func (c *realmClient) do(method, resource string, body any) (int, string, []byte) {
	c.t.Helper()
	status, location, raw := c.send(method, resource, body)
	if status == http.StatusUnauthorized {
		c.token = c.grant(c.t)
		status, location, raw = c.send(method, resource, body)
	}
	return status, location, raw
}

func (c *realmClient) expect(method, resource string, body any, want int) []byte {
	c.t.Helper()
	status, _, raw := c.do(method, resource, body)
	if status != want {
		c.t.Fatalf("%s %s = HTTP %d %s, want %d", method, resource, status, raw, want)
	}
	return raw
}

func (c *realmClient) create(resource string, body any) string {
	c.t.Helper()
	status, location, raw := c.do(http.MethodPost, resource, body)
	if status != http.StatusCreated || location == "" {
		c.t.Fatalf("POST %s = HTTP %d %s, want 201 with the Location of the new resource", resource, status, raw)
	}
	return path.Base(location)
}

func (c *realmClient) remove(resource string) {
	c.t.Helper()
	if status, _, raw := c.do(http.MethodDelete, resource, nil); status != http.StatusNoContent && status != http.StatusNotFound {
		c.t.Errorf("DELETE %s = HTTP %d %s, want 204: the fixture outlives the test", resource, status, raw)
	}
}

func (c *realmClient) decode(raw []byte, into any, what string) {
	c.t.Helper()
	if err := json.Unmarshal(raw, into); err != nil {
		c.t.Fatalf("%s %s is not JSON: %v", what, raw, err)
	}
}

type realmEntity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type realmUser struct {
	ID              string   `json:"id"`
	Username        string   `json:"username"`
	Email           string   `json:"email"`
	Enabled         bool     `json:"enabled"`
	EmailVerified   bool     `json:"emailVerified"`
	RequiredActions []string `json:"requiredActions"`
}

func (c *realmClient) list(resource string, what string) []realmEntity {
	c.t.Helper()
	var found []realmEntity
	c.decode(c.expect(http.MethodGet, resource, nil, http.StatusOK), &found, what)
	return found
}

func namesOf(entities []realmEntity) []string {
	out := make([]string, 0, len(entities))
	for _, e := range entities {
		out = append(out, e.Name)
	}
	slices.Sort(out)
	return out
}

func idsOf(entities []realmEntity) []string {
	out := make([]string, 0, len(entities))
	for _, e := range entities {
		out = append(out, e.ID)
	}
	slices.Sort(out)
	return out
}

func (c *realmClient) organizationByTenant(tenant string) string {
	c.t.Helper()
	found := c.list("/organizations?q="+url.QueryEscape("tenant_id:"+tenant), "organizations")
	if len(found) != 1 {
		c.t.Fatalf("organizations with tenant_id %s = %v, want one", tenant, found)
	}
	return found[0].ID
}

func (c *realmClient) groups(org string) []realmEntity {
	c.t.Helper()
	return c.list("/organizations/"+org+"/groups?first=0&max=1000", "organization groups")
}

func (c *realmClient) groupNamed(org, name string) string {
	c.t.Helper()
	found := slices.DeleteFunc(c.groups(org), func(g realmEntity) bool { return g.Name != name })
	if len(found) != 1 {
		c.t.Fatalf("groups named %s of organization %s = %v, want exactly one", name, org, found)
	}
	return found[0].ID
}

func (c *realmClient) groupRoles(org, group, client string) []string {
	c.t.Helper()
	return namesOf(c.list("/organizations/"+org+"/groups/"+group+"/role-mappings/clients/"+client, "group client roles"))
}

func (c *realmClient) groupMembers(org, group string) []string {
	c.t.Helper()
	return idsOf(c.list("/organizations/"+org+"/groups/"+group+"/members?first=0&max=1000", "group members"))
}

func (c *realmClient) isMember(org, user string) bool {
	c.t.Helper()
	status, _, raw := c.do(http.MethodGet, "/organizations/"+org+"/members/"+user, nil)
	switch status {
	case http.StatusOK:
		return true
	case http.StatusNotFound:
		return false
	}
	c.t.Fatalf("GET member %s of %s = HTTP %d %s, want 200 or 404", user, org, status, raw)
	return false
}

// memberGroups is nil for a user outside the Organization, which answers 404.
func (c *realmClient) memberGroups(org, user string) []string {
	c.t.Helper()
	status, _, raw := c.do(http.MethodGet, "/organizations/"+org+"/members/"+user+"/groups?first=0&max=1000", nil)
	if status == http.StatusNotFound {
		return nil
	}
	if status != http.StatusOK {
		c.t.Fatalf("groups of member %s = HTTP %d %s, want 200 or 404", user, status, raw)
	}
	var found []realmEntity
	c.decode(raw, &found, "member groups")
	return namesOf(found)
}

func (c *realmClient) usersWithEmail(email string) []realmUser {
	c.t.Helper()
	var found []realmUser
	c.decode(c.expect(http.MethodGet, "/users?exact=true&email="+url.QueryEscape(email), nil, http.StatusOK), &found, "users")
	return found
}

func (c *realmClient) user(id string) realmUser {
	c.t.Helper()
	var found realmUser
	c.decode(c.expect(http.MethodGet, "/users/"+id, nil, http.StatusOK), &found, "user")
	return found
}

func (c *realmClient) userByUsername(username string) string {
	c.t.Helper()
	var found []realmUser
	c.decode(c.expect(http.MethodGet, "/users?exact=true&username="+url.QueryEscape(username), nil, http.StatusOK), &found, "users")
	if len(found) != 1 {
		c.t.Fatalf("users named %s = %d, want one: the marketplace seed must declare it", username, len(found))
	}
	return found[0].ID
}

func (c *realmClient) realmRoles(user string) []string {
	c.t.Helper()
	return namesOf(c.list("/users/"+user+"/role-mappings/realm", "realm role mappings"))
}

func (c *realmClient) clientUUID(clientID string) string {
	c.t.Helper()
	var found []struct{ ID string }
	c.decode(c.expect(http.MethodGet, "/clients?clientId="+url.QueryEscape(clientID), nil, http.StatusOK), &found, "clients")
	if len(found) != 1 {
		c.t.Fatalf("clients %s = %d, want one: the marketplace seed must declare it", clientID, len(found))
	}
	return found[0].ID
}

// newStore registers an Organization of the tenant, removed with its groups by
// the cleanup; owner adds the group every seeded store holds.
func (c *realmClient) newStore(tenant string, owner bool) string {
	c.t.Helper()
	id := c.create("/organizations", map[string]any{
		"name": tenant, "alias": tenant, "attributes": map[string][]string{"tenant_id": {tenant}},
	})
	c.t.Cleanup(func() { c.remove("/organizations/" + id) })
	if owner {
		c.create("/organizations/"+id+"/groups", map[string]any{"name": realmOwnerGroup})
	}
	return id
}

// forgetUsers removes at the end of the test every user holding the address,
// whoever created it.
func (c *realmClient) forgetUsers(email string) {
	c.t.Helper()
	c.t.Cleanup(func() {
		for _, user := range c.usersWithEmail(email) {
			c.remove("/users/" + user.ID)
		}
	})
}

func (c *realmClient) newUser(email string) string {
	c.t.Helper()
	c.forgetUsers(email)
	return c.create("/users", map[string]any{"username": email, "email": email, "enabled": true, "firstName": "Iam", "lastName": "Teste"})
}

// guardOperator fails the test when the Organization of the operator tenant,
// which no command may touch, does not read the same at its end.
func (c *realmClient) guardOperator() {
	c.t.Helper()
	before := c.operatorSnapshot()
	c.t.Cleanup(func() {
		if after := c.operatorSnapshot(); after != before {
			c.t.Errorf("the organization of %s changed:\nbefore %s\nafter  %s", realmOperator, before, after)
		}
	})
}

func (c *realmClient) operatorSnapshot() string {
	c.t.Helper()
	org := c.organizationByTenant(realmOperator)
	var representation map[string]any
	c.decode(c.expect(http.MethodGet, "/organizations/"+org, nil, http.StatusOK), &representation, "organization")
	snapshot, err := json.Marshal(map[string]any{
		"organization": representation,
		"members":      idsOf(c.list("/organizations/"+org+"/members?first=0&max=1000", "organization members")),
		"groups":       namesOf(c.groups(org)),
	})
	if err != nil {
		c.t.Fatalf("json.Marshal(snapshot) err = %v", err)
	}
	return string(snapshot)
}

type realmCall struct {
	method string
	path   string
	status int
}

// realmTraffic records each attempt the adapter sent and what Keycloak
// answered, and lets a test hold a request before it leaves.
type realmTraffic struct {
	next  http.RoundTripper
	hold  func(*http.Request)
	mu    sync.Mutex
	calls []realmCall
}

func (r *realmTraffic) RoundTrip(req *http.Request) (*http.Response, error) {
	if r.hold != nil {
		r.hold(req)
	}
	resp, err := r.next.RoundTrip(req)
	call := realmCall{method: req.Method, path: strings.TrimPrefix(req.URL.Path, realmPath)}
	if err == nil {
		call.status = resp.StatusCode
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, call)
	return resp, err
}

func (r *realmTraffic) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = nil
}

// writes are the changes sent to the Admin API, the grant of the token left out.
func (r *realmTraffic) writes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, call := range r.calls {
		if call.method != http.MethodGet && !strings.HasSuffix(call.path, "/protocol/openid-connect/token") {
			out = append(out, call.method+" "+call.path+" "+strconv.Itoa(call.status))
		}
	}
	return out
}

func (r *realmTraffic) statuses(method, resource string) []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []int
	for _, call := range r.calls {
		if call.method == method && call.path == resource {
			out = append(out, call.status)
		}
	}
	slices.Sort(out)
	return out
}

func newRealmKeycloak(t *testing.T, secret string, hold func(*http.Request)) (*provider.Keycloak, *realmTraffic) {
	t.Helper()
	traffic := &realmTraffic{next: http.DefaultTransport, hold: hold}
	k, err := provider.NewKeycloak(provider.KeycloakConfig{
		BaseURL:        keycloakBase(t),
		Realm:          "dmpf",
		ClientID:       "iam-admin",
		Secret:         provider.NewKeycloakSecret(secret),
		OperatorTenant: realmOperator,
	}, provider.KeycloakDeps{Clock: clock.System(), Transport: traffic})
	if err != nil {
		t.Fatalf("NewKeycloak() = %v, want nil", err)
	}
	return k, traffic
}

func requireWrites(t *testing.T, traffic *realmTraffic, want ...string) {
	t.Helper()
	if got := traffic.writes(); !slices.Equal(got, want) {
		t.Fatalf("writes = %v, want %v", got, want)
	}
}

func requireStrings(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

func TestKeycloakRealmAdminAPIAnswersAsTheAdapterExpects(t *testing.T) {
	admin := newRealm(t)
	sa := newRealmServiceAccount(t)
	tenant := "iam-it-" + realmSuffix()
	org := admin.newStore(tenant, false)

	t.Run("a group created twice is 201 with its Location, then 409 without one", func(t *testing.T) {
		admin, sa := admin.on(t), sa.on(t)
		groups := "/organizations/" + org + "/groups"

		status, location, raw := sa.do(http.MethodPost, groups, map[string]string{"name": "duplicado"})
		if status != http.StatusCreated || path.Base(location) != admin.groupNamed(org, "duplicado") {
			t.Fatalf("first POST = HTTP %d %s at %q, want 201 at the group the name finds", status, raw, location)
		}
		status, location, raw = sa.do(http.MethodPost, groups, map[string]string{"name": "duplicado"})
		if status != http.StatusConflict || location != "" {
			t.Fatalf("second POST = HTTP %d %s at %q, want 409 without Location: the loser rereads by name", status, raw, location)
		}
	})

	t.Run("a user enters a group only as a member of the Organization, once", func(t *testing.T) {
		admin, sa := admin.on(t), sa.on(t)
		user := admin.newUser("iam-it-" + realmSuffix() + "@example.com")
		membership := "/organizations/" + org + "/groups/" + admin.create("/organizations/"+org+"/groups", map[string]string{"name": "portao"}) + "/members/" + user

		sa.expect(http.MethodPut, membership, nil, http.StatusBadRequest)
		sa.expect(http.MethodPost, "/organizations/"+org+"/members", user, http.StatusCreated)
		sa.expect(http.MethodPut, membership, nil, http.StatusNoContent)
		sa.expect(http.MethodPut, membership, nil, http.StatusConflict)
	})

	t.Run("removing a user from a group it is not in is 400, from a missing group 404", func(t *testing.T) {
		admin, sa := admin.on(t), sa.on(t)
		user := admin.newUser("iam-it-" + realmSuffix() + "@example.com")
		group := admin.create("/organizations/"+org+"/groups", map[string]string{"name": "fora"})
		sa.expect(http.MethodDelete, "/organizations/"+org+"/groups/"+group+"/members/"+user, nil, http.StatusBadRequest)
		sa.expect(http.MethodPost, "/organizations/"+org+"/members", user, http.StatusCreated)

		sa.expect(http.MethodDelete, "/organizations/"+org+"/groups/"+group+"/members/"+user, nil, http.StatusBadRequest)
		sa.expect(http.MethodDelete, "/organizations/"+org+"/groups/00000000-0000-0000-0000-000000000000/members/"+user, nil, http.StatusNotFound)
	})

	t.Run("removing the Organization member takes it out of its groups", func(t *testing.T) {
		admin, sa := admin.on(t), sa.on(t)
		user := admin.newUser("iam-it-" + realmSuffix() + "@example.com")
		group := admin.create("/organizations/"+org+"/groups", map[string]string{"name": "saida"})
		sa.expect(http.MethodPost, "/organizations/"+org+"/members", user, http.StatusCreated)
		sa.expect(http.MethodPut, "/organizations/"+org+"/groups/"+group+"/members/"+user, nil, http.StatusNoContent)

		sa.expect(http.MethodDelete, "/organizations/"+org+"/members/"+user, nil, http.StatusNoContent)

		requireStrings(t, "members of the group", admin.groupMembers(org, group))
		sa.expect(http.MethodGet, "/organizations/"+org+"/members/"+user+"/groups", nil, http.StatusNotFound)
		sa.expect(http.MethodDelete, "/organizations/"+org+"/members/"+user, nil, http.StatusNotFound)
	})

	t.Run("the tenant_id search is exact", func(t *testing.T) {
		admin, sa := admin.on(t), sa.on(t)
		admin.newStore(tenant+"-b", false)

		for query, want := range map[string][]string{
			tenant:                  {org},
			tenant[:len(tenant)-1]:  nil,
			tenant + "-":            nil,
			tenant[:len("iam-it-")]: nil,
		} {
			found := idsOf(sa.list("/organizations?briefRepresentation=false&q="+url.QueryEscape("tenant_id:"+query), "organizations"))
			requireStrings(t, "organizations of tenant_id:"+query, found, want...)
		}
	})

	t.Run("the groups of a member page by first and max", func(t *testing.T) {
		admin, sa := admin.on(t), sa.on(t)
		user := admin.newUser("iam-it-" + realmSuffix() + "@example.com")
		sa.expect(http.MethodPost, "/organizations/"+org+"/members", user, http.StatusCreated)
		var all []string
		for _, name := range []string{"pagina-a", "pagina-b", "pagina-c"} {
			group := admin.create("/organizations/"+org+"/groups", map[string]string{"name": name})
			sa.expect(http.MethodPut, "/organizations/"+org+"/groups/"+group+"/members/"+user, nil, http.StatusNoContent)
			all = append(all, group)
		}
		slices.Sort(all)

		var seen []string
		for first, want := range map[int]int{0: 2, 2: 1, 4: 0} {
			page := sa.list("/organizations/"+org+"/members/"+user+"/groups?briefRepresentation=true&max=2&first="+strconv.Itoa(first), "member groups")
			if len(page) != want {
				t.Fatalf("page at first=%d = %v, want %d groups", first, page, want)
			}
			seen = append(seen, idsOf(page)...)
		}
		slices.Sort(seen)
		requireStrings(t, "groups over the pages", seen, all...)
	})
}

func TestKeycloakRealmSyncRoleConvergesTheGroupOfARole(t *testing.T) {
	admin := newRealm(t)
	tenant := "iam-it-" + realmSuffix()
	org := admin.newStore(tenant, true)
	store := admin.clientUUID(realmStoreClient)
	k, traffic := newRealmKeycloak(t, iamSecret(t), nil)

	steps := []struct {
		name        string
		status      domain.RoleStatus
		permissions []string
		idempotent  bool
	}{
		{"active", domain.RoleStatusActive, []string{"catalog:read", "catalog:write"}, false},
		{"permissions changed", domain.RoleStatusActive, []string{"catalog:read", "inventory:read"}, false},
		{"reapplied", domain.RoleStatusActive, []string{"catalog:read", "inventory:read"}, true},
		{"retired", domain.RoleStatusRetired, nil, false},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			admin := admin.on(t)
			traffic.reset()

			class, _, err := submit(t, inTenant(t, tenant), k.SyncRole(), rolePayload(t, "catalogo", step.status, step.permissions...))

			requireClass(t, class, err, ports.ClassAccepted)
			requireStrings(t, "client roles of catalogo", admin.groupRoles(org, admin.groupNamed(org, "catalogo"), store), step.permissions...)
			requireStrings(t, "client roles of owner", admin.groupRoles(org, admin.groupNamed(org, realmOwnerGroup), store))
			if step.idempotent {
				requireWrites(t, traffic)
			}
		})
	}
}

func TestKeycloakRealmSyncRoleActsOnlyOnTheOrganizationOfItsExactTenant(t *testing.T) {
	admin := newRealm(t)
	tenant := "iam-it-" + realmSuffix()
	org := admin.newStore(tenant, false)
	shadow := admin.newStore(tenant+"-b", false)
	k, traffic := newRealmKeycloak(t, iamSecret(t), nil)

	class, _, err := submit(t, inTenant(t, tenant), k.SyncRole(), rolePayload(t, "catalogo", domain.RoleStatusActive, "catalog:read"))

	requireClass(t, class, err, ports.ClassAccepted)
	requireStrings(t, "groups of the tenant", namesOf(admin.groups(org)), "catalogo")
	requireStrings(t, "groups of the tenant it prefixes", namesOf(admin.groups(shadow)))

	traffic.reset()
	class, _, err = submit(t, inTenant(t, tenant[:len(tenant)-1]), k.SyncRole(), rolePayload(t, "catalogo", domain.RoleStatusActive, "catalog:read"))

	requireClass(t, class, err, ports.ClassFinalBusiness)
	requireWrites(t, traffic)
}

// realmBarrier releases the requests it holds only once n arrived, so each
// racer has read the group absent before either creates it.
type realmBarrier struct {
	n       int
	mu      sync.Mutex
	arrived int
	open    chan struct{}
}

func (b *realmBarrier) wait() {
	b.mu.Lock()
	b.arrived++
	if b.arrived == b.n {
		close(b.open)
	}
	b.mu.Unlock()
	select {
	case <-b.open:
	case <-time.After(realmHold):
	}
}

func TestKeycloakRealmSyncRoleAndMemberAccessRacingForAGroupLeaveOne(t *testing.T) {
	admin := newRealm(t)
	tenant := "iam-it-" + realmSuffix()
	org := admin.newStore(tenant, true)
	store := admin.clientUUID(realmStoreClient)
	member := admin.newUser(tenant + "@example.com")
	groups := "/organizations/" + org + "/groups"
	race := &realmBarrier{n: 2, open: make(chan struct{})}
	k, traffic := newRealmKeycloak(t, iamSecret(t), func(r *http.Request) {
		if r.Method == http.MethodPost && strings.TrimPrefix(r.URL.Path, realmPath) == groups {
			race.wait()
		}
	})
	ctx := inTenant(t, tenant)
	racers := []struct {
		command ports.ExternalCommand
		payload []byte
	}{
		{k.SyncRole(), rolePayload(t, "expedicao", domain.RoleStatusActive, "fulfillment:read")},
		{k.MemberAccess(), memberPayload(t, member, domain.MembershipStatusActive, "expedicao")},
	}
	classes, errs := make([]ports.ExternalClass, len(racers)), make([]error, len(racers))

	var wg sync.WaitGroup
	for i, racer := range racers {
		wg.Go(func() { classes[i], _, errs[i] = racer.command.Submit(ctx, realmKey, racer.payload) })
	}
	wg.Wait()

	for i := range racers {
		requireClass(t, classes[i], errs[i], ports.ClassAccepted)
	}
	if created := traffic.statuses(http.MethodPost, groups); !slices.Equal(created, []int{http.StatusCreated, http.StatusConflict}) {
		t.Fatalf("group creations answered %v, want one 201 and one 409: both racers must have created it", created)
	}
	requireStrings(t, "client roles of expedicao", admin.groupRoles(org, admin.groupNamed(org, "expedicao"), store), "fulfillment:read")
	requireStrings(t, "groups of the member", admin.memberGroups(org, member), "expedicao")
}

func TestKeycloakRealmProvisionMemberCreatesTheUserTheFirstSignInCompletes(t *testing.T) {
	admin := newRealm(t)
	tenant := "iam-it-" + realmSuffix()
	org := admin.newStore(tenant, true)
	email := tenant + "@example.com"
	admin.forgetUsers(email)
	k, traffic := newRealmKeycloak(t, iamSecret(t), nil)

	class, observation, err := submit(t, inTenant(t, tenant), k.ProvisionMember(), provisionPayload(t, email))

	requireClass(t, class, err, ports.ClassAccepted)
	users := admin.usersWithEmail(email)
	if len(users) != 1 || users[0].ID != observation.ExternalRef {
		t.Fatalf("users of %s = %+v, want only the one of ExternalRef %q", email, users, observation.ExternalRef)
	}
	created := admin.user(observation.ExternalRef)
	if created.Username != email || !created.Enabled || created.EmailVerified {
		t.Fatalf("user = %+v, want enabled, named by the address and with the address unverified", created)
	}
	requireStrings(t, "required actions", slices.Sorted(slices.Values(created.RequiredActions)), "UPDATE_PASSWORD", "VERIFY_EMAIL")
	if !admin.isMember(org, observation.ExternalRef) {
		t.Fatal("the user is not a member of the store")
	}

	traffic.reset()
	class, again, err := submit(t, inTenant(t, tenant), k.ProvisionMember(), provisionPayload(t, email))

	requireClass(t, class, err, ports.ClassAccepted)
	if again.ExternalRef != observation.ExternalRef || len(admin.usersWithEmail(email)) != 1 {
		t.Fatalf("resent ExternalRef = %q, want %q and still one user", again.ExternalRef, observation.ExternalRef)
	}
	requireWrites(t, traffic, "POST /organizations/"+org+"/members 409")
}

func TestKeycloakRealmProvisionMemberReusesTheUserOfTheAddress(t *testing.T) {
	admin := newRealm(t)
	tenant := "iam-it-" + realmSuffix()
	org := admin.newStore(tenant, true)
	email := tenant + "@example.com"
	existing := admin.newUser(email)
	k, traffic := newRealmKeycloak(t, iamSecret(t), nil)

	class, observation, err := submit(t, inTenant(t, tenant), k.ProvisionMember(), provisionPayload(t, email))

	requireClass(t, class, err, ports.ClassAccepted)
	if observation.ExternalRef != existing || len(admin.usersWithEmail(email)) != 1 {
		t.Fatalf("ExternalRef = %q, want the existing %q and no second user", observation.ExternalRef, existing)
	}
	requireStrings(t, "required actions of the existing user", admin.user(existing).RequiredActions)
	if !admin.isMember(org, existing) {
		t.Fatal("the user is not a member of the store")
	}
	requireWrites(t, traffic, "POST /organizations/"+org+"/members 201")
}

func TestKeycloakRealmProvisionMemberOfATenantWithoutOrganizationIsAFinalBusiness(t *testing.T) {
	admin := newRealm(t)
	tenant := "iam-it-" + realmSuffix()
	email := tenant + "@example.com"
	admin.forgetUsers(email)
	k, traffic := newRealmKeycloak(t, iamSecret(t), nil)

	class, _, err := submit(t, inTenant(t, tenant), k.ProvisionMember(), provisionPayload(t, email))

	requireClass(t, class, err, ports.ClassFinalBusiness)
	requireWrites(t, traffic)
	if users := admin.usersWithEmail(email); len(users) != 0 {
		t.Fatalf("users of %s = %+v, want none", email, users)
	}
}

func TestKeycloakRealmMemberAccessConvergesTheMembership(t *testing.T) {
	admin := newRealm(t)
	tenant := "iam-it-" + realmSuffix()
	org := admin.newStore(tenant, true)
	store := admin.clientUUID(realmStoreClient)
	member := admin.newUser(tenant + "@example.com")
	k, traffic := newRealmKeycloak(t, iamSecret(t), nil)

	steps := []struct {
		name       string
		status     domain.MembershipStatus
		roles      []string
		groups     []string
		idempotent bool
	}{
		{"active", domain.MembershipStatusActive, []string{realmOwnerGroup, "catalogo"}, []string{"catalogo", realmOwnerGroup}, false},
		{"roles changed", domain.MembershipStatusActive, []string{"expedicao"}, []string{"expedicao"}, false},
		{"removed", domain.MembershipStatusRemoved, nil, nil, false},
		{"removed again", domain.MembershipStatusRemoved, nil, nil, true},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			admin := admin.on(t)
			traffic.reset()

			class, _, err := submit(t, inTenant(t, tenant), k.MemberAccess(), memberPayload(t, member, step.status, step.roles...))

			requireClass(t, class, err, ports.ClassAccepted)
			if in, want := admin.isMember(org, member), step.status == domain.MembershipStatusActive; in != want {
				t.Fatalf("member of the store = %v, want %v", in, want)
			}
			requireStrings(t, "groups of the member", admin.memberGroups(org, member), step.groups...)
			for _, group := range admin.groups(org) {
				if in, want := slices.Contains(admin.groupMembers(org, group.ID), member), slices.Contains(step.groups, group.Name); in != want {
					t.Fatalf("member in group %s = %v, want %v", group.Name, in, want)
				}
			}
			if step.idempotent {
				requireWrites(t, traffic)
			}
		})
	}
	requireStrings(t, "client roles the command created", admin.groupRoles(org, admin.groupNamed(org, "catalogo"), store))
}

func TestKeycloakRealmMemberAccessWithoutTheOwnerGroupWritesNothing(t *testing.T) {
	admin := newRealm(t)
	tenant := "iam-it-" + realmSuffix()
	org := admin.newStore(tenant, false)
	member := admin.newUser(tenant + "@example.com")
	k, traffic := newRealmKeycloak(t, iamSecret(t), nil)

	class, _, err := submit(t, inTenant(t, tenant), k.MemberAccess(), memberPayload(t, member, domain.MembershipStatusActive, realmOwnerGroup, "catalogo"))

	requireClass(t, class, err, ports.ClassFinalBusiness)
	requireWrites(t, traffic)
	requireStrings(t, "groups of the store", namesOf(admin.groups(org)))
	if admin.isMember(org, member) {
		t.Fatal("the user entered the store")
	}
}

func TestKeycloakRealmBuyerAccessGrantsAndTakesTheBuyerRole(t *testing.T) {
	admin := newRealm(t)
	buyer := admin.newUser("iam-it-" + realmSuffix() + "@example.com")
	k, traffic := newRealmKeycloak(t, iamSecret(t), nil)

	steps := []struct {
		name       string
		status     domain.BuyerStatus
		idempotent bool
	}{
		{"active", domain.BuyerStatusActive, false},
		{"active again", domain.BuyerStatusActive, true},
		{"blocked", domain.BuyerStatusBlocked, false},
		{"blocked again", domain.BuyerStatusBlocked, true},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			admin := admin.on(t)
			traffic.reset()

			class, _, err := submit(t, inTenant(t, realmOperator), k.BuyerAccess(), buyerPayload(t, buyer, step.status))

			requireClass(t, class, err, ports.ClassAccepted)
			if holds, want := slices.Contains(admin.realmRoles(buyer), realmBuyerRole), step.status == domain.BuyerStatusActive; holds != want {
				t.Fatalf("holds %s = %v, want %v", realmBuyerRole, holds, want)
			}
			if !admin.user(buyer).Enabled {
				t.Fatal("the buyer was disabled")
			}
			if step.idempotent {
				requireWrites(t, traffic)
			}
		})
	}
}

func TestKeycloakRealmFGAPKeepsTheOperatorOutOfAStore(t *testing.T) {
	admin := newRealm(t)
	tenant := "iam-it-" + realmSuffix()
	org := admin.newStore(tenant, true)
	operator := admin.userByUsername(realmOperatorUser)
	email := admin.user(operator).Email
	if email == "" {
		t.Fatalf("%s has no address: the marketplace seed must declare it", realmOperatorUser)
	}
	k, _ := newRealmKeycloak(t, iamSecret(t), nil)

	t.Run("ProvisionMember", func(t *testing.T) {
		class, _, err := submit(t, inTenant(t, tenant), k.ProvisionMember(), provisionPayload(t, email))

		requireClass(t, class, err, ports.ClassFinalAuthorization)
		if admin.on(t).isMember(org, operator) {
			t.Fatal("the operator entered the store")
		}
	})

	t.Run("MemberAccess", func(t *testing.T) {
		admin := admin.on(t)

		class, _, err := submit(t, inTenant(t, tenant), k.MemberAccess(), memberPayload(t, operator, domain.MembershipStatusActive, realmOwnerGroup))

		requireClass(t, class, err, ports.ClassFinalAuthorization)
		if admin.isMember(org, operator) || slices.Contains(admin.groupMembers(org, admin.groupNamed(org, realmOwnerGroup)), operator) {
			t.Fatal("the operator entered the store or its owner group")
		}
	})
}

func TestKeycloakRealmFGAPKeepsTheSessionEventsAccountOutOfReach(t *testing.T) {
	admin := newRealm(t)
	tenant := "iam-it-" + realmSuffix()
	org := admin.newStore(tenant, true)
	account := admin.userByUsername(realmSessionEvents)
	k, _ := newRealmKeycloak(t, iamSecret(t), nil)

	t.Run("MemberAccess", func(t *testing.T) {
		class, _, err := submit(t, inTenant(t, tenant), k.MemberAccess(), memberPayload(t, account, domain.MembershipStatusActive, realmOwnerGroup))

		requireClass(t, class, err, ports.ClassFinalAuthorization)
		if admin.on(t).isMember(org, account) {
			t.Fatal("the service account entered the store")
		}
	})

	t.Run("BuyerAccess", func(t *testing.T) {
		class, _, err := submit(t, inTenant(t, realmOperator), k.BuyerAccess(), buyerPayload(t, account, domain.BuyerStatusActive))

		requireClass(t, class, err, ports.ClassFinalAuthorization)
		if slices.Contains(admin.on(t).realmRoles(account), realmBuyerRole) {
			t.Fatalf("the service account holds %s", realmBuyerRole)
		}
	})
}

func TestKeycloakRealmTokenOfTheServiceAccount(t *testing.T) {
	admin := newRealm(t)
	buyer := admin.newUser("iam-it-" + realmSuffix() + "@example.com")

	t.Run("the real secret", func(t *testing.T) {
		k, traffic := newRealmKeycloak(t, iamSecret(t), nil)

		class, _, err := submit(t, inTenant(t, realmOperator), k.BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusBlocked))

		requireClass(t, class, err, ports.ClassAccepted)
		if granted := traffic.statuses(http.MethodPost, "/realms/dmpf/protocol/openid-connect/token"); !slices.Equal(granted, []int{http.StatusOK}) {
			t.Fatalf("token grants answered %v, want one 200", granted)
		}
	})

	t.Run("a wrong secret", func(t *testing.T) {
		wrong := "wrong-" + rand.Text()
		k, traffic := newRealmKeycloak(t, wrong, nil)

		class, _, err := submit(t, inTenant(t, realmOperator), k.BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))

		requireClass(t, class, err, ports.ClassFinalAuthorization)
		if message := err.Error(); strings.Contains(message, wrong) || strings.Contains(message, iamSecret(t)) {
			t.Fatalf("err = %q carries a secret", message)
		}
		if granted := traffic.statuses(http.MethodPost, "/realms/dmpf/protocol/openid-connect/token"); !slices.Equal(granted, []int{http.StatusUnauthorized}) {
			t.Fatalf("token grants answered %v, want one 401", granted)
		}
		requireWrites(t, traffic)
		if slices.Contains(admin.on(t).realmRoles(buyer), realmBuyerRole) {
			t.Fatalf("the buyer holds %s granted without a valid token", realmBuyerRole)
		}
	})
}
