package application_test

import (
	"context"
	"errors"
	"testing"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func requireCode[R any](t *testing.T, outcome usecase.Outcome[R], err error, want kernel.Code) {
	t.Helper()
	rejection, rejected := outcome.Rejection()
	if err != nil || !rejected || rejection.Code() != want {
		t.Fatalf("outcome = (%+v, %v), want the rejection %s", outcome, err, want)
	}
}

func define(t *testing.T, h *harness, cmd application.DefineRole) (usecase.Outcome[domain.DefinedResponse], error) {
	t.Helper()
	return h.service.DefineRole(withExecution(t, context.Background()), cmd)
}

func TestAnUnknownRefIsDefinedActiveWithItsPermissionsNormalized(t *testing.T) {
	h := newHarness(t)

	outcome, err := define(t, h, application.DefineRole{
		Ref: testRef, Name: testName, Permissions: []domain.Permission{"fulfillment:write", "ordering:read", "ordering:read"},
	})

	if err != nil || outcome.Response() != (domain.DefinedResponse{Ref: testRef}) {
		t.Fatalf("DefineRole() = (%+v, %v), want the defined ref", outcome, err)
	}
	want := domain.RoleSnapshot{
		Ref: testRef, Name: testName, Permissions: testPermissions, Status: domain.RoleStatusActive,
		AccessVersion: 1, DefinedAt: testOccurred, ChangedAt: testOccurred,
	}
	if got := h.stored(t, testRef); !got.Equal(want) {
		t.Fatalf("stored = %+v, want %+v", got, want)
	}
}

func TestAKnownRefIsAcceptedWithoutChangeOrEvent(t *testing.T) {
	h := newHarness(t)
	h.seedActive(t)
	before := h.stored(t, testRef)

	outcome, err := define(t, h, application.DefineRole{Ref: testRef, Name: "Outro nome", Permissions: []domain.Permission{"catalog:write"}})

	if _, rejected := outcome.Rejection(); err != nil || rejected {
		t.Fatalf("DefineRole() = (%+v, %v), want accepted", outcome, err)
	}
	if got := h.stored(t, testRef); !got.Equal(before) {
		t.Fatalf("stored = %+v, want it unchanged %+v", got, before)
	}
	if got := h.outboxLen(); got != 0 {
		t.Fatalf("outbox = %d entries, want none", got)
	}
}

func TestADefinitionOutsideTheRulesIsRejectedAndNothingIsWritten(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  application.DefineRole
		want kernel.Code
	}{
		{"owner", application.DefineRole{Ref: domain.OwnerRole, Name: "Dono", Permissions: testPermissions}, domain.CodeRoleReserved},
		{"Exp_1", application.DefineRole{Ref: "Exp_1", Name: testName, Permissions: testPermissions}, domain.CodeRoleRefInvalid},
		{"no permissions", application.DefineRole{Ref: testRef, Name: testName}, domain.CodeRolePermissionsEmpty},
		{"billing:write", application.DefineRole{Ref: testRef, Name: testName, Permissions: []domain.Permission{"billing:write"}}, domain.CodeRolePermissionUnknown},
		{"no name", application.DefineRole{Ref: testRef, Permissions: testPermissions}, domain.CodeRoleNameEmpty},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)

			outcome, err := define(t, h, tc.cmd)

			requireCode(t, outcome, err, tc.want)
			if got := h.outboxLen(); got != 0 {
				t.Fatalf("outbox = %d entries, want none", got)
			}
			if _, _, err := roleTable.Reader(h.store).Load(withExecution(t, context.Background()), tc.cmd.Ref); !errors.Is(err, ports.ErrNotFound) {
				t.Fatalf("Load(%s) = %v, want ErrNotFound: nothing is written", tc.cmd.Ref, err)
			}
		})
	}
}

func TestATakenNameUnderAnotherRefIsRejectedWithoutEvent(t *testing.T) {
	h := newHarness(t)
	h.seedActive(t)

	outcome, err := define(t, h, application.DefineRole{Ref: "outro-papel", Name: testName, Permissions: testPermissions})

	requireCode(t, outcome, err, domain.CodeRoleDuplicateName)
	if got := h.outboxLen(); got != 0 {
		t.Fatalf("outbox = %d entries, want none", got)
	}
}

