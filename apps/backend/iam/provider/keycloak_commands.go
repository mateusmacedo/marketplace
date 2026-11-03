package provider

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"net/http"
	"slices"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

const (
	stepTenant  = "tenant"
	stepPayload = "payload"
)

// keycloakCommand converges the realm to the desired state of its payload. The
// class is the one of the first step that did not converge.
type keycloakCommand struct {
	run func(ctx context.Context, payload []byte) (string, error)
}

func (c keycloakCommand) Submit(ctx context.Context, key ports.OutboundKey, payload []byte) (ports.ExternalClass, ports.Observation, error) {
	observation := ports.Observation{Key: key, Provider: keycloakProvider}
	ref, err := c.run(ctx, payload)
	if err != nil {
		return classOf(err), observation, err
	}
	observation.ExternalRef = ref
	return ports.ClassAccepted, observation, nil
}

func classOf(err error) ports.ExternalClass {
	var f *keycloakFailure
	if errors.As(err, &f) && f.class != 0 {
		return f.class
	}
	return ports.ClassUnknownAmbiguous
}

func invalidPayload(err error) error {
	return &keycloakFailure{class: ports.ClassFinalValidation, step: stepPayload, cause: err}
}

// SyncRole leaves the group of the role with exactly the client roles of its
// permissions while active, and none once retired.
func (k *Keycloak) SyncRole() ports.ExternalCommand { return keycloakCommand{run: k.syncRole} }

// ProvisionMember makes the user of the address a member of the store; the
// observation carries the sub of the user as ExternalRef.
func (k *Keycloak) ProvisionMember() ports.ExternalCommand {
	return keycloakCommand{run: k.provisionMember}
}

// MemberAccess serves SyncMemberAccess and RevokeMemberAccess: both converge
// by the status of the membership, whichever event triggered them.
func (k *Keycloak) MemberAccess() ports.ExternalCommand { return keycloakCommand{run: k.memberAccess} }

// BuyerAccess grants the realm role of the buyer while active and takes it
// when blocked.
func (k *Keycloak) BuyerAccess() ports.ExternalCommand { return keycloakCommand{run: k.buyerAccess} }

func (k *Keycloak) tenantOf(ctx context.Context) (string, error) {
	execution, err := ports.RequireExecutionContext(ctx)
	if err != nil {
		return "", &keycloakFailure{class: ports.ClassFinalValidation, step: stepTenant, cause: err}
	}
	tenant, resolved := execution.Tenant()
	if !resolved {
		return "", failure(stepTenant, ports.ClassFinalValidation, "the execution context resolves no tenant")
	}
	return string(tenant), nil
}

func (k *Keycloak) storeTenant(ctx context.Context) (string, error) {
	tenant, err := k.tenantOf(ctx)
	if err == nil && tenant == k.operator {
		return "", failure(stepTenant, ports.ClassFinalValidation, "a store command never runs in the operator tenant")
	}
	return tenant, err
}

func (k *Keycloak) syncRole(ctx context.Context, payload []byte) (string, error) {
	tenant, err := k.storeTenant(ctx)
	if err != nil {
		return "", err
	}
	access, err := decodeRoleAccess(payload)
	if err != nil {
		return "", invalidPayload(err)
	}
	if access.Ref == domain.OwnerRole {
		return "", failure(stepPayload, ports.ClassFinalValidation, "the owner group follows the seed, never a command")
	}
	org, err := k.organization(ctx, tenant)
	if err != nil {
		return "", err
	}
	client, err := k.storeClient(ctx)
	if err != nil {
		return "", err
	}
	catalog, err := k.clientRoles(ctx, client)
	if err != nil {
		return "", err
	}
	want := map[string]keycloakRole{}
	if access.Status == domain.RoleStatusActive {
		for _, permission := range access.Permissions {
			role, known := catalog[string(permission)]
			if !known {
				return "", failure(routeListClientRoles, ports.ClassFinalValidation, "a permission is no client role of "+keycloakStoreClient)
			}
			want[role.Name] = role
		}
	}
	group, present, err := k.roleGroup(ctx, org, access.Ref, access.Status)
	if err != nil || !present {
		return "", err
	}
	mapped, err := k.groupRoles(ctx, org, group, client)
	if err != nil {
		return "", err
	}
	have := map[string]keycloakRole{}
	for _, role := range mapped {
		have[role.Name] = role
	}
	// WHY: taking before granting leaves a failure halfway with a privilege
	// too few, never one too many.
	target := k.admin("organizations", org, "groups", group, "role-mappings", "clients", client)
	if extra := missingFrom(have, want); len(extra) > 0 {
		if _, err := k.write(ctx, routeRemoveGroupRoles, target, extra); err != nil {
			return "", err
		}
	}
	if missing := missingFrom(want, have); len(missing) > 0 {
		if _, err := k.write(ctx, routeAddGroupRoles, target, missing); err != nil {
			return "", err
		}
	}
	return "", nil
}

// roleGroup is the group of the role; a retired role whose group is absent
// already holds no client role, so it is converged without creating one.
func (k *Keycloak) roleGroup(ctx context.Context, org string, ref domain.RoleRef, status domain.RoleStatus) (string, bool, error) {
	if status == domain.RoleStatusRetired {
		return k.findGroup(ctx, org, string(ref))
	}
	group, err := k.ensureGroup(ctx, org, string(ref))
	return group, err == nil, err
}

