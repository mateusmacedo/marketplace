package main

import (
	"bytes"
	"strings"
	"testing"
)

func lookup(pairs ...string) func(string) string {
	values := map[string]string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		values[pairs[i]] = pairs[i+1]
	}
	return func(name string) string { return values[name] }
}

func TestRunRefusesToStartWithoutARole(t *testing.T) {
	var errOut bytes.Buffer

	code := run(options{role: "", lookup: lookup()}, &errOut)

	if code != exitUsage || !strings.Contains(errOut.String(), "--role api|relay|consumer") {
		t.Fatalf("run() = %d, stderr %q; want usage listing api|relay|consumer", code, errOut.String())
	}
}

func TestRunRefusesARoleThisContextDoesNotHave(t *testing.T) {
	var errOut bytes.Buffer

	code := run(options{role: "buyer", lookup: lookup("PG_DSN", "postgres://x")}, &errOut)

	if code != exitUsage || !strings.Contains(errOut.String(), "api|relay|consumer") {
		t.Fatalf("run() = %d, stderr %q; want the refusal to name the roles it has", code, errOut.String())
	}
}

func TestRunRefusesTheApiRoleWithoutTheDatabase(t *testing.T) {
	var errOut bytes.Buffer

	code := run(options{role: "api", lookup: lookup()}, &errOut)

	if code != exitUsage || !strings.Contains(errOut.String(), "PG_DSN") {
		t.Fatalf("run() = %d, stderr %q; want the refusal to name the missing variable", code, errOut.String())
	}
}

func TestRunRefusesTheRelayRoleWithoutItsBroker(t *testing.T) {
	var errOut bytes.Buffer

	code := run(options{role: "relay", lookup: lookup("PG_DSN", "postgres://x")}, &errOut)

	if code != exitUsage {
		t.Fatalf("run() = %d, want exitUsage: the relay needs the broker", code)
	}
	if !strings.Contains(errOut.String(), "KAFKA_BROKERS") {
		t.Fatalf("stderr = %q, want it to name KAFKA_BROKERS, the first missing variable of the relay", errOut.String())
	}
}

func TestRunRefusesTheConsumerRoleWithoutItsBrokerNorTheOperator(t *testing.T) {
	for _, tc := range []struct {
		pairs []string
		want  string
	}{
		{[]string{"PG_DSN", "postgres://x"}, "KAFKA_BROKERS"},
		{[]string{"PG_DSN", "postgres://x", "KAFKA_BROKERS", "b:9092", "KAFKA_INSECURE", "true", "KAFKA_IAM_TOPIC", "t", "KAFKA_IAM_DLQ", "d", "KAFKA_GROUP", "g"}, "IAM_OPERATOR_TENANT"},
	} {
		var errOut bytes.Buffer

		code := run(options{role: "consumer", lookup: lookup(tc.pairs...)}, &errOut)

		if code != exitUsage || !strings.Contains(errOut.String(), tc.want) {
			t.Fatalf("run(consumer) = %d, stderr %q; want the refusal to name %s", code, errOut.String(), tc.want)
		}
	}
}
