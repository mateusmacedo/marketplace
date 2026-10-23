//go:build integration

package app_test

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"

	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/app/rpc"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/appkit"
	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/tenancy/contract/gen/go/company/tenancy/service/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

type auditSpy struct {
	mu     sync.Mutex
	audits []ports.AuditEvent
}

func (s *auditSpy) BeginOperation(ctx context.Context, _ string) (context.Context, ports.EndOperation) {
	return ctx, func(ports.Result) {}
}

func (s *auditSpy) Audit(_ context.Context, event ports.AuditEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audits = append(s.audits, event)
}

func (s *auditSpy) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.audits)
}

func register(t *testing.T, conn *grpc.ClientConn, ctx context.Context, ref, name string, opts ...grpc.CallOption) (*servicev1.RegisterStoreResponse, error) {
	t.Helper()
	var resp servicev1.RegisterStoreResponse
	err := conn.Invoke(ctx, rpc.FullMethod("RegisterStore"), &servicev1.RegisterStoreRequest{Ref: ref, Name: name}, &resp, opts...)
	return &resp, err
}

func TestADuplicateNameOverGRPCIsRefusedByTheOutcomeAndReplayedWithoutASecondAudit(t *testing.T) {
	pool := appkit.OpenPool(t)
	spy := &auditSpy{}
	conn := dialWith(t, pool, spy)
	if resp, err := register(t, conn, callWithKey(t, operatorTenant, "e2e-name-1"), "loja-acme", "Loja Acme"); err != nil || resp.GetRegistered() == nil {
		t.Fatalf("RegisterStore(first) = %v, %v; want Registered", resp, err)
	}

	refused, err := register(t, conn, callWithKey(t, operatorTenant, "e2e-name-2"), "loja-outra", "Loja Acme")
	if err != nil || refused.GetRejection().GetCode() != string(domain.CodeStoreDuplicateName) {
		t.Fatalf("RegisterStore(same name) = %v, %v; want the rejection %s by the outcome, never a status", refused, err, domain.CodeStoreDuplicateName)
	}
	var header metadata.MD
	replayed, err := register(t, conn, callWithKey(t, operatorTenant, "e2e-name-2"), "loja-outra", "Loja Acme", grpc.Header(&header))
	if err != nil || replayed.GetRejection().GetCode() != string(domain.CodeStoreDuplicateName) || !slices.Equal(header.Get(kernelgrpc.ReplayedHeader), []string{"true"}) {
		t.Fatalf("replay = %v, %v with header %v; want the same rejection marked as replay", replayed, err, header)
	}

	if got := spy.count(); got != 2 {
		t.Fatalf("audits = %d, want 2: the registration and the refusal, never the replay (IDM-08)", got)
	}
	if n := outboxRows(t, pool); n != 1 {
		t.Fatalf("outbox rows = %d, want 1: the refusal enqueues nothing", n)
	}
}

func TestOneNameRegisteredConcurrentlyLetsExactlyOneThrough(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)
	const rounds = 20
	for round := range rounds {
		name := fmt.Sprintf("Loja %d", round)
		outcomes := registerTogether(t, conn, [2]string{fmt.Sprintf("loja-a-%d", round), fmt.Sprintf("loja-b-%d", round)}, name)

		registered, refused := 0, 0
		for _, o := range outcomes {
			switch {
			case o.err != nil:
				t.Fatalf("round %d: RegisterStore() = %v, want an outcome, never %v", round, o.err, status.Code(o.err))
			case o.resp.GetRegistered() != nil:
				registered++
			case o.resp.GetRejection().GetCode() == string(domain.CodeStoreDuplicateName):
				refused++
			}
		}
		if registered != 1 || refused != 1 {
			t.Fatalf("round %d: %d registered and %d refused, want exactly one of each", round, registered, refused)
		}
	}
	if n := outboxRows(t, pool); n != rounds {
		t.Fatalf("outbox rows = %d, want %d: one event per accepted registration", n, rounds)
	}
}

func TestOneRefAndNameRegisteredConcurrentlyIsNeverADuplicateName(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)
	for round := range 20 {
		ref := fmt.Sprintf("loja-same-%d", round)
		outcomes := registerTogether(t, conn, [2]string{ref, ref}, fmt.Sprintf("Mesma %d", round))

		for _, o := range outcomes {
			if o.err != nil && status.Code(o.err) != codes.Aborted {
				t.Fatalf("round %d: RegisterStore() = %v, want Registered or Aborted: a race on the identity stays a version conflict", round, o.err)
			}
			if code := o.resp.GetRejection().GetCode(); code != "" {
				t.Fatalf("round %d: rejection %s, want none: the same identity is never a duplicate name", round, code)
			}
		}
	}
}

type registration struct {
	resp *servicev1.RegisterStoreResponse
	err  error
}

func registerTogether(t *testing.T, conn *grpc.ClientConn, refs [2]string, name string) [2]registration {
	t.Helper()
	var (
		outcomes [2]registration
		start    = make(chan struct{})
		done     sync.WaitGroup
	)
	for i, ref := range refs {
		ctx := call(t, operatorTenant)
		done.Go(func() {
			<-start
			outcomes[i].resp, outcomes[i].err = register(t, conn, ctx, ref, name)
		})
	}
	close(start)
	done.Wait()
	return outcomes
}
