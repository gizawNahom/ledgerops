// request_logging.go implements OPS-5's per-request structured logging
// (feature-delta.md § Wave: DEVOPS / Observability stack), design decisions
// 2 and 3 (fix-ledger-core-observability, 2026-08-27). requestLogger wraps
// the entire router (router.go registers it via router.Use, before any
// grouping) so every response -- success, domain refusal, unidentified-caller
// 401, static asset serve, metrics scrape -- passes through it and gets
// exactly one log line.
//
// Fields, per kpi-contracts.yaml § runtime_instrumentation.logs:
//   - always (this step): request_id, route (chi's matched pattern, never
//     the raw path — avoids cardinality blowup), status, elapsed_ms
//   - posting endpoints, idempotency paths, rejections: set by deeper
//     handlers via fieldsFrom(r.Context()).Set(...) (logfields.go) — later
//     steps (02-02/02-03), not this one.
//
// Hard constraint carried from the RCA, restated here so it travels with
// this file: the raw Authorization header value and the raw idempotency key
// value must never reach a log call, on any path, including 4xx/5xx
// error-echo paths. See milestone-06-observability.feature's
// @release-blocking scenarios. This step's fields (request_id/route/status/
// elapsed_ms) carry no secrets, so that constraint is upheld trivially here
// and enforced properly once later steps add the fields that could.
package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// requestLogger creates the per-request accumulator, stores it on the
// request context, calls next.ServeHTTP, then reads back the matched route
// pattern together with whatever the accumulator collected and makes
// exactly one slog call. It is the only per-request slog call site in this
// package.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()

			ctx, requestFields := contextWithFields(r.Context())
			r = r.WithContext(ctx)

			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(recorder, r)

			elapsed := time.Since(started)
			route := chi.RouteContext(r.Context()).RoutePattern()

			attrs := requestFields.snapshot()
			attrs["request_id"] = middleware.GetReqID(r.Context())
			attrs["route"] = route
			attrs["status"] = recorder.status
			attrs["elapsed_ms"] = float64(elapsed.Microseconds()) / 1000.0

			args := make([]any, 0, len(attrs)*2)
			for key, value := range attrs {
				args = append(args, key, value)
			}

			if recorder.status >= http.StatusInternalServerError {
				logger.Warn("request", args...)
				return
			}
			logger.Info("request", args...)
		})
	}
}

// statusRecorder captures the status code a handler wrote so requestLogger
// can read it back after next.ServeHTTP returns -- http.ResponseWriter has
// no read path of its own.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}
