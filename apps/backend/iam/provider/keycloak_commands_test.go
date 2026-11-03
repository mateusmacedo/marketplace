package provider_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	iamports "github.com/mateusmacedo/marketplace/apps/backend/iam/ports"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

func rolePayload(t *testing.T, ref string, status domain.RoleStatus, permissions ...string) []byte {
	t.Helper()
	granted := make([]domain.Permission, 0, len(permissions))
	for _, p := range permissions {
		granted = append(granted, domain.Permission(p))
	}
	raw, err := provider.KeycloakPayloads{}.RoleAccess(iamports.RoleAccess{Ref: domain.RoleRef(ref), Status: status, Permissions: granted})
	if err != nil {
		t.Fatalf("RoleAccess() = %v", err)
	}
	return raw
}

func provisionPayload(t *testing.T, email string) []byte {
	t.Helper()
	raw, err := provider.KeycloakPayloads{}.MemberProvision(iamports.MemberProvision{Email: domain.Email(email)})
	if err != nil {
		t.Fatalf("MemberProvision() = %v", err)
	}
	return raw
}

func memberPayload(t *testing.T, subject string, status domain.MembershipStatus, roles ...string) []byte {
	t.Helper()
	refs := make([]domain.RoleRef, 0, len(roles))
	for _, r := range roles {
		refs = append(refs, domain.RoleRef(r))
	}
	raw, err := provider.KeycloakPayloads{}.MemberAccess(iamports.MemberAccess{Subject: domain.Subject(subject), Status: status, Roles: refs})
	if err != nil {
		t.Fatalf("MemberAccess() = %v", err)
	}
	return raw
}

func requireClass(t *testing.T, got ports.ExternalClass, err error, want ports.ExternalClass) {
	t.Helper()
	if got != want {
		t.Fatalf("Submit() = %v, %v; want %v", got, err, want)
	}
}

func requireMutations(t *testing.T, d *keycloakDouble, want ...string) {
	t.Helper()
	if got := d.mutations(); !slices.Equal(got, want) {
		t.Fatalf("writes = %v, want %v", got, want)
	}
}

func requireGroupRoles(t *testing.T, d *keycloakDouble, org *doubleOrg, name string, want ...string) {
	t.Helper()
	group, count := d.groupNamed(org, name)
	if count != 1 {
		t.Fatalf("groups named %s = %d, want exactly one", name, count)
	}
	if got := d.rolesOf(group); !slices.Equal(got, want) {
		t.Fatalf("client roles of %s = %v, want %v", name, got, want)
	}
}

func TestKeycloakSyncRoleCreatesTheGroupWithExactlyItsPermissions(t *testing.T) {
	d := newKeycloakDouble(t)

	class, observation, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).SyncRole(), rolePayload(t, "expedicao", domain.RoleStatusActive, "fulfillment:write", "fulfillment:read"))

	requireClass(t, class, err, ports.ClassAccepted)
	requireGroupRoles(t, d, d.organizationOf(storeTenant), "expedicao", "fulfillment:read", "fulfillment:write")
	requireMutations(t, d, "create_group", "add_group_roles")
	if observation.ExternalRef != "" {
		t.Fatalf("ExternalRef = %q, want none outside ProvisionMember", observation.ExternalRef)
	}
}

func TestKeycloakSyncRoleConvergesAnExistingGroupByTheExactDiff(t *testing.T) {
	d := newKeycloakDouble(t)
	org := d.organizationOf(storeTenant)
	d.addGroup(org, "expedicao", "catalog:read", "iam:read")

	class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).SyncRole(), rolePayload(t, "expedicao", domain.RoleStatusActive, "catalog:read", "catalog:write"))

	requireClass(t, class, err, ports.ClassAccepted)
	requireGroupRoles(t, d, org, "expedicao", "catalog:read", "catalog:write")
	requireMutations(t, d, "remove_group_roles", "add_group_roles")
}

func TestKeycloakSyncRoleWhoseRemovalFailsGrantsNothing(t *testing.T) {
	tests := []struct {
		name   string
		status int
		times  int
		want   ports.ExternalClass
	}{
		{"refused", http.StatusForbidden, 1, ports.ClassFinalAuthorization},
		{"unavailable until the retries run out", http.StatusServiceUnavailable, 10, ports.ClassRetryableUpstream},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newKeycloakDouble(t)
			org := d.organizationOf(storeTenant)
			d.addGroup(org, "expedicao", "iam:read")
			d.fail("remove_group_roles", tt.status, tt.times, false)

			class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).SyncRole(), rolePayload(t, "expedicao", domain.RoleStatusActive, "catalog:read"))

			requireClass(t, class, err, tt.want)
			requireGroupRoles(t, d, org, "expedicao", "iam:read")
			if slices.Contains(d.mutations(), "add_group_roles") {
				t.Fatalf("writes = %v, want no grant after the removal failed", d.mutations())
			}
		})
	}
}

