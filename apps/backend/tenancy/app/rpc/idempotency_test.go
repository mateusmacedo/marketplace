package rpc_test

import (
	"slices"
	"testing"

	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/app/rpc"
	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/tenancy/contract/gen/go/company/tenancy/service/v1"
)

func TestEveryMethodThatIsNotAQueryIsACommand(t *testing.T) {
	commands := rpc.Commands()
	for _, command := range commands {
		if !slices.Contains(rpc.Methods(), command) {
			t.Errorf("command %s is not a method of %s", command, rpc.ServiceName)
		}
	}
	for _, method := range rpc.Methods() {
		query := method == "FindStore" || method == "FindStoresByStatus"
		if query == slices.Contains(commands, method) {
			t.Errorf("method %s: query %v, command %v; every method is exactly one of the two", method, query, !query)
		}
	}
}

func TestACommandWithoutAKeyIsRefusedBeforeTheHandler(t *testing.T) {
	h := newHarness(t, nil)

	var resp servicev1.RegisterStoreResponse
	err := h.invoke(withoutKey(t), "RegisterStore", &servicev1.RegisterStoreRequest{Ref: "loja-acme", Name: "Loja Acme"}, &resp)

	if status.Code(err) != codes.InvalidArgument || kernelgrpc.ReasonOf(err) != kernelgrpc.ReasonMissingIdempotencyKey {
		t.Fatalf("RegisterStore() = %v (reason %q), want InvalidArgument with %s", err, kernelgrpc.ReasonOf(err), kernelgrpc.ReasonMissingIdempotencyKey)
	}
}

func TestACommandWithAMalformedKeyIsRefused(t *testing.T) {
	h := newHarness(t, nil)

	var resp servicev1.RegisterStoreResponse
	err := h.invoke(withKey(t, "not a key"), "RegisterStore", &servicev1.RegisterStoreRequest{Ref: "loja-acme", Name: "Loja Acme"}, &resp)

	if status.Code(err) != codes.InvalidArgument || kernelgrpc.ReasonOf(err) != kernelgrpc.ReasonInvalidIdempotencyKey {
		t.Fatalf("RegisterStore() = %v (reason %q), want InvalidArgument with %s", err, kernelgrpc.ReasonOf(err), kernelgrpc.ReasonInvalidIdempotencyKey)
	}
}

func TestARepeatedCommandAnswersTheSameResponseMarkedAsReplay(t *testing.T) {
	h := newHarness(t, nil)
	req := &servicev1.RegisterStoreRequest{Ref: "loja-acme", Name: "Loja Acme"}

	var first, again servicev1.RegisterStoreResponse
	var firstHeader, againHeader metadata.MD
	if err := h.invoke(withKey(t, "k-replay"), "RegisterStore", req, &first, grpc.Header(&firstHeader)); err != nil {
		t.Fatalf("first RegisterStore() = %v", err)
	}
	if err := h.invoke(withKey(t, "k-replay"), "RegisterStore", req, &again, grpc.Header(&againHeader)); err != nil {
		t.Fatalf("repeated RegisterStore() = %v, want the stored response", err)
	}

	if again.GetRegistered().GetRef() != first.GetRegistered().GetRef() {
		t.Fatalf("replay = %v, want %v", &again, &first)
	}
	if got := firstHeader.Get(kernelgrpc.ReplayedHeader); len(got) != 0 {
		t.Fatalf("first response header %s = %v, want none", kernelgrpc.ReplayedHeader, got)
	}
	if got := againHeader.Get(kernelgrpc.ReplayedHeader); !slices.Equal(got, []string{"true"}) {
		t.Fatalf("replay header %s = %v, want [true]", kernelgrpc.ReplayedHeader, got)
	}
}

func TestTheSameKeyWithAnotherRequestIsReused(t *testing.T) {
	h := newHarness(t, nil)
	var resp servicev1.RegisterStoreResponse
	if err := h.invoke(withKey(t, "k-reused"), "RegisterStore", &servicev1.RegisterStoreRequest{Ref: "loja-acme", Name: "Loja Acme"}, &resp); err != nil {
		t.Fatalf("first RegisterStore() = %v", err)
	}

	err := h.invoke(withKey(t, "k-reused"), "RegisterStore", &servicev1.RegisterStoreRequest{Ref: "loja-acme", Name: "Outro nome"}, &resp)

	if status.Code(err) != codes.FailedPrecondition || kernelgrpc.ReasonOf(err) != kernelgrpc.ReasonReusedIdempotencyKey {
		t.Fatalf("RegisterStore(other name) = %v (reason %q), want FailedPrecondition with %s", err, kernelgrpc.ReasonOf(err), kernelgrpc.ReasonReusedIdempotencyKey)
	}
}
