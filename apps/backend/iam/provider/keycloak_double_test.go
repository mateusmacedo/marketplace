package provider_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/retry"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/execctx"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

const (
	doubleRealm      = "dmpf"
	doubleClientID   = "iam-admin"
	doubleStore      = "marketplace-store"
	doubleBuyerRole  = "iam:buyer"
	storeTenant      = "loja-a"
	doubleStoreUUID  = "client-marketplace-store"
	doubleBuyerID    = "role-iam-buyer"
	doubleTokenLife  = 300
	doubleTokenRoute = "token"
)

// keycloakDouble is a stand-in for the HTTP of Keycloak, not for the adapter:
// it keeps the realm state the Admin API changes, so a resend shows whether it
// converges, and it answers each request under the route name the adapter
// declares, so faults and call order read in the adapter's own terms.
type keycloakDouble struct {
	t      *testing.T
	server *httptest.Server

	mu            sync.Mutex
	secret        string
	tokenLifetime int
	tokens        map[string]bool
	grants        int
	ids           int
	orgs          map[string]*doubleOrg
	users         map[string]*doubleUser
	clientRoles   []doubleRole
	userRealm     map[string]map[string]bool
	faults        []*doubleFault
	calls         []string
	grantHold     chan struct{}
	grantEntered  chan struct{}
}

type doubleOrg struct {
	id      string
	tenant  string
	groups  map[string]*doubleGroup
	members map[string]bool
}

type doubleGroup struct {
	id      string
	name    string
	roles   map[string]bool
	members map[string]bool
}

type doubleUser struct {
	ID              string   `json:"id"`
	Username        string   `json:"username"`
	Email           string   `json:"email"`
	Enabled         bool     `json:"enabled"`
	RequiredActions []string `json:"requiredActions"`
}

type doubleRole struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type doubleFault struct {
	route  string
	status int
	times  int
	after  bool
	body   string
	broken connectionFault
}

// connectionFault is a failure of the connection itself, after the request
// arrived: no answer at all, or an answer cut in the middle of its body.
type connectionFault int

const (
	connectionKept connectionFault = iota
	connectionDropped
	connectionTruncated
)

type doubleReply struct {
	status   int
	body     any
	location string
}

func newKeycloakDouble(t *testing.T) *keycloakDouble {
	t.Helper()
	d := &keycloakDouble{
		t:             t,
		secret:        testSecret,
		tokenLifetime: doubleTokenLife,
		tokens:        map[string]bool{},
		orgs:          map[string]*doubleOrg{},
		users:         map[string]*doubleUser{},
		userRealm:     map[string]map[string]bool{},
	}
	for _, permission := range domain.Catalog() {
		d.clientRoles = append(d.clientRoles, doubleRole{ID: "role-" + string(permission), Name: string(permission)})
	}
	owner := d.addOrganization(storeTenant)
	d.addGroup(owner, string(domain.OwnerRole), names(domain.Catalog())...)
	d.addOrganization(testOperator)
	d.server = httptest.NewServer(d)
	t.Cleanup(d.server.Close)
	return d
}

func names(permissions []domain.Permission) []string {
	out := make([]string, 0, len(permissions))
	for _, p := range permissions {
		out = append(out, string(p))
	}
	return out
}

func (d *keycloakDouble) keycloak(t *testing.T) *provider.Keycloak {
	t.Helper()
	return d.keycloakWith(t, provider.KeycloakDeps{})
}

// keycloakWith is the adapter over the double with deps, the clock and the
// jitter filled in when deps leaves them out.
func (d *keycloakDouble) keycloakWith(t *testing.T, deps provider.KeycloakDeps) *provider.Keycloak {
	t.Helper()
	if deps.Clock == nil {
		deps.Clock = clock.System()
	}
	if deps.Rand == nil {
		deps.Rand = func() float64 { return 0 }
	}
	k, err := provider.NewKeycloak(validKeycloakConfig(d.server.URL), deps)
	if err != nil {
		t.Fatalf("NewKeycloak() = %v, want nil", err)
	}
	return k
}

func (d *keycloakDouble) nextID(kind string) string {
	d.ids++
	return fmt.Sprintf("%s-%d", kind, d.ids)
}

