package application_test

import (
	"context"
	"slices"
	"testing"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

var nineSteps = []string{
	"authorize",
	"clock.Now",
	"ids.NewMessageID",
	"within",
	"commands.Register",
	"roles.Load",
	"roles.Save",
	"outbox.Enqueue",
	"commands.Complete",
	"commit",
}

func TestEachCommandWalksTheNineStepsInOrder(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed func(*harness, *testing.T)
		run  func(*testing.T, *harness) error
	}{
		{"DefineRole", nil, func(t *testing.T, h *harness) error {
			_, err := h.service.DefineRole(withExecution(t, context.Background()), application.DefineRole{Ref: testRef, Name: testName, Permissions: testPermissions})
			return err
		}},
		{"ChangeRolePermissions", (*harness).seedActive, func(t *testing.T, h *harness) error {
			_, err := h.service.ChangeRolePermissions(withExecution(t, context.Background()), application.ChangeRolePermissions{Ref: testRef, Permissions: []domain.Permission{"catalog:write"}})
			return err
		}},
		{"RetireRole", (*harness).seedActive, func(t *testing.T, h *harness) error {
			_, err := h.service.RetireRole(withExecution(t, context.Background()), application.RetireRole{Ref: testRef})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			if tc.seed != nil {
				tc.seed(h, t)
			}

			if err := tc.run(t, h); err != nil {
				t.Fatalf("%s() error = %v, want nil", tc.name, err)
			}

			if !slices.Equal(h.rec.Observed(), nineSteps) {
				t.Fatalf("sequence mismatch (FND-04 §3.2)\ngot:  %v\nwant: %v", h.rec.Observed(), nineSteps)
			}
		})
	}
}

func TestDefineEnqueuesTheDefinitionInTheSameTransaction(t *testing.T) {
	h := newHarness(t)

	if _, err := h.service.DefineRole(withExecution(t, context.Background()), application.DefineRole{Ref: testRef, Name: testName, Permissions: testPermissions}); err != nil {
		t.Fatalf("DefineRole() error = %v, want nil", err)
	}

	entries := h.store.Entries()
	if len(entries) != 1 {
		t.Fatalf("outbox entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	event, ok := entry.Event.(domain.RoleDefined)
	if !ok || event.Ref != testRef || event.Name != testName || !slices.Equal(event.Permissions, testPermissions) || event.AccessVersion != 1 || event.At != testOccurred ||
		entry.AggregateType != application.AggregateTypeRole ||
		entry.AggregateID != string(testRef) || entry.AggregateVersion != 1 || entry.Intent.Destination != application.Destination {
		t.Fatalf("outbox entry = %+v, want the definition of the role at version 1", entry)
	}
}

func TestChangePermissionsEnqueuesTheChangeInTheSameTransaction(t *testing.T) {
	h := newHarness(t)
	h.seedActive(t)

	if _, err := h.service.ChangeRolePermissions(withExecution(t, context.Background()), application.ChangeRolePermissions{Ref: testRef, Permissions: []domain.Permission{"catalog:write"}}); err != nil {
		t.Fatalf("ChangeRolePermissions() error = %v, want nil", err)
	}

	entries := h.store.Entries()
	if len(entries) != 1 || entries[0].AggregateVersion != 2 {
		t.Fatalf("outbox = %+v, want one entry at version 2", entries)
	}
	event, ok := entries[0].Event.(domain.RolePermissionsChanged)
	if !ok || event.Ref != testRef || !slices.Equal(event.Permissions, []domain.Permission{"catalog:write"}) || event.AccessVersion != 2 || event.At != testOccurred {
		t.Fatalf("outbox = %+v, want the change of the role at version 2", entries)
	}
}

func TestRetireEnqueuesTheRetirementInTheSameTransaction(t *testing.T) {
	h := newHarness(t)
	h.seedActive(t)

	if _, err := h.service.RetireRole(withExecution(t, context.Background()), application.RetireRole{Ref: testRef}); err != nil {
		t.Fatalf("RetireRole() error = %v, want nil", err)
	}

	entries := h.store.Entries()
	want := domain.RoleRetired{Ref: testRef, AccessVersion: 2, At: testOccurred}
	if len(entries) != 1 || entries[0].Event != want || entries[0].AggregateVersion != 2 {
		t.Fatalf("outbox = %+v, want the retirement of the role at version 2", entries)
	}
}
