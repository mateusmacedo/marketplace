package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

func (s Service) FindStoresByStatus(ctx context.Context, status domain.StoreStatus) ([]domain.StoreSnapshot, error) {
	ctx, end := s.instrumentation().BeginOperation(ctx, OperationFindStoresByStatus)

	if err := s.Authorize(ctx, FindStoresByStatus{Status: status}); err != nil {
		end(ports.AuthorizationResult(err))
		return nil, err
	}

	snapshots, err := s.StatusReader.LoadByStatus(ctx, status)
	var access ports.CrossTenantAccess
	switch {
	case errors.As(err, &access):
		// IDN-13: the caller gets the empty answer of a status nobody holds;
		// the instrumentation still gets the access, which IDN-12 records.
		end(ports.Result{Outcome: ports.OutcomeFailed, Err: access})
		return []domain.StoreSnapshot{}, nil
	case err != nil:
		failed := fmt.Errorf("application: find stores by status %s: %w", status, err)
		end(ports.Result{Outcome: ports.OutcomeFailed, Err: failed})
		return nil, failed
	}

	end(ports.Result{Outcome: ports.OutcomeAccepted})
	if snapshots == nil {
		return []domain.StoreSnapshot{}, nil
	}
	return snapshots, nil
}
