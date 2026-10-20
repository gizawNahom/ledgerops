package support

// SCAFFOLD: true

// Secret holds a value that must never reach output (ADR-018). It redacts
// under String, GoString and slog.LogValuer. RED scaffold: DELIVER implements
// the redaction; Reveal is the only way to read the value.
type Secret struct{ value string }

// NewSecret wraps a value.
func NewSecret(value string) Secret { return Secret{value: value} }

// Reveal returns the wrapped value. Callers pass it only to the SCRAM builder.
func (s Secret) Reveal() string { return s.value }

// String must render a fixed redaction marker, never the value.
func (s Secret) String() string { Scaffold("Secret.String"); return "" }

// GoString must render a fixed redaction marker, never the value.
func (s Secret) GoString() string { Scaffold("Secret.GoString"); return "" }
