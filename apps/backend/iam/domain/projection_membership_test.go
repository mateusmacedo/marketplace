package domain_test

import (
	"strconv"
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/domainkit"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestMembershipsMatchTheProjectionFixture(t *testing.T) {
	f, err := tb.DecodeProjection([]byte(membershipProjection))
	if err != nil {
		t.Fatalf("decode the membership projection: %v", err)
	}
	if f.Identity.Context != "iam" || f.Identity.Aggregate != "membership" || len(f.Cases) != 24 {
		t.Fatalf("fixture identity/cases = %+v/%d", f.Identity, len(f.Cases))
	}
	tb.RunProjection(t, f, runMembership)
}

func runMembership(t *testing.T, c tb.ProjectionCase) (domainkit.Projection, domainkit.Verdict) {
	t.Helper()
	when := kernel.Instant(integer(t, c.Command["at"]))
	before := func() *domain.Membership { return membershipFromState(t, c.StateBefore) }
	subjectOf := func(s domain.Subject) domainkit.Fields { return domainkit.Fields{"subject": string(s)} }
	role := domain.RoleRef(c.Command["role"])
	switch c.Command["upr"] {
	case "establish":
		s := membershipSubject(func(m *domain.Membership) (kernel.Accepted[domain.EstablishedResponse], *kernel.Rejection) {
			return m.Establish(domain.EstablishMembership{
				Subject: domain.Subject(c.Command["subject"]), Email: domain.Email(c.Command["email"]), Roles: refsOf(c.Command["roles"]), At: when,
			})
		}, func(r domain.EstablishedResponse) domainkit.Fields { return subjectOf(r.Subject) })
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	case "grant":
		s := membershipSubject(func(m *domain.Membership) (kernel.Accepted[domain.GrantedResponse], *kernel.Rejection) {
			return m.Grant(domain.GrantRole{Role: role, At: when})
		}, func(r domain.GrantedResponse) domainkit.Fields { return subjectOf(r.Subject) })
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	case "revoke":
		s := membershipSubject(func(m *domain.Membership) (kernel.Accepted[domain.RevokedResponse], *kernel.Rejection) {
			return m.Revoke(domain.RevokeRole{Role: role, At: when})
		}, func(r domain.RevokedResponse) domainkit.Fields { return subjectOf(r.Subject) })
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	case "remove":
		s := membershipSubject(func(m *domain.Membership) (kernel.Accepted[domain.RemovedResponse], *kernel.Rejection) {
			return m.Remove(domain.RemoveMember{At: when})
		}, func(r domain.RemovedResponse) domainkit.Fields { return subjectOf(r.Subject) })
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	case "confirm-access-synced":
		s := membershipSubject(func(m *domain.Membership) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
			return m.ConfirmAccessSynced(domain.ConfirmMemberAccessSynced{Version: integer(t, c.Command["version"])})
		}, func(kernel.Empty) domainkit.Fields { return domainkit.Fields{} })
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	default:
		t.Fatalf("unknown upr %q in case %s", c.Command["upr"], c.Name)
		return domainkit.Projection{}, domainkit.Verdict{}
	}
}

var membershipStatusNames = map[domain.MembershipStatus]string{
	domain.MembershipStatusUndefined: "undefined",
	domain.MembershipStatusActive:    "active",
	domain.MembershipStatusRemoved:   "removed",
}

func membershipState(m *domain.Membership) domainkit.Fields {
	snap := m.Snapshot()
	return domainkit.Fields{
		"subject":        string(snap.Subject),
		"email":          string(snap.Email),
		"roles":          joinedRefs(snap.Roles),
		"status":         membershipStatusNames[snap.Status],
		"access_version": strconv.FormatInt(snap.AccessVersion, 10),
		"synced_version": strconv.FormatInt(snap.SyncedVersion, 10),
		"joined_at":      instant(snap.JoinedAt),
		"changed_at":     instant(snap.ChangedAt),
	}
}

func membershipFromState(t *testing.T, f domainkit.Fields) *domain.Membership {
	t.Helper()
	snap := domain.MembershipSnapshot{
		Subject:       domain.Subject(f["subject"]),
		Email:         domain.Email(f["email"]),
		Roles:         refsOf(f["roles"]),
		AccessVersion: integer(t, f["access_version"]),
		SyncedVersion: integer(t, f["synced_version"]),
		JoinedAt:      kernel.Instant(integer(t, f["joined_at"])),
		ChangedAt:     kernel.Instant(integer(t, f["changed_at"])),
	}
	for status, name := range membershipStatusNames {
		if name == f["status"] {
			snap.Status = status
		}
	}
	return domain.FromMembershipSnapshot(snap)
}

