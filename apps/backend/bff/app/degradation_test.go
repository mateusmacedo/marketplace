package app_test

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/mateusmacedo/dmpf/libs/backend/go/observability/boot"
	"github.com/mateusmacedo/dmpf/libs/backend/go/testkit/obstest"

	"github.com/mateusmacedo/marketplace/apps/backend/bff/app"
)

const (
	requestLimit = 2 * time.Second
	drainLimit   = 5 * time.Second
)

// startEdgeExportingTo starts the edge over its own manifest, which resolves
// identity with AUTH_DEV_MOCK: no request waits on the deny-list, so the only
// I/O left on its path is the telemetry the collector refuses.
func startEdgeExportingTo(t *testing.T, collector string) app.Config {
	t.Helper()
	env := maps.Clone(obstest.ReadEnvFile(t, manifestPath))
	maps.Copy(env, map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://" + collector,
		"OTEL_METRIC_EXPORT_INTERVAL": "10",
		"OTEL_BSP_SCHEDULE_DELAY":     "10",
		"OTEL_BSP_MAX_QUEUE_SIZE":     "8",
		"OTEL_BLRP_SCHEDULE_DELAY":    "10",
		"OTEL_BLRP_MAX_QUEUE_SIZE":    "8",
		"HTTP_ADDR":                   obstest.UnusedLoopbackAddr(t),
		"ADMIN_ADDR":                  obstest.UnusedLoopbackAddr(t),
	})
	obstest.UseOnlyTheOTelEnvironmentOf(t, env)
	cfg, err := app.FromEnv(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("FromEnv() over %s = %v", manifestPath, err)
	}

	runtime, err := boot.StartTelemetry(context.Background(), app.TelemetryOf(cfg))
	if err != nil {
		t.Fatalf("StartTelemetry() = %v", err)
	}
	t.Cleanup(func() {
		grace, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		obstest.ReturnsWithin(t, drainLimit, "the telemetry shutdown", func() { _ = runtime.Shutdown(grace) })
	})
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- app.RunWith(ctx, cfg, runtime) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-served:
			if err != nil {
				t.Errorf("RunWith() = %v, want nil on shutdown", err)
			}
		case <-time.After(drainLimit):
			t.Errorf("the drain did not return in %v with the export failing, want the telemetry never to hold it (NF Degradação)", drainLimit)
		}
	})

	var notReady error
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		probing, stop := context.WithTimeout(context.Background(), requestLimit)
		notReady = app.Probe(probing, cfg)
		stop()
		if notReady == nil {
			return cfg
		}
	}
	t.Fatalf("Probe() = %v, want the edge ready with the export failing", notReady)
	return cfg
}

func TestAFailingExportNeverFailsNorHoldsARequestOfTheEdge(t *testing.T) {
	for name, collector := range map[string]func(testing.TB) *obstest.DownCollector{
		"unreachable endpoint": obstest.UnreachableCollector,
		"stalled collector":    obstest.StallingCollector,
	} {
		t.Run(name, func(t *testing.T) {
			down := collector(t)
			cfg := startEdgeExportingTo(t, down.Addr)
			client := &http.Client{Timeout: requestLimit}
			t.Cleanup(client.CloseIdleConnections)

			requests := int64(cfg.Admission.Burst)
			for n := range requests {
				subject := "user-" + strconv.FormatInt(n, 10)
				req, err := http.NewRequest(http.MethodGet, "http://"+cfg.HTTPAddr+"/v1/me", nil)
				if err != nil {
					t.Fatalf("NewRequest() = %v", err)
				}
				req.Header.Set("Authorization", `Bearer {"sub":"`+subject+`","tenant":"acme","permissions":["profile"]}`)
				began := time.Now()
				res, err := client.Do(req)
				if err != nil {
					t.Fatalf("GET /v1/me as %s = %v after %v with the export failing, want 200 within %v: telemetry never fails nor holds a request (NF Degradação)",
						subject, err, time.Since(began).Round(time.Millisecond), requestLimit)
				}
				var profile struct {
					Subject string `json:"subject"`
				}
				decoded := json.NewDecoder(res.Body).Decode(&profile)
				_ = res.Body.Close()
				if res.StatusCode != http.StatusOK || decoded != nil || profile.Subject != subject {
					t.Fatalf("GET /v1/me as %s = %d with subject %q (%v), want 200 with the subject of the credential", subject, res.StatusCode, profile.Subject, decoded)
				}
			}
			down.RequireEverySignalExporting(t)
		})
	}
}
