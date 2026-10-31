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

// InviteMember leaves the form of the address and of the roles to the domain,
// whose rejections the spec declares; the edge bounds only their count.
func (s Server) InviteMember(ctx context.Context, req *servicev1.InviteMemberRequest) (*servicev1.InviteMemberResponse, error) {
	if len(req.GetRoles()) > maxListItems {
		return nil, tooManyItems("roles")
	}
	return kernelgrpc.Command(ctx,
		func(ctx context.Context) (usecase.Outcome[domain.InvitedResponse], error) {
			return s.Service.InviteMember(ctx, application.InviteMember{Email: req.GetEmail(), Roles: refsOf(req.GetRoles())})
		},
		func(r *kernel.Rejection) *servicev1.InviteMemberResponse {
			return &servicev1.InviteMemberResponse{Result: &servicev1.InviteMemberResponse_Rejection{Rejection: rejectionOf(r)}}
		},
		func(invited domain.InvitedResponse) *servicev1.InviteMemberResponse {
			return &servicev1.InviteMemberResponse{Result: &servicev1.InviteMemberResponse_Invited{
				Invited: &servicev1.Invited{Email: string(invited.Email), Status: invitationStatus(invited.Status)},
			}}
		})
}

// ClaimOwnership takes the subject the bff filled from the token; the domain
// refuses an empty one.
func (s Server) ClaimOwnership(ctx context.Context, req *servicev1.ClaimOwnershipRequest) (*servicev1.ClaimOwnershipResponse, error) {
	return kernelgrpc.Command(ctx,
		func(ctx context.Context) (usecase.Outcome[domain.EstablishedResponse], error) {
			return s.Service.ClaimOwnership(ctx, application.ClaimOwnership{Subject: domain.Subject(req.GetSubject())})
		},
		func(r *kernel.Rejection) *servicev1.ClaimOwnershipResponse {
			return &servicev1.ClaimOwnershipResponse{Result: &servicev1.ClaimOwnershipResponse_Rejection{Rejection: rejectionOf(r)}}
		},
		func(claimed domain.EstablishedResponse) *servicev1.ClaimOwnershipResponse {
			return &servicev1.ClaimOwnershipResponse{Result: &servicev1.ClaimOwnershipResponse_Claimed{
				Claimed: &servicev1.Claimed{Subject: string(claimed.Subject)},
			}}
		})
}

func (s Server) GrantRole(ctx context.Context, req *servicev1.GrantRoleRequest) (*servicev1.GrantRoleResponse, error) {
	subject, err := subjectOf(req.GetSubject())
	if err != nil {
		return nil, err
	}
	return kernelgrpc.Command(ctx,
		func(ctx context.Context) (usecase.Outcome[domain.GrantedResponse], error) {
			return s.Service.GrantRole(ctx, application.GrantRole{Subject: subject, Role: domain.RoleRef(req.GetRole())})
		},
		func(r *kernel.Rejection) *servicev1.GrantRoleResponse {
			return &servicev1.GrantRoleResponse{Result: &servicev1.GrantRoleResponse_Rejection{Rejection: rejectionOf(r)}}
		},
		func(granted domain.GrantedResponse) *servicev1.GrantRoleResponse {
			return &servicev1.GrantRoleResponse{Result: &servicev1.GrantRoleResponse_Granted{
				Granted: &servicev1.Granted{Subject: string(granted.Subject)},
			}}
		})
}

func (s Server) RevokeRole(ctx context.Context, req *servicev1.RevokeRoleRequest) (*servicev1.RevokeRoleResponse, error) {
	subject, err := subjectOf(req.GetSubject())
	if err != nil {
		return nil, err
	}
	return kernelgrpc.Command(ctx,
		func(ctx context.Context) (usecase.Outcome[domain.RevokedResponse], error) {
			return s.Service.RevokeRole(ctx, application.RevokeRole{Subject: subject, Role: domain.RoleRef(req.GetRole())})
		},
		func(r *kernel.Rejection) *servicev1.RevokeRoleResponse {
			return &servicev1.RevokeRoleResponse{Result: &servicev1.RevokeRoleResponse_Rejection{Rejection: rejectionOf(r)}}
		},
		func(revoked domain.RevokedResponse) *servicev1.RevokeRoleResponse {
			return &servicev1.RevokeRoleResponse{Result: &servicev1.RevokeRoleResponse_Revoked{
				Revoked: &servicev1.Revoked{Subject: string(revoked.Subject)},
			}}
		})
}

