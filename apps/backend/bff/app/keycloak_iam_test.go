//go:build integration && keycloak

package app_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/authn"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"
)

const (
	edgeAudience    = "dmpf-bff"
	buyerClient     = "marketplace-buyer"
	sellerClient    = "marketplace-seller"
	storeClient     = "marketplace-store"
	demoStore       = "loja-demo"
	demoOwner       = "loja-demo-owner"
	operatorUser    = "operator"
	iamAdminClient  = "iam-admin"
	buyerPermission = ports.Permission("iam:buyer")
	iamWrite        = ports.Permission("iam:write")

	sessionEventsClient = "dmpf-session-events"
	operatorsGroup      = "operators"
	ownerGroup          = "owner"
)

var (
	storeCatalog = []ports.Permission{
		"iam:read", "iam:write", "seller:read", "seller:write", "catalog:read",
		"catalog:write", "inventory:read", "inventory:write", "pricing:read",
		"pricing:write", "ordering:read", "ordering:write", "fulfillment:read",
		"fulfillment:write", "settlement:read", "settlement:write",
	}
	foreignToStore = []ports.Permission{"tenancy:read", "tenancy:write", buyerPermission}
)

func identityOf(t *testing.T, token string) ports.Identity {
	t.Helper()
	verifier, err := authn.NewVerifier(t.Context(), authn.Config{
		Issuer:      keycloakIssuer(t),
		Audience:    edgeAudience,
		TenantClaim: "organization.*.tenant_id",
		PermissionClaims: []string{
			"scope",
			"realm_access.roles",
			"organization.*.resource_access.marketplace-store.roles",
		},
		DiscoveryTimeout: httpTimeout,
	})
	if err != nil {
		t.Fatalf("authn.NewVerifier(%s) err = %v: the realm must publish its discovery document", keycloakIssuer(t), err)
	}
	identity, err := verifier.Authenticate(t.Context(), ports.Credential{Scheme: "Bearer", Value: token})
	if err != nil {
		t.Fatalf("Authenticate() err = %v, want the token of the seeded realm accepted by the edge", err)
	}
	return identity
}

func passwordGrant(t *testing.T, clientID string, a account, scope string) tokenSet {
	t.Helper()
	status, tokens := requestToken(t, keycloakIssuer(t), url.Values{
		"grant_type": {"password"},
		"client_id":  {clientID},
		"username":   {a.username},
		"password":   {a.password},
		"scope":      {scope},
	})
	if status != http.StatusOK {
		t.Fatalf("direct grant of %s by %s with scope %q = HTTP %d %s (%s), want 200",
			a.username, clientID, scope, status, tokens.Error, tokens.ErrorDescription)
	}
	return tokens
}

func tenantOf(identity ports.Identity) string {
	if identity.Tenant == nil {
		return ""
	}
	return string(*identity.Tenant)
}

func storePermissionsIn(permissions []ports.Permission) []ports.Permission {
	var found []ports.Permission
	for _, p := range permissions {
		if slices.Contains(storeCatalog, p) {
			found = append(found, p)
		}
	}
	return found
}

func TestKeycloakBuyerWithoutOrganizationResolvesTheOperatorTenant(t *testing.T) {
	tokens := passwordGrant(t, buyerClient, seedAccount(t, "buyer-demo"), "openid")

	identity := identityOf(t, tokens.AccessToken)
	if tenantOf(identity) != operatorTenant {
		t.Fatalf("tenant = %q, want %q: a buyer outside any organization belongs to the operator tenant", tenantOf(identity), operatorTenant)
	}
	if !slices.Contains(identity.Permissions, buyerPermission) {
		t.Fatalf("permissions = %v, want %q from the realm role of the buyer", identity.Permissions, buyerPermission)
	}
	if slices.Contains(identity.Permissions, iamWrite) {
		t.Fatalf("permissions = %v, want no %q: the buyer client must not leak operator permissions", identity.Permissions, iamWrite)
	}
}

