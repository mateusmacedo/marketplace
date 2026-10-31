package domain_test

import (
	"slices"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

var specCatalog = []domain.Permission{
	"catalog:read", "catalog:write", "fulfillment:read", "fulfillment:write",
	"iam:read", "iam:write", "inventory:read", "inventory:write",
	"ordering:read", "ordering:write", "pricing:read", "pricing:write",
	"seller:read", "seller:write", "settlement:read", "settlement:write",
}

func TestTheCatalogIsTheClosedListOfTheSpec(t *testing.T) {
	if got := domain.Catalog(); !slices.Equal(got, specCatalog) {
		t.Fatalf("Catalog() = %v, want %v", got, specCatalog)
	}
}

func TestTheCatalogIsACopy(t *testing.T) {
	domain.Catalog()[0] = "billing:write"

	if got := domain.Catalog()[0]; got != "catalog:read" {
		t.Fatalf("Catalog()[0] = %q after a caller wrote to its copy, want catalog:read", got)
	}
}

func TestARoleAcceptsTheWholeCatalog(t *testing.T) {
	_, rej := domain.NewRole(roleRef).Define(domain.DefineRole{Ref: roleRef, Name: roleName, Permissions: domain.Catalog(), At: at})

	tb.RequireAccepted(t, rej)
}

// Permissions from another origin are not grantable by a role of a store: the
// operator's realm roles and the buyer's permission stay out of the catalog.
func TestPermissionsFromAnotherOriginAreUnknown(t *testing.T) {
	for _, permission := range []domain.Permission{"tenancy:read", "tenancy:write", "iam:buyer", "profile", "catalog:admin", "CATALOG:WRITE"} {
		t.Run(string(permission), func(t *testing.T) {
			acc, rej := domain.NewRole(roleRef).Define(domain.DefineRole{Ref: roleRef, Name: roleName, Permissions: []domain.Permission{permission}, At: at})

			tb.RequireRejected(t, acc, rej, domain.CodeRolePermissionUnknown)
		})
	}
}