func (s Server) RemoveMember(ctx context.Context, req *servicev1.RemoveMemberRequest) (*servicev1.RemoveMemberResponse, error) {
	subject, err := subjectOf(req.GetSubject())
	if err != nil {
		return nil, err
	}
	return kernelgrpc.Command(ctx,
		func(ctx context.Context) (usecase.Outcome[domain.RemovedResponse], error) {
			return s.Service.RemoveMember(ctx, application.RemoveMember{Subject: subject})
		},
		func(r *kernel.Rejection) *servicev1.RemoveMemberResponse {
			return &servicev1.RemoveMemberResponse{Result: &servicev1.RemoveMemberResponse_Rejection{Rejection: rejectionOf(r)}}
		},
		func(removed domain.RemovedResponse) *servicev1.RemoveMemberResponse {
			return &servicev1.RemoveMemberResponse{Result: &servicev1.RemoveMemberResponse_Removed{
				Removed: &servicev1.Removed{Subject: string(removed.Subject)},
			}}
		})
}

// FindInvitation refuses an address outside the form at the edge, as the bff
// does with the path, before any use case.
func (s Server) FindInvitation(ctx context.Context, req *servicev1.FindInvitationRequest) (*servicev1.FindInvitationResponse, error) {
	email := domain.Email(req.GetEmail())
	if !domain.ValidEmail(email) {
		return nil, status.Error(codes.InvalidArgument, domain.EmailInvalidMessage)
	}
	if _, err := kernelgrpc.ExecutionOf(ctx); err != nil {
		return nil, err
	}
	snapshot, err := s.Service.FindInvitation(ctx, email)
	if err != nil {
		return nil, kernelgrpc.StatusOf(err)
	}
	return &servicev1.FindInvitationResponse{Invitation: invitationOf(snapshot)}, nil
}

func (s Server) FindInvitationsByStatus(ctx context.Context, req *servicev1.FindInvitationsByStatusRequest) (*servicev1.FindInvitationsByStatusResponse, error) {
	wanted, ok := invitationStatusOf(req.GetStatus())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "status must be INVITATION_STATUS_PENDING, INVITATION_STATUS_PROVISIONED or INVITATION_STATUS_FAILED")
	}
	if _, err := kernelgrpc.ExecutionOf(ctx); err != nil {
		return nil, err
	}
	snapshots, err := s.Service.FindInvitationsByStatus(ctx, wanted)
	if err != nil {
		return nil, kernelgrpc.StatusOf(err)
	}
	invitations := make([]*servicev1.Invitation, 0, len(snapshots))
	for _, snapshot := range snapshots {
		invitations = append(invitations, invitationOf(snapshot))
	}
	return &servicev1.FindInvitationsByStatusResponse{Invitations: invitations}, nil
}

func (s Server) FindMembership(ctx context.Context, req *servicev1.FindMembershipRequest) (*servicev1.FindMembershipResponse, error) {
	subject, err := subjectOf(req.GetSubject())
	if err != nil {
		return nil, err
	}
	if _, err := kernelgrpc.ExecutionOf(ctx); err != nil {
		return nil, err
	}
	snapshot, err := s.Service.FindMembership(ctx, subject)
	if err != nil {
		return nil, kernelgrpc.StatusOf(err)
	}
	return &servicev1.FindMembershipResponse{Membership: membershipOf(snapshot)}, nil
}

func (s Server) FindMembershipsByStatus(ctx context.Context, req *servicev1.FindMembershipsByStatusRequest) (*servicev1.FindMembershipsByStatusResponse, error) {
	wanted, ok := membershipStatusOf(req.GetStatus())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "status must be MEMBERSHIP_STATUS_ACTIVE or MEMBERSHIP_STATUS_REMOVED")
	}
	if _, err := kernelgrpc.ExecutionOf(ctx); err != nil {
		return nil, err
	}
	snapshots, err := s.Service.FindMembershipsByStatus(ctx, wanted)
	if err != nil {
		return nil, kernelgrpc.StatusOf(err)
	}
	memberships := make([]*servicev1.Membership, 0, len(snapshots))
	for _, snapshot := range snapshots {
		memberships = append(memberships, membershipOf(snapshot))
	}
	return &servicev1.FindMembershipsByStatusResponse{Memberships: memberships}, nil
}

