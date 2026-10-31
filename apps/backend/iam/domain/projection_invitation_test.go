package domain_test

import (
	"strconv"
	"strings"
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/domainkit"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestInvitationsMatchTheProjectionFixture(t *testing.T) {
	f, err := tb.DecodeProjection([]byte(invitationProjection))
	if err != nil {
		t.Fatalf("decode the invitation projection: %v", err)
	}
	if f.Identity.Context != "iam" || f.Identity.Aggregate != "invitation" || len(f.Cases) != 14 {
		t.Fatalf("fixture identity/cases = %+v/%d", f.Identity, len(f.Cases))
	}
	tb.RunProjection(t, f, runInvitation)
}

func runInvitation(t *testing.T, c tb.ProjectionCase) (domainkit.Projection, domainkit.Verdict) {
	t.Helper()
	when := kernel.Instant(integer(t, c.Command["at"]))
	before := func() *domain.Invitation { return invitationFromState(t, c.StateBefore) }
	empty := func(kernel.Empty) domainkit.Fields { return domainkit.Fields{} }
	switch c.Command["upr"] {
	case "invite":
		s := invitationSubject(func(i *domain.Invitation) (kernel.Accepted[domain.InvitedResponse], *kernel.Rejection) {
			return i.Invite(domain.InviteMember{Email: c.Command["email"], Roles: refsOf(c.Command["roles"]), At: when})
		}, func(r domain.InvitedResponse) domainkit.Fields {
			return domainkit.Fields{"email": string(r.Email), "status": r.Status.String()}
		})
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	case "confirm":
		s := invitationSubject(func(i *domain.Invitation) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
			return i.Confirm(domain.ConfirmInvitation{Subject: domain.Subject(c.Command["subject"]), At: when})
		}, empty)
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	case "fail":
		s := invitationSubject(func(i *domain.Invitation) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
			return i.Fail(domain.FailInvitation{Reason: c.Command["reason"], At: when})
		}, empty)
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	default:
		t.Fatalf("unknown upr %q in case %s", c.Command["upr"], c.Name)
		return domainkit.Projection{}, domainkit.Verdict{}
	}
}

func refsOf(joined string) []domain.RoleRef {
	if joined == "" {
		return nil
	}
	var refs []domain.RoleRef
	for _, r := range strings.Split(joined, ",") {
		refs = append(refs, domain.RoleRef(r))
	}
	return refs
}

func joinedRefs(refs []domain.RoleRef) string {
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		parts = append(parts, string(r))
	}
	return strings.Join(parts, ",")
}

func instant(i kernel.Instant) string { return strconv.FormatInt(int64(i), 10) }

var invitationStatusNames = map[domain.InvitationStatus]string{
	domain.InvitationStatusUndefined:   "undefined",
	domain.InvitationStatusPending:     "pending",
	domain.InvitationStatusProvisioned: "provisioned",
	domain.InvitationStatusFailed:      "failed",
}

func invitationState(i *domain.Invitation) domainkit.Fields {
	snap := i.Snapshot()
	return domainkit.Fields{
		"email":          string(snap.Email),
		"roles":          joinedRefs(snap.Roles),
		"status":         invitationStatusNames[snap.Status],
		"round":          strconv.FormatInt(snap.Round, 10),
		"subject":        string(snap.Subject),
		"failure_reason": snap.FailureReason,
		"invited_at":     instant(snap.InvitedAt),
		"settled_at":     instant(snap.SettledAt),
	}
}

func invitationFromState(t *testing.T, f domainkit.Fields) *domain.Invitation {
	t.Helper()
	snap := domain.InvitationSnapshot{
		Email:         domain.Email(f["email"]),
		Roles:         refsOf(f["roles"]),
		Round:         integer(t, f["round"]),
		Subject:       domain.Subject(f["subject"]),
		FailureReason: f["failure_reason"],
		InvitedAt:     kernel.Instant(integer(t, f["invited_at"])),
		SettledAt:     kernel.Instant(integer(t, f["settled_at"])),
	}
	for status, name := range invitationStatusNames {
		if name == f["status"] {
			snap.Status = status
		}
	}
	return domain.FromInvitationSnapshot(snap)
}