func membershipEvent(e kernel.DomainEvent) (string, domainkit.Fields) {
	version := func(v int64) string { return strconv.FormatInt(v, 10) }
	switch ev := e.(type) {
	case domain.MembershipEstablished:
		return ev.EventName(), domainkit.Fields{"subject": string(ev.Subject), "email": string(ev.Email), "roles": joinedRefs(ev.Roles), "access_version": version(ev.AccessVersion), "at": instant(ev.At)}
	case domain.MemberAccessChanged:
		return ev.EventName(), domainkit.Fields{"subject": string(ev.Subject), "roles": joinedRefs(ev.Roles), "access_version": version(ev.AccessVersion), "at": instant(ev.At)}
	case domain.MemberRemoved:
		return ev.EventName(), domainkit.Fields{"subject": string(ev.Subject), "access_version": version(ev.AccessVersion), "at": instant(ev.At)}
	default:
		return e.EventName(), domainkit.Fields{}
	}
}

func membershipSubject[R any](decide func(*domain.Membership) (kernel.Accepted[R], *kernel.Rejection), response func(R) domainkit.Fields) domainkit.Subject[*domain.Membership, R] {
	return domainkit.Subject[*domain.Membership, R]{
		Decide:   decide,
		Response: response,
		Event:    membershipEvent,
		Snapshot: membershipState,
		Clone:    func(m *domain.Membership) *domain.Membership { return domain.FromMembershipSnapshot(m.Snapshot()) },
	}
}