func TestKeycloakSyncRoleOfARetiredRoleWithoutGroupCreatesNone(t *testing.T) {
	d := newKeycloakDouble(t)

	class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).SyncRole(), rolePayload(t, "expedicao", domain.RoleStatusRetired))

	requireClass(t, class, err, ports.ClassAccepted)
	requireMutations(t, d)
	if _, count := d.groupNamed(d.organizationOf(storeTenant), "expedicao"); count != 0 {
		t.Fatalf("groups named expedicao = %d, want none: no group already holds no client role", count)
	}
}

func TestKeycloakSyncRoleLeavesARetiredRoleWithNoPermission(t *testing.T) {
	d := newKeycloakDouble(t)
	org := d.organizationOf(storeTenant)
	d.addGroup(org, "expedicao", "catalog:read", "iam:read")

	class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).SyncRole(), rolePayload(t, "expedicao", domain.RoleStatusRetired))

	requireClass(t, class, err, ports.ClassAccepted)
	requireGroupRoles(t, d, org, "expedicao")
	requireMutations(t, d, "remove_group_roles")
}

func TestKeycloakSyncRoleReappliedChangesNothing(t *testing.T) {
	d := newKeycloakDouble(t)
	k := d.keycloak(t)
	payload := rolePayload(t, "expedicao", domain.RoleStatusActive, "fulfillment:read")
	if class, _, err := submit(t, inTenant(t, storeTenant), k.SyncRole(), payload); class != ports.ClassAccepted {
		t.Fatalf("first Submit() = %v, %v", class, err)
	}
	d.resetCalls()

	class, _, err := submit(t, inTenant(t, storeTenant), k.SyncRole(), payload)

	requireClass(t, class, err, ports.ClassAccepted)
	requireMutations(t, d)
	requireGroupRoles(t, d, d.organizationOf(storeTenant), "expedicao", "fulfillment:read")
}

func TestKeycloakSyncRoleRefusesWhatItMustNotRun(t *testing.T) {
	tests := []struct {
		name    string
		tenant  string
		payload func(t *testing.T) []byte
	}{
		{"the owner role", storeTenant, func(t *testing.T) []byte { return rolePayload(t, "owner", domain.RoleStatusActive, "catalog:read") }},
		{"the operator tenant", testOperator, func(t *testing.T) []byte { return rolePayload(t, "expedicao", domain.RoleStatusActive) }},
		{"a payload that does not decode", storeTenant, func(*testing.T) []byte { return []byte(`{"ref":"expedicao","status":"paused"}`) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newKeycloakDouble(t)

			class, _, err := submit(t, inTenant(t, tt.tenant), d.keycloak(t).SyncRole(), tt.payload(t))

			requireClass(t, class, err, ports.ClassFinalValidation)
			if calls := d.callsTo("token"); calls != 0 {
				t.Fatalf("the partner was called, want the refusal before any request")
			}
		})
	}
}

func TestKeycloakSyncRoleRefusesAPermissionOutsideTheStoreClientBeforeAnyWrite(t *testing.T) {
	d := newKeycloakDouble(t)

	class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).SyncRole(), rolePayload(t, "expedicao", domain.RoleStatusActive, "catalog:read", "tenancy:write"))

	requireClass(t, class, err, ports.ClassFinalValidation)
	requireMutations(t, d)
}

func TestKeycloakSyncRoleWithoutOneOrganizationOfTheTenantIsAFinalBusiness(t *testing.T) {
	t.Run("none", func(t *testing.T) {
		d := newKeycloakDouble(t)

		class, _, err := submit(t, inTenant(t, "loja-sem-org"), d.keycloak(t).SyncRole(), rolePayload(t, "expedicao", domain.RoleStatusActive))

		requireClass(t, class, err, ports.ClassFinalBusiness)
		requireMutations(t, d)
	})
	t.Run("two", func(t *testing.T) {
		d := newKeycloakDouble(t)
		d.addOrganization(storeTenant)

		class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).SyncRole(), rolePayload(t, "expedicao", domain.RoleStatusActive))

		requireClass(t, class, err, ports.ClassFinalBusiness)
		requireMutations(t, d)
	})
}

func TestKeycloakSyncRolePicksTheOrganizationWhoseTenantIsExact(t *testing.T) {
	d := newKeycloakDouble(t)
	wider := d.addOrganization(storeTenant + "b")

	class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).SyncRole(), rolePayload(t, "expedicao", domain.RoleStatusActive, "catalog:read"))

	requireClass(t, class, err, ports.ClassAccepted)
	requireGroupRoles(t, d, d.organizationOf(storeTenant), "expedicao", "catalog:read")
	if _, count := d.groupNamed(wider, "expedicao"); count != 0 {
		t.Fatalf("the organization of %sb got the group, want only the one of %s", storeTenant, storeTenant)
	}
}