// missingFrom is what of want the other set lacks, in name order so the body
// of each change is stable.
func missingFrom(want, have map[string]keycloakRole) []keycloakRole {
	var out []keycloakRole
	for name, role := range want {
		if _, held := have[name]; !held {
			out = append(out, role)
		}
	}
	slices.SortFunc(out, func(a, b keycloakRole) int { return cmp.Compare(a.Name, b.Name) })
	return out
}

func (k *Keycloak) provisionMember(ctx context.Context, payload []byte) (string, error) {
	tenant, err := k.storeTenant(ctx)
	if err != nil {
		return "", err
	}
	provision, err := decodeMemberProvision(payload)
	if err != nil {
		return "", invalidPayload(err)
	}
	org, err := k.organization(ctx, tenant)
	if err != nil {
		return "", err
	}
	user, err := k.ensureUser(ctx, string(provision.Email))
	if err != nil {
		return "", err
	}
	if err := k.ensureMember(ctx, org, user); err != nil {
		return "", err
	}
	return user, nil
}

func (k *Keycloak) memberAccess(ctx context.Context, payload []byte) (string, error) {
	tenant, err := k.storeTenant(ctx)
	if err != nil {
		return "", err
	}
	access, err := decodeMemberAccess(payload)
	if err != nil {
		return "", invalidPayload(err)
	}
	org, err := k.organization(ctx, tenant)
	if err != nil {
		return "", err
	}
	if access.Status == domain.MembershipStatusActive {
		return "", k.grantMember(ctx, org, string(access.Subject), access.Roles)
	}
	return "", k.revokeMember(ctx, org, string(access.Subject))
}

// grantMember reads every group of the roles before the first change, so a
// store without its owner group is refused with nothing written; the member
// enters the Organization before any of its groups.
func (k *Keycloak) grantMember(ctx context.Context, org, user string, roles []domain.RoleRef) error {
	want := map[string]bool{}
	var absent []string
	for _, role := range slices.Compact(slices.Sorted(slices.Values(roles))) {
		id, found, err := k.findGroup(ctx, org, string(role))
		switch {
		case err != nil:
			return err
		case found:
			want[id] = true
		case role == domain.OwnerRole:
			return failure(routeFindGroups, ports.ClassFinalBusiness, "the store has no owner group")
		default:
			absent = append(absent, string(role))
		}
	}
	if err := k.ensureMember(ctx, org, user); err != nil {
		return err
	}
	for _, name := range absent {
		id, err := k.ensureGroup(ctx, org, name)
		if err != nil {
			return err
		}
		want[id] = true
	}
	current, found, err := k.memberGroups(ctx, org, user)
	if err != nil {
		return err
	}
	if !found {
		return refused(routeListMemberGroups, http.StatusNotFound)
	}
	have := map[string]bool{}
	for _, group := range current {
		have[group.ID] = true
	}
	// WHY: taking before granting leaves a failure halfway with a group too
	// few, never one too many, as revokeMember does.
	for _, id := range slices.Sorted(maps.Keys(have)) {
		if !want[id] {
			if err := k.removeGroupMember(ctx, org, id, user); err != nil {
				return err
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(want)) {
		if !have[id] {
			if err := k.addGroupMember(ctx, org, id, user); err != nil {
				return err
			}
		}
	}
	return nil
}

// revokeMember takes the member out of its groups before the Organization.
// Keycloak 26.8.1 drops the groups along with the membership; removing them
// first keeps that from being what alone stands between a user outside the
// store and the roles of its groups.
func (k *Keycloak) revokeMember(ctx context.Context, org, user string) error {
	current, found, err := k.memberGroups(ctx, org, user)
	if err != nil || !found {
		return err
	}
	for _, group := range current {
		if err := k.removeGroupMember(ctx, org, group.ID, user); err != nil {
			return err
		}
	}
	_, err = k.write(ctx, routeRemoveMember, k.admin("organizations", org, "members", user), nil, http.StatusNotFound)
	return err
}

func (k *Keycloak) buyerAccess(ctx context.Context, payload []byte) (string, error) {
	tenant, err := k.tenantOf(ctx)
	if err != nil {
		return "", err
	}
	if tenant != k.operator {
		return "", failure(stepTenant, ports.ClassFinalValidation, "a buyer command runs only in the operator tenant")
	}
	access, err := decodeBuyerAccess(payload)
	if err != nil {
		return "", invalidPayload(err)
	}
	role, err := k.realmRole(ctx, keycloakBuyerRole)
	if err != nil {
		return "", err
	}
	mapped, err := k.userRealmRoles(ctx, string(access.Subject))
	if err != nil {
		return "", err
	}
	holds := slices.ContainsFunc(mapped, func(r keycloakRole) bool { return r.Name == keycloakBuyerRole })
	target := k.admin("users", string(access.Subject), "role-mappings", "realm")
	switch {
	case access.Status == domain.BuyerStatusActive && !holds:
		_, err = k.write(ctx, routeAddUserRealmRoles, target, []keycloakRole{role})
	case access.Status == domain.BuyerStatusBlocked && holds:
		_, err = k.write(ctx, routeRemoveUserRealmRoles, target, []keycloakRole{role})
	}
	return "", err
}