func invitationEvent(e kernel.DomainEvent) (string, domainkit.Fields) {
	switch ev := e.(type) {
	case domain.MemberInvited:
		return ev.EventName(), domainkit.Fields{"email": string(ev.Email), "roles": joinedRefs(ev.Roles), "round": strconv.FormatInt(ev.Round, 10), "at": instant(ev.At)}
	case domain.InvitationProvisioned:
		return ev.EventName(), domainkit.Fields{"email": string(ev.Email), "subject": string(ev.Subject), "roles": joinedRefs(ev.Roles), "at": instant(ev.At)}
	case domain.InvitationFailed:
		return ev.EventName(), domainkit.Fields{"email": string(ev.Email), "reason": ev.Reason, "at": instant(ev.At)}
	default:
		return e.EventName(), domainkit.Fields{}
	}
}

func invitationSubject[R any](decide func(*domain.Invitation) (kernel.Accepted[R], *kernel.Rejection), response func(R) domainkit.Fields) domainkit.Subject[*domain.Invitation, R] {
	return domainkit.Subject[*domain.Invitation, R]{
		Decide:   decide,
		Response: response,
		Event:    invitationEvent,
		Snapshot: invitationState,
		Clone:    func(i *domain.Invitation) *domain.Invitation { return domain.FromInvitationSnapshot(i.Snapshot()) },
	}
}

