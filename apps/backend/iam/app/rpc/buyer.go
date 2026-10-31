package rpc

import (
	"context"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	kernelgrpc "github.com/mateusmacedo/dmpf/libs/backend/go/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	servicev1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/service/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

// RegisterBuyer takes the subject the bff filled from the token, whose form
// the domain judges; only the name is shaped at the edge.
func (s Server) RegisterBuyer(ctx context.Context, req *servicev1.RegisterBuyerRequest) (*servicev1.RegisterBuyerResponse, error) {
	if !validText(req.GetName()) {
		return nil, invalidText("name")
	}
	return kernelgrpc.Command(ctx,
		func(ctx context.Context) (usecase.Outcome[domain.RegisteredResponse], error) {
			return s.Service.RegisterBuyer(ctx, application.RegisterBuyer{Subject: domain.Subject(req.GetSubject()), Name: domain.BuyerName(req.GetName())})
		},
		func(r *kernel.Rejection) *servicev1.RegisterBuyerResponse {
			return &servicev1.RegisterBuyerResponse{Result: &servicev1.RegisterBuyerResponse_Rejection{Rejection: rejectionOf(r)}}
		},
		func(registered domain.RegisteredResponse) *servicev1.RegisterBuyerResponse {
			return &servicev1.RegisterBuyerResponse{Result: &servicev1.RegisterBuyerResponse_Registered{
				Registered: &servicev1.Registered{Subject: string(registered.Subject), Status: buyerStatus(registered.Status)},
			}}
		})
}

func (s Server) UpdateBuyerProfile(ctx context.Context, req *servicev1.UpdateBuyerProfileRequest) (*servicev1.UpdateBuyerProfileResponse, error) {
	subject, err := subjectOf(req.GetSubject())
	if err != nil {
		return nil, err
	}
	if !validText(req.GetName()) {
		return nil, invalidText("name")
	}
	return kernelgrpc.Command(ctx,
		func(ctx context.Context) (usecase.Outcome[domain.ProfileUpdatedResponse], error) {
			return s.Service.UpdateBuyerProfile(ctx, application.UpdateBuyerProfile{Subject: subject, Name: domain.BuyerName(req.GetName())})
		},
		func(r *kernel.Rejection) *servicev1.UpdateBuyerProfileResponse {
			return &servicev1.UpdateBuyerProfileResponse{Result: &servicev1.UpdateBuyerProfileResponse_Rejection{Rejection: rejectionOf(r)}}
		},
		func(updated domain.ProfileUpdatedResponse) *servicev1.UpdateBuyerProfileResponse {
			return &servicev1.UpdateBuyerProfileResponse{Result: &servicev1.UpdateBuyerProfileResponse_ProfileUpdated{
				ProfileUpdated: &servicev1.ProfileUpdated{Subject: string(updated.Subject)},
			}}
		})
}

func (s Server) BlockBuyer(ctx context.Context, req *servicev1.BlockBuyerRequest) (*servicev1.BlockBuyerResponse, error) {
	subject, err := subjectOf(req.GetSubject())
	if err != nil {
		return nil, err
	}
	if !validText(req.GetReason()) {
		return nil, invalidText("reason")
	}
	return kernelgrpc.Command(ctx,
		func(ctx context.Context) (usecase.Outcome[domain.BlockedResponse], error) {
			return s.Service.BlockBuyer(ctx, application.BlockBuyer{Subject: subject, Reason: req.GetReason()})
		},
		func(r *kernel.Rejection) *servicev1.BlockBuyerResponse {
			return &servicev1.BlockBuyerResponse{Result: &servicev1.BlockBuyerResponse_Rejection{Rejection: rejectionOf(r)}}
		},
		func(blocked domain.BlockedResponse) *servicev1.BlockBuyerResponse {
			return &servicev1.BlockBuyerResponse{Result: &servicev1.BlockBuyerResponse_Blocked{
				Blocked: &servicev1.Blocked{Subject: string(blocked.Subject)},
			}}
		})
}

func (s Server) UnblockBuyer(ctx context.Context, req *servicev1.UnblockBuyerRequest) (*servicev1.UnblockBuyerResponse, error) {
	subject, err := subjectOf(req.GetSubject())
	if err != nil {
		return nil, err
	}
	return kernelgrpc.Command(ctx,
		func(ctx context.Context) (usecase.Outcome[domain.UnblockedResponse], error) {
			return s.Service.UnblockBuyer(ctx, application.UnblockBuyer{Subject: subject})
		},
		func(r *kernel.Rejection) *servicev1.UnblockBuyerResponse {
			return &servicev1.UnblockBuyerResponse{Result: &servicev1.UnblockBuyerResponse_Rejection{Rejection: rejectionOf(r)}}
		},
		func(unblocked domain.UnblockedResponse) *servicev1.UnblockBuyerResponse {
			return &servicev1.UnblockBuyerResponse{Result: &servicev1.UnblockBuyerResponse_Unblocked{
				Unblocked: &servicev1.Unblocked{Subject: string(unblocked.Subject)},
			}}
		})
}

func (s Server) FindBuyer(ctx context.Context, req *servicev1.FindBuyerRequest) (*servicev1.FindBuyerResponse, error) {
	subject, err := subjectOf(req.GetSubject())
	if err != nil {
		return nil, err
	}
	if _, err := kernelgrpc.ExecutionOf(ctx); err != nil {
		return nil, err
	}
	snapshot, err := s.Service.FindBuyer(ctx, subject)
	if err != nil {
		return nil, kernelgrpc.StatusOf(err)
	}
	return &servicev1.FindBuyerResponse{Buyer: buyerOf(snapshot)}, nil
}

func (s Server) FindBuyersByStatus(ctx context.Context, req *servicev1.FindBuyersByStatusRequest) (*servicev1.FindBuyersByStatusResponse, error) {
	wanted, ok := buyerStatusOf(req.GetStatus())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "status must be BUYER_STATUS_ACTIVE or BUYER_STATUS_BLOCKED")
	}
	if _, err := kernelgrpc.ExecutionOf(ctx); err != nil {
		return nil, err
	}
	snapshots, err := s.Service.FindBuyersByStatus(ctx, wanted)
	if err != nil {
		return nil, kernelgrpc.StatusOf(err)
	}
	buyers := make([]*servicev1.Buyer, 0, len(snapshots))
	for _, snapshot := range snapshots {
		buyers = append(buyers, buyerOf(snapshot))
	}
	return &servicev1.FindBuyersByStatusResponse{Buyers: buyers}, nil
}

