package support_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"ledgerops/internal/support"
)

func TestSecretNeverRendersItsValue(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		value := rapid.StringMatching(`[A-Za-z0-9]{8,24}`).Draw(t, "value")
		secret := support.NewSecret(value)
		var logged bytes.Buffer
		slog.New(slog.NewJSONHandler(&logged, nil)).Info("m", "secret", secret)
		encoded, _ := json.Marshal(struct{ S support.Secret }{secret})
		rendered := []string{
			fmt.Sprintf("%v|%+v|%#v|%s|%q|%x|%d", secret, secret, secret, secret, secret, secret, secret),
			fmt.Sprintf("%v", &secret), fmt.Sprintf("%+v", struct{ S support.Secret }{secret}),
			secret.String(), secret.GoString(), logged.String(), string(encoded),
		}
		for _, text := range rendered {
			if strings.Contains(text, value) {
				t.Fatalf("secret value leaked in %q", text)
			}
		}
	})
}

func TestSecretRevealReturnsTheValue(t *testing.T) {
	if got := support.NewSecret("app-secret").Reveal(); got != "app-secret" {
		t.Fatalf("Reveal = %q", got)
	}
}
