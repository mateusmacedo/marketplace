package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

var errBoom = errors.New("boom")

func TestEachFailureOfACommandCarriesTheUseCasePrefix(t *testing.T) {
	define := func(t *testing.T, h *harness) error {
		_, err := h.service.DefineRole(withExecution(t, context.Background()), application.DefineRole{Ref: testRef, Name: testName, Permissions: testPermissions})
		return err
	}
	change := func(t *testing.T, h *harness) error {
		_, err := h.service.ChangeRolePermissions(withExecution(t, context.Background()), application.ChangeRolePermissions{Ref: testRef, Permissions: []domain.Permission{"catalog:write"}})
		return err
	}
	retire := func(t *testing.T, h *harness) error {
		_, err := h.service.RetireRole(withExecution(t, context.Background()), application.RetireRole{Ref: testRef})
		return err
	}
	for _, tc := range []struct {
		name    string
		options []option
		seed    func(*harness, *testing.T)
		run     func(*testing.T, *harness) error
		wantIs  error
		wantMsg string
	}{
		{"DefineRole, load", []option{withLoadError(errBoom)}, nil, define, errBoom, "application: define role expedicao: boom"},
		{"DefineRole, save", []option{withSaveError(errBoom)}, nil, define, errBoom, "application: define role expedicao: boom"},
		{"DefineRole, enqueue", []option{withEnqueueError(errBoom)}, nil, define, errBoom, "application: define role expedicao: enqueue: boom"},
		{"ChangeRolePermissions, absent", nil, nil, change, ports.ErrNotFound, "application: change role permissions expedicao: ports: aggregate not found"},
		{"ChangeRolePermissions, load", []option{withLoadError(errBoom)}, (*harness).seedActive, change, errBoom, "application: change role permissions expedicao: boom"},
		{"ChangeRolePermissions, save", []option{withSaveError(errBoom)}, (*harness).seedActive, change, errBoom, "application: change role permissions expedicao: boom"},
		{"ChangeRolePermissions, enqueue", []option{withEnqueueError(errBoom)}, (*harness).seedActive, change, errBoom, "application: change role permissions expedicao: enqueue: boom"},
		{"RetireRole, absent", nil, nil, retire, ports.ErrNotFound, "application: retire role expedicao: ports: aggregate not found"},
		{"RetireRole, load", []option{withLoadError(errBoom)}, (*harness).seedActive, retire, errBoom, "application: retire role expedicao: boom"},
		{"RetireRole, save", []option{withSaveError(errBoom)}, (*harness).seedActive, retire, errBoom, "application: retire role expedicao: boom"},
		{"RetireRole, enqueue", []option{withEnqueueError(errBoom)}, (*harness).seedActive, retire, errBoom, "application: retire role expedicao: enqueue: boom"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, tc.options...)
			if tc.seed != nil {
				tc.seed(h, t)
			}

			err := tc.run(t, h)

			if !errors.Is(err, tc.wantIs) || err.Error() != tc.wantMsg {
				t.Fatalf("err = %v, want %q matching %v", err, tc.wantMsg, tc.wantIs)
			}
		})
	}
}
