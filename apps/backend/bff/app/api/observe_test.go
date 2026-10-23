package api_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

const severityKey = "severity"

func serverSpan(t *testing.T, f fixture, name string) tracetest.SpanStub {
	t.Helper()
	var found []tracetest.SpanStub
	for _, s := range f.spans.GetSpans() {
		if s.SpanKind == trace.SpanKindServer {
			found = append(found, s)
		}
	}
	if len(found) != 1 || found[0].Name != name {
		names := make([]string, len(found))
		for i, s := range found {
			names[i] = s.Name
		}
		t.Fatalf("SERVER spans = %v, want one %q", names, name)
	}
	return found[0]
}

func attributesOf(span tracetest.SpanStub) map[attribute.Key]attribute.Value {
	values := make(map[attribute.Key]attribute.Value, len(span.Attributes))
	for _, kv := range span.Attributes {
		values[kv.Key] = kv.Value
	}
	return values
}

func durationCount(t *testing.T, f fixture) uint64 {
	t.Helper()
	var collected metricdata.ResourceMetrics
	if err := f.metrics.Collect(context.Background(), &collected); err != nil {
		t.Fatalf("Collect() = %v", err)
	}
	var count uint64
	for _, scope := range collected.ScopeMetrics {
		for _, m := range scope.Metrics {
			if histogram, ok := m.Data.(metricdata.Histogram[float64]); ok && m.Name == "http.server.request.duration" {
				for _, point := range histogram.DataPoints {
					count += point.Count
				}
			}
		}
	}
	return count
}

func accessRecords(t *testing.T, f fixture) []sdklog.Record {
	t.Helper()
	if err := f.logProvider.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush() = %v", err)
	}
	var records []sdklog.Record
	for _, record := range f.records.all() {
		if record.Body().AsString() == "http request" {
			records = append(records, record)
		}
	}
	return records
}

func accessLogs(t *testing.T, f fixture) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, record := range accessRecords(t, f) {
		fields := map[string]any{severityKey: record.SeverityText()}
		record.WalkAttributes(func(kv attribute.KeyValue) bool {
			fields[string(kv.Key)] = kv.Value.AsInterface()
			return true
		})
		records = append(records, fields)
	}
	return records
}

func onlyAccessLog(t *testing.T, f fixture) map[string]any {
	t.Helper()
	records := accessLogs(t, f)
	if len(records) != 1 {
		t.Fatalf("access log = %v, want one record", records)
	}
	return records[0]
}

func accessLogBeforeExport(t *testing.T, f fixture) map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(f.logs.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err == nil && record["msg"] == "http request" {
			records = append(records, record)
		}
	}
	if len(records) != 1 {
		t.Fatalf("emitted access log = %v, want one record", f.logs.String())
	}
	return records[0]
}