func TestKeycloakStoreOwnerCarriesTheRolesOfHisGroupInTheStoreTenant(t *testing.T) {
	tokens := passwordGrant(t, sellerClient, seedAccount(t, demoOwner), "openid organization:"+demoStore)

	identity := identityOf(t, tokens.AccessToken)
	if tenantOf(identity) != demoStore {
		t.Fatalf("tenant = %q, want %q of the organization the owner chose", tenantOf(identity), demoStore)
	}
	for _, want := range storeCatalog {
		if !slices.Contains(identity.Permissions, want) {
			t.Fatalf("permissions = %v, want %q: the owner group maps the whole store catalog", identity.Permissions, want)
		}
	}
	for _, p := range identity.Permissions {
		if strings.HasPrefix(string(p), "/") {
			t.Fatalf("permissions = %v carry the group path %q, want role names only", identity.Permissions, p)
		}
		if slices.Contains(foreignToStore, p) {
			t.Fatalf("permissions = %v carry %q, want no operator or buyer permission in a store token", identity.Permissions, p)
		}
	}
}

func TestKeycloakStoreRolesStayInTheirStore(t *testing.T) {
	admin := newKeycloakAdmin(t)
	store := admin.clientUUID(storeClient)
	roles := admin.clientRoles(store)
	demo := admin.organizationByTenant(demoStore)

	suffix := strings.ToLower(rand.Text()[:8])
	other := "loja-teste-" + suffix
	otherID := admin.create("/organizations", map[string]any{
		"name": other, "alias": other, "attributes": map[string][]string{"tenant_id": {other}},
	})
	t.Cleanup(func() { admin.remove("/organizations/" + otherID) })

	member := account{username: "multi-" + suffix, password: rand.Text()}
	memberID := admin.create("/users", map[string]any{
		"username": member.username, "enabled": true, "emailVerified": true,
		"email": member.username + "@example.com", "firstName": "Multi", "lastName": suffix,
		"credentials": []map[string]any{{"type": "password", "value": member.password, "temporary": false}},
	})
	t.Cleanup(func() { admin.remove("/users/" + memberID) })

	admin.expect(http.MethodPost, "/organizations/"+demo+"/members", memberID, http.StatusCreated)
	admin.expect(http.MethodPost, "/organizations/"+otherID+"/members", memberID, http.StatusCreated)
	groupID := admin.create("/organizations/"+demo+"/groups", map[string]any{"name": "multi-" + suffix})
	t.Cleanup(func() { admin.remove("/organizations/" + demo + "/groups/" + groupID) })
	admin.expect(http.MethodPut, "/organizations/"+demo+"/groups/"+groupID+"/members/"+memberID, nil, http.StatusNoContent)
	admin.expect(http.MethodPost, "/organizations/"+demo+"/groups/"+groupID+"/role-mappings/clients/"+store, roles, http.StatusNoContent)

	tokens := passwordGrant(t, sellerClient, member, "openid organization:"+other)

	organizations, _ := tokenClaims(t, tokens.AccessToken)["organization"].(map[string]any)
	if _, chosen := organizations[other]; len(organizations) != 1 || !chosen {
		t.Fatalf("organization claim = %v, want exactly the chosen %q", organizations, other)
	}
	identity := identityOf(t, tokens.AccessToken)
	if tenantOf(identity) != other {
		t.Fatalf("tenant = %q, want %q of the organization the member chose", tenantOf(identity), other)
	}
	if leaked := storePermissionsIn(identity.Permissions); len(leaked) > 0 {
		t.Fatalf("permissions = %v carry %v, want none: the roles of %s must not reach %s", identity.Permissions, leaked, demoStore, other)
	}
}

func TestKeycloakStoreRolesDoNotReachOtherClients(t *testing.T) {
	owner := seedAccount(t, demoOwner)

	t.Run("buyer", func(t *testing.T) {
		identity := identityOf(t, passwordGrant(t, buyerClient, owner, "openid").AccessToken)
		if tenantOf(identity) != operatorTenant {
			t.Fatalf("tenant = %q, want %q fixed by the buyer client", tenantOf(identity), operatorTenant)
		}
		if leaked := storePermissionsIn(identity.Permissions); len(leaked) > 0 {
			t.Fatalf("permissions = %v carry %v, want none: store roles belong to %s only", identity.Permissions, leaked, sellerClient)
		}
	})

	t.Run("portal", func(t *testing.T) {
		tokens, _ := browserFor(t, portalClient).signIn(owner, "organization:"+demoStore)
		identity := identityOf(t, tokens.AccessToken)
		if leaked := storePermissionsIn(identity.Permissions); len(leaked) > 0 {
			t.Fatalf("permissions = %v carry %v, want none: store roles belong to %s only", identity.Permissions, leaked, sellerClient)
		}
	})
}

