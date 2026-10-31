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

	"github.com/mateusmacedo/marketplace/apps/backend/iam/app/rpc"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/appkit"
	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/service/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
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

func defineRole(t *testing.T, conn *grpc.ClientConn, ctx context.Context, ref, name string, opts ...grpc.CallOption) (*servicev1.DefineRoleResponse, error) {
	t.Helper()
	var resp servicev1.DefineRoleResponse
	err := conn.Invoke(ctx, rpc.FullMethod("DefineRole"), defineRequest(ref, name), &resp, opts...)
	return &resp, err
}

func TestADuplicateNameOverGRPCIsRefusedByTheOutcomeAndReplayedWithoutASecondAudit(t *testing.T) {
	pool := appkit.OpenPool(t)
	spy := &auditSpy{}
	conn := dialWith(t, pool, spy)
	if resp, err := defineRole(t, conn, callWithKey(t, storeTenant, "e2e-name-1"), "expedicao", "Expedição"); err != nil || resp.GetDefined() == nil {
		t.Fatalf("DefineRole(first) = %v, %v; want Defined", resp, err)
	}

	refused, err := defineRole(t, conn, callWithKey(t, storeTenant, "e2e-name-2"), "outro-papel", "Expedição")
	if err != nil || refused.GetRejection().GetCode() != string(domain.CodeRoleDuplicateName) {
		t.Fatalf("DefineRole(same name) = %v, %v; want the rejection %s by the outcome, never a status", refused, err, domain.CodeRoleDuplicateName)
	}
	var header metadata.MD
	replayed, err := defineRole(t, conn, callWithKey(t, storeTenant, "e2e-name-2"), "outro-papel", "Expedição", grpc.Header(&header))
	if err != nil || replayed.GetRejection().GetCode() != string(domain.CodeRoleDuplicateName) || !slices.Equal(header.Get(kernelgrpc.ReplayedHeader), []string{"true"}) {
		t.Fatalf("replay = %v, %v with header %v; want the same rejection marked as replay", replayed, err, header)
	}

	if got := spy.count(); got != 2 {
		t.Fatalf("audits = %d, want 2: the definition and the refusal, never the replay (IDM-08)", got)
	}
	if n := outboxRows(t, pool); n != 1 {
		t.Fatalf("outbox rows = %d, want 1: the refusal enqueues nothing", n)
	}
}

// The name is unique per tenant (ADR-066): another store may reuse it.
func TestTheSameNameInAnotherStoreIsAccepted(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)
	if resp, err := defineRole(t, conn, call(t, storeTenant), "expedicao", "Expedição"); err != nil || resp.GetDefined() == nil {
		t.Fatalf("DefineRole(loja-acme) = %v, %v; want Defined", resp, err)
	}

	resp, err := defineRole(t, conn, call(t, "loja-outra"), "outro-papel", "Expedição")

	if err != nil || resp.GetDefined().GetRef() != "outro-papel" {
		t.Fatalf("DefineRole(loja-outra, same name) = %v, %v; want Defined", resp, err)
	}
}

func TestOneNameDefinedConcurrentlyLetsExactlyOneThrough(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)
	const rounds = 20
	for round := range rounds {
		name := fmt.Sprintf("Papel %d", round)
		outcomes := defineTogether(t, conn, [2]string{fmt.Sprintf("papel-a-%d", round), fmt.Sprintf("papel-b-%d", round)}, name)

		defined, refused := 0, 0
		for _, o := range outcomes {
			switch {
			case o.err != nil:
				t.Fatalf("round %d: DefineRole() = %v, want an outcome, never %v", round, o.err, status.Code(o.err))
			case o.resp.GetDefined() != nil:
				defined++
			case o.resp.GetRejection().GetCode() == string(domain.CodeRoleDuplicateName):
				refused++
			}
		}
		if defined != 1 || refused != 1 {
			t.Fatalf("round %d: %d defined and %d refused, want exactly one of each", round, defined, refused)
		}
	}
	if n := outboxRows(t, pool); n != rounds {
		t.Fatalf("outbox rows = %d, want %d: one event per accepted definition", n, rounds)
	}
}

func TestOneRefAndNameDefinedConcurrentlyIsNeverADuplicateName(t *testing.T) {
	pool := appkit.OpenPool(t)
	conn := dial(t, pool)
	for round := range 20 {
		ref := fmt.Sprintf("papel-mesmo-%d", round)
		outcomes := defineTogether(t, conn, [2]string{ref, ref}, fmt.Sprintf("Mesmo %d", round))

		for _, o := range outcomes {
			if o.err != nil && status.Code(o.err) != codes.Aborted {
				t.Fatalf("round %d: DefineRole() = %v, want Defined or Aborted: a race on the identity stays a version conflict", round, o.err)
			}
			if code := o.resp.GetRejection().GetCode(); code != "" {
				t.Fatalf("round %d: rejection %s, want none: the same identity is never a duplicate name", round, code)
			}
		}
	}
}

type definition struct {
	resp *servicev1.DefineRoleResponse
	err  error
}

func defineTogether(t *testing.T, conn *grpc.ClientConn, refs [2]string, name string) [2]definition {
	t.Helper()
	var (
		outcomes [2]definition
		start    = make(chan struct{})
		done     sync.WaitGroup
	)
	for i, ref := range refs {
		ctx := call(t, storeTenant)
		done.Go(func() {
			<-start
			outcomes[i].resp, outcomes[i].err = defineRole(t, conn, ctx, ref, name)
		})
	}
	close(start)
	done.Wait()
	return outcomes
}
