package domain_test

import (
	"strconv"
	"testing"

	kernel "github.com/mateusmacedo/dmpf/libs/backend/go/domain"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/domainkit"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
)

func TestBuyersMatchTheProjectionFixture(t *testing.T) {
	f, err := tb.DecodeProjection([]byte(buyerProjection))
	if err != nil {
		t.Fatalf("decode the buyer projection: %v", err)
	}
	if f.Identity.Context != "iam" || f.Identity.Aggregate != "buyer" || len(f.Cases) != 15 {
		t.Fatalf("fixture identity/cases = %+v/%d", f.Identity, len(f.Cases))
	}
	tb.RunProjection(t, f, runBuyer)
}

func runBuyer(t *testing.T, c tb.ProjectionCase) (domainkit.Projection, domainkit.Verdict) {
	t.Helper()
	when := kernel.Instant(integer(t, c.Command["at"]))
	before := func() *domain.Buyer { return buyerFromState(t, c.StateBefore) }
	subjectOf := func(s domain.Subject) domainkit.Fields { return domainkit.Fields{"subject": string(s)} }
	switch c.Command["upr"] {
	case "register":
		s := buyerSubject(func(b *domain.Buyer) (kernel.Accepted[domain.RegisteredResponse], *kernel.Rejection) {
			return b.Register(domain.RegisterBuyer{Subject: domain.Subject(c.Command["subject"]), Name: domain.BuyerName(c.Command["name"]), At: when})
		}, func(r domain.RegisteredResponse) domainkit.Fields {
			return domainkit.Fields{"subject": string(r.Subject), "status": r.Status.String()}
		})
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	case "update-profile":
		s := buyerSubject(func(b *domain.Buyer) (kernel.Accepted[domain.ProfileUpdatedResponse], *kernel.Rejection) {
			return b.UpdateProfile(domain.UpdateBuyerProfile{Name: domain.BuyerName(c.Command["name"]), At: when})
		}, func(r domain.ProfileUpdatedResponse) domainkit.Fields { return subjectOf(r.Subject) })
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	case "block":
		s := buyerSubject(func(b *domain.Buyer) (kernel.Accepted[domain.BlockedResponse], *kernel.Rejection) {
			return b.Block(domain.BlockBuyer{Reason: c.Command["reason"], At: when})
		}, func(r domain.BlockedResponse) domainkit.Fields { return subjectOf(r.Subject) })
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	case "unblock":
		s := buyerSubject(func(b *domain.Buyer) (kernel.Accepted[domain.UnblockedResponse], *kernel.Rejection) {
			return b.Unblock(domain.UnblockBuyer{At: when})
		}, func(r domain.UnblockedResponse) domainkit.Fields { return subjectOf(r.Subject) })
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	case "confirm-access-synced":
		s := buyerSubject(func(b *domain.Buyer) (kernel.Accepted[kernel.Empty], *kernel.Rejection) {
			return b.ConfirmAccessSynced(domain.ConfirmBuyerAccessSynced{Version: integer(t, c.Command["version"])})
		}, func(kernel.Empty) domainkit.Fields { return domainkit.Fields{} })
		return domainkit.Run(before(), s), domainkit.ReadTwice(before(), s)
	default:
		t.Fatalf("unknown upr %q in case %s", c.Command["upr"], c.Name)
		return domainkit.Projection{}, domainkit.Verdict{}
	}
}

var buyerStatusNames = map[domain.BuyerStatus]string{
	domain.BuyerStatusUndefined: "undefined",
	domain.BuyerStatusActive:    "active",
	domain.BuyerStatusBlocked:   "blocked",
}

func buyerState(b *domain.Buyer) domainkit.Fields {
	snap := b.Snapshot()
	return domainkit.Fields{
		"subject":        string(snap.Subject),
		"name":           string(snap.Name),
		"status":         buyerStatusNames[snap.Status],
		"block_reason":   snap.BlockReason,
		"access_version": strconv.FormatInt(snap.AccessVersion, 10),
		"synced_version": strconv.FormatInt(snap.SyncedVersion, 10),
		"registered_at":  instant(snap.RegisteredAt),
		"changed_at":     instant(snap.ChangedAt),
	}
}