func staffRepresentation(username, email string) map[string]any {
	return map[string]any{
		"username": username, "enabled": true, "emailVerified": true,
		"email": email, "firstName": "Staff", "lastName": username,
	}
}

func TestKeycloakIamServiceAccountManagesStoreAccess(t *testing.T) {
	sa := newIamServiceAccount(t)
	admin := newKeycloakAdmin(t)
	suffix := strings.ToLower(rand.Text()[:8])

	demo := sa.organizationByTenant(demoStore)
	store := sa.clientUUID(storeClient)
	groupName := "role-" + suffix
	groupID := sa.create("/organizations/"+demo+"/groups", map[string]any{"name": groupName})
	t.Cleanup(func() { admin.remove("/organizations/" + demo + "/groups/" + groupID) })
	var groups []struct{ ID, Name string }
	groupSearch := "/organizations/" + demo + "/groups?exact=true&search=" + url.QueryEscape(groupName)
	sa.decode(sa.expect(http.MethodGet, groupSearch, nil, http.StatusOK), &groups, "organization groups")
	if len(groups) != 1 || groups[0].ID != groupID || groups[0].Name != groupName {
		t.Fatalf("organization groups named %s = %v, want only %s", groupName, groups, groupID)
	}

	var catalogRead roleRepresentation
	sa.decode(sa.expect(http.MethodGet, "/clients/"+store+"/roles/"+url.PathEscape("catalog:read"), nil, http.StatusOK), &catalogRead, "client role")
	groupRoles := "/organizations/" + demo + "/groups/" + groupID + "/role-mappings/clients/" + store
	sa.expect(http.MethodPost, groupRoles, []roleRepresentation{catalogRead}, http.StatusNoContent)
	var mapped []roleRepresentation
	sa.decode(sa.expect(http.MethodGet, groupRoles, nil, http.StatusOK), &mapped, "group client roles")
	if len(mapped) != 1 || mapped[0].Name != catalogRead.Name {
		t.Fatalf("client roles of group %s = %v, want only %s", groupID, mapped, catalogRead.Name)
	}
	sa.expect(http.MethodDelete, groupRoles, []roleRepresentation{catalogRead}, http.StatusNoContent)
	mapped = nil
	sa.decode(sa.expect(http.MethodGet, groupRoles, nil, http.StatusOK), &mapped, "group client roles")
	if len(mapped) != 0 {
		t.Fatalf("client roles of group %s = %v after the unmapping, want none", groupID, mapped)
	}

	username := "staff-" + suffix
	email := username + "@example.com"
	staffID := sa.create("/users", staffRepresentation(username, email))
	t.Cleanup(func() { admin.remove("/users/" + staffID) })
	sa.expect(http.MethodPost, "/organizations/"+demo+"/members", staffID, http.StatusCreated)
	groupMembership := "/organizations/" + demo + "/groups/" + groupID + "/members/" + staffID
	sa.expect(http.MethodPut, groupMembership, nil, http.StatusNoContent)
	sa.expect(http.MethodDelete, groupMembership, nil, http.StatusNoContent)
	storeMembership := "/organizations/" + demo + "/members/" + staffID
	// WHY: the 200 before the removal keeps the 404 after it from meaning "never readable".
	sa.expect(http.MethodGet, storeMembership, nil, http.StatusOK)
	sa.expect(http.MethodDelete, storeMembership, nil, http.StatusNoContent)
	sa.expect(http.MethodGet, storeMembership, nil, http.StatusNotFound)

	var found []struct{ ID string }
	sa.decode(sa.expect(http.MethodGet, "/users?exact=true&email="+url.QueryEscape(email), nil, http.StatusOK), &found, "users")
	if len(found) != 1 || found[0].ID != staffID {
		t.Fatalf("users with email %s = %v, want only %s", email, found, staffID)
	}

	buyer := []roleRepresentation{sa.realmRole(string(buyerPermission))}
	sa.expect(http.MethodPost, "/users/"+staffID+"/role-mappings/realm", buyer, http.StatusNoContent)
	sa.expect(http.MethodDelete, "/users/"+staffID+"/role-mappings/realm", buyer, http.StatusNoContent)
}