func TestKeycloakSyncRoleRereadsTheGroupAnotherCommandCreated(t *testing.T) {
	d := newKeycloakDouble(t)
	d.fail("create_group", http.StatusConflict, 1, true)

	class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).SyncRole(), rolePayload(t, "expedicao", domain.RoleStatusActive, "catalog:read"))

	requireClass(t, class, err, ports.ClassAccepted)
	requireGroupRoles(t, d, d.organizationOf(storeTenant), "expedicao", "catalog:read")
}

func TestKeycloakSyncRoleReadsEveryPageOfTheClientRoles(t *testing.T) {
	d := newKeycloakDouble(t)
	for i := range 150 {
		d.addClientRole(fmt.Sprintf("role-extra-%d", i), fmt.Sprintf("extra:%03d", i))
	}

	class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).SyncRole(), rolePayload(t, "expedicao", domain.RoleStatusActive, "extra:149"))

	requireClass(t, class, err, ports.ClassAccepted)
	requireGroupRoles(t, d, d.organizationOf(storeTenant), "expedicao", "extra:149")
	if pages := d.callsTo("list_client_roles"); pages < 2 {
		t.Fatalf("client role pages read = %d, want every page", pages)
	}
}

func TestKeycloakSyncRoleResentAfterAnAppliedWriteAnswered500Converges(t *testing.T) {
	for _, route := range []string{"create_group", "add_group_roles"} {
		t.Run(route, func(t *testing.T) {
			d := newKeycloakDouble(t)
			k := d.keycloak(t)
			payload := rolePayload(t, "expedicao", domain.RoleStatusActive, "catalog:read")
			d.fail(route, http.StatusInternalServerError, 1, true)
			class, _, err := submit(t, inTenant(t, storeTenant), k.SyncRole(), payload)
			requireClass(t, class, err, ports.ClassRetryableUpstream)
			d.resetCalls()

			class, _, err = submit(t, inTenant(t, storeTenant), k.SyncRole(), payload)

			requireClass(t, class, err, ports.ClassAccepted)
			requireGroupRoles(t, d, d.organizationOf(storeTenant), "expedicao", "catalog:read")
			if slices.Contains(d.mutations(), route) {
				t.Fatalf("writes on the resend = %v, want no repeat of the applied %s", d.mutations(), route)
			}
		})
	}
}

func TestKeycloakSyncRoleInterruptedBetweenStepsConvergesOnTheResend(t *testing.T) {
	d := newKeycloakDouble(t)
	org := d.organizationOf(storeTenant)
	d.addGroup(org, "expedicao", "iam:read")
	k := d.keycloak(t)
	payload := rolePayload(t, "expedicao", domain.RoleStatusActive, "catalog:read")
	d.fail("add_group_roles", http.StatusServiceUnavailable, 1, false)
	class, _, err := submit(t, inTenant(t, storeTenant), k.SyncRole(), payload)
	requireClass(t, class, err, ports.ClassRetryableUpstream)
	requireGroupRoles(t, d, org, "expedicao")
	d.resetCalls()

	class, _, err = submit(t, inTenant(t, storeTenant), k.SyncRole(), payload)

	requireClass(t, class, err, ports.ClassAccepted)
	requireGroupRoles(t, d, org, "expedicao", "catalog:read")
	requireMutations(t, d, "add_group_roles")
}

func TestKeycloakProvisionMemberCreatesTheUserAndPutsItInTheStore(t *testing.T) {
	d := newKeycloakDouble(t)

	class, observation, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).ProvisionMember(), provisionPayload(t, "ana@loja.example"))

	requireClass(t, class, err, ports.ClassAccepted)
	users := d.usersWithEmail("ana@loja.example")
	if len(users) != 1 {
		t.Fatalf("users with the address = %d, want one", len(users))
	}
	user := users[0]
	if user.Username != "ana@loja.example" || !user.Enabled || !slices.Equal(user.RequiredActions, []string{"UPDATE_PASSWORD", "VERIFY_EMAIL"}) {
		t.Fatalf("created user = %+v, want username = email, enabled and the required actions", user)
	}
	if !d.isMember(d.organizationOf(storeTenant), user.ID) {
		t.Fatal("the user is not a member of the store")
	}
	if observation.ExternalRef != user.ID {
		t.Fatalf("ExternalRef = %q, want the sub %q", observation.ExternalRef, user.ID)
	}
	requireMutations(t, d, "create_user", "add_member")
}

func TestKeycloakProvisionMemberReusesTheUserOfTheAddress(t *testing.T) {
	d := newKeycloakDouble(t)
	existing := d.addUser("ana@loja.example")

	class, observation, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).ProvisionMember(), provisionPayload(t, "ana@loja.example"))

	requireClass(t, class, err, ports.ClassAccepted)
	if observation.ExternalRef != existing || !d.isMember(d.organizationOf(storeTenant), existing) {
		t.Fatalf("ExternalRef = %q, want the existing %q made a member", observation.ExternalRef, existing)
	}
	requireMutations(t, d, "add_member")
}

