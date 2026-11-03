package provider_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	usecase "github.com/mateusmacedo/dmpf/libs/backend/go/application"
	kernelhttp "github.com/mateusmacedo/dmpf/libs/backend/go/http"
	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/clock"
	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/providerkit"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/tb"

	"github.com/mateusmacedo/marketplace/apps/backend/iam/domain"
	iamports "github.com/mateusmacedo/marketplace/apps/backend/iam/ports"
	"github.com/mateusmacedo/marketplace/apps/backend/iam/provider"
)

const (
	testSecret   = "s3cr3t-of-the-iam-admin"
	testOperator = "marketplace"
)

func validKeycloakConfig(baseURL string) provider.KeycloakConfig {
	return provider.KeycloakConfig{
		BaseURL:        baseURL,
		Realm:          "dmpf",
		ClientID:       "iam-admin",
		Secret:         provider.NewKeycloakSecret(testSecret),
		OperatorTenant: testOperator,
	}
}

func TestKeycloakOutcomesPassTheExternalCommandKit(t *testing.T) {
	tb.Require(t, providerkit.ExternalCommand(func() providerkit.ExternalCommandSubject {
		table := provider.KeycloakOutcomes()
		return providerkit.ExternalCommandSubject{
			Table: table.Status,
			Classify: func(status int) ports.ExternalClass {
				return table.Classify(&http.Response{StatusCode: status}, nil)
			},
			Validate: func(status map[int]ports.ExternalClass) error {
				return kernelhttp.OutcomeTable{Version: table.Version, Status: status}.Validate()
			},
		}
	}))
}

func TestKeycloakOutcomesClassifyEachStatusTheAdminAPIDocuments(t *testing.T) {
	table := provider.KeycloakOutcomes()
	want := map[int]ports.ExternalClass{
		200: ports.ClassAccepted, 201: ports.ClassAccepted, 204: ports.ClassAccepted,
		400: ports.ClassFinalValidation,
		401: ports.ClassFinalAuthorization, 403: ports.ClassFinalAuthorization,
		404: ports.ClassFinalBusiness, 409: ports.ClassFinalBusiness,
		429: ports.ClassRetryableUpstream, 500: ports.ClassRetryableUpstream, 502: ports.ClassRetryableUpstream,
		503: ports.ClassRetryableUpstream, 504: ports.ClassRetryableUpstream,
	}

	if table.Version != "keycloak-admin-26.8/1" {
		t.Fatalf("Version = %q, want keycloak-admin-26.8/1", table.Version)
	}
	if len(table.Status) != len(want) {
		t.Fatalf("Status declares %d codes, want %d: %v", len(table.Status), len(want), table.Status)
	}
	for status, class := range want {
		if got := table.Classify(&http.Response{StatusCode: status}, nil); got != class {
			t.Errorf("Classify(%d) = %v, want %v", status, got, class)
		}
	}
	if got := table.Classify(&http.Response{StatusCode: http.StatusAccepted}, nil); got != ports.ClassUnknownAmbiguous {
		t.Errorf("Classify(202) = %v, want unknown_ambiguous: the Admin API never documents it", got)
	}
}

func TestKeycloakOutcomesIsACopyACallerCannotRewrite(t *testing.T) {
	provider.KeycloakOutcomes().Status[http.StatusNotFound] = ports.ClassAccepted

	if got := provider.KeycloakOutcomes().Status[http.StatusNotFound]; got != ports.ClassFinalBusiness {
		t.Fatalf("Status[404] = %v after a caller rewrote its copy, want final_business", got)
	}
}

func TestKeycloakProfileIsAValidExternalProfile(t *testing.T) {
	k, err := provider.NewKeycloak(validKeycloakConfig("http://keycloak.test"), provider.KeycloakDeps{Clock: clock.System()})
	if err != nil {
		t.Fatalf("NewKeycloak() = %v, want nil", err)
	}

	profile := k.Profile()

	if err := usecase.ValidateExternalProfile(profile); err != nil {
		t.Fatalf("ValidateExternalProfile(Profile()) = %v, want nil", err)
	}
	want := ports.ExternalProfile{
		KeySemantics:         ports.KeyNone,
		Efficacy:             ports.EfficacyOnSubmit,
		Quiescence:           0,
		AuthoritativeAbsence: false,
		MaxQueries:           1,
		KeyAlphabet:          "0123456789abcdef",
		KeyMaxLength:         64,
	}
	if profile != want {
		t.Fatalf("Profile() = %+v, want %+v", profile, want)
	}
}

