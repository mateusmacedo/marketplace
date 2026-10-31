package domain

import "slices"

// Permission is the right to read or write a context, as the route of the bff
// demands it: <context>:read or <context>:write.
type Permission string

// catalog is the closed set a role of a store may hold, sorted. Each entry is
// a client role of marketplace-store in Keycloak.
var catalog = []Permission{
	"catalog:read", "catalog:write", "fulfillment:read", "fulfillment:write",
	"iam:read", "iam:write", "inventory:read", "inventory:write",
	"ordering:read", "ordering:write", "pricing:read", "pricing:write",
	"seller:read", "seller:write", "settlement:read", "settlement:write",
}

// Catalog is every permission a role may hold, which is also what the owner
// holds.
func Catalog() []Permission {
	return slices.Clone(catalog)
}

func inCatalog(p Permission) bool {
	_, found := slices.BinarySearch(catalog, p)
	return found
}
