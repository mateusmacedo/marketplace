package domain

import (
	"strconv"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
)

type Buyer struct {
	subject       Subject
	name          BuyerName
	status        BuyerStatus
	blockReason   string
	accessVersion int64
	syncedVersion int64
	registeredAt  kernel.Instant
	changedAt     kernel.Instant
}

func NewBuyer(subject Subject) *Buyer {
	return &Buyer{subject: subject}
}

func FromBuyerSnapshot(s BuyerSnapshot) *Buyer {
	return &Buyer{
		subject:       s.Subject,
		name:          s.Name,
		status:        s.Status,
		blockReason:   s.BlockReason,
		accessVersion: s.AccessVersion,
		syncedVersion: s.SyncedVersion,
		registeredAt:  s.RegisteredAt,
		changedAt:     s.ChangedAt,
	}
}

type BuyerSnapshot struct {
	Subject       Subject
	Name          BuyerName
	Status        BuyerStatus
	BlockReason   string
	AccessVersion int64
	SyncedVersion int64
	RegisteredAt  kernel.Instant
	ChangedAt     kernel.Instant
}

func (b *Buyer) Snapshot() BuyerSnapshot {
	return BuyerSnapshot{
		Subject:       b.subject,
		Name:          b.name,
		Status:        b.status,
		BlockReason:   b.blockReason,
		AccessVersion: b.accessVersion,
		SyncedVersion: b.syncedVersion,
		RegisteredAt:  b.registeredAt,
		ChangedAt:     b.changedAt,
	}
}

func (s BuyerSnapshot) Equal(other BuyerSnapshot) bool {
	return s == other
}

func (b *Buyer) clone() Buyer {
	return *b
}

// Register is idempotent by identity: a known buyer, blocked included, is
// accepted as it stands, so registering again never lifts a block.
func (b *Buyer) Register(cmd RegisterBuyer) (kernel.Accepted[RegisteredResponse], *kernel.Rejection) {
	return kernel.DecideOver(b, (*Buyer).clone, func(next *Buyer) (kernel.Accepted[RegisteredResponse], *kernel.Rejection) {
		if !ValidSubject(cmd.Subject) {
			return kernel.Refuse[RegisteredResponse](CodeBuyerSubjectInvalid, SubjectInvalidMessage)
		}
		if cmd.Name == "" {
			return kernel.Refuse[RegisteredResponse](CodeBuyerNameEmpty, "name must not be empty")
		}
		if next.status != BuyerStatusUndefined {
			return kernel.Accept(RegisteredResponse{Subject: next.subject, Status: next.status}), nil
		}
		next.name = cmd.Name
		next.status = BuyerStatusActive
		next.blockReason = ""
		next.accessVersion = 1
		next.syncedVersion = 0
		next.registeredAt = cmd.At
		next.changedAt = cmd.At
		return kernel.Accept(
			RegisteredResponse{Subject: next.subject, Status: next.status},
			BuyerRegistered{Subject: next.subject, Name: next.name, AccessVersion: next.accessVersion, At: cmd.At},
		), nil
	})
}

func (b *Buyer) UpdateProfile(cmd UpdateBuyerProfile) (kernel.Accepted[ProfileUpdatedResponse], *kernel.Rejection) {
	return kernel.DecideOver(b, (*Buyer).clone, func(next *Buyer) (kernel.Accepted[ProfileUpdatedResponse], *kernel.Rejection) {
		if next.status != BuyerStatusActive {
			return kernel.Refuse[ProfileUpdatedResponse](CodeBuyerBlocked, "buyer is blocked")
		}
		if cmd.Name == "" {
			return kernel.Refuse[ProfileUpdatedResponse](CodeBuyerNameEmpty, "name must not be empty")
		}
		next.name = cmd.Name
		next.changedAt = cmd.At
		return kernel.Accept(
			ProfileUpdatedResponse{Subject: next.subject},
			BuyerProfileUpdated{Subject: next.subject, Name: next.name, At: cmd.At},
		), nil
	})
}

// Block takes the permission to buy and keeps the user: the same person may
// still operate a store, so nothing here disables it in Keycloak.
func (b *Buyer) Block(cmd BlockBuyer) (kernel.Accepted[BlockedResponse], *kernel.Rejection) {
	return kernel.DecideOver(b, (*Buyer).clone, func(next *Buyer) (kernel.Accepted[BlockedResponse], *kernel.Rejection) {
		if next.status != BuyerStatusActive {
			return kernel.Refuse[BlockedResponse](CodeBuyerNotActive, "buyer is not active")
		}
		if cmd.Reason == "" {
			return kernel.Refuse[BlockedResponse](CodeBuyerReasonEmpty, "reason must not be empty")
		}
		next.status = BuyerStatusBlocked
		next.blockReason = cmd.Reason
		next.accessVersion++
		next.changedAt = cmd.At
		return kernel.Accept(
			BlockedResponse{Subject: next.subject},
			BuyerBlocked{Subject: next.subject, Reason: next.blockReason, AccessVersion: next.accessVersion, At: cmd.At},
		), nil
	})
}

func (b *Buyer) Unblock(cmd UnblockBuyer) (kernel.Accepted[UnblockedResponse], *kernel.Rejection) {
	return kernel.DecideOver(b, (*Buyer).clone, func(next *Buyer) (kernel.Accepted[UnblockedResponse], *kernel.Rejection) {
		if next.status != BuyerStatusBlocked {
			return kernel.Refuse[UnblockedResponse](CodeBuyerNotBlocked, "buyer is not blocked")
		}
		next.status = BuyerStatusActive
		next.blockReason = ""
		next.accessVersion++
		next.changedAt = cmd.At
		return kernel.Accept(
			UnblockedResponse{Subject: next.subject},
			BuyerUnblocked{Subject: next.subject, AccessVersion: next.accessVersion, At: cmd.At},
		), nil
	})
}

// ConfirmAccessSynced keeps the highest version confirmed, so a confirmation
// that arrives late never undoes a newer one.
func (b *Buyer) ConfirmAccessSynced(cmd ConfirmBuyerAccessSynced) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
	return kernel.DecideOver(b, (*Buyer).clone, func(next *Buyer) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
		if cmd.Version > next.accessVersion {
			return kernel.Refuse[kernel.Empty](CodeBuyerVersionAhead, "version is ahead of the access version of the buyer",
				kernel.Detail{Key: "version", Value: strconv.FormatInt(cmd.Version, 10)},
				kernel.Detail{Key: "accessVersion", Value: strconv.FormatInt(next.accessVersion, 10)},
			)
		}
		next.syncedVersion = max(next.syncedVersion, cmd.Version)
		return kernel.Accept(kernel.Empty{}), nil
	})
}