func TestNewKeycloakRefusesAnIncompleteConfig(t *testing.T) {
	tests := []struct {
		name  string
		edit  func(*provider.KeycloakConfig, *provider.KeycloakDeps)
		field string
	}{
		{"without base URL", func(c *provider.KeycloakConfig, _ *provider.KeycloakDeps) { c.BaseURL = "" }, "base URL"},
		{"with a relative base URL", func(c *provider.KeycloakConfig, _ *provider.KeycloakDeps) { c.BaseURL = "keycloak.test" }, "base URL"},
		{"with a base URL outside HTTP", func(c *provider.KeycloakConfig, _ *provider.KeycloakDeps) { c.BaseURL = "ftp://keycloak.test" }, "base URL"},
		{"with credentials in the base URL", func(c *provider.KeycloakConfig, _ *provider.KeycloakDeps) {
			c.BaseURL = "http://admin:" + testSecret + "@keycloak.test"
		}, "base URL"},
		{"with a query in the base URL", func(c *provider.KeycloakConfig, _ *provider.KeycloakDeps) { c.BaseURL = "http://keycloak.test?x=1" }, "base URL"},
		{"without realm", func(c *provider.KeycloakConfig, _ *provider.KeycloakDeps) { c.Realm = "" }, "realm"},
		{"without client id", func(c *provider.KeycloakConfig, _ *provider.KeycloakDeps) { c.ClientID = "" }, "client id"},
		{"without secret", func(c *provider.KeycloakConfig, _ *provider.KeycloakDeps) { c.Secret = provider.KeycloakSecret{} }, "secret"},
		{"with an empty secret", func(c *provider.KeycloakConfig, _ *provider.KeycloakDeps) { c.Secret = provider.NewKeycloakSecret("") }, "secret"},
		{"without operator tenant", func(c *provider.KeycloakConfig, _ *provider.KeycloakDeps) { c.OperatorTenant = "" }, "operator tenant"},
		{"without clock", func(_ *provider.KeycloakConfig, d *provider.KeycloakDeps) { d.Clock = nil }, "clock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, deps := validKeycloakConfig("http://keycloak.test"), provider.KeycloakDeps{Clock: clock.System()}
			tt.edit(&cfg, &deps)

			k, err := provider.NewKeycloak(cfg, deps)

			if !errors.Is(err, provider.ErrKeycloakConfig) || !strings.Contains(err.Error(), tt.field) || k != nil {
				t.Fatalf("NewKeycloak() = %v, %v; want ErrKeycloakConfig naming the %s", k, err, tt.field)
			}
			if strings.Contains(err.Error(), testSecret) {
				t.Fatalf("NewKeycloak() error %q carries the secret", err)
			}
		})
	}
}

func TestNewKeycloakAcceptsABaseURLWithAPathPrefix(t *testing.T) {
	if _, err := provider.NewKeycloak(validKeycloakConfig("https://sso.example/auth/"), provider.KeycloakDeps{Clock: clock.System()}); err != nil {
		t.Fatalf("NewKeycloak(https://sso.example/auth/) = %v, want nil", err)
	}
}

