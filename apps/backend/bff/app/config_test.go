package app_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mateusmacedo/dmpf/libs/backend/go/redis"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app"
)

func lookup(pairs ...string) func(string) string {
	values := map[string]string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		values[pairs[i]] = pairs[i+1]
	}
	return func(name string) string { return values[name] }
}

// required is what every start of the api role needs besides identity: the
// deny-list it consults before accepting a token.
var required = []string{"REDIS_URL", "redis://revocation:sentinel-redis-password@redis:6379/0"}

// devMock declares how these starts resolve identity. Since the edge demands a
// subject on every route, a start that declares no verifier and no mock is
// refused, so the cases that expect a usable config have to say which one.
const devMock = "AUTH_DEV_MOCK"

func withRequired(env ...string) []string { return append(slices.Clone(required), env...) }

func TestFromEnvRefusesAStartThatDeclaresNoIdentity(t *testing.T) {
	_, err := app.FromEnv(lookup(required...))

	if err == nil {
		t.Fatal("FromEnv() = nil: a start that could authenticate nobody must be refused")
	}
	if !strings.Contains(err.Error(), devMock) {
		t.Fatalf("FromEnv() = %v, want the error to name %s", err, devMock)
	}
}

func TestFromEnvAppliesTheDefaults(t *testing.T) {
	cfg, err := app.FromEnv(lookup(withRequired(devMock, "true")...))

	if err != nil {
		t.Fatalf("FromEnv() = %v, want a start that needs no gRPC target while no context exists", err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.Service != "bff" || len(cfg.CORSOrigins) != 0 {
		t.Fatalf("cfg = %+v, want :8080, bff and no browser origin", cfg)
	}
	if cfg.RouteBudget.Validate() != nil || cfg.RouteBudget.Limit <= 0 {
		t.Fatalf("RouteBudget = %+v, want a valid budget", cfg.RouteBudget)
	}
}

func TestFromEnvReadsTheBrowserOrigins(t *testing.T) {
	cfg, err := app.FromEnv(lookup(withRequired(devMock, "true", "CORS_ORIGINS", "http://a, http://b")...))

	if err != nil {
		t.Fatalf("FromEnv() = %v", err)
	}
	if !slices.Equal(cfg.CORSOrigins, []string{"http://a", "http://b"}) {
		t.Fatalf("CORSOrigins = %v, want both declared origins", cfg.CORSOrigins)
	}
}

func TestFromEnvRefusesAnInvalidBoolean(t *testing.T) {
	_, err := app.FromEnv(lookup(withRequired(devMock, "maybe")...))

	if !errors.Is(err, app.ErrInvalidVariable) {
		t.Fatalf("FromEnv() = %v, want ErrInvalidVariable", err)
	}
}

func TestFromEnvReadsTheDrainDelay(t *testing.T) {
	base := withRequired(devMock, "true")

	cfg, err := app.FromEnv(lookup(base...))
	if err != nil || cfg.DrainDelay != 0 {
		t.Fatalf("FromEnv() = %v, %v; want no drain by default", cfg.DrainDelay, err)
	}
	cfg, err = app.FromEnv(lookup(append(base, "DRAIN_DELAY", "5s")...))
	if err != nil || cfg.DrainDelay != 5*time.Second {
		t.Fatalf("FromEnv() = %v, %v; want 5s", cfg.DrainDelay, err)
	}
	for _, invalid := range []string{"soon", "-1s"} {
		if _, err := app.FromEnv(lookup(append(base, "DRAIN_DELAY", invalid)...)); err == nil || !strings.Contains(err.Error(), "DRAIN_DELAY") {
			t.Fatalf("FromEnv(DRAIN_DELAY=%s) = %v, want the variable named", invalid, err)
		}
	}
}

func TestFromEnvReadsTheAdministrationAddress(t *testing.T) {
	base := withRequired(devMock, "true")

	cfg, err := app.FromEnv(lookup(base...))
	if err != nil || cfg.AdminAddr != ":8090" || cfg.AdminAddr == cfg.HTTPAddr {
		t.Fatalf("FromEnv() = %q, %v; want :8090, apart from the public %q", cfg.AdminAddr, err, cfg.HTTPAddr)
	}
	cfg, err = app.FromEnv(lookup(append(base, "ADMIN_ADDR", "127.0.0.1:9100")...))
	if err != nil || cfg.AdminAddr != "127.0.0.1:9100" {
		t.Fatalf("FromEnv() = %q, %v; want 127.0.0.1:9100", cfg.AdminAddr, err)
	}
	if _, err := app.FromEnv(lookup(append(base, "ADMIN_ADDR", "8090")...)); !errors.Is(err, app.ErrInvalidVariable) || !strings.Contains(err.Error(), "ADMIN_ADDR") {
		t.Fatalf("FromEnv(ADMIN_ADDR=8090) = %v, want ErrInvalidVariable naming ADMIN_ADDR", err)
	}
}

func TestFromEnvReadsNoLegacyTelemetryVariable(t *testing.T) {
	cfg, err := app.FromEnv(lookup(withRequired(devMock, "true",
		"SERVICE", "legacy", "SERVICE_VERSION", "9.9.9", "INSTANCE_ID", "legacy-1", "OTLP_ENDPOINT", "legacy:4317", "OTLP_INSECURE", "not-a-bool")...))

	if err != nil {
		t.Fatalf("FromEnv() = %v, want the legacy telemetry variables ignored (RF-E1)", err)
	}
	telemetry := app.TelemetryOf(cfg)
	if telemetry.Service != "bff" || telemetry.Version == "9.9.9" || telemetry.Instance == "legacy-1" {
		t.Fatalf("TelemetryOf() = %+v, want the identity left to OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES (RF-E1, RF-E3)", telemetry)
	}
}

func TestTheIdentityOfTheProcessIsLeftToTheEnvironment(t *testing.T) {
	cfg, err := app.FromEnv(lookup(withRequired(devMock, "true")...))
	if err != nil {
		t.Fatalf("FromEnv() = %v, want nil", err)
	}

	if telemetry := app.TelemetryOf(cfg); telemetry.Version != "" || telemetry.Instance != "" {
		t.Fatalf("TelemetryOf() declares version %q and instance %q, want neither: both come from OTEL_RESOURCE_ATTRIBUTES (RF-E3)", telemetry.Version, telemetry.Instance)
	}
}

func TestFromEnvRefusesAStartWithoutTheRevocationStore(t *testing.T) {
	_, err := app.FromEnv(lookup(devMock, "true"))

	if !errors.Is(err, app.ErrMissingVariable) || !strings.Contains(err.Error(), "REDIS_URL") {
		t.Fatalf("FromEnv() = %v, want REDIS_URL named: the edge never serves without the deny-list", err)
	}
}

func TestFromEnvRefusesAnUnusableRevocationStoreWithoutEchoingIt(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr error
	}{
		{"not a redis URL", "http://revocation:sentinel-redis-password@redis:6379/0", redis.ErrInvalidURL},
		{"database other than zero", "redis://revocation:sentinel-redis-password@redis:6379/1", redis.ErrDatabaseNotZero},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := app.FromEnv(lookup(devMock, "true", "REDIS_URL", tc.url))

			if !errors.Is(err, tc.wantErr) || strings.Contains(err.Error(), "sentinel-redis-password") {
				t.Fatalf("FromEnv() = %v, want %v with the URL left out", err, tc.wantErr)
			}
		})
	}
}

func TestFromEnvReadsTheRevocationStore(t *testing.T) {
	cfg, err := app.FromEnv(lookup(withRequired(devMock, "true")...))

	if err != nil {
		t.Fatalf("FromEnv() = %v", err)
	}
	if cfg.Redis.Options == nil || cfg.Redis.Options.Addr != "redis:6379" || cfg.Redis.Options.Username != "revocation" {
		t.Fatalf("cfg.Redis = %v, want redis:6379 as revocation", cfg.Redis)
	}
}