func TestKeycloakIamServiceAccountStaysOutOfTheOperator(t *testing.T) {
	sa := newIamServiceAccount(t)
	lookup := "/organizations?q=" + url.QueryEscape("tenant_id:"+demoStore)
	status, _, raw := sa.do(http.MethodGet, lookup, nil)
	var stores []struct{ ID string }
	if status != http.StatusOK || json.Unmarshal(raw, &stores) != nil || len(stores) != 1 {
		t.Fatalf("GET %s by %s = HTTP %d %s, want 200 with %s: the service account must work before its refusals mean anything",
			lookup, iamAdminClient, status, raw, demoStore)
	}
	admin := newKeycloakAdmin(t)
	suffix := strings.ToLower(rand.Text()[:8])
	username := "staff-" + suffix
	staffID := admin.create("/users", staffRepresentation(username, username+"@example.com"))
	t.Cleanup(func() { admin.remove("/users/" + staffID) })
	operator := admin.organizationByTenant(operatorTenant)
	operatorRoles := map[string][]roleRepresentation{}
	for _, name := range []string{"iam:write", "tenancy:write"} {
		operatorRoles[name] = []roleRepresentation{admin.realmRole(name)}
	}
	operatorID := admin.userByUsername(operatorUser)
	operatorAccount := "/users/" + operatorID
	iamAccount := "/users/" + admin.userByUsername("service-account-"+iamAdminClient)
	sessionEventsAccount := "/users/" + admin.userByUsername("service-account-"+sessionEventsClient)
	operatorsGroupID := admin.realmGroup(operatorsGroup)
	demo := admin.organizationByTenant(demoStore)
	ownerMembership := "/organizations/" + demo + "/groups/" + admin.organizationGroup(demo, ownerGroup) + "/members/" + operatorID
	realmManagement := admin.clientUUID("realm-management")
	var manageUsers roleRepresentation
	admin.decode(admin.expect(http.MethodGet, "/clients/"+realmManagement+"/roles/manage-users", nil, http.StatusOK), &manageUsers, "client role")
	selfGrant := []roleRepresentation{manageUsers}
	selfMapping := iamAccount + "/role-mappings/clients/" + realmManagement
	fgap := "/clients/" + admin.clientUUID("admin-permissions") + "/authz/resource-server"
	membershipDeny, membershipDenyBody := admin.fgapPermission(fgap, "deny-operators-membership")
	membershipGrant := maps.Clone(membershipDenyBody)
	membershipGrant["policies"] = []string{"iam-admin-sa"}
	buyer := []roleRepresentation{admin.realmRole(string(buyerPermission))}
	var operatorOrganization map[string]any
	admin.decode(admin.expect(http.MethodGet, "/organizations/"+operator, nil, http.StatusOK), &operatorOrganization, "organization")
	hijacked := map[string]any{}
	for field, value := range operatorOrganization {
		hijacked[field] = value
	}
	hijacked["attributes"] = map[string][]string{"tenant_id": {demoStore}}

	// WHY: two fences refuse below, and both must hold as the iam grows. No grant: it holds
	// no manage-* role, iam-map-buyer is its only realm role grant and iam-map-store-roles
	// its only client one, so other realm roles, clients, the FGAP and realm-management
	// stay out of reach. The seed denies: the marketplace Organization, the operators group
	// (the operator and the service accounts) and map-roles on those protected accounts.
	attempts := []struct {
		name     string
		method   string
		resource string
		body     any
		want     int
		undo     func(admin *keycloakAdmin)
	}{
		{"grant realm role iam:write", http.MethodPost, "/users/" + staffID + "/role-mappings/realm", operatorRoles["iam:write"], http.StatusForbidden, nil},
		{"grant realm role tenancy:write", http.MethodPost, "/users/" + staffID + "/role-mappings/realm", operatorRoles["tenancy:write"], http.StatusForbidden, nil},
		{"join the operator organization", http.MethodPost, "/organizations/" + operator + "/members", staffID, http.StatusForbidden, nil},
		{"create a client", http.MethodPost, "/clients", map[string]any{"clientId": "probe-" + suffix}, http.StatusForbidden, nil},
		// WHY: the seed password keeps a wrong success harmless, so it needs no undo.
		{"reset the operator password", http.MethodPut, operatorAccount + "/reset-password",
			map[string]any{"type": "password", "value": seedAccount(t, operatorUser).password, "temporary": false}, http.StatusForbidden, nil},
		{"disable the operator", http.MethodPut, operatorAccount, map[string]any{"enabled": false}, http.StatusForbidden,
			func(admin *keycloakAdmin) {
				admin.expect(http.MethodPut, operatorAccount, map[string]any{"enabled": true}, http.StatusNoContent)
			}},
		{"grant the operator realm role iam:buyer", http.MethodPost, operatorAccount + "/role-mappings/realm", buyer, http.StatusForbidden,
			func(admin *keycloakAdmin) {
				admin.expect(http.MethodDelete, operatorAccount+"/role-mappings/realm", buyer, http.StatusNoContent)
			}},
		{"move the operator organization to a store tenant", http.MethodPut, "/organizations/" + operator, hijacked, http.StatusForbidden,
			func(admin *keycloakAdmin) {
				admin.expect(http.MethodPut, "/organizations/"+operator, operatorOrganization, http.StatusNoContent)
			}},
		{"take the operator out of the operators group", http.MethodDelete, operatorAccount + "/groups/" + operatorsGroupID, nil, http.StatusForbidden,
			func(admin *keycloakAdmin) {
				admin.expect(http.MethodPut, operatorAccount+"/groups/"+operatorsGroupID, nil, http.StatusNoContent)
			}},
		{"put the operator in the store organization", http.MethodPost, "/organizations/" + demo + "/members", operatorID, http.StatusForbidden,
			func(admin *keycloakAdmin) {
				admin.expect(http.MethodDelete, "/organizations/"+demo+"/members/"+operatorID, nil, http.StatusNoContent)
			}},
		{"put the operator in the store owner group", http.MethodPut, ownerMembership, nil, http.StatusForbidden,
			func(admin *keycloakAdmin) {
				admin.expect(http.MethodDelete, ownerMembership, nil, http.StatusNoContent)
			}},
		{"take the operator out of the operator organization", http.MethodDelete, "/organizations/" + operator + "/members/" + operatorID, nil, http.StatusForbidden,
			func(admin *keycloakAdmin) {
				admin.expect(http.MethodPost, "/organizations/"+operator+"/members", operatorID, http.StatusCreated)
			}},
		{"grant itself realm-management manage-users", http.MethodPost, selfMapping, selfGrant, http.StatusForbidden,
			func(admin *keycloakAdmin) {
				admin.expect(http.MethodDelete, selfMapping, selfGrant, http.StatusNoContent)
			}},
		{"turn the operators membership deny into a grant", http.MethodPut, membershipDeny, membershipGrant, http.StatusForbidden,
			func(admin *keycloakAdmin) {
				admin.expect(http.MethodPut, membershipDeny, membershipDenyBody, http.StatusCreated)
			}},
		{"disable the session events service account", http.MethodPut, sessionEventsAccount, map[string]any{"enabled": false}, http.StatusForbidden,
			func(admin *keycloakAdmin) {
				admin.expect(http.MethodPut, sessionEventsAccount, map[string]any{"enabled": true}, http.StatusNoContent)
			}},
		{"grant the session events service account realm role iam:buyer", http.MethodPost, sessionEventsAccount + "/role-mappings/realm", buyer, http.StatusForbidden,
			func(admin *keycloakAdmin) {
				admin.expect(http.MethodDelete, sessionEventsAccount+"/role-mappings/realm", buyer, http.StatusNoContent)
			}},
		// INVARIANT: last, since every attempt above needs the operator. A deletion has no
		// undo short of the seed, so a wrong success leaves that remedy in the failure.
		{"delete the operator", http.MethodDelete, operatorAccount, nil, http.StatusForbidden,
			func(admin *keycloakAdmin) {
				admin.t.Errorf("%s deleted by %s: rerun the marketplace seed to recreate it", operatorUser, iamAdminClient)
			}},
	}
	for _, attempt := range attempts {
		t.Run(attempt.name, func(t *testing.T) {
			status, location, raw := sa.on(t).do(attempt.method, attempt.resource, attempt.body)
			if location != "" {
				t.Cleanup(func() { admin.on(t).remove(strings.TrimPrefix(location, admin.realm)) })
			}
			if attempt.undo != nil && status >= 200 && status < 300 {
				t.Cleanup(func() { attempt.undo(admin.on(t)) })
			}
			if status != attempt.want {
				t.Fatalf("%s %s by %s = HTTP %d %s, want %d: the iam must not reach the operator",
					attempt.method, attempt.resource, iamAdminClient, status, raw, attempt.want)
			}
		})
	}
}

