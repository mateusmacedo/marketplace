package domain_test

import (
	"strconv"
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/domainkit"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/tenancy/domain"
)

func TestStoresMatchTheProjectionFixture(t *testing.T) {
	f, err := tb.DecodeProjection([]byte(storeProjection))
	if err != nil {
		t.Fatalf("decode the store projection: %v", err)
	}
	if f.Identity.Context != "tenancy" || f.Identity.Aggregate != "store" || len(f.Cases) != 9 {
		t.Fatalf("fixture identity/cases = %+v/%d", f.Identity, len(f.Cases))
	}
	tb.RunProjection(t, f, runStore)
}

func runStore(t *testing.T, c tb.ProjectionCase) (domainkit.Projection, domainkit.Verdict) {
	t.Helper()
	when := kernel.Instant(integer(t, c.Command["at"]))
	before := func() *domain.Store { return storeFromState(t, c.StateBefore) }
	switch c.Command["upr"] {
	case "register":
		s := subject(func(st *domain.Store) (kernel.Accepted[domain.RegisteredResponse], *kernel.Rejection) {
			return st.Register(domain.RegisterStore{Ref: domain.StoreRef(c.Command["ref"]), Name: domain.StoreName(c.Command["name"]), At: when})
		}, func(r domain.RegisteredResponse) domainkit.Fields { return domainkit.Fields{"ref": string(r.Ref)} })
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	case "suspend":
		s := subject(func(st *domain.Store) (kernel.Accepted[domain.SuspendedResponse], *kernel.Rejection) {
			return st.Suspend(domain.SuspendStore{Reason: c.Command["reason"], At: when})
		}, func(r domain.SuspendedResponse) domainkit.Fields { return domainkit.Fields{"ref": string(r.Ref)} })
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	case "reactivate":
		s := subject(func(st *domain.Store) (kernel.Accepted[domain.ReactivatedResponse], *kernel.Rejection) {
			return st.Reactivate(domain.ReactivateStore{At: when})
		}, func(r domain.ReactivatedResponse) domainkit.Fields { return domainkit.Fields{"ref": string(r.Ref)} })
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	default:
		t.Fatalf("unknown upr %q in case %s", c.Command["upr"], c.Name)
		return domainkit.Projection{}, domainkit.Verdict{}
	}
}

func integer(t *testing.T, s string) int64 {
	t.Helper()
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatalf("integer %q: %v", s, err)
	}
	return n
}

var statusNames = map[domain.StoreStatus]string{
	domain.Unregistered: "unregistered",
	domain.Active:       "active",
	domain.Suspended:    "suspended",
}

func storeState(s *domain.Store) domainkit.Fields {
	snap := s.Snapshot()
	return domainkit.Fields{
		"ref":               string(snap.Ref),
		"name":              string(snap.Name),
		"status":            statusNames[snap.Status],
		"suspension_reason": snap.SuspensionReason,
		"registered_at":     strconv.FormatInt(int64(snap.RegisteredAt), 10),
		"status_changed_at": strconv.FormatInt(int64(snap.StatusChangedAt), 10),
	}
}

func storeFromState(t *testing.T, f domainkit.Fields) *domain.Store {
	t.Helper()
	snap := domain.StoreSnapshot{
		Ref:              domain.StoreRef(f["ref"]),
		Name:             domain.StoreName(f["name"]),
		SuspensionReason: f["suspension_reason"],
		RegisteredAt:     kernel.Instant(integer(t, f["registered_at"])),
		StatusChangedAt:  kernel.Instant(integer(t, f["status_changed_at"])),
	}
	for status, name := range statusNames {
		if name == f["status"] {
			snap.Status = status
		}
	}
	return domain.FromStoreSnapshot(snap)
}

func storeEvent(e kernel.DomainEvent) (string, domainkit.Fields) {
	switch ev := e.(type) {
	case domain.StoreRegistered:
		return ev.EventName(), domainkit.Fields{"ref": string(ev.Ref), "name": string(ev.Name), "at": strconv.FormatInt(int64(ev.At), 10)}
	case domain.StoreSuspended:
		return ev.EventName(), domainkit.Fields{"ref": string(ev.Ref), "reason": ev.Reason, "at": strconv.FormatInt(int64(ev.At), 10)}
	case domain.StoreReactivated:
		return ev.EventName(), domainkit.Fields{"ref": string(ev.Ref), "at": strconv.FormatInt(int64(ev.At), 10)}
	default:
		return e.EventName(), domainkit.Fields{}
	}
}