func buyerFromState(t *testing.T, f domainkit.Fields) *domain.Buyer {
	t.Helper()
	snap := domain.BuyerSnapshot{
		Subject:       domain.Subject(f["subject"]),
		Name:          domain.BuyerName(f["name"]),
		BlockReason:   f["block_reason"],
		AccessVersion: integer(t, f["access_version"]),
		SyncedVersion: integer(t, f["synced_version"]),
		RegisteredAt:  kernel.Instant(integer(t, f["registered_at"])),
		ChangedAt:     kernel.Instant(integer(t, f["changed_at"])),
	}
	for status, name := range buyerStatusNames {
		if name == f["status"] {
			snap.Status = status
		}
	}
	return domain.FromBuyerSnapshot(snap)
}

func buyerEvent(e kernel.DomainEvent) (string, domainkit.Fields) {
	version := func(v int64) string { return strconv.FormatInt(v, 10) }
	switch ev := e.(type) {
	case domain.BuyerRegistered:
		return ev.EventName(), domainkit.Fields{"subject": string(ev.Subject), "name": string(ev.Name), "access_version": version(ev.AccessVersion), "at": instant(ev.At)}
	case domain.BuyerProfileUpdated:
		return ev.EventName(), domainkit.Fields{"subject": string(ev.Subject), "name": string(ev.Name), "at": instant(ev.At)}
	case domain.BuyerBlocked:
		return ev.EventName(), domainkit.Fields{"subject": string(ev.Subject), "reason": ev.Reason, "access_version": version(ev.AccessVersion), "at": instant(ev.At)}
	case domain.BuyerUnblocked:
		return ev.EventName(), domainkit.Fields{"subject": string(ev.Subject), "access_version": version(ev.AccessVersion), "at": instant(ev.At)}
	default:
		return e.EventName(), domainkit.Fields{}
	}
}

func buyerSubject[R any](decide func(*domain.Buyer) (kernel.Accepted[R], *kernel.Rejection), response func(R) domainkit.Fields) domainkit.Subject[*domain.Buyer, R] {
	return domainkit.Subject[*domain.Buyer, R]{
		Decide:   decide,
		Response: response,
		Event:    buyerEvent,
		Snapshot: buyerState,
		Clone:    func(b *domain.Buyer) *domain.Buyer { return domain.FromBuyerSnapshot(b.Snapshot()) },
	}
}