type keycloakAdmin struct {
	t     *testing.T
	realm string
	token string
}

func newKeycloakAdmin(t *testing.T) *keycloakAdmin {
	t.Helper()
	status, tokens := requestToken(t, keycloakURL(t)+"/realms/master", url.Values{
		"grant_type": {"password"},
		"client_id":  {"admin-cli"},
		"username":   {tb.Env(t, "KEYCLOAK_ADMIN_USER")},
		"password":   {tb.Env(t, "KEYCLOAK_ADMIN_PASSWORD")},
	})
	if status != http.StatusOK {
		t.Fatalf("admin token = HTTP %d %s (%s), want 200 from admin-cli of the master realm", status, tokens.Error, tokens.ErrorDescription)
	}
	return &keycloakAdmin{t: t, realm: keycloakURL(t) + "/admin/realms/dmpf", token: tokens.AccessToken}
}

func newIamServiceAccount(t *testing.T) *keycloakAdmin {
	t.Helper()
	status, tokens := requestToken(t, keycloakIssuer(t), url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {iamAdminClient},
		"client_secret": {tb.Env(t, "KEYCLOAK_IAM_ADMIN_SECRET")},
	})
	if status != http.StatusOK {
		t.Fatalf("client credentials of %s = HTTP %d %s (%s), want 200: the marketplace seed must declare the iam service account",
			iamAdminClient, status, tokens.Error, tokens.ErrorDescription)
	}
	return &keycloakAdmin{t: t, realm: keycloakURL(t) + "/admin/realms/dmpf", token: tokens.AccessToken}
}

