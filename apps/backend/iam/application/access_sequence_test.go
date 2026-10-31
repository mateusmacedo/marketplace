package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/application"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func stepsOver(table string) []string {
	steps := slices.Clone(nineSteps)
	for i, step := range steps {
		switch step {
		case "roles.Load":
			steps[i] = table + ".Load"
		case "roles.Save":
			steps[i] = table + ".Save"
		}
	}
	return steps
}

type accessCommand struct {
	name    string
	table   string
	seed    func(*harness, *testing.T)
	run     func(*testing.T, *harness) error
	prefix  string
	action  string
	object  string
	aggType string
}

// audited is the Object the trail records: the address of an invitation goes
// as its digest, never in clear, while the outbox keeps it as the aggregate.
func (c accessCommand) audited(h *harness) string {
	if c.aggType == application.AggregateTypeInvitation {
		return application.EmailDigest(h.service, domain.Email(c.object))
	}
	return c.object
}

func (c accessCommand) withSeed(h *harness, t *testing.T) {
	if c.seed != nil {
		c.seed(h, t)
	}
}

var accessCommands = []accessCommand{
	{"InviteMember", "invitations", nil, func(t *testing.T, h *harness) error {
		_, err := h.service.InviteMember(withExecution(t, context.Background()), application.InviteMember{Email: string(testEmail), Roles: []domain.RoleRef{"expedicao"}})
		return err
	}, "application: invite member: ", application.OperationInviteMember, string(testEmail), application.AggregateTypeInvitation},
	{"ClaimOwnership", "memberships", nil, func(t *testing.T, h *harness) error {
		_, err := h.service.ClaimOwnership(withExecution(t, context.Background()), application.ClaimOwnership{Subject: testSubject})
		return err
	}, "application: claim ownership kc-123: ", application.OperationClaimOwnership, string(testSubject), application.AggregateTypeMembership},
	{"GrantRole", "memberships", func(h *harness, t *testing.T) { h.seedMember(t, "expedicao") }, func(t *testing.T, h *harness) error {
		_, err := h.service.GrantRole(withExecution(t, context.Background()), application.GrantRole{Subject: testSubject, Role: "catalogo"})
		return err
	}, "application: grant role kc-123: ", application.OperationGrantRole, string(testSubject), application.AggregateTypeMembership},
	{"RevokeRole", "memberships", func(h *harness, t *testing.T) { h.seedMember(t, "expedicao", "catalogo") }, func(t *testing.T, h *harness) error {
		_, err := h.service.RevokeRole(withExecution(t, context.Background()), application.RevokeRole{Subject: testSubject, Role: "catalogo"})
		return err
	}, "application: revoke role kc-123: ", application.OperationRevokeRole, string(testSubject), application.AggregateTypeMembership},
	{"RemoveMember", "memberships", func(h *harness, t *testing.T) { h.seedMember(t, "expedicao") }, func(t *testing.T, h *harness) error {
		_, err := h.service.RemoveMember(withExecution(t, context.Background()), application.RemoveMember{Subject: testSubject})
		return err
	}, "application: remove member kc-123: ", application.OperationRemoveMember, string(testSubject), application.AggregateTypeMembership},
}

func TestEachAccessCommandWalksTheNineStepsInOrderAndEnqueuesOneEvent(t *testing.T) {
	for _, c := range accessCommands {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			c.withSeed(h, t)

			if err := c.run(t, h); err != nil {
				t.Fatalf("%s() error = %v, want nil", c.name, err)
			}

			if want := stepsOver(c.table); !slices.Equal(h.rec.Observed(), want) {
				t.Fatalf("sequence mismatch (FND-04 §3.2)\ngot:  %v\nwant: %v", h.rec.Observed(), want)
			}
			entries := h.store.Entries()
			if len(entries) != 1 || entries[0].AggregateType != c.aggType || entries[0].AggregateID != c.object || entries[0].Intent.Destination != application.Destination {
				t.Fatalf("outbox = %+v, want one entry of %s %s to %s", entries, c.aggType, c.object, application.Destination)
			}
		})
	}
}

func TestEachAccessCommandAuditsOnce(t *testing.T) {
	for _, c := range accessCommands {
		t.Run(c.name, func(t *testing.T) {
			h, instr := newInstrumentedHarness(t)
			c.withSeed(h, t)

			if err := c.run(t, h); err != nil {
				t.Fatalf("%s() error = %v, want nil", c.name, err)
			}

			instr.requireOnlyAudit(t, ports.AuditEvent{Object: c.audited(h), Action: c.action, Outcome: ports.OutcomeAccepted, At: testOccurred})
		})
	}
}

func TestEachFailureOfAnAccessCommandCarriesTheUseCasePrefix(t *testing.T) {
	for _, c := range accessCommands {
		for name, fault := range map[string]option{"load": withLoadError(errBoom), "save": withSaveError(errBoom), "enqueue": withEnqueueError(errBoom)} {
			t.Run(c.name+", "+name, func(t *testing.T) {
				h := newHarness(t, fault)
				c.withSeed(h, t)

				err := c.run(t, h)

				want := c.prefix + "boom"
				if name == "enqueue" {
					want = c.prefix + "enqueue: boom"
				}
				if !errors.Is(err, errBoom) || err.Error() != want {
					t.Fatalf("err = %v, want %q", err, want)
				}
			})
		}
	}
}

func TestTheEventsOfTheAccessCommandsCarryTheirState(t *testing.T) {
	h := newHarness(t)
	h.seedMember(t, "expedicao")

	if _, err := h.service.GrantRole(withExecution(t, context.Background()), application.GrantRole{Subject: testSubject, Role: "catalogo"}); err != nil {
		t.Fatalf("GrantRole() = %v", err)
	}
	if _, err := h.service.InviteMember(withExecution(t, context.Background()), application.InviteMember{Email: " Ana@Loja.com ", Roles: []domain.RoleRef{"vendas"}}); err != nil {
		t.Fatalf("InviteMember() = %v", err)
	}

	entries := h.store.Entries()
	changed, ok := entries[0].Event.(domain.MemberAccessChanged)
	if !ok || changed.Subject != testSubject || !slices.Equal(changed.Roles, []domain.RoleRef{"catalogo", "expedicao"}) || changed.AccessVersion != 2 || entries[0].AggregateVersion != 2 {
		t.Fatalf("first entry = %+v, want the access of kc-123 at version 2", entries[0])
	}
	invited, ok := entries[1].Event.(domain.MemberInvited)
	if !ok || invited.Email != testEmail || invited.Round != 1 || !slices.Equal(invited.Roles, []domain.RoleRef{"vendas"}) {
		t.Fatalf("second entry = %+v, want ana@loja.com invited in round 1", entries[1])
	}
}
