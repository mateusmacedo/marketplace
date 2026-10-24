package app_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mateusmacedo/dmpf/libs/backend/go/authn"
	"github.com/mateusmacedo/dmpf/libs/backend/go/redis"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/obstest"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app"
)

var edgeTelemetry = obstest.Subject[app.Config, string]{
	Service:     "bff",
	Roles:       []string{"api"},
	API:         "api",
	Defaults:    func(string) app.Config { return app.Defaults() },
	FromEnv:     func(_ string, getenv func(string) string) (app.Config, error) { return app.FromEnv(getenv) },
	TelemetryOf: app.TelemetryOf,
	Scope:       reflect.TypeFor[app.Config]().PkgPath(),
}

var sentinels = []string{"sentinel-tenant", "sentinel-redis-password"}

func configWithSecrets() app.Config {
	cfg := app.Defaults()
	cfg.CORSOrigins = []string{"https://app.example"}
	cfg.MetricTenants = []string{"sentinel-tenant-a", "sentinel-tenant-b"}
	cfg.DrainDelay = 5 * time.Second
	cfg.Auth = authn.Config{Issuer: "https://issuer.example", Audience: "dmpf", TenantClaim: "tenant",
		PermissionClaims: []string{"permissions"}, DiscoveryTimeout: 5 * time.Second}
	var err error
	if cfg.Redis, err = redis.FromEnv(lookup("REDIS_URL", "redis://revocation:sentinel-redis-password@redis:6379/0")); err != nil {
		panic(err)
	}
	return cfg
}

func TestTheProcessConfiguredRecordCarriesTheAllowlistOfTheEdge(t *testing.T) {
	record, _ := edgeTelemetry.ProcessConfigured(t, configWithSecrets())

	got := obstest.SettingKeys(record)
	want := []string{"admin_addr", "authn.audience", "authn.dev_mock", "authn.discovery_timeout", "authn.issuer", "authn.permission_claims",
		"authn.revocation", "authn.tenant_claim", "cors_origins", "drain_delay", "http_addr", "metric_tenants", "redis.addr",
		"redis.command_timeout", "redis.dial_timeout", "redis.tls", "redis.username"}
	if !slices.Equal(got, want) {
		t.Fatalf("dmpf.config.* = %v, want exactly the settings of the edge %v", got, want)
	}
}

func TestTheProcessConfiguredRecordDeclaresTheDenyListTheEdgeServesWith(t *testing.T) {
	cfg, closeRevocations, err := app.WithRevocations(configWithSecrets())
	if err != nil {
		t.Fatalf("WithRevocations() = %v", err)
	}
	t.Cleanup(func() { _ = closeRevocations() })

	record, _ := edgeTelemetry.ProcessConfigured(t, cfg)

	if got := record["dmpf.config.authn.revocation"].GetStringValue(); got != "enabled" {
		t.Fatalf("dmpf.config.authn.revocation = %q, want enabled: the edge never serves without the deny-list", got)
	}
}

func TestTheProcessConfiguredRecordCarriesNoSecret(t *testing.T) {
	record, out := edgeTelemetry.ProcessConfigured(t, configWithSecrets())

	for _, sentinel := range sentinels {
		if strings.Contains(out, sentinel) {
			t.Errorf("the telemetry carries %q (LOG-09): %s", sentinel, out)
		}
	}
	if got := record["dmpf.config.metric_tenants"]; got.GetIntValue() != 2 {
		t.Errorf("dmpf.config.metric_tenants = %v, want the count 2 and not the names", got)
	}
}

func TestTheTelemetryDeclaresTheApiRole(t *testing.T) {
	edgeTelemetry.RunRolesDeclared(t)
}

func TestARecordNamesTheExecutionOfTheCallFromTheBaggage(t *testing.T) {
	edgeTelemetry.RunTenantBaggage(t)
}