func TestKeycloakProvisionMemberAcceptsAMemberAlready(t *testing.T) {
	d := newKeycloakDouble(t)
	k := d.keycloak(t)
	if class, _, err := submit(t, inTenant(t, storeTenant), k.ProvisionMember(), provisionPayload(t, "ana@loja.example")); class != ports.ClassAccepted {
		t.Fatalf("first Submit() = %v, %v", class, err)
	}

	class, _, err := submit(t, inTenant(t, storeTenant), k.ProvisionMember(), provisionPayload(t, "ana@loja.example"))

	requireClass(t, class, err, ports.ClassAccepted)
	if users := d.usersWithEmail("ana@loja.example"); len(users) != 1 {
		t.Fatalf("users with the address = %d, want one", len(users))
	}
}

func TestKeycloakProvisionMemberRefusesAConflictItCannotRead(t *testing.T) {
	t.Run("username taken by another address", func(t *testing.T) {
		d := newKeycloakDouble(t)
		other := d.addUser("outro@loja.example")
		d.renameUser(other, "ana@loja.example")

		class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).ProvisionMember(), provisionPayload(t, "ana@loja.example"))

		requireClass(t, class, err, ports.ClassFinalBusiness)
	})
	t.Run("membership conflict without membership", func(t *testing.T) {
		d := newKeycloakDouble(t)
		d.fail("add_member", http.StatusConflict, 1, false)

		class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).ProvisionMember(), provisionPayload(t, "ana@loja.example"))

		requireClass(t, class, err, ports.ClassFinalBusiness)
		if d.callsTo("read_member") != 1 {
			t.Fatal("the 409 was not checked against the membership")
		}
	})
}

func TestKeycloakProvisionMemberRefusesWhatItMustNotRun(t *testing.T) {
	tests := []struct {
		name    string
		tenant  string
		payload []byte
		want    ports.ExternalClass
	}{
		{"the operator tenant", testOperator, []byte(`{"email":"ana@loja.example"}`), ports.ClassFinalValidation},
		{"an address not normalized", storeTenant, []byte(`{"email":"Ana@Loja.example"}`), ports.ClassFinalValidation},
		{"a store without organization", "loja-sem-org", []byte(`{"email":"ana@loja.example"}`), ports.ClassFinalBusiness},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newKeycloakDouble(t)

			class, _, err := submit(t, inTenant(t, tt.tenant), d.keycloak(t).ProvisionMember(), tt.payload)

			requireClass(t, class, err, tt.want)
			requireMutations(t, d)
		})
	}
}

func TestKeycloakProvisionMemberResentAfterAnAppliedWriteAnswered500Converges(t *testing.T) {
	for _, route := range []string{"create_user", "add_member"} {
		t.Run(route, func(t *testing.T) {
			d := newKeycloakDouble(t)
			k := d.keycloak(t)
			d.fail(route, http.StatusInternalServerError, 1, true)
			class, _, err := submit(t, inTenant(t, storeTenant), k.ProvisionMember(), provisionPayload(t, "ana@loja.example"))
			requireClass(t, class, err, ports.ClassRetryableUpstream)

			class, observation, err := submit(t, inTenant(t, storeTenant), k.ProvisionMember(), provisionPayload(t, "ana@loja.example"))

			requireClass(t, class, err, ports.ClassAccepted)
			users := d.usersWithEmail("ana@loja.example")
			if len(users) != 1 || observation.ExternalRef != users[0].ID || !d.isMember(d.organizationOf(storeTenant), users[0].ID) {
				t.Fatalf("users = %+v, ref %q; want one user, made a member, as the reference", users, observation.ExternalRef)
			}
		})
	}
}

func TestKeycloakProvisionMemberInterruptedBetweenStepsConvergesOnTheResend(t *testing.T) {
	d := newKeycloakDouble(t)
	k := d.keycloak(t)
	d.fail("add_member", http.StatusServiceUnavailable, 1, false)
	class, _, err := submit(t, inTenant(t, storeTenant), k.ProvisionMember(), provisionPayload(t, "ana@loja.example"))
	requireClass(t, class, err, ports.ClassRetryableUpstream)
	d.resetCalls()

	class, _, err = submit(t, inTenant(t, storeTenant), k.ProvisionMember(), provisionPayload(t, "ana@loja.example"))

	requireClass(t, class, err, ports.ClassAccepted)
	requireMutations(t, d, "add_member")
}

func TestKeycloakMemberAccessPutsAnActiveMemberInExactlyTheGroupsOfItsRoles(t *testing.T) {
	d := newKeycloakDouble(t)
	org := d.organizationOf(storeTenant)
	member := d.addUser("ana@loja.example")

	class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).MemberAccess(), memberPayload(t, member, domain.MembershipStatusActive, "owner", "expedicao"))

	requireClass(t, class, err, ports.ClassAccepted)
	if !d.isMember(org, member) {
		t.Fatal("the member is not in the organization")
	}
	if groups := d.groupsOfMember(org, member); !slices.Equal(groups, []string{"expedicao", "owner"}) {
		t.Fatalf("groups = %v, want [expedicao owner]", groups)
	}
	requireGroupRoles(t, d, org, "expedicao")
	requireMutations(t, d, "add_member", "create_group", "add_group_member", "add_group_member")
}

