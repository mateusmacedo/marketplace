// Package rpc holds the gRPC clients of the edge, one per bounded context it
// forwards to. It is empty until the first context: each context brings its
// client here, with the transport policy and the readiness of its connection.
package rpc
