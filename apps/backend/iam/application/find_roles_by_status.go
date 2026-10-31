package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func (s Service) FindRolesByStatus(ctx context.Context, status domain.RoleStatus) ([]domain.RoleSnapshot, error) {
	return findByStatus(ctx, s, OperationFindRolesByStatus, FindRolesByStatus{Status: status}, "find roles by status", status, s.RolesByStatus.LoadByStatus)
}

// findByStatus is the query of many aggregates in one status, outside any unit
// of work (UOW-11), for every aggregate of the context alike.
func findByStatus[St fmt.Stringer, S any](ctx context.Context, s Service, name string, op Operation, what string, status St, load func(context.Context, St) ([]S, error)) ([]S, error) {
	ctx, end := s.instrumentation().BeginOperation(ctx, name)

	if err := s.Authorize(ctx, op); err != nil {
		end(ports.AuthorizationResult(err))
		return nil, err
	}

	snapshots, err := load(ctx, status)
	var access ports.CrossTenantAccess
	switch {
	case errors.As(err, &access):
		// IDN-13: the caller gets the empty answer of a status nobody holds;
		// the instrumentation still gets the access, which IDN-12 records.
		end(ports.Result{Outcome: ports.OutcomeFailed, Err: access})
		return []S{}, nil
	case err != nil:
		failed := fmt.Errorf("application: %s %s: %w", what, status, err)
		end(ports.Result{Outcome: ports.OutcomeFailed, Err: failed})
		return nil, failed
	}

	end(ports.Result{Outcome: ports.OutcomeAccepted})
	if snapshots == nil {
		return []S{}, nil
	}
	return snapshots, nil
}
