package api

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/mateusmacedo/dmpf/libs/backend/go/ports"
)

func derivedFor(subject, key string) string {
	digest := sha256.Sum256([]byte(subject + "\x00" + key))
	return hex.EncodeToString(digest[:])
}

func TestTheDerivedKeyBelongsToTheSubject(t *testing.T) {
	tester, another := ports.SubjectID("tester"), ports.SubjectID("another")

	if got := deriveIdempotencyKey(&tester, ""); got != "" {
		t.Fatalf("deriveIdempotencyKey(no key) = %q, want none", got)
	}
	if deriveIdempotencyKey(&tester, "k-1") == deriveIdempotencyKey(&another, "k-1") {
		t.Fatal("two subjects with the same client key share an entry (IDM-03)")
	}
	if got := deriveIdempotencyKey(&tester, "k-1"); got != derivedFor("tester", "k-1") || !ports.ValidIdempotencyKey(got) {
		t.Fatalf("deriveIdempotencyKey() = %q, want hex(sha256(subject 0x00 key))", got)
	}
	if got := deriveIdempotencyKey(nil, "k-1"); got != derivedFor("", "k-1") {
		t.Fatalf("deriveIdempotencyKey(nil subject) = %q, want the key scoped to no owner", got)
	}
}
