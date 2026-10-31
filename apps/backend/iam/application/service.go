package application

import (
	"context"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	port "github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/ports"
)

const (
	AggregateTypeRole       = "iam.Role"
	AggregateTypeInvitation = "iam.Invitation"
	AggregateTypeMembership = "iam.Membership"
	AggregateTypeBuyer      = "iam.Buyer"

	Destination = "iam.events"

	OperationDefineRole            = "iam.DefineRole"
	OperationChangeRolePermissions = "iam.ChangeRolePermissions"
	OperationRetireRole            = "iam.RetireRole"
	OperationFindRole              = "iam.FindRole"
	OperationFindRolesByStatus     = "iam.FindRolesByStatus"

	OperationInviteMember            = "iam.InviteMember"
	OperationClaimOwnership          = "iam.ClaimOwnership"
	OperationGrantRole               = "iam.GrantRole"
	OperationRevokeRole              = "iam.RevokeRole"
	OperationRemoveMember            = "iam.RemoveMember"
	OperationFindInvitation          = "iam.FindInvitation"
	OperationFindInvitationsByStatus = "iam.FindInvitationsByStatus"
	OperationFindMembership          = "iam.FindMembership"
	OperationFindMembershipsByStatus = "iam.FindMembershipsByStatus"

	OperationRegisterBuyer      = "iam.RegisterBuyer"
	OperationUpdateBuyerProfile = "iam.UpdateBuyerProfile"
	OperationBlockBuyer         = "iam.BlockBuyer"
	OperationUnblockBuyer       = "iam.UnblockBuyer"
	OperationFindBuyer          = "iam.FindBuyer"
	OperationFindBuyersByStatus = "iam.FindBuyersByStatus"
)

const maxEventsPerCommand = 1

type Resources struct {
	// Inbox records the delivered messages the consumer handles, apart from the
	// command inbox of the synchronous use cases (IDM-03).
	Inbox       port.Inbox
	Roles       port.Repository[domain.RoleRef, domain.RoleSnapshot]
	Invitations port.Repository[domain.Email, domain.InvitationSnapshot]
	Memberships port.Repository[domain.Subject, domain.MembershipSnapshot]
	Buyers      port.Repository[domain.Subject, domain.BuyerSnapshot]
	Outbox      port.Outbox
	Commands    port.Inbox
}

type Operation interface{ isOperation() }

type DefineRole struct {
	Ref         domain.RoleRef
	Name        domain.RoleName
	Permissions []domain.Permission
}

func (DefineRole) isOperation() {}

type ChangeRolePermissions struct {
	Ref         domain.RoleRef
	Permissions []domain.Permission
}

func (ChangeRolePermissions) isOperation() {}

type RetireRole struct {
	Ref domain.RoleRef
}

func (RetireRole) isOperation() {}

// FindRole and FindRolesByStatus ask for state without changing it, and are
// authorized like any other entry point: reaching the edge is not permission
// to read what the operation returns.
type FindRole struct {
	Ref domain.RoleRef
}

func (FindRole) isOperation() {}

type FindRolesByStatus struct {
	Status domain.RoleStatus
}

func (FindRolesByStatus) isOperation() {}

// InviteMember carries the address as typed; the invitation lives under the
// normalized one.
type InviteMember struct {
	Email string
	Roles []domain.RoleRef
}

func (InviteMember) isOperation() {}

// ClaimOwnership carries the subject the bff took from the token, since the
// subject does not cross the gRPC hop by itself.
type ClaimOwnership struct {
	Subject domain.Subject
}

func (ClaimOwnership) isOperation() {}

type GrantRole struct {
	Subject domain.Subject
	Role    domain.RoleRef
}

func (GrantRole) isOperation() {}

type RevokeRole struct {
	Subject domain.Subject
	Role    domain.RoleRef
}

func (RevokeRole) isOperation() {}

type RemoveMember struct {
	Subject domain.Subject
}

func (RemoveMember) isOperation() {}

type FindInvitation struct {
	Email domain.Email
}

func (FindInvitation) isOperation() {}

type FindInvitationsByStatus struct {
	Status domain.InvitationStatus
}

func (FindInvitationsByStatus) isOperation() {}

type FindMembership struct {
	Subject domain.Subject
}

func (FindMembership) isOperation() {}

type FindMembershipsByStatus struct {
	Status domain.MembershipStatus
}

func (FindMembershipsByStatus) isOperation() {}

// ConsumeInvitationProvisioned establishes the membership of the subject
// Keycloak returned for an invitation, from one delivered message the
// consumer adapter has already decoded.
type ConsumeInvitationProvisioned struct {
	MessageID   port.MessageID
	MessageType string
	PayloadHash string
	ReceivedAt  port.Instant
	Subject     domain.Subject
	Email       domain.Email
	Roles       []domain.RoleRef
}

func (ConsumeInvitationProvisioned) isOperation() {}

// RegisterBuyer carries the subject the bff took from the token; the body of
// the route never names one.
type RegisterBuyer struct {
	Subject domain.Subject
	Name    domain.BuyerName
}