func TestChangingThePermissionsOfAnActiveRoleRaisesItsAccessVersion(t *testing.T) {
	h := newHarness(t)
	h.seedActive(t)

	outcome, err := h.service.ChangeRolePermissions(withExecution(t, context.Background()), application.ChangeRolePermissions{
		Ref: testRef, Permissions: []domain.Permission{"catalog:write"},
	})

	if err != nil || outcome.Response() != (domain.PermissionsChangedResponse{Ref: testRef}) {
		t.Fatalf("ChangeRolePermissions() = (%+v, %v), want the changed ref", outcome, err)
	}
	got := h.stored(t, testRef)
	if len(got.Permissions) != 1 || got.Permissions[0] != "catalog:write" || got.AccessVersion != 2 || got.ChangedAt != testOccurred {
		t.Fatalf("stored = %+v, want [catalog:write] at access version 2", got)
	}
}

func TestChangingToTheSameSetIsAcceptedWithoutEvent(t *testing.T) {
	h := newHarness(t)
	h.seedActive(t)
	before := h.stored(t, testRef)

	outcome, err := h.service.ChangeRolePermissions(withExecution(t, context.Background()), application.ChangeRolePermissions{
		Ref: testRef, Permissions: []domain.Permission{"ordering:read", "fulfillment:write"},
	})

	if _, rejected := outcome.Rejection(); err != nil || rejected {
		t.Fatalf("ChangeRolePermissions() = (%+v, %v), want accepted", outcome, err)
	}
	if got := h.stored(t, testRef); !got.Equal(before) || h.outboxLen() != 0 {
		t.Fatalf("stored = %+v with %d outbox entries, want it unchanged and no event", got, h.outboxLen())
	}
}

func TestRetiringAnActiveRoleRaisesItsAccessVersion(t *testing.T) {
	h := newHarness(t)
	h.seedActive(t)

	outcome, err := h.service.RetireRole(withExecution(t, context.Background()), application.RetireRole{Ref: testRef})

	if err != nil || outcome.Response() != (domain.RetiredResponse{Ref: testRef}) {
		t.Fatalf("RetireRole() = (%+v, %v), want the retired ref", outcome, err)
	}
	if got := h.stored(t, testRef); got.Status != domain.RoleStatusRetired || got.AccessVersion != 2 || got.ChangedAt != testOccurred {
		t.Fatalf("stored = %+v, want retired at access version 2", got)
	}
}

func TestARetiredRoleRefusesBothChangesAndLeavesTheStateUnchanged(t *testing.T) {
	h := newHarness(t)
	h.seedRetired(t)
	before := h.stored(t, testRef)

	changed, err := h.service.ChangeRolePermissions(withExecution(t, context.Background()), application.ChangeRolePermissions{Ref: testRef, Permissions: []domain.Permission{"catalog:write"}})
	requireCode(t, changed, err, domain.CodeRoleRetired)
	retired, err := h.service.RetireRole(withExecution(t, context.Background()), application.RetireRole{Ref: testRef})
	requireCode(t, retired, err, domain.CodeRoleRetired)

	if got := h.stored(t, testRef); !got.Equal(before) {
		t.Fatalf("stored = %+v, want it unchanged %+v", got, before)
	}
}

func TestChangingOrRetiringAnUnknownRefIsNotFound(t *testing.T) {
	h := newHarness(t)

	if _, err := h.service.ChangeRolePermissions(withExecution(t, context.Background()), application.ChangeRolePermissions{Ref: testRef, Permissions: testPermissions}); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("ChangeRolePermissions(unknown) = %v, want ErrNotFound", err)
	}
	if _, err := h.service.RetireRole(withExecution(t, context.Background()), application.RetireRole{Ref: testRef}); !errors.Is(err, ports.ErrNotFound) {
		t.Fatalf("RetireRole(unknown) = %v, want ErrNotFound", err)
	}
}
