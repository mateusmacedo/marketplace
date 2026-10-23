// Package app is the public edge of the marketplace: the only REST/JSON
// surface, which authenticates the caller against the identity provider and
// resolves the tenant before any route runs. It has no domain of its own; the
// bounded contexts are reached over gRPC through package rpc as they arrive.
package app