// buyerProjection is the ORA-30 fixture of the buyer, kept in the test for the
// same reason as the role's.
const buyerProjection = `{
  "format_version": "1",
  "identity": {
    "context": "iam",
    "aggregate": "buyer"
  },
  "cases": [
    {
      "name": "register-accepted",
      "doc": "Ramo Accepted de Register: sujeito desconhecido no operador nasce active com versão de acesso 1; um buyer-registered.",
      "state_before": {
        "subject": "kc-123",
        "name": "",
        "status": "undefined",
        "block_reason": "",
        "access_version": "0",
        "synced_version": "0",
        "registered_at": "0",
        "changed_at": "0"
      },
      "command": {
        "upr": "register",
        "subject": "kc-123",
        "name": "Bia",
        "at": "1755432000000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "subject": "kc-123",
          "status": "active"
        },
        "events": [
          {
            "name": "iam.buyer-registered",
            "fields": {
              "subject": "kc-123",
              "name": "Bia",
              "access_version": "1",
              "at": "1755432000000000000"
            }
          }
        ],
        "state_after": {
          "subject": "kc-123",
          "name": "Bia",
          "status": "active",
          "block_reason": "",
          "access_version": "1",
          "synced_version": "0",
          "registered_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "register-known-active",
      "doc": "Ramo Accepted de Register sobre comprador active: aceita sem alterar o nome e sem emitir.",
      "state_before": {
        "subject": "kc-123",
        "name": "Bia",
        "status": "active",
        "block_reason": "",
        "access_version": "1",
        "synced_version": "1",
        "registered_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "register",
        "subject": "kc-123",
        "name": "Outro nome",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "subject": "kc-123",
          "status": "active"
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "name": "Bia",
          "status": "active",
          "block_reason": "",
          "access_version": "1",
          "synced_version": "1",
          "registered_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "register-known-blocked",
      "doc": "Ramo Accepted de Register sobre comprador blocked: aceita sem alterar, o bloqueio inclusive, e sem emitir.",
      "state_before": {
        "subject": "kc-123",
        "name": "Bia",
        "status": "blocked",
        "block_reason": "chargeback recorrente",
        "access_version": "2",
        "synced_version": "2",
        "registered_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "register",
        "subject": "kc-123",
        "name": "Bia",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "subject": "kc-123",
          "status": "blocked"
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "name": "Bia",
          "status": "blocked",
          "block_reason": "chargeback recorrente",
          "access_version": "2",
          "synced_version": "2",
          "registered_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "register-subject-invalid",
      "doc": "Ramo Rejected de Register com subject vazio; estado inalterado.",
      "state_before": {
        "subject": "",
        "name": "",
        "status": "undefined",
        "block_reason": "",
        "access_version": "0",
        "synced_version": "0",
        "registered_at": "0",
        "changed_at": "0"
      },
      "command": {
        "upr": "register",
        "subject": "",
        "name": "Bia",
        "at": "1755432000000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/buyer/subject-invalid",
          "message": "subject must have 1 to 255 characters and no control character",
          "details": {}
        },
        "events": [],
        "state_after": {
          "subject": "",
          "name": "",
          "status": "undefined",
          "block_reason": "",
          "access_version": "0",
          "synced_version": "0",
          "registered_at": "0",
          "changed_at": "0"
        }
      }
    },
    {
      "name": "register-name-empty",
      "doc": "Ramo Rejected de Register com name vazio; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "name": "",
        "status": "undefined",
        "block_reason": "",
        "access_version": "0",
        "synced_version": "0",
        "registered_at": "0",
        "changed_at": "0"
      },
      "command": {
        "upr": "register",
        "subject": "kc-123",
        "name": "",
        "at": "1755432000000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/buyer/name-empty",
          "message": "name must not be empty",
          "details": {}
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "name": "",
          "status": "undefined",
          "block_reason": "",
          "access_version": "0",
          "synced_version": "0",
          "registered_at": "0",
          "changed_at": "0"
        }
      }
    },
    {
      "name": "update-profile-accepted",
      "doc": "Ramo Accepted de UpdateProfile: o comprador active troca o nome; um buyer-profile-updated, sem mudança de acesso.",
      "state_before": {
        "subject": "kc-123",
        "name": "Bia",
        "status": "active",
        "block_reason": "",
        "access_version": "1",
        "synced_version": "1",
        "registered_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "update-profile",
        "name": "Bia Souza",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "subject": "kc-123"
        },
        "events": [
          {
            "name": "iam.buyer-profile-updated",
            "fields": {
              "subject": "kc-123",
              "name": "Bia Souza",
              "at": "1755435600000000000"
            }
          }
        ],
        "state_after": {
          "subject": "kc-123",
          "name": "Bia Souza",
          "status": "active",
          "block_reason": "",
          "access_version": "1",
          "synced_version": "1",
          "registered_at": "1755432000000000000",
          "changed_at": "1755435600000000000"
        }
      }
    },
    {
      "name": "update-profile-blocked",
      "doc": "Ramo Rejected de UpdateProfile sobre comprador blocked; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "name": "Bia",
        "status": "blocked",
        "block_reason": "chargeback recorrente",
        "access_version": "2",
        "synced_version": "2",
        "registered_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "update-profile",
        "name": "Bia Souza",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/buyer/blocked",
          "message": "buyer is blocked",
          "details": {}
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "name": "Bia",
          "status": "blocked",
          "block_reason": "chargeback recorrente",
          "access_version": "2",
          "synced_version": "2",
          "registered_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "update-profile-name-empty",
      "doc": "Ramo Rejected de UpdateProfile com name vazio; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "name": "Bia",
        "status": "active",
        "block_reason": "",
        "access_version": "1",
        "synced_version": "1",
        "registered_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "update-profile",
        "name": "",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/buyer/name-empty",
          "message": "name must not be empty",
          "details": {}
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "name": "Bia",
          "status": "active",
          "block_reason": "",
          "access_version": "1",
          "synced_version": "1",
          "registered_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "block-accepted",
      "doc": "Ramo Accepted de Block: o comprador active vai a blocked com o motivo e a versão de acesso sobe; um buyer-blocked.",
      "state_before": {
        "subject": "kc-123",
        "name": "Bia",
        "status": "active",
        "block_reason": "",
        "access_version": "1",
        "synced_version": "1",
        "registered_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "block",
        "reason": "chargeback recorrente",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "subject": "kc-123"
        },
        "events": [
          {
            "name": "iam.buyer-blocked",
            "fields": {
              "subject": "kc-123",
              "reason": "chargeback recorrente",
              "access_version": "2",
              "at": "1755435600000000000"
            }
          }
        ],
        "state_after": {
          "subject": "kc-123",
          "name": "Bia",
          "status": "blocked",
          "block_reason": "chargeback recorrente",
          "access_version": "2",
          "synced_version": "1",
          "registered_at": "1755432000000000000",
          "changed_at": "1755435600000000000"
        }
      }
    },
    {
      "name": "block-not-active",
      "doc": "Ramo Rejected de Block sobre comprador blocked; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "name": "Bia",
        "status": "blocked",
        "block_reason": "chargeback recorrente",
        "access_version": "2",
        "synced_version": "2",
        "registered_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "block",
        "reason": "outro",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/buyer/not-active",
          "message": "buyer is not active",
          "details": {}
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "name": "Bia",
          "status": "blocked",
          "block_reason": "chargeback recorrente",
          "access_version": "2",
          "synced_version": "2",
          "registered_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "block-reason-empty",
      "doc": "Ramo Rejected de Block sem motivo; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "name": "Bia",
        "status": "active",
        "block_reason": "",
        "access_version": "1",
        "synced_version": "1",
        "registered_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "block",
        "reason": "",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/buyer/reason-empty",
          "message": "reason must not be empty",
          "details": {}
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "name": "Bia",
          "status": "active",
          "block_reason": "",
          "access_version": "1",
          "synced_version": "1",
          "registered_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "unblock-accepted",
      "doc": "Ramo Accepted de Unblock: o comprador blocked volta a active sem motivo e a versão de acesso sobe; um buyer-unblocked.",
      "state_before": {
        "subject": "kc-123",
        "name": "Bia",
        "status": "blocked",
        "block_reason": "chargeback recorrente",
        "access_version": "2",
        "synced_version": "2",
        "registered_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "unblock",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "accepted",
        "response": {
          "subject": "kc-123"
        },
        "events": [
          {
            "name": "iam.buyer-unblocked",
            "fields": {
              "subject": "kc-123",
              "access_version": "3",
              "at": "1755435600000000000"
            }
          }
        ],
        "state_after": {
          "subject": "kc-123",
          "name": "Bia",
          "status": "active",
          "block_reason": "",
          "access_version": "3",
          "synced_version": "2",
          "registered_at": "1755432000000000000",
          "changed_at": "1755435600000000000"
        }
      }
    },
    {
      "name": "unblock-not-blocked",
      "doc": "Ramo Rejected de Unblock sobre comprador active; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "name": "Bia",
        "status": "active",
        "block_reason": "",
        "access_version": "1",
        "synced_version": "1",
        "registered_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "unblock",
        "at": "1755435600000000000"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/buyer/not-blocked",
          "message": "buyer is not blocked",
          "details": {}
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "name": "Bia",
          "status": "active",
          "block_reason": "",
          "access_version": "1",
          "synced_version": "1",
          "registered_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "confirm-access-synced-accepted",
      "doc": "Ramo Accepted de ConfirmAccessSynced: a versão confirmada sobe até a de acesso, sem evento.",
      "state_before": {
        "subject": "kc-123",
        "name": "Bia",
        "status": "blocked",
        "block_reason": "chargeback recorrente",
        "access_version": "2",
        "synced_version": "1",
        "registered_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
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
          "name": "Bia",
          "status": "blocked",
          "block_reason": "chargeback recorrente",
          "access_version": "2",
          "synced_version": "2",
          "registered_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    },
    {
      "name": "confirm-access-synced-version-ahead",
      "doc": "Ramo Rejected de ConfirmAccessSynced com versão acima da de acesso; estado inalterado.",
      "state_before": {
        "subject": "kc-123",
        "name": "Bia",
        "status": "active",
        "block_reason": "",
        "access_version": "1",
        "synced_version": "1",
        "registered_at": "1755432000000000000",
        "changed_at": "1755432000000000000"
      },
      "command": {
        "upr": "confirm-access-synced",
        "version": "2"
      },
      "expected": {
        "branch": "rejected",
        "rejection": {
          "code": "iam/buyer/version-ahead",
          "message": "version is ahead of the access version of the buyer",
          "details": {
            "version": "2",
            "accessVersion": "1"
          }
        },
        "events": [],
        "state_after": {
          "subject": "kc-123",
          "name": "Bia",
          "status": "active",
          "block_reason": "",
          "access_version": "1",
          "synced_version": "1",
          "registered_at": "1755432000000000000",
          "changed_at": "1755432000000000000"
        }
      }
    }
  ]
}
`
