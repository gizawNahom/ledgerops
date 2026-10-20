package postgres_test

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"ledgerops/internal/adapters/postgres"
	"ledgerops/internal/support"
)

// RFC 7677 section 3 test vector: password "pencil", salt W22ZaJ0SNY7soEsUEjb6gQ==, 4096 iterations.
func TestBuildSCRAMVerifierMatchesTheRFC7677Vector(t *testing.T) {
	salt := []byte{0x5b, 0x6d, 0x99, 0x68, 0x9d, 0x12, 0x35, 0x8e, 0xec, 0xa0, 0x4b, 0x14, 0x12, 0x36, 0xfa, 0x81}
	want := "SCRAM-SHA-256$4096:W22ZaJ0SNY7soEsUEjb6gQ==$WG5d8oPm3OtcPnkdi4Uo7BkeZkBFzpcXkuLmtbsT4qY=:wfPLwcE6nTWhTAmQ7tl2KeoiWGPlZqQxSrmfPwDl2dU="
	got := postgres.BuildSCRAMVerifier(support.NewSecret("pencil"), salt, 4096)
	if got != want {
		t.Fatalf("verifier\n got %s\nwant %s", got, want)
	}
}

func TestBuildSCRAMVerifierNormalisesUnicodeAndIsDeterministic(t *testing.T) {
	salt := []byte("0123456789abcdef")
	composed := postgres.BuildSCRAMVerifier(support.NewSecret("café"), salt, 4096)
	decomposed := postgres.BuildSCRAMVerifier(support.NewSecret("café"), salt, 4096)
	if composed != decomposed {
		t.Fatalf("NFC and NFD spellings must yield one verifier: %s vs %s", composed, decomposed)
	}
	nbsp := postgres.BuildSCRAMVerifier(support.NewSecret("a b"), salt, 4096)
	space := postgres.BuildSCRAMVerifier(support.NewSecret("a b"), salt, 4096)
	if nbsp != space {
		t.Fatal("SASLprep maps non-ASCII space to space")
	}
}

func TestBuildSCRAMVerifierNeverContainsThePlaintext(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		password := rapid.StringMatching(`[A-Za-z0-9]{10,24}`).Draw(t, "password")
		salt := rapid.SliceOfN(rapid.Byte(), 16, 16).Draw(t, "salt")
		verifier := postgres.BuildSCRAMVerifier(support.NewSecret(password), salt, 4096)
		if strings.Contains(verifier, password) || !strings.HasPrefix(verifier, "SCRAM-SHA-256$4096:") {
			t.Fatalf("bad verifier %q", verifier)
		}
	})
}

func TestRedactDSNNeverLeavesThePassword(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		password := rapid.StringMatching(`[A-Za-z0-9!#$%^*()_+=-]{8,24}`).Draw(t, "password")
		dsn := (&url.URL{Scheme: "postgres", User: url.UserPassword("ledgerops_migrate", password), Host: "db:5432", Path: "/ledgerops"}).String()
		texts := []string{
			dsn,
			fmt.Sprintf("connect %q failed: dial tcp: refused", dsn),
			fmt.Sprintf("host=db user=u password=%s dbname=x", password),
			fmt.Sprintf("host=db password='%s' dbname=x", password),
		}
		for _, text := range texts {
			if redacted := postgres.RedactDSN(text); strings.Contains(redacted, password) {
				t.Fatalf("password %q survived in %q", password, redacted)
			}
		}
	})
}

func TestRedactDSNKeepsNonSecretText(t *testing.T) {
	got := postgres.RedactDSN("postgres://ledgerops_migrate:hunter2@db:5432/ledgerops?sslmode=disable")
	want := "postgres://ledgerops_migrate:[REDACTED]@db:5432/ledgerops?sslmode=disable"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}