func (d *keycloakDouble) addOrganization(tenant string) *doubleOrg {
	d.mu.Lock()
	defer d.mu.Unlock()
	org := &doubleOrg{id: d.nextID("org"), tenant: tenant, groups: map[string]*doubleGroup{}, members: map[string]bool{}}
	d.orgs[org.id] = org
	return org
}

func (d *keycloakDouble) addGroup(org *doubleOrg, name string, roles ...string) *doubleGroup {
	d.mu.Lock()
	defer d.mu.Unlock()
	group := &doubleGroup{id: d.nextID("group"), name: name, roles: map[string]bool{}, members: map[string]bool{}}
	for _, role := range roles {
		group.roles[role] = true
	}
	org.groups[group.id] = group
	return group
}

func (d *keycloakDouble) addUser(email string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	user := &doubleUser{ID: d.nextID("user"), Username: email, Email: email, Enabled: true}
	d.users[user.ID] = user
	return user.ID
}

func (d *keycloakDouble) setTokenLifetime(seconds int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tokenLifetime = seconds
}

func (d *keycloakDouble) setSecret(secret string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.secret = secret
}

func (d *keycloakDouble) addClientRole(id, name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.clientRoles = append(d.clientRoles, doubleRole{ID: id, Name: name})
}

func (d *keycloakDouble) renameUser(id, username string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.users[id].Username = username
}

// join makes the user a member of the organization and of each group.
func (d *keycloakDouble) join(org *doubleOrg, user string, groups ...*doubleGroup) {
	d.mu.Lock()
	defer d.mu.Unlock()
	org.members[user] = true
	for _, group := range groups {
		group.members[user] = true
	}
}

func (d *keycloakDouble) grantBuyer(user string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.userRealm[user] = map[string]bool{doubleBuyerRole: true}
}

// fail answers the route with status the next times calls; after applies the
// effect first, the way a write the partner committed before failing looks.
func (d *keycloakDouble) fail(route string, status, times int, after bool) {
	d.failWith(route, status, times, after, "")
}

func (d *keycloakDouble) failWith(route string, status, times int, after bool, body string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.faults = append(d.faults, &doubleFault{route: route, status: status, times: times, after: after, body: body})
}

// breakConnection fails the route the next times calls on the connection,
// before the effect, once the whole request was read.
func (d *keycloakDouble) breakConnection(route string, broken connectionFault, times int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.faults = append(d.faults, &doubleFault{route: route, times: times, broken: broken})
}

// holdGrants keeps every grant of the token waiting until release; entered
// receives once per grant that reached the double.
func (d *keycloakDouble) holdGrants(t *testing.T) (entered <-chan struct{}, release func()) {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.grantHold, d.grantEntered = make(chan struct{}), make(chan struct{}, 16)
	hold := d.grantHold
	var once sync.Once
	release = func() { once.Do(func() { close(hold) }) }
	t.Cleanup(release)
	return d.grantEntered, release
}

func (d *keycloakDouble) waitForGrant(r *http.Request) {
	if r.URL.Path != "/realms/"+doubleRealm+"/protocol/openid-connect/token" {
		return
	}
	d.mu.Lock()
	hold, entered := d.grantHold, d.grantEntered
	d.mu.Unlock()
	if hold == nil {
		return
	}
	entered <- struct{}{}
	<-hold
}

func (d *keycloakDouble) revokeTokens() {
	d.mu.Lock()
	defer d.mu.Unlock()
	clear(d.tokens)
}

func (d *keycloakDouble) callsTo(route string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, call := range d.calls {
		if call == route {
			n++
		}
	}
	return n
}

func (d *keycloakDouble) adminCalls() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := 0
	for _, call := range d.calls {
		if call != doubleTokenRoute {
			n++
		}
	}
	return n
}

// mutations is the order of the writes the adapter sent, reads left out.
func (d *keycloakDouble) mutations() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []string
	for _, call := range d.calls {
		if !strings.HasPrefix(call, "find_") && !strings.HasPrefix(call, "list_") && !strings.HasPrefix(call, "read_") && call != doubleTokenRoute {
			out = append(out, call)
		}
	}
	return out
}

func (d *keycloakDouble) resetCalls() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = nil
}

