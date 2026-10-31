package provider_test

import (
	"errors"
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	eventv1 "github.com/mateusmacedo/marketplace/apps/backend/iam/contract/gen/go/company/iam/event/v1"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

type strayEvent struct{}

func (strayEvent) EventName() string { return "iam.stray" }

func requireMapped(t *testing.T, event kernel.DomainEvent, want proto.Message, wantType string) {
	t.Helper()

	mapped, err := provider.Mapper{}.Map(event)

	if err != nil {
		t.Fatalf("Map() = %v, want nil", err)
	}
	if !proto.Equal(mapped.Message, want) || mapped.Type != wantType {
		t.Errorf("Map() = %v %q, want %v %s (PTB-03)", mapped.Message, mapped.Type, want, wantType)
	}
}

func TestMapperMapsRoleDefined(t *testing.T) {
	t.Parallel()

	requireMapped(t,
		domain.RoleDefined{Ref: "expedicao", Name: "Expedição", Permissions: []domain.Permission{"fulfillment:write", "ordering:read"}, AccessVersion: 1, At: 1_755_432_000_123_456_789},
		&eventv1.RoleDefined{
			Ref: "expedicao", Name: "Expedição", Permissions: []string{"fulfillment:write", "ordering:read"}, AccessVersion: 1,
			DefinedAt: &timestamppb.Timestamp{Seconds: 1_755_432_000, Nanos: 123_456_789},
		},
		"com.company.iam.role-defined.v1")
}

func TestMapperMapsRolePermissionsChanged(t *testing.T) {
	t.Parallel()

	requireMapped(t,
		domain.RolePermissionsChanged{Ref: "expedicao", Permissions: []domain.Permission{"catalog:write"}, AccessVersion: 2, At: 1_755_432_000_000_000_001},
		&eventv1.RolePermissionsChanged{
			Ref: "expedicao", Permissions: []string{"catalog:write"}, AccessVersion: 2,
			ChangedAt: &timestamppb.Timestamp{Seconds: 1_755_432_000, Nanos: 1},
		},
		"com.company.iam.role-permissions-changed.v1")
}

func TestMapperMapsRoleRetired(t *testing.T) {
	t.Parallel()

	requireMapped(t,
		domain.RoleRetired{Ref: "expedicao", AccessVersion: 3, At: 1_755_432_000_000_000_002},
		&eventv1.RoleRetired{Ref: "expedicao", AccessVersion: 3, RetiredAt: &timestamppb.Timestamp{Seconds: 1_755_432_000, Nanos: 2}},
		"com.company.iam.role-retired.v1")
}

func TestMapperMapsTheEventsOfInvitationAndMembership(t *testing.T) {
	t.Parallel()
	ts := &timestamppb.Timestamp{Seconds: 1_755_432_000, Nanos: 7}
	const at = 1_755_432_000_000_000_007

	requireMapped(t, domain.MemberInvited{Email: "ana@loja.com", Roles: []domain.RoleRef{"expedicao"}, Round: 2, At: at},
		&eventv1.MemberInvited{Email: "ana@loja.com", Roles: []string{"expedicao"}, Round: 2, InvitedAt: ts},
		"com.company.iam.member-invited.v1")
	requireMapped(t, domain.InvitationProvisioned{Email: "ana@loja.com", Subject: "kc-123", Roles: []domain.RoleRef{"expedicao"}, At: at},
		&eventv1.InvitationProvisioned{Email: "ana@loja.com", Subject: "kc-123", Roles: []string{"expedicao"}, ProvisionedAt: ts},
		"com.company.iam.invitation-provisioned.v1")
	requireMapped(t, domain.InvitationFailed{Email: "ana@loja.com", Reason: "recusado", At: at},
		&eventv1.InvitationFailed{Email: "ana@loja.com", Reason: "recusado", FailedAt: ts},
		"com.company.iam.invitation-failed.v1")
	requireMapped(t, domain.MembershipEstablished{Subject: "kc-owner", Roles: []domain.RoleRef{domain.OwnerRole}, AccessVersion: 1, At: at},
		&eventv1.MembershipEstablished{Subject: "kc-owner", Roles: []string{"owner"}, AccessVersion: 1, EstablishedAt: ts},
		"com.company.iam.membership-established.v1")
	requireMapped(t, domain.MemberAccessChanged{Subject: "kc-123", Roles: []domain.RoleRef{"catalogo", "expedicao"}, AccessVersion: 3, At: at},
		&eventv1.MemberAccessChanged{Subject: "kc-123", Roles: []string{"catalogo", "expedicao"}, AccessVersion: 3, ChangedAt: ts},
		"com.company.iam.member-access-changed.v1")
	requireMapped(t, domain.MemberRemoved{Subject: "kc-123", AccessVersion: 4, At: at},
		&eventv1.MemberRemoved{Subject: "kc-123", AccessVersion: 4, RemovedAt: ts},
		"com.company.iam.member-removed.v1")
}

func TestMapperMapsTheEventsOfBuyer(t *testing.T) {
	t.Parallel()
	ts := &timestamppb.Timestamp{Seconds: 1_755_432_000, Nanos: 9}
	const at = 1_755_432_000_000_000_009

	requireMapped(t, domain.BuyerRegistered{Subject: "kc-123", Name: "Bia", AccessVersion: 1, At: at},
		&eventv1.BuyerRegistered{Subject: "kc-123", Name: "Bia", AccessVersion: 1, RegisteredAt: ts},
		"com.company.iam.buyer-registered.v1")
	requireMapped(t, domain.BuyerProfileUpdated{Subject: "kc-123", Name: "Bia Souza", At: at},
		&eventv1.BuyerProfileUpdated{Subject: "kc-123", Name: "Bia Souza", UpdatedAt: ts},
		"com.company.iam.buyer-profile-updated.v1")
	requireMapped(t, domain.BuyerBlocked{Subject: "kc-123", Reason: "chargeback recorrente", AccessVersion: 2, At: at},
		&eventv1.BuyerBlocked{Subject: "kc-123", Reason: "chargeback recorrente", AccessVersion: 2, BlockedAt: ts},
		"com.company.iam.buyer-blocked.v1")
	requireMapped(t, domain.BuyerUnblocked{Subject: "kc-123", AccessVersion: 3, At: at},
		&eventv1.BuyerUnblocked{Subject: "kc-123", AccessVersion: 3, UnblockedAt: ts},
		"com.company.iam.buyer-unblocked.v1")
}

func TestMapperReportsUnmappedEvent(t *testing.T) {
	t.Parallel()

	_, err := provider.Mapper{}.Map(strayEvent{})

	if !errors.Is(err, postgres.ErrUnmappedEvent) {
		t.Fatalf("Map() = %v, want ErrUnmappedEvent", err)
	}
}

var _ kernel.DomainEvent = strayEvent{}