func (a *keycloakAdmin) on(t *testing.T) *keycloakAdmin {
	scoped := *a
	scoped.t = t
	return &scoped
}

func (a *keycloakAdmin) realmRole(name string) roleRepresentation {
	a.t.Helper()
	var role roleRepresentation
	a.decode(a.expect(http.MethodGet, "/roles/"+url.PathEscape(name), nil, http.StatusOK), &role, "realm role")
	return role
}

func (a *keycloakAdmin) do(method, resource string, body any) (int, string, []byte) {
	a.t.Helper()
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			a.t.Fatalf("json.Marshal(%T) err = %v", body, err)
		}
		payload = bytes.NewReader(encoded)
	}
	// WHY: cleanups run after t.Context() is canceled and must still reach the Admin API.
	ctx := context.WithoutCancel(a.t.Context())
	request, err := http.NewRequestWithContext(ctx, method, a.realm+resource, payload)
	if err != nil {
		a.t.Fatalf("http.NewRequest(%s %s) err = %v", method, resource, err)
	}
	request.Header.Set("Authorization", "Bearer "+a.token)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := apiClient.Do(request)
	if err != nil {
		a.t.Fatalf("%s %s err = %v: the Admin API must answer", method, resource, err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		a.t.Fatalf("reading %s %s err = %v", method, resource, err)
	}
	return response.StatusCode, response.Header.Get("Location"), raw
}

func (a *keycloakAdmin) expect(method, resource string, body any, want int) []byte {
	a.t.Helper()
	status, _, raw := a.do(method, resource, body)
	if status != want {
		a.t.Fatalf("%s %s = HTTP %d %s, want %d", method, resource, status, raw, want)
	}
	return raw
}

func (a *keycloakAdmin) create(resource string, body any) string {
	a.t.Helper()
	status, location, raw := a.do(http.MethodPost, resource, body)
	if status != http.StatusCreated || location == "" {
		a.t.Fatalf("POST %s = HTTP %d %s, want 201 with the Location of the new resource", resource, status, raw)
	}
	return path.Base(location)
}

func (a *keycloakAdmin) remove(resource string) {
	a.t.Helper()
	if status, _, raw := a.do(http.MethodDelete, resource, nil); status != http.StatusNoContent && status != http.StatusNotFound {
		a.t.Errorf("DELETE %s = HTTP %d %s, want 204: the fixture outlives the test", resource, status, raw)
	}
}

func (a *keycloakAdmin) decode(raw []byte, into any, what string) {
	a.t.Helper()
	if err := json.Unmarshal(raw, into); err != nil {
		a.t.Fatalf("%s %s is not JSON: %v", what, raw, err)
	}
}