func buyerOf(s domain.BuyerSnapshot) *servicev1.Buyer {
	return &servicev1.Buyer{
		Subject:       string(s.Subject),
		Name:          string(s.Name),
		Status:        buyerStatus(s.Status),
		BlockReason:   s.BlockReason,
		AccessVersion: s.AccessVersion,
		SyncedVersion: s.SyncedVersion,
		RegisteredAt:  int64(s.RegisteredAt),
		ChangedAt:     int64(s.ChangedAt),
	}
}

func buyerStatus(s domain.BuyerStatus) servicev1.BuyerStatus {
	switch s {
	case domain.BuyerStatusActive:
		return servicev1.BuyerStatus_BUYER_STATUS_ACTIVE
	case domain.BuyerStatusBlocked:
		return servicev1.BuyerStatus_BUYER_STATUS_BLOCKED
	default:
		return servicev1.BuyerStatus_BUYER_STATUS_UNSPECIFIED
	}
}

func buyerStatusOf(s servicev1.BuyerStatus) (domain.BuyerStatus, bool) {
	switch s {
	case servicev1.BuyerStatus_BUYER_STATUS_ACTIVE:
		return domain.BuyerStatusActive, true
	case servicev1.BuyerStatus_BUYER_STATUS_BLOCKED:
		return domain.BuyerStatusBlocked, true
	default:
		return domain.BuyerStatusUndefined, false
	}
}
