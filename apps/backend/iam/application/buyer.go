package application

import (
	"context"
	"fmt"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func (s Service) RegisterBuyer(ctx context.Context, cmd RegisterBuyer) (usecase.Outcome[domain.RegisteredResponse], error) {
	return usecase.Execute(ctx, s.executor(), command[domain.RegisteredResponse]{
		Operation:   OperationRegisterBuyer,
		Object:      string(cmd.Subject),
		Input:       cmd,
		Fingerprint: usecase.NewFingerprint(OperationRegisterBuyer).String(string(cmd.Subject)).String(string(cmd.Name)),
		Codec:       registeredCodec,
		Run: func(ctx context.Context, res Resources, identity usecase.Identity) (usecase.Outcome[domain.RegisteredResponse], error) {
			outcome, err := usecase.Decide(ctx, res.Buyers, res.Outbox, buyerOrigin(cmd.Subject), cmd.Subject, identity,
				loadBuyer,
				func(b *domain.Buyer) (kernel.Accepted[domain.RegisteredResponse], *kernel.Rejection) {
					return b.Register(domain.RegisterBuyer{Subject: cmd.Subject, Name: cmd.Name, At: identity.OccurredAt})
				})
			if err != nil {
				return outcome, fmt.Errorf("application: register buyer %s: %w", cmd.Subject, err)
			}
			return outcome, nil
		},
	})
}

func (s Service) UpdateBuyerProfile(ctx context.Context, cmd UpdateBuyerProfile) (usecase.Outcome[domain.ProfileUpdatedResponse], error) {
	return usecase.Execute(ctx, s.executor(), command[domain.ProfileUpdatedResponse]{
		Operation:   OperationUpdateBuyerProfile,
		Object:      string(cmd.Subject),
		Input:       cmd,
		Fingerprint: usecase.NewFingerprint(OperationUpdateBuyerProfile).String(string(cmd.Subject)).String(string(cmd.Name)),
		Codec:       profileUpdatedCodec,
		Run: func(ctx context.Context, res Resources, identity usecase.Identity) (usecase.Outcome[domain.ProfileUpdatedResponse], error) {
			outcome, err := usecase.Decide(ctx, res.Buyers, res.Outbox, buyerOrigin(cmd.Subject), cmd.Subject, identity,
				loadExistingBuyer,
				func(b *domain.Buyer) (kernel.Accepted[domain.ProfileUpdatedResponse], *kernel.Rejection) {
					return b.UpdateProfile(domain.UpdateBuyerProfile{Name: cmd.Name, At: identity.OccurredAt})
				})
			if err != nil {
				return outcome, fmt.Errorf("application: update buyer profile %s: %w", cmd.Subject, err)
			}
			return outcome, nil
		},
	})
}

func (s Service) BlockBuyer(ctx context.Context, cmd BlockBuyer) (usecase.Outcome[domain.BlockedResponse], error) {
	return usecase.Execute(ctx, s.executor(), command[domain.BlockedResponse]{
		Operation:   OperationBlockBuyer,
		Object:      string(cmd.Subject),
		Input:       cmd,
		Fingerprint: usecase.NewFingerprint(OperationBlockBuyer).String(string(cmd.Subject)).String(cmd.Reason),
		Codec:       blockedCodec,
		Run: func(ctx context.Context, res Resources, identity usecase.Identity) (usecase.Outcome[domain.BlockedResponse], error) {
			outcome, err := usecase.Decide(ctx, res.Buyers, res.Outbox, buyerOrigin(cmd.Subject), cmd.Subject, identity,
				loadExistingBuyer,
				func(b *domain.Buyer) (kernel.Accepted[domain.BlockedResponse], *kernel.Rejection) {
					return b.Block(domain.BlockBuyer{Reason: cmd.Reason, At: identity.OccurredAt})
				})
			if err != nil {
				return outcome, fmt.Errorf("application: block buyer %s: %w", cmd.Subject, err)
			}
			return outcome, nil
		},
	})
}

func (s Service) UnblockBuyer(ctx context.Context, cmd UnblockBuyer) (usecase.Outcome[domain.UnblockedResponse], error) {
	return usecase.Execute(ctx, s.executor(), command[domain.UnblockedResponse]{
		Operation:   OperationUnblockBuyer,
		Object:      string(cmd.Subject),
		Input:       cmd,
		Fingerprint: usecase.NewFingerprint(OperationUnblockBuyer).String(string(cmd.Subject)),
		Codec:       unblockedCodec,
		Run: func(ctx context.Context, res Resources, identity usecase.Identity) (usecase.Outcome[domain.UnblockedResponse], error) {
			outcome, err := usecase.Decide(ctx, res.Buyers, res.Outbox, buyerOrigin(cmd.Subject), cmd.Subject, identity,
				loadExistingBuyer,
				func(b *domain.Buyer) (kernel.Accepted[domain.UnblockedResponse], *kernel.Rejection) {
					return b.Unblock(domain.UnblockBuyer{At: identity.OccurredAt})
				})
			if err != nil {
				return outcome, fmt.Errorf("application: unblock buyer %s: %w", cmd.Subject, err)
			}
			return outcome, nil
		},
	})
}

func (s Service) FindBuyer(ctx context.Context, subject domain.Subject) (domain.BuyerSnapshot, error) {
	return usecase.Query[Operation](ctx, s.instrumentation(), s.Authorize, OperationFindBuyer, FindBuyer{Subject: subject},
		func(ctx context.Context) (domain.BuyerSnapshot, error) {
			snapshot, _, err := s.Buyers.Load(ctx, subject)
			if err != nil {
				return domain.BuyerSnapshot{}, fmt.Errorf("application: find buyer %s: %w", subject, err)
			}
			return snapshot, nil
		})
}

func (s Service) FindBuyersByStatus(ctx context.Context, status domain.BuyerStatus) ([]domain.BuyerSnapshot, error) {
	return findByStatus(ctx, s, OperationFindBuyersByStatus, FindBuyersByStatus{Status: status}, "find buyers by status", status, s.BuyersByStatus.LoadByStatus)
}