func (d *keycloakDouble) organizationOf(tenant string) *doubleOrg {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, org := range d.orgs {
		if org.tenant == tenant {
			return org
		}
	}
	d.t.Fatalf("the double holds no organization of %s", tenant)
	return nil
}

func (d *keycloakDouble) groupNamed(org *doubleOrg, name string) (*doubleGroup, int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var found *doubleGroup
	count := 0
	for _, group := range org.groups {
		if group.name == name {
			found = group
			count++
		}
	}
	return found, count
}

func (d *keycloakDouble) rolesOf(group *doubleGroup) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return sortedKeys(group.roles)
}

func (d *keycloakDouble) groupsOfMember(org *doubleOrg, user string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []string
	for _, group := range org.groups {
		if group.members[user] {
			out = append(out, group.name)
		}
	}
	slices.Sort(out)
	return out
}

func (d *keycloakDouble) isMember(org *doubleOrg, user string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return org.members[user]
}

func (d *keycloakDouble) usersWithEmail(email string) []doubleUser {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []doubleUser
	for _, user := range d.users {
		if user.Email == email {
			out = append(out, *user)
		}
	}
	return out
}

func (d *keycloakDouble) hasBuyerRole(user string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.userRealm[user][doubleBuyerRole]
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}

func (d *keycloakDouble) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	d.waitForGrant(r)
	d.mu.Lock()
	defer d.mu.Unlock()

	route, handle := d.route(r)
	d.calls = append(d.calls, route)
	if fault := d.takeFault(route, false); fault != nil {
		writeFault(w, fault)
		return
	}
	if route != doubleTokenRoute && !d.tokens[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")] {
		writeJSON(w, doubleReply{status: http.StatusUnauthorized, body: map[string]string{"error": "HTTP 401 Unauthorized"}})
		return
	}
	reply := handle(r, body)
	if fault := d.takeFault(route, true); fault != nil {
		writeFault(w, fault)
		return
	}
	writeJSON(w, reply)
}

func (d *keycloakDouble) takeFault(route string, after bool) *doubleFault {
	for _, fault := range d.faults {
		if fault.route == route && fault.after == after && fault.times > 0 {
			fault.times--
			return fault
		}
	}
	return nil
}