// subjectOf refuses at the edge a subject outside the form, as roleRef does a
// ref: the spec gives the subject of a path or of the token no refusal of its own.
func subjectOf(raw string) (domain.Subject, error) {
	subject := domain.Subject(raw)
	if !domain.ValidSubject(subject) {
		return "", status.Error(codes.InvalidArgument, domain.SubjectInvalidMessage)
	}
	return subject, nil
}

func refsOf(names []string) []domain.RoleRef {
	refs := make([]domain.RoleRef, 0, len(names))
	for _, name := range names {
		refs = append(refs, domain.RoleRef(name))
	}
	return refs
}

func namesOf(refs []domain.RoleRef) []string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, string(ref))
	}
	return names
}

func invitationOf(s domain.InvitationSnapshot) *servicev1.Invitation {
	return &servicev1.Invitation{
		Email:         string(s.Email),
		Roles:         namesOf(s.Roles),
		Status:        invitationStatus(s.Status),
		Round:         s.Round,
		Subject:       string(s.Subject),
		FailureReason: s.FailureReason,
		InvitedAt:     int64(s.InvitedAt),
		SettledAt:     int64(s.SettledAt),
	}
}

func membershipOf(s domain.MembershipSnapshot) *servicev1.Membership {
	return &servicev1.Membership{
		Subject:       string(s.Subject),
		Email:         string(s.Email),
		Roles:         namesOf(s.Roles),
		Status:        membershipStatus(s.Status),
		AccessVersion: s.AccessVersion,
		SyncedVersion: s.SyncedVersion,
		JoinedAt:      int64(s.JoinedAt),
		ChangedAt:     int64(s.ChangedAt),
	}
}

func invitationStatus(s domain.InvitationStatus) servicev1.InvitationStatus {
	switch s {
	case domain.InvitationStatusPending:
		return servicev1.InvitationStatus_INVITATION_STATUS_PENDING
	case domain.InvitationStatusProvisioned:
		return servicev1.InvitationStatus_INVITATION_STATUS_PROVISIONED
	case domain.InvitationStatusFailed:
		return servicev1.InvitationStatus_INVITATION_STATUS_FAILED
	default:
		return servicev1.InvitationStatus_INVITATION_STATUS_UNSPECIFIED
	}
}

func invitationStatusOf(s servicev1.InvitationStatus) (domain.InvitationStatus, bool) {
	switch s {
	case servicev1.InvitationStatus_INVITATION_STATUS_PENDING:
		return domain.InvitationStatusPending, true
	case servicev1.InvitationStatus_INVITATION_STATUS_PROVISIONED:
		return domain.InvitationStatusProvisioned, true
	case servicev1.InvitationStatus_INVITATION_STATUS_FAILED:
		return domain.InvitationStatusFailed, true
	default:
		return domain.InvitationStatusUndefined, false
	}
}

func membershipStatus(s domain.MembershipStatus) servicev1.MembershipStatus {
	switch s {
	case domain.MembershipStatusActive:
		return servicev1.MembershipStatus_MEMBERSHIP_STATUS_ACTIVE
	case domain.MembershipStatusRemoved:
		return servicev1.MembershipStatus_MEMBERSHIP_STATUS_REMOVED
	default:
		return servicev1.MembershipStatus_MEMBERSHIP_STATUS_UNSPECIFIED
	}
}

func membershipStatusOf(s servicev1.MembershipStatus) (domain.MembershipStatus, bool) {
	switch s {
	case servicev1.MembershipStatus_MEMBERSHIP_STATUS_ACTIVE:
		return domain.MembershipStatusActive, true
	case servicev1.MembershipStatus_MEMBERSHIP_STATUS_REMOVED:
		return domain.MembershipStatusRemoved, true
	default:
		return domain.MembershipStatusUndefined, false
	}
}
