// logfields.go implements OPS-5's write-only per-request accumulator
// (fix-ledger-core-observability, design decision 2). requestLogger
// (request_logging.go) is the sole reader: it constructs the accumulator,
// stores it on the request context, and drains it into exactly one slog
// call after next.ServeHTTP returns.
//
// fields exposes only Set -- no Get, no iteration. A handler deep in the
// call stack can record a fact ("transaction_id", "violation_kind", ...) at
// the exact point it already knows the value, but it can never read back
// what a different part of the same request already recorded. This mirrors
// "driving ports that only read must not expose write methods" in reverse:
// a capability that only writes must not expose read.
package http

import (
	"context"
	"sync"
)

// fields is the write-only per-request accumulator. Safe for concurrent
// Set calls -- a single request is handled by one goroutine in net/http,
// but nothing here assumes that stays true forever.
type fields struct {
	mu sync.Mutex
	m  map[string]any
}

// Set records one field. The only method this type exposes.
func (f *fields) Set(key string, value any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.m == nil {
		f.m = make(map[string]any)
	}
	f.m[key] = value
}

// snapshot returns a shallow copy of everything recorded so far. Unexported
// -- only requestLogger, the package-private reader that constructed the
// accumulator, may drain it.
func (f *fields) snapshot() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]any, len(f.m))
	for k, v := range f.m {
		out[k] = v
	}
	return out
}

type fieldsContextKey struct{}

// contextWithFields stores a fresh accumulator on ctx, returning both the
// new context and the accumulator itself so the caller can keep a direct
// reference without a second context lookup.
func contextWithFields(ctx context.Context) (context.Context, *fields) {
	f := &fields{}
	return context.WithValue(ctx, fieldsContextKey{}, f), f
}

// fieldsFrom reads the accumulator off ctx. When no accumulator was ever
// stored (a handler logging outside requestLogger's scope, which should
// never happen), it returns a fresh no-op *fields rather than nil -- Set
// degrades to silently discarding the value instead of panicking.
func fieldsFrom(ctx context.Context) *fields {
	if f, ok := ctx.Value(fieldsContextKey{}).(*fields); ok {
		return f
	}
	return &fields{}
}
