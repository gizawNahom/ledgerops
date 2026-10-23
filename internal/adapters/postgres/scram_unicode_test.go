package postgres_test

import (
	"testing"

	"ledgerops/internal/adapters/postgres"
	"ledgerops/internal/support"
)

func TestBuildSCRAMVerifierFoldsFullwidthAndDecomposedFormsToOneVerifier(t *testing.T) {
	salt := []byte("0123456789abcdef")
	verifier := func(password string) string {
		return postgres.BuildSCRAMVerifier(support.NewSecret(password), salt, 4096)
	}
	composed := verifier("café-Pw1")
	if got := verifier("café-Pw1"); got != composed {
		t.Errorf("decomposed form must match composed: %s vs %s", got, composed)
	}
	if got := verifier("ｃafé-Pw1"); got != composed {
		t.Errorf("fullwidth form must match composed: %s vs %s", got, composed)
	}
	if got := verifier("café -Pw1"); got == composed {
		t.Error("a non-breaking space is a space, not nothing")
	}
}