func TestKeycloakMemberAccessConvergesAChangeOfRoles(t *testing.T) {
	d := newKeycloakDouble(t)
	org := d.organizationOf(storeTenant)
	member := d.addUser("ana@loja.example")
	owner, _ := d.groupNamed(org, "owner")
	d.join(org, member, owner, d.addGroup(org, "catalogo"))
	d.addGroup(org, "expedicao")

	class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).MemberAccess(), memberPayload(t, member, domain.MembershipStatusActive, "expedicao"))

	requireClass(t, class, err, ports.ClassAccepted)
	if groups := d.groupsOfMember(org, member); !slices.Equal(groups, []string{"expedicao"}) {
		t.Fatalf("groups = %v, want only expedicao", groups)
	}
	requireMutations(t, d, "add_member", "remove_group_member", "remove_group_member", "add_group_member")
}

func TestKeycloakMemberAccessWhoseRemovalFailsGrantsNothing(t *testing.T) {
	tests := []struct {
		name   string
		status int
		times  int
		want   ports.ExternalClass
	}{
		{"refused", http.StatusForbidden, 1, ports.ClassFinalAuthorization},
		{"unavailable until the retries run out", http.StatusServiceUnavailable, 10, ports.ClassRetryableUpstream},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newKeycloakDouble(t)
			org := d.organizationOf(storeTenant)
			member := d.addUser("ana@loja.example")
			d.join(org, member, d.addGroup(org, "catalogo"))
			d.addGroup(org, "expedicao")
			d.fail("remove_group_member", tt.status, tt.times, false)

			class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).MemberAccess(), memberPayload(t, member, domain.MembershipStatusActive, "expedicao"))

			requireClass(t, class, err, tt.want)
			if groups := d.groupsOfMember(org, member); !slices.Equal(groups, []string{"catalogo"}) {
				t.Fatalf("groups = %v, want [catalogo]: nothing granted after the removal failed", groups)
			}
			if slices.Contains(d.mutations(), "add_group_member") {
				t.Fatalf("writes = %v, want no grant after the removal failed", d.mutations())
			}
		})
	}
}

func TestKeycloakAccessOfADotSubjectIsRefusedBeforeAnyCall(t *testing.T) {
	for _, subject := range []string{".", ".."} {
		t.Run(subject, func(t *testing.T) {
			d := newKeycloakDouble(t)
			k := d.keycloak(t)
			commands := map[string]struct {
				tenant  string
				command ports.ExternalCommand
				payload string
			}{
				"MemberAccess": {storeTenant, k.MemberAccess(), `{"subject":"` + subject + `","status":"active","roles":["owner"]}`},
				"BuyerAccess":  {testOperator, k.BuyerAccess(), `{"subject":"` + subject + `","status":"blocked"}`},
			}
			for name, c := range commands {
				class, _, err := submit(t, inTenant(t, c.tenant), c.command, []byte(c.payload))

				if class != ports.ClassFinalValidation {
					t.Errorf("%s Submit() = %v, %v; want final_validation", name, class, err)
				}
			}
			if calls := d.adminCalls() + d.callsTo("token"); calls != 0 {
				t.Fatalf("the partner was called %d times, want none: the subject would resolve to another resource", calls)
			}
		})
	}
}

func TestKeycloakMemberAccessWithoutTheOwnerGroupIsAFinalBusinessBeforeAnyWrite(t *testing.T) {
	d := newKeycloakDouble(t)
	d.addOrganization("loja-b")
	member := d.addUser("ana@loja.example")

	class, _, err := submit(t, inTenant(t, "loja-b"), d.keycloak(t).MemberAccess(), memberPayload(t, member, domain.MembershipStatusActive, "owner"))

	requireClass(t, class, err, ports.ClassFinalBusiness)
	requireMutations(t, d)
}

func TestKeycloakMemberAccessTakesARemovedMemberOutOfTheGroupsFirst(t *testing.T) {
	d := newKeycloakDouble(t)
	org := d.organizationOf(storeTenant)
	member := d.addUser("ana@loja.example")
	d.join(org, member, d.addGroup(org, "catalogo"), d.addGroup(org, "expedicao"))

	class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).MemberAccess(), memberPayload(t, member, domain.MembershipStatusRemoved))

	requireClass(t, class, err, ports.ClassAccepted)
	if d.isMember(org, member) || len(d.groupsOfMember(org, member)) != 0 {
		t.Fatalf("member = %v in %v, want out of the organization and of every group", d.isMember(org, member), d.groupsOfMember(org, member))
	}
	requireMutations(t, d, "remove_group_member", "remove_group_member", "remove_member")
}

