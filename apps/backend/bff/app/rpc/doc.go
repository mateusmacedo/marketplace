// Package rpc is the gRPC side of the edge: one client per bounded context it
// forwards to, with the method names read from the generated descriptors and a
// policy per method (GRP-08, GRP-09, GRP-16) over grpc. Each context brings its
// client here; the transport, the metadata of the call, the replay mark, the
// readiness of the channels and the HTTP face of a failure are shared.
package rpc