func subject[R any](decide func(*domain.Store) (kernel.Accepted[R], *kernel.Rejection), response func(R) domainkit.Fields) domainkit.Subject[*domain.Store, R] {
	return domainkit.Subject[*domain.Store, R]{
		Decide:   decide,
		Response: response,
		Event:    storeEvent,
		Snapshot: storeState,
		Clone:    func(s *domain.Store) *domain.Store { return domain.FromStoreSnapshot(s.Snapshot()) },
	}
}

// storeProjection is the ORA-30 fixture of the store, kept in the test: the
// domain block may not read the disk, and tb.LoadProjection resolves the root
// from the kit's own path, which a consuming workspace keeps in the module cache.
const storeProjection = `{
  "format_version": "1",
  "identity": {
    "context": "tenancy",
    "aggregate": "store"
  },
  "cases": [
    {
      "name": "register-accepted",
      "doc": "Ramo Accepted de Register: ref desconhecida no operador nasce active, com os dois instantes iguais ao at; um store-registered na sequência.",
      "state_before": {
        "ref": "loja-acme",
        "name": "",
        "status": "unregistered",
        "suspension_reason": "",
        "registered_at": "0",
        "status_changed_at": "0"
      },
      "command": {
        "upr": "register",
        "ref": "loja-acme",
        "name": "Loja Acme",
        "at": "1755432000000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "ref": "loja-acme"
        },
        "events": [
          {
            "name": "tenancy.store-registered",
            "fields": {
              "ref": "loja-acme",
              "name": "Loja Acme",
              "at": "1755432000000000000"
            }
          }
        ],
        "state_after": {
          "ref": "loja-acme",
          "name": "Loja Acme",
          "status": "active",
          "suspension_reason": "",
          "registered_at": "1755432000000000000",
          "status_changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "register-known-ref",
      "doc": "Ramo Accepted de Register sobre ref já registrada: aceita sem alterar nome nem status e sem emitir.",
      "state_before": {
        "ref": "loja-acme",
        "name": "Loja Acme",
        "status": "active",
        "suspension_reason": "",
        "registered_at": "1755432000000000000",
        "status_changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "register",
        "ref": "loja-acme",
        "name": "Outro nome",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "ref": "loja-acme"
        },
        "events": [],
        "state_after": {
          "ref": "loja-acme",
          "name": "Loja Acme",
          "status": "active",
          "suspension_reason": "",
          "registered_at": "1755432000000000000",
          "status_changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "register-ref-invalid",
      "doc": "Ramo Rejected de Register: ref fora da forma de rótulo DNS não registra nada; estado inalterado.",
      "state_before": {
        "ref": "Loja_1",
        "name": "",
        "status": "unregistered",
        "suspension_reason": "",
        "registered_at": "0",
        "status_changed_at": "0"
      },
      "command": {
        "upr": "register",
        "ref": "Loja_1",
        "name": "Loja Acme",
        "at": "1755432000000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "tenancy/store/ref-invalid",
          "message": "ref must have 3 to 63 characters of a-z, 0-9 and -, without a leading or trailing hyphen",
          "details": {
            "ref": "Loja_1"
          }
        },
        "events": [],
        "state_after": {
          "ref": "Loja_1",
          "name": "",
          "status": "unregistered",
          "suspension_reason": "",
          "registered_at": "0",
          "status_changed_at": "0"
        }
      }
    },
    {
      "name": "register-name-empty",
      "doc": "Ramo Rejected de Register: name vazio não registra nada; estado inalterado.",
      "state_before": {
        "ref": "loja-acme",
        "name": "",
        "status": "unregistered",
        "suspension_reason": "",
        "registered_at": "0",
        "status_changed_at": "0"
      },
      "command": {
        "upr": "register",
        "ref": "loja-acme",
        "name": "",
        "at": "1755432000000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "tenancy/store/name-empty",
          "message": "name must not be empty",
          "details": {}
        },
        "events": [],
        "state_after": {
          "ref": "loja-acme",
          "name": "",
          "status": "unregistered",
          "suspension_reason": "",
          "registered_at": "0",
          "status_changed_at": "0"
        }
      }
    },
    {
      "name": "suspend-accepted",
      "doc": "Ramo Accepted de Suspend: loja active vai a suspended com o motivo e statusChangedAt no at; um store-suspended na sequência.",
      "state_before": {
        "ref": "loja-acme",
        "name": "Loja Acme",
        "status": "active",
        "suspension_reason": "",
        "registered_at": "1755432000000000000",
        "status_changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "suspend",
        "reason": "fraude em análise",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "ref": "loja-acme"
        },
        "events": [
          {
            "name": "tenancy.store-suspended",
            "fields": {
              "ref": "loja-acme",
              "reason": "fraude em análise",
              "at": "1755435600000000000"
            }
          }
        ],
        "state_after": {
          "ref": "loja-acme",
          "name": "Loja Acme",
          "status": "suspended",
          "suspension_reason": "fraude em análise",
          "registered_at": "1755432000000000000",
          "status_changed_at": "1755435600000000000"
        }
      }
    },
    {
      "name": "suspend-reason-empty",
      "doc": "Ramo Rejected de Suspend: motivo vazio não suspende; estado inalterado.",
      "state_before": {
        "ref": "loja-acme",
        "name": "Loja Acme",
        "status": "active",
        "suspension_reason": "",
        "registered_at": "1755432000000000000",
        "status_changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "suspend",
        "reason": "",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "tenancy/store/reason-empty",
          "message": "reason must not be empty",
          "details": {}
        },
        "events": [],
        "state_after": {
          "ref": "loja-acme",
          "name": "Loja Acme",
          "status": "active",
          "suspension_reason": "",
          "registered_at": "1755432000000000000",
          "status_changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "suspend-not-active",
      "doc": "Ramo Rejected de Suspend: loja já suspended não se suspende de novo; estado inalterado.",
      "state_before": {
        "ref": "loja-acme",
        "name": "Loja Acme",
        "status": "suspended",
        "suspension_reason": "fraude em análise",
        "registered_at": "1755432000000000000",
        "status_changed_at": "1755435600000000000"
      },
      "command": {
        "upr": "suspend",
        "reason": "outro motivo",
        "at": "1755439200000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "tenancy/store/not-active",
          "message": "store is not active",
          "details": {}
        },
        "events": [],
        "state_after": {
          "ref": "loja-acme",
          "name": "Loja Acme",
          "status": "suspended",
          "suspension_reason": "fraude em análise",
          "registered_at": "1755432000000000000",
          "status_changed_at": "1755435600000000000"
        }
      }
    },
    {
      "name": "reactivate-accepted",
      "doc": "Ramo Accepted de Reactivate: loja suspended volta a active, com o motivo vazio e statusChangedAt no at; um store-reactivated na sequência.",
      "state_before": {
        "ref": "loja-acme",
        "name": "Loja Acme",
        "status": "suspended",
        "suspension_reason": "fraude em análise",
        "registered_at": "1755432000000000000",
        "status_changed_at": "1755435600000000000"
      },
      "command": {
        "upr": "reactivate",
        "at": "1755439200000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "ref": "loja-acme"
        },
        "events": [
          {
            "name": "tenancy.store-reactivated",
            "fields": {
              "ref": "loja-acme",
              "at": "1755439200000000000"
            }
          }
        ],
        "state_after": {
          "ref": "loja-acme",
          "name": "Loja Acme",
          "status": "active",
          "suspension_reason": "",
          "registered_at": "1755432000000000000",
          "status_changed_at": "1755439200000000000"
        }
      }
    },
    {
      "name": "reactivate-not-suspended",
      "doc": "Ramo Rejected de Reactivate: loja active não se reativa; estado inalterado.",
      "state_before": {
        "ref": "loja-acme",
        "name": "Loja Acme",
        "status": "active",
        "suspension_reason": "",
        "registered_at": "1755432000000000000",
        "status_changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "reactivate",
        "at": "1755439200000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "tenancy/store/not-suspended",
          "message": "store is not suspended",
          "details": {}
        },
        "events": [],
        "state_after": {
          "ref": "loja-acme",
          "name": "Loja Acme",
          "status": "active",
          "suspension_reason": "",
          "registered_at": "1755432000000000000",
          "status_changed_at": "1755432000000000000"
        }
      }
    }
  ]
}`