func TestKeycloakMemberAccessOfAMemberAlreadyOutChangesNothing(t *testing.T) {
	d := newKeycloakDouble(t)
	member := d.addUser("ana@loja.example")

	class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).MemberAccess(), memberPayload(t, member, domain.MembershipStatusRemoved))

	requireClass(t, class, err, ports.ClassAccepted)
	requireMutations(t, d)
}

func TestKeycloakMemberAccessRefusesTheOperatorTenant(t *testing.T) {
	d := newKeycloakDouble(t)

	class, _, err := submit(t, inTenant(t, testOperator), d.keycloak(t).MemberAccess(), memberPayload(t, "sub-1", domain.MembershipStatusActive, "owner"))

	requireClass(t, class, err, ports.ClassFinalValidation)
	if calls := d.callsTo("token"); calls != 0 {
		t.Fatal("the partner was called, want the refusal before any request")
	}
}

func TestKeycloakMemberAccessRereadsAGroupSyncRoleCreatedMeanwhile(t *testing.T) {
	d := newKeycloakDouble(t)
	member := d.addUser("ana@loja.example")
	d.fail("create_group", http.StatusConflict, 1, true)

	class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).MemberAccess(), memberPayload(t, member, domain.MembershipStatusActive, "expedicao"))

	requireClass(t, class, err, ports.ClassAccepted)
	if groups := d.groupsOfMember(d.organizationOf(storeTenant), member); !slices.Equal(groups, []string{"expedicao"}) {
		t.Fatalf("groups = %v, want expedicao", groups)
	}
}

func TestKeycloakMemberAccessResentAfterAnAppliedWriteAnswered500Converges(t *testing.T) {
	for _, route := range []string{"add_member", "create_group", "add_group_member"} {
		t.Run(route, func(t *testing.T) {
			d := newKeycloakDouble(t)
			org := d.organizationOf(storeTenant)
			member := d.addUser("ana@loja.example")
			k := d.keycloak(t)
			payload := memberPayload(t, member, domain.MembershipStatusActive, "owner", "expedicao")
			d.fail(route, http.StatusInternalServerError, 1, true)
			class, _, err := submit(t, inTenant(t, storeTenant), k.MemberAccess(), payload)
			requireClass(t, class, err, ports.ClassRetryableUpstream)

			class, _, err = submit(t, inTenant(t, storeTenant), k.MemberAccess(), payload)

			requireClass(t, class, err, ports.ClassAccepted)
			if groups := d.groupsOfMember(org, member); !slices.Equal(groups, []string{"expedicao", "owner"}) {
				t.Fatalf("groups = %v, want [expedicao owner]", groups)
			}
			if _, count := d.groupNamed(org, "expedicao"); count != 1 {
				t.Fatalf("groups named expedicao = %d, want one", count)
			}
		})
	}
}

// The kernel repeats a PUT or a DELETE answered 502; when the first one was
// applied, Keycloak answers the repetition 409 or 400, which only a reread
// tells apart from a refusal.
func TestKeycloakMemberAccessRetriedGroupChangeThatWasAppliedConverges(t *testing.T) {
	tests := []struct {
		name   string
		route  string
		join   bool
		roles  []string
		groups []string
	}{
		{"put", "add_group_member", false, []string{"expedicao"}, []string{"expedicao"}},
		{"delete", "remove_group_member", true, []string{"owner"}, []string{"owner"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newKeycloakDouble(t)
			org := d.organizationOf(storeTenant)
			owner, _ := d.groupNamed(org, "owner")
			expedicao := d.addGroup(org, "expedicao")
			member := d.addUser("ana@loja.example")
			if tt.join {
				d.join(org, member, owner, expedicao)
			}
			d.fail(tt.route, http.StatusBadGateway, 1, true)

			class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).MemberAccess(), memberPayload(t, member, domain.MembershipStatusActive, tt.roles...))

			requireClass(t, class, err, ports.ClassAccepted)
			if groups := d.groupsOfMember(org, member); !slices.Equal(groups, tt.groups) {
				t.Fatalf("groups = %v, want %v", groups, tt.groups)
			}
			if calls := d.callsTo(tt.route); calls != 2 {
				t.Fatalf("%s calls = %d, want 2: the kernel repeats it once", tt.route, calls)
			}
		})
	}
}

func TestKeycloakMemberAccessGroupRefusalThatDidNotConvergeStaysARefusal(t *testing.T) {
	tests := []struct {
		name   string
		route  string
		status int
		join   bool
		want   ports.ExternalClass
	}{
		{"put answered 409", "add_group_member", http.StatusConflict, false, ports.ClassFinalBusiness},
		{"delete answered 400", "remove_group_member", http.StatusBadRequest, true, ports.ClassFinalValidation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newKeycloakDouble(t)
			org := d.organizationOf(storeTenant)
			catalogo := d.addGroup(org, "catalogo")
			member := d.addUser("ana@loja.example")
			roles := []string{"catalogo"}
			if tt.join {
				d.join(org, member, catalogo)
				roles = nil
			}
			d.fail(tt.route, tt.status, 1, false)

			class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).MemberAccess(), memberPayload(t, member, domain.MembershipStatusActive, roles...))

			requireClass(t, class, err, tt.want)
			if groups, want := d.groupsOfMember(org, member), map[bool][]string{true: {"catalogo"}}[tt.join]; !slices.Equal(groups, want) {
				t.Fatalf("groups = %v, want %v: the refusal changed nothing", groups, want)
			}
		})
	}
}

