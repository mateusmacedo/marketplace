package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
)

const (
	// keycloakMaxBody bounds what is read of any answer; no representation the
	// adapter reads comes near it, so a longer one is not the documented schema.
	keycloakMaxBody  = 1 << 20
	keycloakPageSize = 100
	keycloakMaxPages = 100
)

var requiredActions = []string{"UPDATE_PASSWORD", "VERIFY_EMAIL"}

// keycloakFailure is the class of the step that did not converge, with what a
// diagnosis needs: the step, the status and a fixed detail, never a body, an
// address or a credential.
type keycloakFailure struct {
	class  ports.ExternalClass
	step   string
	status int
	detail string
	cause  error
}

func (f *keycloakFailure) Error() string {
	var b strings.Builder
	b.WriteString("provider: keycloak ")
	b.WriteString(f.step)
	b.WriteString(": ")
	b.WriteString(f.class.String())
	if f.status != 0 {
		fmt.Fprintf(&b, " (HTTP %d)", f.status)
	}
	if f.detail != "" {
		b.WriteString(": ")
		b.WriteString(f.detail)
	}
	if f.cause != nil {
		b.WriteString(": ")
		b.WriteString(f.cause.Error())
	}
	return b.String()
}

func (f *keycloakFailure) Unwrap() error { return f.cause }

func failure(step string, class ports.ExternalClass, detail string) error {
	return &keycloakFailure{class: class, step: step, detail: detail}
}

// refused classifies a status the step did not expect by the table.
func refused(step string, status int) error {
	return &keycloakFailure{class: classOfStatus(status), step: step, status: status}
}

// sent classifies the error of a call by the kernel. The URL never reaches the
// cause: a query may carry the address of a member.
//
// WHY: a read, or the grant of the token, changes nothing in the realm, and a
// command stops at its first failure, so every write before it already has its
// answer; an unknown fate is then safe to repeat instead of reconciling.
func sent(step string, repeatable bool, err error) error {
	class := keycloakOutcomes.Classify(nil, err)
	if repeatable && class == ports.ClassUnknownSent {
		class = ports.ClassRetryableTransport
	}
	var withURL *url.Error
	if errors.As(err, &withURL) {
		err = withURL.Err
	}
	return &keycloakFailure{class: class, step: step, cause: err}
}

func classOfStatus(status int) ports.ExternalClass {
	return keycloakOutcomes.Classify(&http.Response{StatusCode: status}, nil)
}

func accepted(status int) bool { return classOfStatus(status) == ports.ClassAccepted }

type keycloakReply struct {
	status   int
	body     []byte
	location string
}

func readReply(step string, repeatable bool, resp *http.Response) (keycloakReply, error) {
	defer func() { _ = resp.Body.Close() }()
	reply := keycloakReply{status: resp.StatusCode, location: resp.Header.Get("Location")}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, keycloakMaxBody+1))
	if err != nil {
		class := ports.ClassUnknownAmbiguous
		if repeatable {
			class = ports.ClassRetryableTransport
		}
		return reply, &keycloakFailure{class: class, step: step, status: resp.StatusCode, detail: "the answer could not be read"}
	}
	if len(raw) > keycloakMaxBody {
		if accepted(resp.StatusCode) {
			return reply, &keycloakFailure{class: ports.ClassSchemaMismatch, step: step, status: resp.StatusCode, detail: "the answer exceeds the body limit"}
		}
		raw = raw[:keycloakMaxBody]
	}
	reply.body = raw
	return reply, nil
}

func decodeReply[T any](step string, reply keycloakReply) (T, error) {
	var out T
	if err := json.Unmarshal(reply.body, &out); err != nil {
		return out, &keycloakFailure{class: ports.ClassSchemaMismatch, step: step, status: reply.status, detail: "the answer is not the documented representation"}
	}
	return out, nil
}

// adminTarget is the URL of a resource of the realm. unsafe marks a segment a
// server resolves to another resource, which call refuses before any request.
type adminTarget struct {
	url    string
	unsafe bool
}

// admin is the URL of a resource of the realm, each segment escaped.
// WHY: url.PathEscape keeps "." and "..", and an empty segment drops a level,
// so an id read from a payload or from the partner could reach an Organization.
func (k *Keycloak) admin(segments ...string) adminTarget {
	escaped := make([]string, len(segments))
	unsafe := false
	for i, segment := range segments {
		unsafe = unsafe || unsafeSegment(segment)
		escaped[i] = url.PathEscape(segment)
	}
	return adminTarget{url: k.adminURL + "/" + strings.Join(escaped, "/"), unsafe: unsafe}
}