func (a *keycloakAdmin) clientUUID(clientID string) string {
	a.t.Helper()
	var clients []struct{ ID string }
	a.decode(a.expect(http.MethodGet, "/clients?clientId="+url.QueryEscape(clientID), nil, http.StatusOK), &clients, "clients")
	if len(clients) != 1 {
		a.t.Fatalf("client %s found %d times in realm dmpf, want 1: the marketplace seed must declare it", clientID, len(clients))
	}
	return clients[0].ID
}

func (a *keycloakAdmin) userByUsername(username string) string {
	a.t.Helper()
	var users []struct{ ID string }
	resource := "/users?exact=true&username=" + url.QueryEscape(username)
	a.decode(a.expect(http.MethodGet, resource, nil, http.StatusOK), &users, "users")
	if len(users) != 1 {
		a.t.Fatalf("user %s found %d times in realm dmpf, want 1: the marketplace seed must declare it", username, len(users))
	}
	return users[0].ID
}

type roleRepresentation struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (a *keycloakAdmin) clientRoles(clientUUID string) []roleRepresentation {
	a.t.Helper()
	var roles []roleRepresentation
	a.decode(a.expect(http.MethodGet, "/clients/"+clientUUID+"/roles", nil, http.StatusOK), &roles, "client roles")
	if len(roles) == 0 {
		a.t.Fatalf("client %s has no roles, want the store catalog", clientUUID)
	}
	return roles
}

func (a *keycloakAdmin) organizationByTenant(tenant string) string {
	a.t.Helper()
	var organizations []struct{ ID string }
	resource := "/organizations?q=" + url.QueryEscape("tenant_id:"+tenant)
	a.decode(a.expect(http.MethodGet, resource, nil, http.StatusOK), &organizations, "organizations")
	if len(organizations) != 1 {
		a.t.Fatalf("organization with tenant_id %s found %d times, want 1: the marketplace seed must declare it", tenant, len(organizations))
	}
	return organizations[0].ID
}

func (a *keycloakAdmin) realmGroup(name string) string {
	a.t.Helper()
	var groups []struct{ ID, Name string }
	a.decode(a.expect(http.MethodGet, "/groups?exact=true&search="+url.QueryEscape(name), nil, http.StatusOK), &groups, "groups")
	groups = slices.DeleteFunc(groups, func(g struct{ ID, Name string }) bool { return g.Name != name })
	if len(groups) != 1 {
		a.t.Fatalf("realm group %s found %d times in realm dmpf, want 1: the marketplace seed must declare it", name, len(groups))
	}
	return groups[0].ID
}

func (a *keycloakAdmin) organizationGroup(organization, name string) string {
	a.t.Helper()
	var groups []struct{ ID, Name string }
	resource := "/organizations/" + organization + "/groups?exact=true&search=" + url.QueryEscape(name)
	a.decode(a.expect(http.MethodGet, resource, nil, http.StatusOK), &groups, "organization groups")
	if len(groups) != 1 || groups[0].Name != name {
		a.t.Fatalf("groups named %s of organization %s = %v, want one: the marketplace seed must declare it", name, organization, groups)
	}
	return groups[0].ID
}

// fgapPermission returns the resource of the FGAP v2 scope permission named name and
// the body that rewrites it as it is.
func (a *keycloakAdmin) fgapPermission(resourceServer, name string) (string, map[string]any) {
	a.t.Helper()
	var found []map[string]any
	a.decode(a.expect(http.MethodGet, resourceServer+"/permission/scope?name="+url.QueryEscape(name), nil, http.StatusOK), &found, "permissions")
	found = slices.DeleteFunc(found, func(p map[string]any) bool { return p["name"] != name })
	if len(found) != 1 {
		a.t.Fatalf("FGAP permission %s found %d times, want 1: the marketplace seed must declare it", name, len(found))
	}
	body := found[0]
	id, _ := body["id"].(string)
	// WHY: a v2 resource is named after the id of its target, and the update resolves
	// scopes, resources and policies by name, so names rebuild the permission exactly.
	for field, link := range map[string]string{"scopes": "scopes", "resources": "resources", "policies": "associatedPolicies"} {
		var linked []struct{ Name string }
		a.decode(a.expect(http.MethodGet, resourceServer+"/policy/"+id+"/"+link, nil, http.StatusOK), &linked, "permission "+link)
		names := make([]string, 0, len(linked))
		for _, l := range linked {
			names = append(names, l.Name)
		}
		body[field] = names
	}
	return resourceServer + "/permission/scope/" + id, body
}
