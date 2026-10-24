package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/application"
)

var errBoom = errors.New("boom")

func TestEachFailureOfACommandCarriesTheUseCasePrefix(t *testing.T) {
	register := func(t *testing.T, h *harness) error {
		_, err := h.service.RegisterStore(withExecution(t, context.Background()), application.RegisterStore{Ref: testRef, Name: testName})
		return err
	}
	suspend := func(t *testing.T, h *harness) error {
		_, err := h.service.SuspendStore(withExecution(t, context.Background()), application.SuspendStore{Ref: testRef, Reason: "motivo"})
		return err
	}
	reactivate := func(t *testing.T, h *harness) error {
		_, err := h.service.ReactivateStore(withExecution(t, context.Background()), application.ReactivateStore{Ref: testRef})
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
		{"RegisterStore, load", []option{withLoadError(errBoom)}, nil, register, errBoom, "application: register loja-acme: boom"},
		{"RegisterStore, save", []option{withSaveError(errBoom)}, nil, register, errBoom, "application: register loja-acme: boom"},
		{"RegisterStore, enqueue", []option{withEnqueueError(errBoom)}, nil, register, errBoom, "application: register loja-acme: enqueue: boom"},
		{"SuspendStore, absent", nil, nil, suspend, ports.ErrNotFound, "application: suspend loja-acme: ports: aggregate not found"},
		{"SuspendStore, load", []option{withLoadError(errBoom)}, (*harness).seedActive, suspend, errBoom, "application: suspend loja-acme: boom"},
		{"SuspendStore, save", []option{withSaveError(errBoom)}, (*harness).seedActive, suspend, errBoom, "application: suspend loja-acme: boom"},
		{"SuspendStore, enqueue", []option{withEnqueueError(errBoom)}, (*harness).seedActive, suspend, errBoom, "application: suspend loja-acme: enqueue: boom"},
		{"ReactivateStore, absent", nil, nil, reactivate, ports.ErrNotFound, "application: reactivate loja-acme: ports: aggregate not found"},
		{"ReactivateStore, load", []option{withLoadError(errBoom)}, (*harness).seedSuspended, reactivate, errBoom, "application: reactivate loja-acme: boom"},
		{"ReactivateStore, save", []option{withSaveError(errBoom)}, (*harness).seedSuspended, reactivate, errBoom, "application: reactivate loja-acme: boom"},
		{"ReactivateStore, enqueue", []option{withEnqueueError(errBoom)}, (*harness).seedSuspended, reactivate, errBoom, "application: reactivate loja-acme: enqueue: boom"},
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