// membershipProjection is the ORA-30 fixture of the membership, kept in the
// test for the same reason as the role's.
const membershipProjection = `{
  "format_version": "1",
  "identity": {
    "context": "iam",
    "aggregate": "membership"
  },
  "cases": [
    {
      "name": "establish-accepted",
      "doc": "Ramo Accepted de Establish: sujeito desconhecido nasce active, versão de acesso 1, papéis ordenados; um membership-established.",
      "state_before": {
        "subject": "kc-123",
        "email": "",
        "roles": "",
        "status": "undefined",
        "access_version": "0",
        "synced_version": "0",
        "joined_at": "0",
        "changed_at": "0"
      },
      "command": {
        "upr": "establish",
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "expedicao,catalogo,expedicao",
        "at": "1755432000000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "subject": "kc-123"
        },
        "events": [
          {
            "name": "iam.membership-established",
            "fields": {
              "subject": "kc-123",
              "email": "ana@loja.com",
              "roles": "catalogo,expedicao",
              "access_version": "1",
              "at": "1755432000000000000"
            }
          }
        ],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "catalogo,expedicao",
          "status": "active",
          "access_version": "1",
          "synced_version": "0",
          "joined_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "establish-claims-ownership",
      "doc": "Ramo Accepted de Establish pelo ClaimOwnership: o dono nasce active com owner e sem e-mail.",
      "state_before": {
        "subject": "kc-owner",
        "email": "",
        "roles": "",
        "status": "undefined",
        "access_version": "0",
        "synced_version": "0",
        "joined_at": "0",
        "changed_at": "0"
      },
      "command": {
        "upr": "establish",
        "subject": "kc-owner",
        "email": "",
        "roles": "owner",
        "at": "1755432000000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "subject": "kc-owner"
        },
        "events": [
          {
            "name": "iam.membership-established",
            "fields": {
              "subject": "kc-owner",
              "email": "",
              "roles": "owner",
              "access_version": "1",
              "at": "1755432000000000000"
            }
          }
        ],
        "state_after": {
          "subject": "kc-owner",
          "email": "",
          "roles": "owner",
          "status": "active",
          "access_version": "1",
          "synced_version": "0",
          "joined_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "establish-returns",
      "doc": "Ramo Accepted de Establish sobre vínculo removed: volta a active com os papéis do comando e mantém o e-mail que tinha; um membership-established.",
      "state_before": {
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "removed",
        "access_version": "2",
        "synced_version": "2",
        "joined_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "establish",
        "subject": "kc-123",
        "email": "",
        "roles": "vendas",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "subject": "kc-123"
        },
        "events": [
          {
            "name": "iam.membership-established",
            "fields": {
              "subject": "kc-123",
              "email": "ana@loja.com",
              "roles": "vendas",
              "access_version": "3",
              "at": "1755435600000000000"
            }
          }
        ],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "vendas",
          "status": "active",
          "access_version": "3",
          "synced_version": "2",
          "joined_at": "1755432000000000000",
          "changed_at": "1755435600000000000"
        }
      }
    },
    {
      "name": "establish-adds-roles",
      "doc": "Ramo Accepted de Establish sobre vínculo active: soma os papéis, preenche o e-mail ausente; um member-access-changed.",
      "state_before": {
        "subject": "kc-123",
        "email": "",
        "roles": "expedicao",
        "status": "active",
        "access_version": "1",
        "synced_version": "1",
        "joined_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "establish",
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "catalogo",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "subject": "kc-123"
        },
        "events": [
          {
            "name": "iam.member-access-changed",
            "fields": {
              "subject": "kc-123",
              "roles": "catalogo,expedicao",
              "access_version": "2",
              "at": "1755435600000000000"
            }
          }
        ],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "catalogo,expedicao",
          "status": "active",
          "access_version": "2",
          "synced_version": "1",
          "joined_at": "1755432000000000000",
          "changed_at": "1755435600000000000"
        }
      }
    },
    {
      "name": "establish-held-roles",
      "doc": "Ramo Accepted de Establish com papéis já mantidos: o dono reclama de novo sem efeito.",
      "state_before": {
        "subject": "kc-owner",
        "email": "",
        "roles": "owner",
        "status": "active",
        "access_version": "1",
        "synced_version": "1",
        "joined_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "establish",
        "subject": "kc-owner",
        "email": "",
        "roles": "owner",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "subject": "kc-owner"
        },
        "events": [],
        "state_after": {
          "subject": "kc-owner",
          "email": "",
          "roles": "owner",
          "status": "active",
          "access_version": "1",
          "synced_version": "1",
          "joined_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "establish-subject-invalid",
      "doc": "Ramo Rejected de Establish com subject vazio; estado inalterado.",
      "state_before": {
        "subject": "",
        "email": "",
        "roles": "",
        "status": "undefined",
        "access_version": "0",
        "synced_version": "0",
        "joined_at": "0",
        "changed_at": "0"
      },
      "command": {
        "upr": "establish",
        "subject": "",
        "email": "",
        "roles": "expedicao",
        "at": "1755432000000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/membership/subject-invalid",
          "message": "subject must have 1 to 255 characters and no control character",
          "details": {}
        },
        "events": [],
        "state_after": {
          "subject": "",
          "email": "",
          "roles": "",
          "status": "undefined",
          "access_version": "0",
          "synced_version": "0",
          "joined_at": "0",
          "changed_at": "0"
        }
      }
    },
    {
      "name": "establish-last-role",
      "doc": "Ramo Rejected de Establish sem papéis; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "email": "",
        "roles": "",
        "status": "undefined",
        "access_version": "0",
        "synced_version": "0",
        "joined_at": "0",
        "changed_at": "0"
      },
      "command": {
        "upr": "establish",
        "subject": "kc-123",
        "email": "",
        "roles": "",
        "at": "1755432000000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/membership/last-role",
          "message": "roles must not be empty",
          "details": {}
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "email": "",
          "roles": "",
          "status": "undefined",
          "access_version": "0",
          "synced_version": "0",
          "joined_at": "0",
          "changed_at": "0"
        }
      }
    },
    {
      "name": "establish-role-invalid",
      "doc": "Ramo Rejected de Establish com papel fora da forma de ref; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "email": "",
        "roles": "",
        "status": "undefined",
        "access_version": "0",
        "synced_version": "0",
        "joined_at": "0",
        "changed_at": "0"
      },
      "command": {
        "upr": "establish",
        "subject": "kc-123",
        "email": "",
        "roles": "Exp_1",
        "at": "1755432000000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/membership/role-invalid",
          "message": "role must be a ref of 3 to 63 characters of a-z, 0-9 and -, without a leading or trailing hyphen",
          "details": {
            "role": "Exp_1"
          }
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "email": "",
          "roles": "",
          "status": "undefined",
          "access_version": "0",
          "synced_version": "0",
          "joined_at": "0",
          "changed_at": "0"
        }
      }
    },
    {
      "name": "grant-accepted",
      "doc": "Ramo Accepted de Grant: o papel entra e a versão de acesso sobe; um member-access-changed.",
      "state_before": {
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "active",
        "access_version": "1",
        "synced_version": "1",
        "joined_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "grant",
        "role": "catalogo",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "subject": "kc-123"
        },
        "events": [
          {
            "name": "iam.member-access-changed",
            "fields": {
              "subject": "kc-123",
              "roles": "catalogo,expedicao",
              "access_version": "2",
              "at": "1755435600000000000"
            }
          }
        ],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "catalogo,expedicao",
          "status": "active",
          "access_version": "2",
          "synced_version": "1",
          "joined_at": "1755432000000000000",
          "changed_at": "1755435600000000000"
        }
      }
    },
    {
      "name": "grant-held",
      "doc": "Ramo Accepted de Grant de papel já mantido: aceita sem alterar e sem emitir.",
      "state_before": {
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "active",
        "access_version": "1",
        "synced_version": "1",
        "joined_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "grant",
        "role": "expedicao",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "subject": "kc-123"
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "active",
          "access_version": "1",
          "synced_version": "1",
          "joined_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "grant-removed",
      "doc": "Ramo Rejected de Grant sobre vínculo removed; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "removed",
        "access_version": "2",
        "synced_version": "2",
        "joined_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "grant",
        "role": "catalogo",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/membership/removed",
          "message": "member is removed",
          "details": {}
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "removed",
          "access_version": "2",
          "synced_version": "2",
          "joined_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "grant-role-invalid",
      "doc": "Ramo Rejected de Grant com papel fora da forma de ref; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "active",
        "access_version": "1",
        "synced_version": "1",
        "joined_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "grant",
        "role": "Exp_1",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/membership/role-invalid",
          "message": "role must be a ref of 3 to 63 characters of a-z, 0-9 and -, without a leading or trailing hyphen",
          "details": {
            "role": "Exp_1"
          }
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "active",
          "access_version": "1",
          "synced_version": "1",
          "joined_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "grant-owner-reserved",
      "doc": "Ramo Rejected de Grant de owner; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "active",
        "access_version": "1",
        "synced_version": "1",
        "joined_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "grant",
        "role": "owner",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/membership/owner-reserved",
          "message": "owner is reserved for the owner of the store",
          "details": {}
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "active",
          "access_version": "1",
          "synced_version": "1",
          "joined_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "revoke-accepted",
      "doc": "Ramo Accepted de Revoke: o papel sai e a versão de acesso sobe; um member-access-changed.",
      "state_before": {
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "catalogo,expedicao",
        "status": "active",
        "access_version": "1",
        "synced_version": "1",
        "joined_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "revoke",
        "role": "catalogo",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "subject": "kc-123"
        },
        "events": [
          {
            "name": "iam.member-access-changed",
            "fields": {
              "subject": "kc-123",
              "roles": "expedicao",
              "access_version": "2",
              "at": "1755435600000000000"
            }
          }
        ],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "active",
          "access_version": "2",
          "synced_version": "1",
          "joined_at": "1755432000000000000",
          "changed_at": "1755435600000000000"
        }
      }
    },
    {
      "name": "revoke-removed",
      "doc": "Ramo Rejected de Revoke sobre vínculo removed; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "removed",
        "access_version": "2",
        "synced_version": "2",
        "joined_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "revoke",
        "role": "expedicao",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/membership/removed",
          "message": "member is removed",
          "details": {}
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "removed",
          "access_version": "2",
          "synced_version": "2",
          "joined_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "revoke-owner-protected",
      "doc": "Ramo Rejected de Revoke de owner do dono; estado inalterado.",
      "state_before": {
        "subject": "kc-owner",
        "email": "",
        "roles": "owner",
        "status": "active",
        "access_version": "1",
        "synced_version": "1",
        "joined_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "revoke",
        "role": "owner",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/membership/owner-protected",
          "message": "the owner of the store keeps the owner role",
          "details": {}
        },
        "events": [],
        "state_after": {
          "subject": "kc-owner",
          "email": "",
          "roles": "owner",
          "status": "active",
          "access_version": "1",
          "synced_version": "1",
          "joined_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "revoke-role-not-held",
      "doc": "Ramo Rejected de Revoke de papel não mantido, nomeado no detalhe; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "catalogo,expedicao",
        "status": "active",
        "access_version": "1",
        "synced_version": "1",
        "joined_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "revoke",
        "role": "vendas",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/membership/role-not-held",
          "message": "member does not hold the role",
          "details": {
            "role": "vendas"
          }
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "catalogo,expedicao",
          "status": "active",
          "access_version": "1",
          "synced_version": "1",
          "joined_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "revoke-last-role",
      "doc": "Ramo Rejected de Revoke do único papel; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "active",
        "access_version": "1",
        "synced_version": "1",
        "joined_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "revoke",
        "role": "expedicao",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/membership/last-role",
          "message": "the member would be left without a role",
          "details": {}
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "active",
          "access_version": "1",
          "synced_version": "1",
          "joined_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "remove-accepted",
      "doc": "Ramo Accepted de Remove: o vínculo vai a removed, mantém os papéis e a versão de acesso sobe; um member-removed.",
      "state_before": {
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "active",
        "access_version": "1",
        "synced_version": "1",
        "joined_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "remove",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "subject": "kc-123"
        },
        "events": [
          {
            "name": "iam.member-removed",
            "fields": {
              "subject": "kc-123",
              "access_version": "2",
              "at": "1755435600000000000"
            }
          }
        ],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "removed",
          "access_version": "2",
          "synced_version": "1",
          "joined_at": "1755432000000000000",
          "changed_at": "1755435600000000000"
        }
      }
    },
    {
      "name": "remove-removed",
      "doc": "Ramo Rejected de Remove sobre vínculo removed; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "removed",
        "access_version": "2",
        "synced_version": "2",
        "joined_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "remove",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/membership/removed",
          "message": "member is removed",
          "details": {}
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "removed",
          "access_version": "2",
          "synced_version": "2",
          "joined_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "remove-owner-protected",
      "doc": "Ramo Rejected de Remove do dono; estado inalterado.",
      "state_before": {
        "subject": "kc-owner",
        "email": "",
        "roles": "owner",
        "status": "active",
        "access_version": "1",
        "synced_version": "1",
        "joined_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "remove",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/membership/owner-protected",
          "message": "the owner of the store is never removed",
          "details": {}
        },
        "events": [],
        "state_after": {
          "subject": "kc-owner",
          "email": "",
          "roles": "owner",
          "status": "active",
          "access_version": "1",
          "synced_version": "1",
          "joined_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "confirm-access-synced-accepted",
      "doc": "Ramo Accepted de ConfirmAccessSynced: a versão confirmada sobe até a de acesso, sem evento.",
      "state_before": {
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "active",
        "access_version": "3",
        "synced_version": "1",
        "joined_at": "1755432000000000000",
        "changed_at": "1755435600000000000"
      },
      "command": {
        "upr": "confirm-access-synced",
        "version": "3"
      },
      "expected": {
        "branch": "accepted",
        "response": {},
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "active",
          "access_version": "3",
          "synced_version": "3",
          "joined_at": "1755432000000000000",
          "changed_at": "1755435600000000000"
        }
      }
    },
    {
      "name": "confirm-access-synced-late",
      "doc": "Ramo Accepted de ConfirmAccessSynced atrasado: a versão confirmada não volta; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "active",
        "access_version": "3",
        "synced_version": "3",
        "joined_at": "1755432000000000000",
        "changed_at": "1755435600000000000"
      },
      "command": {
        "upr": "confirm-access-synced",
        "version": "2"
      },
      "expected": {
        "branch": "accepted",
        "response": {},
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "active",
          "access_version": "3",
          "synced_version": "3",
          "joined_at": "1755432000000000000",
          "changed_at": "1755435600000000000"
        }
      }
    },
    {
      "name": "confirm-access-synced-version-ahead",
      "doc": "Ramo Rejected de ConfirmAccessSynced com versão acima da de acesso; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "email": "ana@loja.com",
        "roles": "expedicao",
        "status": "active",
        "access_version": "2",
        "synced_version": "0",
        "joined_at": "1755432000000000000",
        "changed_at": "1755435600000000000"
      },
      "command": {
        "upr": "confirm-access-synced",
        "version": "3"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/membership/version-ahead",
          "message": "version is ahead of the access version of the member",
          "details": {
            "version": "3",
            "accessVersion": "2"
          }
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "email": "ana@loja.com",
          "roles": "expedicao",
          "status": "active",
          "access_version": "2",
          "synced_version": "0",
          "joined_at": "1755432000000000000",
          "changed_at": "1755435600000000000"
        }
      }
    }
  ]
}
`