func TestKeycloakPayloadsWriteTheWireFormat(t *testing.T) {
	payloads := provider.KeycloakPayloads{}
	tests := []struct {
		name   string
		encode func() ([]byte, error)
		want   string
	}{
		{"active role", func() ([]byte, error) {
			return payloads.RoleAccess(iamports.RoleAccess{Ref: "expedicao", Status: domain.RoleStatusActive, Permissions: []domain.Permission{"fulfillment:read", "fulfillment:write"}})
		}, `{"ref":"expedicao","status":"active","permissions":["fulfillment:read","fulfillment:write"]}`},
		{"retired role", func() ([]byte, error) {
			return payloads.RoleAccess(iamports.RoleAccess{Ref: "expedicao", Status: domain.RoleStatusRetired})
		}, `{"ref":"expedicao","status":"retired","permissions":[]}`},
		{"member provision", func() ([]byte, error) {
			return payloads.MemberProvision(iamports.MemberProvision{Email: "ana@loja.example"})
		}, `{"email":"ana@loja.example"}`},
		{"active member", func() ([]byte, error) {
			return payloads.MemberAccess(iamports.MemberAccess{Subject: "sub-1", Status: domain.MembershipStatusActive, Roles: []domain.RoleRef{domain.OwnerRole, "expedicao"}})
		}, `{"subject":"sub-1","status":"active","roles":["owner","expedicao"]}`},
		{"removed member", func() ([]byte, error) {
			return payloads.MemberAccess(iamports.MemberAccess{Subject: "sub-1", Status: domain.MembershipStatusRemoved})
		}, `{"subject":"sub-1","status":"removed","roles":[]}`},
		{"active buyer", func() ([]byte, error) {
			return payloads.BuyerAccess(iamports.BuyerAccess{Subject: "sub-2", Status: domain.BuyerStatusActive})
		}, `{"subject":"sub-2","status":"active"}`},
		{"blocked buyer", func() ([]byte, error) {
			return payloads.BuyerAccess(iamports.BuyerAccess{Subject: "sub-2", Status: domain.BuyerStatusBlocked})
		}, `{"subject":"sub-2","status":"blocked"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := tt.encode()

			if err != nil || string(raw) != tt.want {
				t.Fatalf("encode = %s, %v; want %s", raw, err, tt.want)
			}
		})
	}
}

func TestKeycloakPayloadsRefuseAStatusWithoutWireValue(t *testing.T) {
	payloads := provider.KeycloakPayloads{}
	tests := []struct {
		name   string
		encode func() ([]byte, error)
	}{
		{"role", func() ([]byte, error) { return payloads.RoleAccess(iamports.RoleAccess{Ref: "expedicao"}) }},
		{"member", func() ([]byte, error) {
			return payloads.MemberAccess(iamports.MemberAccess{Subject: "sub-1", Status: 9})
		}},
		{"buyer", func() ([]byte, error) { return payloads.BuyerAccess(iamports.BuyerAccess{Subject: "sub-2"}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := tt.encode()

			if !errors.Is(err, provider.ErrKeycloakPayload) || raw != nil {
				t.Fatalf("encode = %s, %v; want no payload and ErrKeycloakPayload", raw, err)
			}
		})
	}
}

func TestKeycloakPayloadsRefuseWhatTheirDecoderRefuses(t *testing.T) {
	payloads := provider.KeycloakPayloads{}
	tests := []struct {
		name   string
		encode func() ([]byte, error)
	}{
		{"role with an invalid ref", func() ([]byte, error) {
			return payloads.RoleAccess(iamports.RoleAccess{Ref: "Expedição", Status: domain.RoleStatusActive})
		}},
		{"role with an empty permission", func() ([]byte, error) {
			return payloads.RoleAccess(iamports.RoleAccess{Ref: "expedicao", Status: domain.RoleStatusActive, Permissions: []domain.Permission{""}})
		}},
		{"provision with an address not normalized", func() ([]byte, error) {
			return payloads.MemberProvision(iamports.MemberProvision{Email: "Ana@Loja.example"})
		}},
		{"member without subject", func() ([]byte, error) {
			return payloads.MemberAccess(iamports.MemberAccess{Status: domain.MembershipStatusActive})
		}},
		{"member with a dot subject", func() ([]byte, error) {
			return payloads.MemberAccess(iamports.MemberAccess{Subject: "..", Status: domain.MembershipStatusRemoved})
		}},
		{"member with an invalid role", func() ([]byte, error) {
			return payloads.MemberAccess(iamports.MemberAccess{Subject: "sub-1", Status: domain.MembershipStatusActive, Roles: []domain.RoleRef{"X"}})
		}},
		{"buyer without subject", func() ([]byte, error) {
			return payloads.BuyerAccess(iamports.BuyerAccess{Status: domain.BuyerStatusActive})
		}},
		{"buyer with a dot subject", func() ([]byte, error) {
			return payloads.BuyerAccess(iamports.BuyerAccess{Subject: ".", Status: domain.BuyerStatusBlocked})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := tt.encode()

			if !errors.Is(err, provider.ErrKeycloakPayload) || raw != nil {
				t.Fatalf("encode = %s, %v; want no payload and ErrKeycloakPayload", raw, err)
			}
		})
	}
}

func TestKeycloakSecretNeverReachesARendering(t *testing.T) {
	secret := provider.NewKeycloakSecret(testSecret)
	cfg := validKeycloakConfig("http://keycloak.test")
	k, err := provider.NewKeycloak(cfg, provider.KeycloakDeps{Clock: clock.System()})
	if err != nil {
		t.Fatalf("NewKeycloak() = %v", err)
	}

	renderings := map[string]string{}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d"} {
		renderings["secret "+verb] = fmt.Sprintf(verb, secret)
		renderings["config "+verb] = fmt.Sprintf(verb, cfg)
		renderings["keycloak "+verb] = fmt.Sprintf(verb, k)
	}
	renderings["String"], renderings["GoString"] = secret.String(), secret.GoString()
	for name, value := range map[string]any{"secret": secret, "config": cfg} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("json.Marshal(%s) = %v", name, err)
		}
		renderings["json "+name] = string(raw)
		var text strings.Builder
		slog.New(slog.NewTextHandler(&text, nil)).Info("start", "value", value)
		renderings["text log "+name] = text.String()
		var structured strings.Builder
		slog.New(slog.NewJSONHandler(&structured, nil)).Info("start", "value", value)
		renderings["json log "+name] = structured.String()
	}
	text, err := secret.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText() = %v", err)
	}
	renderings["MarshalText"] = string(text)

	for name, rendered := range renderings {
		if strings.Contains(rendered, testSecret) {
			t.Errorf("%s = %q carries the secret", name, rendered)
		}
	}
	if !strings.Contains(renderings["secret %v"], "redacted") {
		t.Errorf("secret %%v = %q, want the redaction placeholder", renderings["secret %v"])
	}
}

func TestKeycloakSecretTellsAbsenceApart(t *testing.T) {
	if !(provider.KeycloakSecret{}).IsZero() || !provider.NewKeycloakSecret("").IsZero() {
		t.Fatal("IsZero() = false for an absent secret, want true")
	}
	if provider.NewKeycloakSecret(testSecret).IsZero() {
		t.Fatal("IsZero() = true for a set secret, want false")
	}
}
