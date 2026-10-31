//go:build integration

package provider_test

import (
	"context"
	"slices"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/postgres"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb/pg"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/appkit"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

func TestTheInvitationsOfOneStatusAreReadExactly(t *testing.T) {
	pool := appkit.OpenPool(t)
	ctx := withExecution(t, context.Background())
	for email, status := range map[domain.Email]domain.InvitationStatus{
		"ana@loja.com": domain.InvitationStatusPending, "bia@loja.com": domain.InvitationStatusPending, "caio@loja.com": domain.InvitationStatusFailed,
	} {
		snap := pendingInvitation(1)
		snap.Email, snap.Status = email, status
		pg.Seed(t, ctx, pool, provider.NewInvitationRepository, email, snap)
	}
	reader := provider.NewInvitationsByStatusReader(postgres.NewReadPool(pool))

	pending, err := reader.LoadByStatus(withExecution(t, context.Background()), domain.InvitationStatusPending)
	emails := make([]string, 0, len(pending))
	for _, s := range pending {
		emails = append(emails, string(s.Email))
	}
	slices.Sort(emails)
	if err != nil || !slices.Equal(emails, []string{"ana@loja.com", "bia@loja.com"}) {
		t.Fatalf("LoadByStatus(pending) = %v, %v; want ana and bia", emails, err)
	}
	provisioned, err := reader.LoadByStatus(withExecution(t, context.Background()), domain.InvitationStatusProvisioned)
	if err != nil || len(provisioned) != 0 {
		t.Fatalf("LoadByStatus(provisioned) = %v, %v; want none", provisioned, err)
	}
}

func TestTheMembershipsOfOneStatusAreReadExactly(t *testing.T) {
	pool := appkit.OpenPool(t)
	ctx := withExecution(t, context.Background())
	for subject, status := range map[domain.Subject]domain.MembershipStatus{
		"kc-a": domain.MembershipStatusActive, "kc-b": domain.MembershipStatusRemoved, "kc-c": domain.MembershipStatusRemoved,
	} {
		snap := activeMember(1)
		snap.Subject, snap.Status = subject, status
		pg.Seed(t, ctx, pool, provider.NewMembershipRepository, subject, snap)
	}
	reader := provider.NewMembershipsByStatusReader(postgres.NewReadPool(pool))

	removed, err := reader.LoadByStatus(withExecution(t, context.Background()), domain.MembershipStatusRemoved)
	subjects := make([]string, 0, len(removed))
	for _, s := range removed {
		subjects = append(subjects, string(s.Subject))
	}
	slices.Sort(subjects)
	if err != nil || !slices.Equal(subjects, []string{"kc-b", "kc-c"}) {
		t.Fatalf("LoadByStatus(removed) = %v, %v; want kc-b and kc-c", subjects, err)
	}
}