// invitationProjection is the ORA-30 fixture of the invitation, kept in the
// test for the same reason as the role's.
const invitationProjection = `{
  "format_version": "1",
  "identity": {
    "context": "iam",
    "aggregate": "invitation"
  },
  "cases": [
    {
      "name": "invite-accepted",
      "doc": "Ramo Accepted de Invite: e-mail desconhecido, normalizado, nasce pending na rodada 1 com os papéis ordenados e sem repetição; um member-invited.",
      "state_before": {
        "email": "ana@loja.com",
        "roles": "",
        "status": "undefined",
        "round": "0",
        "subject": "",
        "failure_reason": "",
        "invited_at": "0",
        "settled_at": "0"
      },
      "command": {
        "upr": "invite",
        "email": " Ana@Loja.com ",
        "roles": "expedicao,catalogo,expedicao",
        "at": "1755432000000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "email": "ana@loja.com",
          "status": "pending"
        },
        "events": [
          {
            "name": "iam.member-invited",
            "fields": {
              "email": "ana@loja.com",
              "roles": "catalogo,expedicao",
              "round": "1",
              "at": "1755432000000000000"
            }
          }
        ],
        "state_after": {
          "email": "ana@loja.com",
          "roles": "catalogo,expedicao",
          "status": "pending",
          "round": "1",
          "subject": "",
          "failure_reason": "",
          "invited_at": "1755432000000000000",
          "settled_at": "0"
        }
      }
    },
    {
      "name": "invite-reopens-failed",
      "doc": "Ramo Accepted de Invite sobre convite failed: volta a pending na rodada 2, sem motivo nem fechamento; um member-invited.",
      "state_before": {
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "failed",
        "round": "1",
        "subject": "",
        "failure_reason": "recusado",
        "invited_at": "1755432000000000000",
        "settled_at": "1755432000000000000"
      },
      "command": {
        "upr": "invite",
        "email": "ana@loja.com",
        "roles": "catalogo",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "email": "ana@loja.com",
          "status": "pending"
        },
        "events": [
          {
            "name": "iam.member-invited",
            "fields": {
              "email": "ana@loja.com",
              "roles": "catalogo",
              "round": "2",
              "at": "1755435600000000000"
            }
          }
        ],
        "state_after": {
          "email": "ana@loja.com",
          "roles": "catalogo",
          "status": "pending",
          "round": "2",
          "subject": "",
          "failure_reason": "",
          "invited_at": "1755435600000000000",
          "settled_at": "0"
        }
      }
    },
    {
      "name": "invite-reopens-provisioned",
      "doc": "Ramo Accepted de Invite sobre convite provisioned: volta a pending na rodada 2 com o subject vazio; um member-invited.",
      "state_before": {
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "provisioned",
        "round": "1",
        "subject": "kc-123",
        "failure_reason": "",
        "invited_at": "1755432000000000000",
        "settled_at": "1755432000000000000"
      },
      "command": {
        "upr": "invite",
        "email": "ana@loja.com",
        "roles": "catalogo",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "email": "ana@loja.com",
          "status": "pending"
        },
        "events": [
          {
            "name": "iam.member-invited",
            "fields": {
              "email": "ana@loja.com",
              "roles": "catalogo",
              "round": "2",
              "at": "1755435600000000000"
            }
          }
        ],
        "state_after": {
          "email": "ana@loja.com",
          "roles": "catalogo",
          "status": "pending",
          "round": "2",
          "subject": "",
          "failure_reason": "",
          "invited_at": "1755435600000000000",
          "settled_at": "0"
        }
      }
    },
    {
      "name": "invite-pending",
      "doc": "Ramo Accepted de Invite sobre convite pending: aceita sem alterar e sem emitir.",
      "state_before": {
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "pending",
        "round": "1",
        "subject": "",
        "failure_reason": "",
        "invited_at": "1755432000000000000",
        "settled_at": "0"
      },
      "command": {
        "upr": "invite",
        "email": "ana@loja.com",
        "roles": "catalogo",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "email": "ana@loja.com",
          "status": "pending"
        },
        "events": [],
        "state_after": {
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "pending",
          "round": "1",
          "subject": "",
          "failure_reason": "",
          "invited_at": "1755432000000000000",
          "settled_at": "0"
        }
      }
    },
    {
      "name": "invite-email-invalid",
      "doc": "Ramo Rejected de Invite: e-mail fora da forma; estado inalterado.",
      "state_before": {
        "email": "ana",
        "roles": "",
        "status": "undefined",
        "round": "0",
        "subject": "",
        "failure_reason": "",
        "invited_at": "0",
        "settled_at": "0"
      },
      "command": {
        "upr": "invite",
        "email": "ana",
        "roles": "expedicao",
        "at": "1755432000000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/invitation/email-invalid",
          "message": "email must have up to 254 characters, exactly one @, a non-empty local part, a domain with a dot and no control character",
          "details": {
            "email": "ana"
          }
        },
        "events": [],
        "state_after": {
          "email": "ana",
          "roles": "",
          "status": "undefined",
          "round": "0",
          "subject": "",
          "failure_reason": "",
          "invited_at": "0",
          "settled_at": "0"
        }
      }
    },
    {
      "name": "invite-roles-empty",
      "doc": "Ramo Rejected de Invite: sem papéis; estado inalterado.",
      "state_before": {
        "email": "ana@loja.com",
        "roles": "",
        "status": "undefined",
        "round": "0",
        "subject": "",
        "failure_reason": "",
        "invited_at": "0",
        "settled_at": "0"
      },
      "command": {
        "upr": "invite",
        "email": "ana@loja.com",
        "roles": "",
        "at": "1755432000000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/invitation/roles-empty",
          "message": "roles must not be empty",
          "details": {}
        },
        "events": [],
        "state_after": {
          "email": "ana@loja.com",
          "roles": "",
          "status": "undefined",
          "round": "0",
          "subject": "",
          "failure_reason": "",
          "invited_at": "0",
          "settled_at": "0"
        }
      }
    },
    {
      "name": "invite-role-invalid",
      "doc": "Ramo Rejected de Invite: papel fora da forma de ref, nomeado no detalhe; estado inalterado.",
      "state_before": {
        "email": "ana@loja.com",
        "roles": "",
        "status": "undefined",
        "round": "0",
        "subject": "",
        "failure_reason": "",
        "invited_at": "0",
        "settled_at": "0"
      },
      "command": {
        "upr": "invite",
        "email": "ana@loja.com",
        "roles": "Exp_1",
        "at": "1755432000000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/invitation/role-invalid",
          "message": "role must be a ref of 3 to 63 characters of a-z, 0-9 and -, without a leading or trailing hyphen",
          "details": {
            "role": "Exp_1"
          }
        },
        "events": [],
        "state_after": {
          "email": "ana@loja.com",
          "roles": "",
          "status": "undefined",
          "round": "0",
          "subject": "",
          "failure_reason": "",
          "invited_at": "0",
          "settled_at": "0"
        }
      }
    },
    {
      "name": "invite-role-reserved",
      "doc": "Ramo Rejected de Invite: owner não é convidável; estado inalterado.",
      "state_before": {
        "email": "ana@loja.com",
        "roles": "",
        "status": "undefined",
        "round": "0",
        "subject": "",
        "failure_reason": "",
        "invited_at": "0",
        "settled_at": "0"
      },
      "command": {
        "upr": "invite",
        "email": "ana@loja.com",
        "roles": "owner",
        "at": "1755432000000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/invitation/role-reserved",
          "message": "owner is not invitable: the owner is seeded with the store",
          "details": {}
        },
        "events": [],
        "state_after": {
          "email": "ana@loja.com",
          "roles": "",
          "status": "undefined",
          "round": "0",
          "subject": "",
          "failure_reason": "",
          "invited_at": "0",
          "settled_at": "0"
        }
      }
    },
    {
      "name": "confirm-accepted",
      "doc": "Ramo Accepted de Confirm: o Keycloak devolveu o sujeito; o convite vai a provisioned; um invitation-provisioned com os papéis.",
      "state_before": {
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "pending",
        "round": "1",
        "subject": "",
        "failure_reason": "",
        "invited_at": "1755432000000000000",
        "settled_at": "0"
      },
      "command": {
        "upr": "confirm",
        "subject": "kc-123",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {},
        "events": [
          {
            "name": "iam.invitation-provisioned",
            "fields": {
              "email": "ana@loja.com",
              "subject": "kc-123",
              "roles": "expedicao",
              "at": "1755435600000000000"
            }
          }
        ],
        "state_after": {
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "provisioned",
          "round": "1",
          "subject": "kc-123",
          "failure_reason": "",
          "invited_at": "1755432000000000000",
          "settled_at": "1755435600000000000"
        }
      }
    },
    {
      "name": "confirm-not-pending",
      "doc": "Ramo Rejected de Confirm por reentrega sobre convite provisioned; estado inalterado.",
      "state_before": {
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "provisioned",
        "round": "1",
        "subject": "kc-123",
        "failure_reason": "",
        "invited_at": "1755432000000000000",
        "settled_at": "1755432000000000000"
      },
      "command": {
        "upr": "confirm",
        "subject": "kc-456",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/invitation/not-pending",
          "message": "invitation is not pending",
          "details": {}
        },
        "events": [],
        "state_after": {
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "provisioned",
          "round": "1",
          "subject": "kc-123",
          "failure_reason": "",
          "invited_at": "1755432000000000000",
          "settled_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "confirm-subject-invalid",
      "doc": "Ramo Rejected de Confirm com subject vazio; estado inalterado.",
      "state_before": {
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "pending",
        "round": "1",
        "subject": "",
        "failure_reason": "",
        "invited_at": "1755432000000000000",
        "settled_at": "0"
      },
      "command": {
        "upr": "confirm",
        "subject": "",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/invitation/subject-invalid",
          "message": "subject must have 1 to 255 characters and no control character",
          "details": {}
        },
        "events": [],
        "state_after": {
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "pending",
          "round": "1",
          "subject": "",
          "failure_reason": "",
          "invited_at": "1755432000000000000",
          "settled_at": "0"
        }
      }
    },
    {
      "name": "fail-accepted",
      "doc": "Ramo Accepted de Fail: recusa final do Keycloak; o convite vai a failed com o motivo; um invitation-failed.",
      "state_before": {
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "pending",
        "round": "1",
        "subject": "",
        "failure_reason": "",
        "invited_at": "1755432000000000000",
        "settled_at": "0"
      },
      "command": {
        "upr": "fail",
        "reason": "recusado",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {},
        "events": [
          {
            "name": "iam.invitation-failed",
            "fields": {
              "email": "ana@loja.com",
              "reason": "recusado",
              "at": "1755435600000000000"
            }
          }
        ],
        "state_after": {
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "failed",
          "round": "1",
          "subject": "",
          "failure_reason": "recusado",
          "invited_at": "1755432000000000000",
          "settled_at": "1755435600000000000"
        }
      }
    },
    {
      "name": "fail-not-pending",
      "doc": "Ramo Rejected de Fail sobre convite failed; estado inalterado.",
      "state_before": {
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "failed",
        "round": "1",
        "subject": "",
        "failure_reason": "recusado",
        "invited_at": "1755432000000000000",
        "settled_at": "1755432000000000000"
      },
      "command": {
        "upr": "fail",
        "reason": "outro",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/invitation/not-pending",
          "message": "invitation is not pending",
          "details": {}
        },
        "events": [],
        "state_after": {
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "failed",
          "round": "1",
          "subject": "",
          "failure_reason": "recusado",
          "invited_at": "1755432000000000000",
          "settled_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "fail-reason-empty",
      "doc": "Ramo Rejected de Fail sem motivo; estado inalterado.",
      "state_before": {
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "pending",
        "round": "1",
        "subject": "",
        "failure_reason": "",
        "invited_at": "1755432000000000000",
        "settled_at": "0"
      },
      "command": {
        "upr": "fail",
        "reason": "",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/invitation/reason-empty",
          "message": "reason must not be empty",
          "details": {}
        },
        "events": [],
        "state_after": {
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "pending",
          "round": "1",
          "subject": "",
          "failure_reason": "",
          "invited_at": "1755432000000000000",
          "settled_at": "0"
        }
      }
    }
  ]
}
`