func (RegisterBuyer) isOperation() {}

type UpdateBuyerProfile struct {
	Subject domain.Subject
	Name    domain.BuyerName
}

func (UpdateBuyerProfile) isOperation() {}

type BlockBuyer struct {
	Subject domain.Subject
	Reason  string
}

func (BlockBuyer) isOperation() {}

type UnblockBuyer struct {
	Subject domain.Subject
}

func (UnblockBuyer) isOperation() {}

type FindBuyer struct {
	Subject domain.Subject
}

func (FindBuyer) isOperation() {}

type FindBuyersByStatus struct {
	Status domain.BuyerStatus
}

func (FindBuyersByStatus) isOperation() {}

type Service struct {
	UoW           port.UnitOfWork[Resources]
	Roles         port.Reader[domain.RoleRef, domain.RoleSnapshot]
	RolesByStatus ports.RolesByStatusReader

	Invitations         port.Reader[domain.Email, domain.InvitationSnapshot]
	InvitationsByStatus ports.InvitationsByStatusReader
	Memberships         port.Reader[domain.Subject, domain.MembershipSnapshot]
	MembershipsByStatus ports.MembershipsByStatusReader
	Buyers              port.Reader[domain.Subject, domain.BuyerSnapshot]
	BuyersByStatus      ports.BuyersByStatusReader

	Clock       port.Clock
	IDs         port.IDGenerator
	Authorize   usecase.Authorize[Operation]
	Idempotency usecase.IdempotencyPolicy

	// Consumer names the inbox entries of the delivered messages (INB-01).
	Consumer string

	Instrumentation port.Instrumentation
}

type command[R any] = usecase.Command[Resources, Operation, R]

func (s Service) executor() usecase.Executor[Resources, Operation] {
	return usecase.Executor[Resources, Operation]{
		UoW:             s.UoW,
		Inbox:           func(res Resources) port.Inbox { return res.Commands },
		Consumer:        CommandConsumer,
		Clock:           s.Clock,
		IDs:             s.IDs,
		MaxEvents:       maxEventsPerCommand,
		Policy:          s.Idempotency,
		Authorize:       s.Authorize,
		Instrumentation: s.Instrumentation,
	}
}

// A nil hook is the inert realization, so every operation opens and closes
// unconditionally.
func (s Service) instrumentation() port.Instrumentation {
	if s.Instrumentation == nil {
		return port.NoInstrumentation()
	}
	return s.Instrumentation
}

func roleOrigin(ref domain.RoleRef) usecase.Origin {
	return usecase.Origin{Destination: Destination, AggregateType: AggregateTypeRole, AggregateID: string(ref)}
}

func invitationOrigin(email domain.Email) usecase.Origin {
	return usecase.Origin{Destination: Destination, AggregateType: AggregateTypeInvitation, AggregateID: string(email)}
}

func membershipOrigin(subject domain.Subject) usecase.Origin {
	return usecase.Origin{Destination: Destination, AggregateType: AggregateTypeMembership, AggregateID: string(subject)}
}

func buyerOrigin(subject domain.Subject) usecase.Origin {
	return usecase.Origin{Destination: Destination, AggregateType: AggregateTypeBuyer, AggregateID: string(subject)}
}

// withList leads the list with its length, so the request stays unambiguous
// once another field follows it.
func withList[T ~string](f *usecase.Fingerprint, values []T) *usecase.Fingerprint {
	f.Int(int64(len(values)))
	for _, value := range values {
		f.String(string(value))
	}
	return f
}

// formedOrNew starts the fresh aggregate without reading an id outside the
// form: none was ever stored, Postgres refuses to compare U+0000, and the
// domain refuses the id with the code the spec declares.
func formedOrNew[ID comparable, S, A any](valid func(ID) bool, fresh func(ID) A, from func(S) A) usecase.Loader[ID, S, A] {
	load := usecase.OrNew(fresh, from)
	return func(ctx context.Context, reader port.Reader[ID, S], id ID) (A, port.Version, error) {
		if !valid(id) {
			return fresh(id), 0, nil
		}
		return load(ctx, reader, id)
	}
}

var (
	loadRole         = formedOrNew(domain.ValidRef, domain.NewRole, domain.FromRoleSnapshot)
	loadExistingRole = usecase.Existing[domain.RoleRef](domain.FromRoleSnapshot)

	loadInvitation = formedOrNew(domain.ValidEmail, domain.NewInvitation, domain.FromInvitationSnapshot)

	loadMembership         = formedOrNew(domain.ValidSubject, domain.NewMembership, domain.FromMembershipSnapshot)
	loadExistingMembership = usecase.Existing[domain.Subject](domain.FromMembershipSnapshot)

	loadBuyer         = formedOrNew(domain.ValidSubject, domain.NewBuyer, domain.FromBuyerSnapshot)
	loadExistingBuyer = usecase.Existing[domain.Subject](domain.FromBuyerSnapshot)
)
