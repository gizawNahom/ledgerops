package support

import (
	"fmt"
	"log/slog"
)

// RedactionMarker is what a Secret renders under every formatting verb.
const RedactionMarker = "[REDACTED]"

// Secret holds a value that must never reach output (ADR-018). It redacts
// under String, GoString, every fmt verb, slog and text/JSON marshalling;
// Reveal is the only way to read the value.
type Secret struct{ value string }

// NewSecret wraps a value.
func NewSecret(value string) Secret { return Secret{value: value} }

// Reveal returns the wrapped value. Callers pass it only to the SCRAM builder
// or to the connection that needs the credential.
func (s Secret) Reveal() string { return s.value }

// String renders the fixed redaction marker, never the value.
func (s Secret) String() string { return RedactionMarker }

// GoString renders the fixed redaction marker, never the value.
func (s Secret) GoString() string { return RedactionMarker }

// Format renders the marker under every verb (%v %+v %#v %s %q %x ...).
func (s Secret) Format(state fmt.State, _ rune) {
	_, _ = state.Write([]byte(RedactionMarker))
}

// LogValue keeps the value out of structured logs.
func (s Secret) LogValue() slog.Value { return slog.StringValue(RedactionMarker) }

// MarshalText keeps the value out of text and JSON encodings.
func (s Secret) MarshalText() ([]byte, error) { return []byte(RedactionMarker), nil }

var _ slog.LogValuer = Secret{}
var _ fmt.Formatter = Secret{}
