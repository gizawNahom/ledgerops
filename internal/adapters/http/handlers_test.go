package http

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// hashIdempotencyKey is a pure function: input in, digest out, no side
// effects (OPS-5 design decision 5). These are the paired unit tests
// nw-acceptance-designer's AT contract expects alongside the acceptance
// scenarios in milestone-06-observability.feature.

func TestHashIdempotencyKey_KnownDigests(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "empty string",
			key:  "",
			want: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name: "obs-5",
			key:  "obs-5",
			want: hex.EncodeToString(sha256Sum("obs-5")),
		},
		{
			name: "the-callers-own-secret-key",
			key:  "the-callers-own-secret-key",
			want: hex.EncodeToString(sha256Sum("the-callers-own-secret-key")),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hashIdempotencyKey(tc.key)
			if got != tc.want {
				t.Fatalf("hashIdempotencyKey(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

func sha256Sum(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

// TestHashIdempotencyKey_Properties asserts the invariants the design
// decision states directly: deterministic, never leaks the raw input as a
// substring of its own output, and always exactly 64 lowercase hex
// characters -- regardless of what the raw key looks like.
func TestHashIdempotencyKey_Properties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Keys shorter than 6 characters are excluded: a short key drawn
		// from the hex alphabet has a non-negligible chance of appearing
		// as a substring of ANY 64-character hex digest purely by
		// coincidence, which would make the substring check flaky rather
		// than meaningful. Realistic idempotency keys (UUIDs, opaque
		// tokens) are far longer than this floor.
		key := rapid.StringN(6, 128, -1).Draw(t, "key")

		digest := hashIdempotencyKey(key)

		if len(digest) != 64 {
			t.Fatalf("digest %q is %d characters, want 64", digest, len(digest))
		}
		for _, r := range digest {
			isLowerHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
			if !isLowerHex {
				t.Fatalf("digest %q contains non-lowercase-hex character %q", digest, r)
			}
		}
		if key != "" && strings.Contains(digest, key) {
			t.Fatalf("digest %q contains the raw key %q as a substring", digest, key)
		}
		if hashIdempotencyKey(key) != digest {
			t.Fatalf("hashIdempotencyKey(%q) is not deterministic", key)
		}
	})
}
