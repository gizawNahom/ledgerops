// Package support holds small, pure, cross-adapter primitives that carry no
// business meaning of their own -- shared plumbing the composition root and
// its adapters need, but that belongs to none of them individually. It is
// not the domain (it decides nothing about accounts, transfers, or
// tenants) and it is not an adapter (it performs no I/O) -- a pure leaf
// dependency any package may import.
package support

import (
	"crypto/sha256"
	"encoding/hex"
)

// SHA256Hex returns the SHA-256 digest of s, hex-encoded. Pure function:
// input in, digest out, no side effects.
//
// Before this package existed, this exact two-line computation was
// independently repeated under three different names --
// postgres.hashCredential (tenant_key), http.hashIdempotencyKey
// (idempotency key), and cmd/api's own hashCredential (the demo tenant key
// comparison) -- each with a comment pointing at one of the others as "the
// same approach, reused rather than reinvented" without actually sharing
// code. This is the one place that computation now lives; the three
// call-site wrappers keep their own names because each documents WHY its
// caller hashes what it hashes, which SHA256Hex itself should not have to
// know.
func SHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