func writeFault(w http.ResponseWriter, fault *doubleFault) {
	if fault.broken != connectionKept {
		breakOff(w, fault.broken)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(fault.status)
	_, _ = io.WriteString(w, fault.body)
}

// breakOff takes the connection from the server and closes it, after the
// head and part of the body when truncated: what the client reads is cut.
func breakOff(w http.ResponseWriter, broken connectionFault) {
	conn, buf, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	if broken == connectionTruncated {
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 64\r\n\r\n[{\"id\":")
		_ = buf.Flush()
	}
}

func writeJSON(w http.ResponseWriter, reply doubleReply) {
	if reply.location != "" {
		w.Header().Set("Location", reply.location)
	}
	if reply.body == nil {
		w.WriteHeader(reply.status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(reply.status)
	_ = json.NewEncoder(w).Encode(reply.body)
}

type doubleHandler func(r *http.Request, body []byte) doubleReply

// route maps a request to the route name the adapter declares, and to the
// handler that changes or reads the realm state.
func (d *keycloakDouble) route(r *http.Request) (string, doubleHandler) {
	if r.URL.Path == "/realms/"+doubleRealm+"/protocol/openid-connect/token" && r.Method == http.MethodPost {
		return doubleTokenRoute, d.token
	}
	rest, admin := strings.CutPrefix(r.URL.Path, "/admin/realms/"+doubleRealm+"/")
	if !admin {
		return "unrouted", d.unrouted
	}
	s := strings.Split(rest, "/")
	method := r.Method
	switch {
	case len(s) == 1 && s[0] == "organizations" && method == http.MethodGet:
		return "find_organizations", d.findOrganizations
	case len(s) == 1 && s[0] == "clients" && method == http.MethodGet:
		return "find_clients", d.findClients
	case len(s) == 3 && s[0] == "clients" && s[2] == "roles" && method == http.MethodGet:
		return "list_client_roles", d.withClient(s[1], d.listClientRoles)
	case len(s) == 3 && s[0] == "organizations" && s[2] == "groups" && method == http.MethodGet:
		return "find_groups", d.withOrg(s[1], d.findGroups)
	case len(s) == 3 && s[0] == "organizations" && s[2] == "groups" && method == http.MethodPost:
		return "create_group", d.withOrg(s[1], d.createGroup)
	case len(s) == 7 && s[0] == "organizations" && s[2] == "groups" && s[4] == "role-mappings" && s[5] == "clients":
		return map[string]string{http.MethodGet: "read_group_roles", http.MethodPost: "add_group_roles", http.MethodDelete: "remove_group_roles"}[method],
			d.withGroup(s[1], s[3], func(org *doubleOrg, group *doubleGroup, r *http.Request, body []byte) doubleReply {
				return d.groupRoles(group, s[6], r, body)
			})
	case len(s) == 6 && s[0] == "organizations" && s[2] == "groups" && s[4] == "members":
		return map[string]string{http.MethodPut: "add_group_member", http.MethodDelete: "remove_group_member"}[method],
			d.withGroup(s[1], s[3], func(org *doubleOrg, group *doubleGroup, r *http.Request, _ []byte) doubleReply {
				return d.groupMember(org, group, s[5], r)
			})
	case len(s) == 1 && s[0] == "users" && method == http.MethodGet:
		return "find_users", d.findUsers
	case len(s) == 1 && s[0] == "users" && method == http.MethodPost:
		return "create_user", d.createUser
	case len(s) == 3 && s[0] == "organizations" && s[2] == "members" && method == http.MethodPost:
		return "add_member", d.withOrg(s[1], d.addMember)
	case len(s) == 4 && s[0] == "organizations" && s[2] == "members":
		return map[string]string{http.MethodGet: "read_member", http.MethodDelete: "remove_member"}[method],
			d.withOrg(s[1], func(org *doubleOrg, r *http.Request, _ []byte) doubleReply { return d.member(org, s[3], r) })
	case len(s) == 5 && s[0] == "organizations" && s[2] == "members" && s[4] == "groups" && method == http.MethodGet:
		return "list_member_groups", d.withOrg(s[1], func(org *doubleOrg, r *http.Request, _ []byte) doubleReply {
			return d.memberGroups(org, s[3], r)
		})
	case len(s) == 2 && s[0] == "roles" && method == http.MethodGet:
		return "read_realm_role", func(*http.Request, []byte) doubleReply { return d.realmRole(s[1]) }
	case len(s) == 4 && s[0] == "users" && s[2] == "role-mappings" && s[3] == "realm":
		return map[string]string{http.MethodGet: "read_user_realm_roles", http.MethodPost: "add_user_realm_roles", http.MethodDelete: "remove_user_realm_roles"}[method],
			func(r *http.Request, body []byte) doubleReply { return d.userRealmRoles(s[1], r, body) }
	default:
		return "unrouted", d.unrouted
	}
}

func (d *keycloakDouble) unrouted(r *http.Request, _ []byte) doubleReply {
	d.t.Errorf("the adapter sent %s %s, which the Admin API the spec uses does not hold", r.Method, r.URL.Path)
	return doubleReply{status: http.StatusNotFound}
}

func (d *keycloakDouble) token(r *http.Request, body []byte) doubleReply {
	form, err := url.ParseQuery(string(body))
	switch {
	case err != nil || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded":
		return doubleReply{status: http.StatusBadRequest, body: map[string]string{"error": "invalid_request"}}
	case form.Get("grant_type") != "client_credentials":
		return doubleReply{status: http.StatusBadRequest, body: map[string]string{"error": "unsupported_grant_type"}}
	case form.Get("client_id") != doubleClientID || form.Get("client_secret") != d.secret:
		return doubleReply{status: http.StatusUnauthorized, body: map[string]string{"error": "invalid_client", "error_description": "Invalid client or Invalid client credentials"}}
	}
	d.grants++
	token := fmt.Sprintf("access-token-%d", d.grants)
	d.tokens[token] = true
	return doubleReply{status: http.StatusOK, body: map[string]any{"access_token": token, "expires_in": d.tokenLifetime, "token_type": "Bearer"}}
}

func page[T any](items []T, r *http.Request) []T {
	first, _ := strconv.Atoi(r.URL.Query().Get("first"))
	first = min(max(first, 0), len(items))
	items = items[first:]
	if limit, err := strconv.Atoi(r.URL.Query().Get("max")); err == nil && limit >= 0 && limit < len(items) {
		items = items[:limit]
	}
	return items
}

// findOrganizations matches the tenant by prefix, wider than the attribute
// search of Keycloak, so only an exact filter in the adapter picks one.
func (d *keycloakDouble) findOrganizations(r *http.Request, _ []byte) doubleReply {
	tenant, _ := strings.CutPrefix(r.URL.Query().Get("q"), "tenant_id:")
	found := []map[string]any{}
	for _, org := range d.sortedOrgs() {
		if strings.HasPrefix(org.tenant, tenant) {
			found = append(found, map[string]any{"id": org.id, "name": org.tenant, "alias": org.tenant, "attributes": map[string][]string{"tenant_id": {org.tenant}}})
		}
	}
	return doubleReply{status: http.StatusOK, body: page(found, r)}
}

func (d *keycloakDouble) sortedOrgs() []*doubleOrg {
	out := make([]*doubleOrg, 0, len(d.orgs))
	for _, org := range d.orgs {
		out = append(out, org)
	}
	slices.SortFunc(out, func(a, b *doubleOrg) int { return strings.Compare(a.id, b.id) })
	return out
}

func (d *keycloakDouble) findClients(r *http.Request, _ []byte) doubleReply {
	found := []map[string]string{}
	if r.URL.Query().Get("clientId") == doubleStore {
		found = append(found, map[string]string{"id": doubleStoreUUID, "clientId": doubleStore})
	}
	return doubleReply{status: http.StatusOK, body: found}
}

func (d *keycloakDouble) withClient(id string, next doubleHandler) doubleHandler {
	return func(r *http.Request, body []byte) doubleReply {
		if id != doubleStoreUUID {
			return doubleReply{status: http.StatusNotFound}
		}
		return next(r, body)
	}
}

func (d *keycloakDouble) listClientRoles(r *http.Request, _ []byte) doubleReply {
	return doubleReply{status: http.StatusOK, body: page(d.clientRoles, r)}
}

func (d *keycloakDouble) withOrg(id string, next func(*doubleOrg, *http.Request, []byte) doubleReply) doubleHandler {
	return func(r *http.Request, body []byte) doubleReply {
		org, found := d.orgs[id]
		if !found {
			return doubleReply{status: http.StatusNotFound}
		}
		return next(org, r, body)
	}
}

func (d *keycloakDouble) withGroup(orgID, groupID string, next func(*doubleOrg, *doubleGroup, *http.Request, []byte) doubleReply) doubleHandler {
	return d.withOrg(orgID, func(org *doubleOrg, r *http.Request, body []byte) doubleReply {
		group, found := org.groups[groupID]
		if !found {
			return doubleReply{status: http.StatusNotFound}
		}
		return next(org, group, r, body)
	})
}

func (d *keycloakDouble) sortedGroups(org *doubleOrg) []*doubleGroup {
	out := make([]*doubleGroup, 0, len(org.groups))
	for _, group := range org.groups {
		out = append(out, group)
	}
	slices.SortFunc(out, func(a, b *doubleGroup) int { return strings.Compare(a.id, b.id) })
	return out
}

func (d *keycloakDouble) findGroups(org *doubleOrg, r *http.Request, _ []byte) doubleReply {
	query := r.URL.Query()
	search, exact := query.Get("search"), query.Get("exact") == "true"
	found := []map[string]string{}
	for _, group := range d.sortedGroups(org) {
		if group.name == search || (!exact && strings.Contains(group.name, search)) {
			found = append(found, map[string]string{"id": group.id, "name": group.name, "path": "/" + group.name})
		}
	}
	return doubleReply{status: http.StatusOK, body: page(found, r)}
}

func (d *keycloakDouble) createGroup(org *doubleOrg, _ *http.Request, body []byte) doubleReply {
	var rep struct{ Name string }
	if json.Unmarshal(body, &rep) != nil || rep.Name == "" {
		return doubleReply{status: http.StatusBadRequest}
	}
	for _, group := range org.groups {
		if group.name == rep.Name {
			return doubleReply{status: http.StatusConflict, body: map[string]string{"errorMessage": "Group with the given name already exists."}}
		}
	}
	group := &doubleGroup{id: d.nextID("group"), name: rep.Name, roles: map[string]bool{}, members: map[string]bool{}}
	org.groups[group.id] = group
	return doubleReply{status: http.StatusCreated, location: d.server.URL + "/admin/realms/" + doubleRealm + "/organizations/" + org.id + "/groups/" + group.id}
}

func (d *keycloakDouble) groupRoles(group *doubleGroup, client string, r *http.Request, body []byte) doubleReply {
	if client != doubleStoreUUID {
		return doubleReply{status: http.StatusNotFound}
	}
	if r.Method == http.MethodGet {
		mapped := []doubleRole{}
		for _, role := range d.clientRoles {
			if group.roles[role.Name] {
				mapped = append(mapped, role)
			}
		}
		return doubleReply{status: http.StatusOK, body: mapped}
	}
	var roles []doubleRole
	if json.Unmarshal(body, &roles) != nil {
		return doubleReply{status: http.StatusBadRequest}
	}
	for _, role := range roles {
		if !slices.Contains(d.clientRoles, role) {
			return doubleReply{status: http.StatusNotFound, body: map[string]string{"error": "Role not found"}}
		}
	}
	for _, role := range roles {
		if r.Method == http.MethodPost {
			group.roles[role.Name] = true
		} else {
			delete(group.roles, role.Name)
		}
	}
	return doubleReply{status: http.StatusNoContent}
}

// groupMember answers as Keycloak 26.8.1 does: neither change is idempotent,
// a second PUT is 409 and a DELETE of a user outside the group is 400.
func (d *keycloakDouble) groupMember(org *doubleOrg, group *doubleGroup, user string, r *http.Request) doubleReply {
	if r.Method == http.MethodPut {
		switch {
		case !org.members[user]:
			return doubleReply{status: http.StatusBadRequest, body: map[string]string{"errorMessage": "User is not member of the organization"}}
		case group.members[user]:
			return doubleReply{status: http.StatusConflict, body: map[string]string{"errorMessage": "User is already a member of the group"}}
		}
		group.members[user] = true
		return doubleReply{status: http.StatusNoContent}
	}
	if !group.members[user] {
		return doubleReply{status: http.StatusBadRequest, body: map[string]string{"errorMessage": "User not a member"}}
	}
	delete(group.members, user)
	return doubleReply{status: http.StatusNoContent}
}

func (d *keycloakDouble) sortedUsers() []*doubleUser {
	out := make([]*doubleUser, 0, len(d.users))
	for _, user := range d.users {
		out = append(out, user)
	}
	slices.SortFunc(out, func(a, b *doubleUser) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func (d *keycloakDouble) findUsers(r *http.Request, _ []byte) doubleReply {
	query := r.URL.Query()
	email, exact := strings.ToLower(query.Get("email")), query.Get("exact") == "true"
	found := []doubleUser{}
	for _, user := range d.sortedUsers() {
		if user.Email == email || (!exact && strings.Contains(user.Email, email)) {
			found = append(found, *user)
		}
	}
	return doubleReply{status: http.StatusOK, body: page(found, r)}
}

func (d *keycloakDouble) createUser(_ *http.Request, body []byte) doubleReply {
	var rep doubleUser
	if json.Unmarshal(body, &rep) != nil || rep.Username == "" {
		return doubleReply{status: http.StatusBadRequest}
	}
	for _, user := range d.users {
		if user.Username == rep.Username || user.Email == rep.Email {
			return doubleReply{status: http.StatusConflict, body: map[string]string{"errorMessage": "User exists with same username"}}
		}
	}
	rep.ID = d.nextID("user")
	d.users[rep.ID] = &rep
	return doubleReply{status: http.StatusCreated, location: d.server.URL + "/admin/realms/" + doubleRealm + "/users/" + rep.ID}
}

func (d *keycloakDouble) addMember(org *doubleOrg, _ *http.Request, body []byte) doubleReply {
	var user string
	if json.Unmarshal(body, &user) != nil {
		return doubleReply{status: http.StatusBadRequest}
	}
	if _, found := d.users[user]; !found {
		return doubleReply{status: http.StatusBadRequest, body: map[string]string{"errorMessage": "User does not exist"}}
	}
	if org.members[user] {
		return doubleReply{status: http.StatusConflict, body: map[string]string{"errorMessage": "User is already a member of the organization."}}
	}
	org.members[user] = true
	return doubleReply{status: http.StatusCreated}
}

func (d *keycloakDouble) member(org *doubleOrg, user string, r *http.Request) doubleReply {
	if !org.members[user] {
		return doubleReply{status: http.StatusNotFound}
	}
	if r.Method == http.MethodGet {
		return doubleReply{status: http.StatusOK, body: d.users[user]}
	}
	// WHY: Keycloak takes the user out of the groups of the Organization along
	// with the membership.
	delete(org.members, user)
	for _, group := range org.groups {
		delete(group.members, user)
	}
	return doubleReply{status: http.StatusNoContent}
}

func (d *keycloakDouble) memberGroups(org *doubleOrg, user string, r *http.Request) doubleReply {
	if !org.members[user] {
		return doubleReply{status: http.StatusNotFound}
	}
	found := []map[string]string{}
	for _, group := range d.sortedGroups(org) {
		if group.members[user] {
			found = append(found, map[string]string{"id": group.id, "name": group.name, "path": "/" + group.name})
		}
	}
	return doubleReply{status: http.StatusOK, body: page(found, r)}
}

func (d *keycloakDouble) realmRole(name string) doubleReply {
	if name != doubleBuyerRole {
		return doubleReply{status: http.StatusNotFound}
	}
	return doubleReply{status: http.StatusOK, body: doubleRole{ID: doubleBuyerID, Name: doubleBuyerRole}}
}

func (d *keycloakDouble) userRealmRoles(user string, r *http.Request, body []byte) doubleReply {
	if _, found := d.users[user]; !found {
		return doubleReply{status: http.StatusNotFound, body: map[string]string{"error": "User not found"}}
	}
	if r.Method == http.MethodGet {
		mapped := []doubleRole{}
		if d.userRealm[user][doubleBuyerRole] {
			mapped = append(mapped, doubleRole{ID: doubleBuyerID, Name: doubleBuyerRole})
		}
		return doubleReply{status: http.StatusOK, body: mapped}
	}
	var roles []doubleRole
	if json.Unmarshal(body, &roles) != nil || !slices.Equal(roles, []doubleRole{{ID: doubleBuyerID, Name: doubleBuyerRole}}) {
		return doubleReply{status: http.StatusNotFound, body: map[string]string{"error": "Role not found"}}
	}
	if d.userRealm[user] == nil {
		d.userRealm[user] = map[string]bool{}
	}
	if r.Method == http.MethodPost {
		d.userRealm[user][doubleBuyerRole] = true
	} else {
		delete(d.userRealm[user], doubleBuyerRole)
	}
	return doubleReply{status: http.StatusNoContent}
}

// inTenant is the context the consumer edge runs a command under: the tenant
// of the message, a deadline, which the kernel client refuses to go without,
// and the retry budget, without which it retries nothing (RES-31).
func inTenant(t *testing.T, tenant string) context.Context {
	t.Helper()
	return inTenantWithin(t, tenant, 10*time.Second)
}

func inTenantWithin(t *testing.T, tenant string, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return execctx.WithExecution(t, retry.WithBudget(ctx), execctx.WithTenant(ports.TenantID(tenant)))
}

func submit(t *testing.T, ctx context.Context, command ports.ExternalCommand, payload []byte) (ports.ExternalClass, ports.Observation, error) {
	t.Helper()
	class, observation, err := command.Submit(ctx, "0123456789abcdef0123456789abcdef", payload)
	if class == 0 {
		t.Fatalf("Submit() class = 0, err = %v: a zero class is a provider defect", err)
	}
	if (class == ports.ClassAccepted) != (err == nil) {
		t.Fatalf("Submit() = %v, %v; want an error exactly when the class is not accepted", class, err)
	}
	if observation.Provider != "keycloak" || observation.Key != "0123456789abcdef0123456789abcdef" || observation.Finding != 0 {
		t.Fatalf("Observation = %+v, want the key, provider keycloak and no finding", observation)
	}
	return class, observation, err
}
