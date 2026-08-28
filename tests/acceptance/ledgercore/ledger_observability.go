package ledgercore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// OPS-5 support code (fix-ledger-core-observability, 2026-08-26): capturing
// the two driven-external, non-deterministic ports the infrastructure policy
// permits fakes for — the log sink and the metrics exposition — with output
// capture, per the Architecture of Reference ("driven external /
// non-deterministic: fake/stub with output capture, so a Then can observe the
// side effect"). Neither reaches into the domain or the store: both read back
// exactly what a real Prometheus scraper or a real log shipper would see.

// --- the log sink ------------------------------------------------------

// logCapture is the fake log sink: an io.Writer safe for concurrent use (the
// contended scenarios elsewhere in this suite post from many goroutines at
// once, and OPS-5's logging must not serialise them). It stands in for
// stdout, which is where the real slog.NewJSONHandler writes in production
// (cmd/api/main.go) — capturing it is what lets a Then read a side effect a
// real operator would only see in their log aggregator.
type logCapture struct {
	mu  sync.Mutex
	buf strings.Builder
}

func newLogCapture() *logCapture { return &logCapture{} }

func (c *logCapture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

// allLines returns every non-blank line captured since the ledger started —
// the whole corpus, for the release-blocking scenarios that must prove a
// secret is absent everywhere, not just in the most recent request.
func (c *logCapture) allLines() []string {
	c.mu.Lock()
	raw := c.buf.String()
	c.mu.Unlock()

	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// linesFrom returns the lines captured at or after the given marker — the
// per-request delta a driving-port call reads immediately after making it.
func (c *logCapture) linesFrom(marker int) []string {
	all := c.allLines()
	if marker >= len(all) {
		return nil
	}
	return all[marker:]
}

func (c *logCapture) markerCount() int {
	return len(c.allLines())
}

// rawText is the whole corpus as one string — the release-blocking
// scenarios search this directly for a raw secret rather than parsing
// structure first, because the property under test ("never appears") must
// hold of the bytes actually written, not of a field a parser happened to
// recognise.
func (c *logCapture) rawText() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

// parseLogLines decodes each captured JSON line into the OPS-5 vocabulary.
// A line that is not valid JSON, or carries none of the known fields, is
// silently skipped rather than failing the scenario here — an unparseable
// line is a production-logging defect a Then step reports on its own terms
// ("no log line was captured"), not a parser error obscuring the real
// assertion.
func parseLogLines(raw []string) []LogLine {
	var out []LogLine
	for _, line := range raw {
		var generic map[string]any
		if err := json.Unmarshal([]byte(line), &generic); err != nil {
			continue
		}
		out = append(out, logLineFrom(generic, line))
	}
	return out
}

func logLineFrom(m map[string]any, raw string) LogLine {
	ll := LogLine{Raw: raw}
	if v, ok := m["request_id"].(string); ok {
		ll.RequestID = v
	}
	if v, ok := m["route"].(string); ok {
		ll.Route = v
	}
	if v, ok := m["status"].(float64); ok {
		ll.Status = int(v)
	}
	if v, ok := m["elapsed_ms"]; ok {
		ll.HasElapsedMillis = true
		if f, ok := v.(float64); ok {
			ll.ElapsedMillis = f
		}
	}
	if v, ok := m["transaction_id"].(string); ok {
		ll.TransactionID = v
	}
	if v, ok := m["account_ids"].([]any); ok {
		for _, a := range v {
			if s, ok := a.(string); ok {
				ll.AccountIDs = append(ll.AccountIDs, s)
			}
		}
	}
	if v, ok := m["amount_minor"].(float64); ok {
		ll.AmountMinor = int64(v)
	}
	if v, ok := m["currency"].(string); ok {
		ll.Currency = v
	}
	if v, ok := m["idempotency_key_hash"].(string); ok {
		ll.IdempotencyKeyHash = v
	}
	if v, ok := m["replayed"]; ok {
		ll.HasReplayedField = true
		if b, ok := v.(bool); ok {
			ll.Replayed = b
		}
	}
	if v, ok := m["violation_kind"].(string); ok {
		ll.ViolationKind = v
	}
	return ll
}

// --- the metrics exposition ---------------------------------------------

// metricLineRE matches one line of Prometheus text exposition format:
// `metric_name{optional="labels"} value`. Comment lines (# HELP, # TYPE) and
// blank lines are filtered by the caller before this ever sees them.
var metricLineRE = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{[^}]*\})?\s+([0-9eE+\-.]+|NaN|\+Inf|-Inf)\s*$`)

// parseMetricValue reads one series from a scraped exposition body. An empty
// labelFragment reads a label-free series (a gauge); a non-empty one must
// appear verbatim inside the matched line's label set (e.g. `result="posted"`).
func parseMetricValue(body string, series MetricSeries, labelFragment string) (float64, bool) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		match := metricLineRE.FindStringSubmatch(line)
		if match == nil || match[1] != string(series) {
			continue
		}
		labels := match[2]
		if labelFragment != "" && !strings.Contains(labels, labelFragment) {
			continue
		}
		value, err := strconv.ParseFloat(match[3], 64)
		if err != nil {
			continue
		}
		return value, true
	}
	return 0, false
}

// ScrapeMetrics reaches GET /metrics presenting the given credentials. OPS-5
// mounts it unauthenticated (feature-delta.md § Wave: DEVOPS / Observability
// stack, confirmed 2026-08-26 — a scrape target cannot easily present an
// operator bearer key), so the scenarios that matter are the ones presenting
// NoKey or UnissuedKey and still expecting success.
func (l *Ledger) ScrapeMetrics(ctx context.Context, as Credentials) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, l.server.URL+"/metrics", nil)
	if err != nil {
		return err
	}
	l.authenticateAs(request, as)

	response, err := l.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	l.lastMetricsStatus = response.StatusCode
	l.lastMetricsBody = string(raw)
	return nil
}

// MetricValue reads one series from the most recently scraped body.
func (l *Ledger) MetricValue(series MetricSeries, labelFragment string) (float64, bool) {
	return parseMetricValue(l.lastMetricsBody, series, labelFragment)
}

// HistogramObservationCount reads a histogram's `_count` companion series —
// the number of observations recorded, regardless of their distribution.
func (l *Ledger) HistogramObservationCount(series MetricSeries) (float64, bool) {
	return parseMetricValue(l.lastMetricsBody, MetricSeries(string(series)+"_count"), "")
}

// CaptureMetricsBaseline scrapes so later "increased by N" assertions have a
// zero point to compare against. Called once from serve() (ledger_world.go)
// as a startup default, and re-called immediately before the measured
// transfer in the "the integrator moves ... under key ..." step
// (steps_ledger_test.go) — re-timed there (fix-ledger-core-observability,
// 2026-08-28) because a scenario's own Background/Given funding (e.g. "a
// wallet account X funded with N", which posts a real POST /transfers from
// "treasury") happens AFTER serve() and itself moves postings_total. Without
// the re-time, the serve()-time baseline predates that funding transfer, so a
// "posted" outcome's "increased by 1" assertion would see the funding's own
// increment folded in. Re-capturing right before the measured transfer makes
// the baseline reflect "state right before the action under test", not
// "state right after the server booted" — best-effort either way; while GET
// /metrics is still the scaffold (RED, pre-DELIVER), the scrape fails to
// parse and every series reads as absent, which CaptureMetricValue below
// treats as a baseline of 0. That is the correct RED-time behaviour: it is
// what "the counter has never been incremented because it does not exist
// yet" looks like.
func (l *Ledger) CaptureMetricsBaseline(ctx context.Context) {
	_ = l.ScrapeMetrics(ctx, ApplicationRole)
	l.metricsBaseline = make(MetricsSnapshot)
	for _, series := range DeclaredMetricSeries() {
		if v, ok := l.MetricValue(series, ""); ok {
			l.metricsBaseline[MetricKey(series, "")] = v
		}
		if v, ok := l.HistogramObservationCount(series); ok {
			l.metricsBaseline[MetricKey(MetricSeries(string(series)+"_count"), "")] = v
		}
	}
	for _, outcome := range []PostingResult{PostedResult, RejectedResult, ReplayedResult} {
		label := fmt.Sprintf("result=%q", string(outcome))
		if v, ok := l.MetricValue(PostingsTotal, label); ok {
			l.metricsBaseline[MetricKey(PostingsTotal, label)] = v
		}
	}
}

// baselineFor reads a captured baseline value, defaulting to zero for a
// series the baseline scrape never saw — which is exactly what "not
// incremented yet" and "does not exist yet" both look like, and the only
// safe default when the exposition endpoint is still a RED scaffold.
func (l *Ledger) baselineFor(key string) float64 {
	return l.metricsBaseline[key]
}

// --- the driving port for an unauthenticated request to any protected route

// RequestAsUnidentifiedCaller reaches the named "METHOD path" endpoint
// presenting no operator key, for the regression coverage asserting the
// router restructuring this fix requires (moving GET /metrics out of the
// requireOperatorKey group) does not accidentally widen it further.
func (l *Ledger) RequestAsUnidentifiedCaller(ctx context.Context, endpoint string) error {
	parts := strings.SplitN(strings.TrimSpace(endpoint), " ", 2)
	if len(parts) != 2 {
		return fmt.Errorf("endpoint %q is not of the form \"METHOD path\"", endpoint)
	}
	method, path := parts[0], parts[1]
	var body any
	if method == http.MethodPost {
		body = map[string]any{}
	}
	answer, err := l.callAs(ctx, NoKey, method, path, body, NoIdempotencyKey)
	l.lastAnswer = answer
	return err
}
