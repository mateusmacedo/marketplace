package provider

import (
	"fmt"
	"time"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventv1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/event/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

const (
	roleDefinedType            = "com.company.iam.role-defined.v1"
	rolePermissionsChangedType = "com.company.iam.role-permissions-changed.v1"
	roleRetiredType            = "com.company.iam.role-retired.v1"

	memberInvitedType         = "com.company.iam.member-invited.v1"
	invitationProvisionedType = "com.company.iam.invitation-provisioned.v1"
	invitationFailedType      = "com.company.iam.invitation-failed.v1"
	membershipEstablishedType = "com.company.iam.membership-established.v1"
	memberAccessChangedType   = "com.company.iam.member-access-changed.v1"
	memberRemovedType         = "com.company.iam.member-removed.v1"

	buyerRegisteredType     = "com.company.iam.buyer-registered.v1"
	buyerProfileUpdatedType = "com.company.iam.buyer-profile-updated.v1"
	buyerBlockedType        = "com.company.iam.buyer-blocked.v1"
	buyerUnblockedType      = "com.company.iam.buyer-unblocked.v1"
)

type Mapper struct{}

func (Mapper) Map(event kernel.DomainEvent) (postgres.Mapped, error) {
	switch e := event.(type) {
	case domain.RoleDefined:
		return postgres.Mapped{
			Message: &eventv1.RoleDefined{
				Ref:           string(e.Ref),
				Name:          string(e.Name),
				Permissions:   names(e.Permissions),
				AccessVersion: e.AccessVersion,
				DefinedAt:     timestampOf(e.At),
			},
			Type: roleDefinedType,
		}, nil
	case domain.RolePermissionsChanged:
		return postgres.Mapped{
			Message: &eventv1.RolePermissionsChanged{
				Ref:           string(e.Ref),
				Permissions:   names(e.Permissions),
				AccessVersion: e.AccessVersion,
				ChangedAt:     timestampOf(e.At),
			},
			Type: rolePermissionsChangedType,
		}, nil
	case domain.RoleRetired:
		return postgres.Mapped{
			Message: &eventv1.RoleRetired{
				Ref:           string(e.Ref),
				AccessVersion: e.AccessVersion,
				RetiredAt:     timestampOf(e.At),
			},
			Type: roleRetiredType,
		}, nil
	default:
		return mapAccess(event)
	}
}

func mapAccess(event kernel.DomainEvent) (postgres.Mapped, error) {
	switch e := event.(type) {
	case domain.MemberInvited:
		return postgres.Mapped{
			Message: &eventv1.MemberInvited{Email: string(e.Email), Roles: names(e.Roles), Round: e.Round, InvitedAt: timestampOf(e.At)},
			Type:    memberInvitedType,
		}, nil
	case domain.InvitationProvisioned:
		return postgres.Mapped{
			Message: &eventv1.InvitationProvisioned{Email: string(e.Email), Subject: string(e.Subject), Roles: names(e.Roles), ProvisionedAt: timestampOf(e.At)},
			Type:    invitationProvisionedType,
		}, nil
	case domain.InvitationFailed:
		return postgres.Mapped{
			Message: &eventv1.InvitationFailed{Email: string(e.Email), Reason: e.Reason, FailedAt: timestampOf(e.At)},
			Type:    invitationFailedType,
		}, nil
	case domain.MembershipEstablished:
		return postgres.Mapped{
			Message: &eventv1.MembershipEstablished{
				Subject: string(e.Subject), Email: string(e.Email), Roles: names(e.Roles), AccessVersion: e.AccessVersion, EstablishedAt: timestampOf(e.At),
			},
			Type: membershipEstablishedType,
		}, nil
	case domain.MemberAccessChanged:
		return postgres.Mapped{
			Message: &eventv1.MemberAccessChanged{Subject: string(e.Subject), Roles: names(e.Roles), AccessVersion: e.AccessVersion, ChangedAt: timestampOf(e.At)},
			Type:    memberAccessChangedType,
		}, nil
	case domain.MemberRemoved:
		return postgres.Mapped{
			Message: &eventv1.MemberRemoved{Subject: string(e.Subject), AccessVersion: e.AccessVersion, RemovedAt: timestampOf(e.At)},
			Type:    memberRemovedType,
		}, nil
	default:
		return mapBuyer(event)
	}
}

func mapBuyer(event kernel.DomainEvent) (postgres.Mapped, error) {
	switch e := event.(type) {
	case domain.BuyerRegistered:
		return postgres.Mapped{
			Message: &eventv1.BuyerRegistered{Subject: string(e.Subject), Name: string(e.Name), AccessVersion: e.AccessVersion, RegisteredAt: timestampOf(e.At)},
			Type:    buyerRegisteredType,
		}, nil
	case domain.BuyerProfileUpdated:
		return postgres.Mapped{
			Message: &eventv1.BuyerProfileUpdated{Subject: string(e.Subject), Name: string(e.Name), UpdatedAt: timestampOf(e.At)},
			Type:    buyerProfileUpdatedType,
		}, nil
	case domain.BuyerBlocked:
		return postgres.Mapped{
			Message: &eventv1.BuyerBlocked{Subject: string(e.Subject), Reason: e.Reason, AccessVersion: e.AccessVersion, BlockedAt: timestampOf(e.At)},
			Type:    buyerBlockedType,
		}, nil
	case domain.BuyerUnblocked:
		return postgres.Mapped{
			Message: &eventv1.BuyerUnblocked{Subject: string(e.Subject), AccessVersion: e.AccessVersion, UnblockedAt: timestampOf(e.At)},
			Type:    buyerUnblockedType,
		}, nil
	default:
		return postgres.Mapped{}, fmt.Errorf("%w: %s", postgres.ErrUnmappedEvent, event.EventName())
	}
}

func timestampOf(at kernel.Instant) *timestamppb.Timestamp {
	return timestamppb.New(time.Unix(0, int64(at)))
}