func TestKeycloakMemberAccessRemovalRacingAnotherConverges(t *testing.T) {
	d := newKeycloakDouble(t)
	org := d.organizationOf(storeTenant)
	member := d.addUser("ana@loja.example")
	d.join(org, member, d.addGroup(org, "catalogo"))
	d.fail("remove_group_member", http.StatusBadRequest, 1, true)

	class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).MemberAccess(), memberPayload(t, member, domain.MembershipStatusRemoved))

	requireClass(t, class, err, ports.ClassAccepted)
	if d.isMember(org, member) {
		t.Fatal("the member is still in the organization")
	}
}

func TestKeycloakMemberAccessInterruptedBetweenStepsConvergesOnTheResend(t *testing.T) {
	d := newKeycloakDouble(t)
	org := d.organizationOf(storeTenant)
	member := d.addUser("ana@loja.example")
	d.join(org, member, d.addGroup(org, "catalogo"))
	k := d.keycloak(t)
	payload := memberPayload(t, member, domain.MembershipStatusRemoved)
	d.fail("remove_member", http.StatusServiceUnavailable, 3, false)
	class, _, err := submit(t, inTenant(t, storeTenant), k.MemberAccess(), payload)
	requireClass(t, class, err, ports.ClassRetryableUpstream)
	if len(d.groupsOfMember(org, member)) != 0 || !d.isMember(org, member) {
		t.Fatal("want the groups gone and the membership kept by the interruption")
	}
	d.resetCalls()

	class, _, err = submit(t, inTenant(t, storeTenant), k.MemberAccess(), payload)

	requireClass(t, class, err, ports.ClassAccepted)
	if d.isMember(org, member) {
		t.Fatal("the member is still in the organization after the resend")
	}
	requireMutations(t, d, "remove_member")
}

func TestKeycloakBuyerAccessGrantsAndTakesTheBuyerRole(t *testing.T) {
	tests := []struct {
		name   string
		holds  bool
		status domain.BuyerStatus
		want   bool
		writes []string
	}{
		{"active without the role", false, domain.BuyerStatusActive, true, []string{"add_user_realm_roles"}},
		{"active with the role", true, domain.BuyerStatusActive, true, nil},
		{"blocked with the role", true, domain.BuyerStatusBlocked, false, []string{"remove_user_realm_roles"}},
		{"blocked without the role", false, domain.BuyerStatusBlocked, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newKeycloakDouble(t)
			buyer := d.addUser("comprador@example.com")
			if tt.holds {
				d.grantBuyer(buyer)
			}

			class, _, err := submit(t, inTenant(t, testOperator), d.keycloak(t).BuyerAccess(), buyerPayload(t, buyer, tt.status))

			requireClass(t, class, err, ports.ClassAccepted)
			if d.hasBuyerRole(buyer) != tt.want {
				t.Fatalf("holds iam:buyer = %v, want %v", d.hasBuyerRole(buyer), tt.want)
			}
			requireMutations(t, d, tt.writes...)
		})
	}
}

func TestKeycloakBuyerAccessOfAnUnknownUserIsAFinalBusiness(t *testing.T) {
	d := newKeycloakDouble(t)

	class, _, err := submit(t, inTenant(t, testOperator), d.keycloak(t).BuyerAccess(), buyerPayload(t, "sub-unknown", domain.BuyerStatusActive))

	requireClass(t, class, err, ports.ClassFinalBusiness)
	requireMutations(t, d)
}

func TestKeycloakBuyerAccessRefusesAStoreTenant(t *testing.T) {
	d := newKeycloakDouble(t)

	class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).BuyerAccess(), buyerPayload(t, "sub-1", domain.BuyerStatusActive))

	requireClass(t, class, err, ports.ClassFinalValidation)
	if calls := d.callsTo("token"); calls != 0 {
		t.Fatal("the partner was called, want the refusal before any request")
	}
}

func TestKeycloakBuyerAccessResentAfterAnAppliedWriteAnswered500Converges(t *testing.T) {
	d := newKeycloakDouble(t)
	buyer := d.addUser("comprador@example.com")
	k := d.keycloak(t)
	d.fail("add_user_realm_roles", http.StatusInternalServerError, 1, true)
	class, _, err := submit(t, inTenant(t, testOperator), k.BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))
	requireClass(t, class, err, ports.ClassRetryableUpstream)
	d.resetCalls()

	class, _, err = submit(t, inTenant(t, testOperator), k.BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))

	requireClass(t, class, err, ports.ClassAccepted)
	requireMutations(t, d)
	if !d.hasBuyerRole(buyer) {
		t.Fatal("the buyer lost iam:buyer")
	}
}