func unsafeSegment(segment string) bool { return segment == "" || segment == "." || segment == ".." }

func (t adminTarget) with(query url.Values) adminTarget {
	t.url += "?" + query.Encode()
	return t
}

// call sends one request with the token of the service account, renewing the
// token once when the Admin API answers 401 to one it no longer honors.
func (k *Keycloak) call(ctx context.Context, route string, target adminTarget, body any) (keycloakReply, error) {
	if target.unsafe {
		return keycloakReply{}, failure(route, ports.ClassFinalValidation, "a path segment is empty or a dot segment")
	}
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return keycloakReply{}, failure(route, ports.ClassFinalValidation, "the body could not be encoded")
		}
	}
	token, err := k.tokens.get(ctx)
	if err != nil {
		return keycloakReply{}, err
	}
	reply, err := k.send(ctx, route, target.url, payload, token)
	if err != nil || reply.status != http.StatusUnauthorized {
		return reply, err
	}
	k.tokens.invalidate(token)
	if token, err = k.tokens.get(ctx); err != nil {
		return keycloakReply{}, err
	}
	return k.send(ctx, route, target.url, payload, token)
}

func (k *Keycloak) send(ctx context.Context, route, target string, payload []byte, token string) (keycloakReply, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	method := k.routes[route].Method
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return keycloakReply{}, failure(route, ports.ClassFinalValidation, "request could not be built")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := k.client.Do(ctx, route, req)
	if err != nil {
		return keycloakReply{}, sent(route, method == http.MethodGet, err)
	}
	return readReply(route, method == http.MethodGet, resp)
}

func getJSON[T any](ctx context.Context, k *Keycloak, route string, target adminTarget) (T, error) {
	var zero T
	reply, err := k.call(ctx, route, target, nil)
	if err != nil {
		return zero, err
	}
	if !accepted(reply.status) {
		return zero, refused(route, reply.status)
	}
	return decodeReply[T](route, reply)
}

// listPresent reads every page of a collection; found is false when the owner
// of the collection answers 404.
func listPresent[T any](ctx context.Context, k *Keycloak, route string, target adminTarget, query url.Values) ([]T, bool, error) {
	var all []T
	for n := range keycloakMaxPages {
		q := maps.Clone(query)
		if q == nil {
			q = url.Values{}
		}
		q.Set("first", strconv.Itoa(n*keycloakPageSize))
		q.Set("max", strconv.Itoa(keycloakPageSize))
		reply, err := k.call(ctx, route, target.with(q), nil)
		if err != nil {
			return nil, false, err
		}
		if reply.status == http.StatusNotFound {
			return nil, false, nil
		}
		if !accepted(reply.status) {
			return nil, false, refused(route, reply.status)
		}
		items, err := decodeReply[[]T](route, reply)
		if err != nil {
			return nil, false, err
		}
		all = append(all, items...)
		if len(items) < keycloakPageSize {
			return all, true, nil
		}
	}
	return nil, false, failure(route, ports.ClassUnknownAmbiguous, "the collection has more pages than the adapter reads")
}

func list[T any](ctx context.Context, k *Keycloak, route string, target adminTarget, query url.Values) ([]T, error) {
	items, found, err := listPresent[T](ctx, k, route, target, query)
	if err == nil && !found {
		return nil, refused(route, http.StatusNotFound)
	}
	return items, err
}

// write sends a change; a status in done also counts as converged, such as the
// 404 of a removal that is already done.
func (k *Keycloak) write(ctx context.Context, route string, target adminTarget, body any, done ...int) (keycloakReply, error) {
	reply, err := k.call(ctx, route, target, body)
	if err != nil {
		return reply, err
	}
	if !accepted(reply.status) && !slices.Contains(done, reply.status) {
		return reply, refused(route, reply.status)
	}
	return reply, nil
}

type keycloakOrganization struct {
	ID         string              `json:"id"`
	Attributes map[string][]string `json:"attributes"`
}

type keycloakClient struct {
	ID       string `json:"id"`
	ClientID string `json:"clientId"`
}

type keycloakRole struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type keycloakGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type keycloakUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type keycloakNewUser struct {
	Username        string   `json:"username"`
	Email           string   `json:"email"`
	Enabled         bool     `json:"enabled"`
	RequiredActions []string `json:"requiredActions"`
}