func TestKeycloakBuyerAccessInterruptedBetweenStepsConvergesOnTheResend(t *testing.T) {
	d := newKeycloakDouble(t)
	buyer := d.addUser("comprador@example.com")
	k := d.keycloak(t)
	d.fail("read_user_realm_roles", http.StatusServiceUnavailable, 3, false)
	class, _, err := submit(t, inTenant(t, testOperator), k.BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))
	requireClass(t, class, err, ports.ClassRetryableUpstream)
	d.resetCalls()

	class, _, err = submit(t, inTenant(t, testOperator), k.BuyerAccess(), buyerPayload(t, buyer, domain.BuyerStatusActive))

	requireClass(t, class, err, ports.ClassAccepted)
	requireMutations(t, d, "add_user_realm_roles")
}

func TestKeycloakCreationAnsweredWithoutLocationRereadsTheResource(t *testing.T) {
	tests := []struct {
		route   string
		command func(k *provider.Keycloak) ports.ExternalCommand
		payload func(t *testing.T) []byte
		reread  string
	}{
		{"create_group", (*provider.Keycloak).SyncRole, func(t *testing.T) []byte {
			return rolePayload(t, "expedicao", domain.RoleStatusActive, "catalog:read")
		}, "find_groups"},
		{"create_user", (*provider.Keycloak).ProvisionMember, func(t *testing.T) []byte {
			return provisionPayload(t, "ana@loja.example")
		}, "find_users"},
	}
	for _, tt := range tests {
		t.Run(tt.route, func(t *testing.T) {
			d := newKeycloakDouble(t)
			d.fail(tt.route, http.StatusCreated, 1, true)

			class, _, err := submit(t, inTenant(t, storeTenant), tt.command(d.keycloak(t)), tt.payload(t))

			requireClass(t, class, err, ports.ClassAccepted)
			if reads := d.callsTo(tt.reread); reads != 2 {
				t.Fatalf("%s calls = %d, want 2: a 201 without Location is read back", tt.reread, reads)
			}
			if creations := d.callsTo(tt.route); creations != 1 {
				t.Fatalf("%s calls = %d, want 1", tt.route, creations)
			}
		})
	}
}

func TestKeycloakCollectionLongerThanThePagesTheAdapterReadsIsUnknownAmbiguous(t *testing.T) {
	d := newKeycloakDouble(t)
	full := make([]map[string]string, 100)
	for i := range full {
		full[i] = map[string]string{"id": fmt.Sprintf("role-page-%d", i), "name": fmt.Sprintf("page:%03d", i)}
	}
	body, err := json.Marshal(full)
	if err != nil {
		t.Fatalf("json.Marshal() = %v", err)
	}
	d.failWith("list_client_roles", http.StatusOK, 101, false, string(body))

	class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).SyncRole(), rolePayload(t, "expedicao", domain.RoleStatusActive, "catalog:read"))

	requireClass(t, class, err, ports.ClassUnknownAmbiguous)
	if pages := d.callsTo("list_client_roles"); pages != 100 {
		t.Fatalf("pages read = %d, want the cap of 100", pages)
	}
	requireMutations(t, d)
}

func TestKeycloakNameHeldTwiceIsAFinalBusiness(t *testing.T) {
	t.Run("group", func(t *testing.T) {
		d := newKeycloakDouble(t)
		org := d.organizationOf(storeTenant)
		d.addGroup(org, "expedicao")
		d.addGroup(org, "expedicao")

		class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).SyncRole(), rolePayload(t, "expedicao", domain.RoleStatusActive, "catalog:read"))

		requireClass(t, class, err, ports.ClassFinalBusiness)
		requireMutations(t, d)
	})
	t.Run("user", func(t *testing.T) {
		d := newKeycloakDouble(t)
		d.addUser("ana@loja.example")
		d.addUser("ana@loja.example")

		class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).ProvisionMember(), provisionPayload(t, "ana@loja.example"))

		requireClass(t, class, err, ports.ClassFinalBusiness)
		requireMutations(t, d)
	})
}

func TestKeycloakStoreClientAbsentOrDuplicatedIsAFinalBusiness(t *testing.T) {
	for name, body := range map[string]string{
		"absent":     `[]`,
		"duplicated": `[{"id":"a","clientId":"marketplace-store"},{"id":"b","clientId":"marketplace-store"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			d := newKeycloakDouble(t)
			d.failWith("find_clients", http.StatusOK, 1, false, body)

			class, _, err := submit(t, inTenant(t, storeTenant), d.keycloak(t).SyncRole(), rolePayload(t, "expedicao", domain.RoleStatusActive, "catalog:read"))

			requireClass(t, class, err, ports.ClassFinalBusiness)
			requireMutations(t, d)
		})
	}
}