// organization is the one Organization whose tenant_id is exactly the tenant;
// the attribute search is not trusted to be exact. The adapter never writes it.
func (k *Keycloak) organization(ctx context.Context, tenant string) (string, error) {
	found, err := list[keycloakOrganization](ctx, k, routeFindOrganizations, k.admin("organizations"),
		url.Values{"q": {"tenant_id:" + tenant}, "briefRepresentation": {"false"}})
	if err != nil {
		return "", err
	}
	var ids []string
	for _, org := range found {
		if slices.Equal(org.Attributes["tenant_id"], []string{tenant}) {
			ids = append(ids, org.ID)
		}
	}
	switch {
	case len(ids) == 0:
		return "", failure(routeFindOrganizations, ports.ClassFinalBusiness, "no organization holds the tenant")
	case len(ids) > 1:
		return "", failure(routeFindOrganizations, ports.ClassFinalBusiness, "more than one organization holds the tenant")
	case ids[0] == "":
		return "", failure(routeFindOrganizations, ports.ClassSchemaMismatch, "the organization carries no id")
	}
	return ids[0], nil
}

func (k *Keycloak) storeClient(ctx context.Context) (string, error) {
	found, err := list[keycloakClient](ctx, k, routeFindClients, k.admin("clients"), url.Values{"clientId": {keycloakStoreClient}})
	if err != nil {
		return "", err
	}
	found = slices.DeleteFunc(found, func(c keycloakClient) bool { return c.ClientID != keycloakStoreClient || c.ID == "" })
	if len(found) != 1 {
		return "", failure(routeFindClients, ports.ClassFinalBusiness, "the realm holds no single "+keycloakStoreClient)
	}
	return found[0].ID, nil
}

func (k *Keycloak) clientRoles(ctx context.Context, client string) (map[string]keycloakRole, error) {
	found, err := list[keycloakRole](ctx, k, routeListClientRoles, k.admin("clients", client, "roles"), url.Values{"briefRepresentation": {"true"}})
	if err != nil {
		return nil, err
	}
	roles := make(map[string]keycloakRole, len(found))
	for _, role := range found {
		roles[role.Name] = role
	}
	return roles, nil
}

func (k *Keycloak) findGroup(ctx context.Context, org, name string) (string, bool, error) {
	found, err := list[keycloakGroup](ctx, k, routeFindGroups, k.admin("organizations", org, "groups"),
		url.Values{"search": {name}, "exact": {"true"}, "briefRepresentation": {"true"}})
	if err != nil {
		return "", false, err
	}
	found = slices.DeleteFunc(found, func(g keycloakGroup) bool { return g.Name != name })
	switch {
	case len(found) == 0:
		return "", false, nil
	case len(found) > 1:
		return "", false, failure(routeFindGroups, ports.ClassFinalBusiness, "more than one group holds the name")
	case found[0].ID == "":
		return "", false, failure(routeFindGroups, ports.ClassSchemaMismatch, "the group carries no id")
	}
	return found[0].ID, true, nil
}

// ensureGroup reads the group again after a 409: SyncRole and SyncMemberAccess
// race to create the group of a role, and the loser takes the winner's.
func (k *Keycloak) ensureGroup(ctx context.Context, org, name string) (string, error) {
	if id, found, err := k.findGroup(ctx, org, name); err != nil || found {
		return id, err
	}
	reply, err := k.write(ctx, routeCreateGroup, k.admin("organizations", org, "groups"), map[string]string{"name": name}, http.StatusConflict)
	if err != nil {
		return "", err
	}
	if id := createdID(reply); id != "" && reply.status != http.StatusConflict {
		return id, nil
	}
	id, found, err := k.findGroup(ctx, org, name)
	if err == nil && !found {
		return "", refused(routeCreateGroup, http.StatusConflict)
	}
	return id, err
}

func createdID(reply keycloakReply) string {
	location, err := url.Parse(reply.location)
	if err != nil || location.Path == "" {
		return ""
	}
	if id := path.Base(location.Path); id != "/" && id != "." {
		return id
	}
	return ""
}

func (k *Keycloak) groupRoles(ctx context.Context, org, group, client string) ([]keycloakRole, error) {
	return getJSON[[]keycloakRole](ctx, k, routeReadGroupRoles, k.admin("organizations", org, "groups", group, "role-mappings", "clients", client))
}

func (k *Keycloak) findUser(ctx context.Context, email string) (string, bool, error) {
	found, err := list[keycloakUser](ctx, k, routeFindUsers, k.admin("users"),
		url.Values{"email": {email}, "exact": {"true"}, "briefRepresentation": {"true"}})
	if err != nil {
		return "", false, err
	}
	found = slices.DeleteFunc(found, func(u keycloakUser) bool { return !strings.EqualFold(u.Email, email) })
	switch {
	case len(found) == 0:
		return "", false, nil
	case len(found) > 1:
		return "", false, failure(routeFindUsers, ports.ClassFinalBusiness, "more than one user holds the address")
	case found[0].ID == "":
		return "", false, failure(routeFindUsers, ports.ClassSchemaMismatch, "the user carries no id")
	}
	return found[0].ID, true, nil
}

// ensureUser creates the user the first sign-in completes: the password and
// the verified address are the user's to set, never the adapter's.
func (k *Keycloak) ensureUser(ctx context.Context, email string) (string, error) {
	if id, found, err := k.findUser(ctx, email); err != nil || found {
		return id, err
	}
	user := keycloakNewUser{Username: email, Email: email, Enabled: true, RequiredActions: requiredActions}
	reply, err := k.write(ctx, routeCreateUser, k.admin("users"), user, http.StatusConflict)
	if err != nil {
		return "", err
	}
	if id := createdID(reply); id != "" && reply.status != http.StatusConflict {
		return id, nil
	}
	id, found, err := k.findUser(ctx, email)
	if err == nil && !found {
		return "", refused(routeCreateUser, http.StatusConflict)
	}
	return id, err
}

// ensureMember takes a 409 as converged only when the membership reads back:
// Keycloak answers 409 to other refusals of the same request too.
func (k *Keycloak) ensureMember(ctx context.Context, org, user string) error {
	reply, err := k.write(ctx, routeAddMember, k.admin("organizations", org, "members"), user, http.StatusConflict)
	if err != nil || reply.status != http.StatusConflict {
		return err
	}
	check, err := k.call(ctx, routeReadMember, k.admin("organizations", org, "members", user), nil)
	switch {
	case err != nil:
		return err
	case accepted(check.status):
		return nil
	case check.status == http.StatusNotFound:
		return refused(routeAddMember, http.StatusConflict)
	default:
		return refused(routeReadMember, check.status)
	}
}

func (k *Keycloak) memberGroups(ctx context.Context, org, user string) ([]keycloakGroup, bool, error) {
	return listPresent[keycloakGroup](ctx, k, routeListMemberGroups, k.admin("organizations", org, "members", user, "groups"),
		url.Values{"briefRepresentation": {"true"}})
}

func (k *Keycloak) inGroup(ctx context.Context, org, group, user string) (bool, error) {
	groups, found, err := k.memberGroups(ctx, org, user)
	if err != nil || !found {
		return false, err
	}
	return slices.ContainsFunc(groups, func(g keycloakGroup) bool { return g.ID == group }), nil
}

// addGroupMember takes a 409 as converged only when the group reads back:
// Keycloak answers 409 to a member already in it, which is also what the
// kernel's repetition of an applied PUT receives.
func (k *Keycloak) addGroupMember(ctx context.Context, org, group, user string) error {
	reply, err := k.write(ctx, routeAddGroupMember, k.admin("organizations", org, "groups", group, "members", user), nil, http.StatusConflict)
	if err != nil || reply.status != http.StatusConflict {
		return err
	}
	in, err := k.inGroup(ctx, org, group, user)
	if err != nil || in {
		return err
	}
	return refused(routeAddGroupMember, http.StatusConflict)
}

// removeGroupMember takes a 400 as converged only when the group no longer
// reads back: Keycloak answers 400 to a user outside the group, and 404 once
// the group is gone.
func (k *Keycloak) removeGroupMember(ctx context.Context, org, group, user string) error {
	reply, err := k.write(ctx, routeRemoveGroupMember, k.admin("organizations", org, "groups", group, "members", user), nil,
		http.StatusNotFound, http.StatusBadRequest)
	if err != nil || reply.status != http.StatusBadRequest {
		return err
	}
	in, err := k.inGroup(ctx, org, group, user)
	if err != nil || !in {
		return err
	}
	return refused(routeRemoveGroupMember, http.StatusBadRequest)
}

func (k *Keycloak) realmRole(ctx context.Context, name string) (keycloakRole, error) {
	role, err := getJSON[keycloakRole](ctx, k, routeReadRealmRole, k.admin("roles", name))
	if err == nil && (role.ID == "" || role.Name != name) {
		return keycloakRole{}, failure(routeReadRealmRole, ports.ClassSchemaMismatch, "the realm role is not the one asked for")
	}
	return role, err
}

func (k *Keycloak) userRealmRoles(ctx context.Context, user string) ([]keycloakRole, error) {
	return getJSON[[]keycloakRole](ctx, k, routeReadUserRealmRoles, k.admin("users", user, "role-mappings", "realm"))
}
