// metrics.go is the Prometheus collector composition for OPS-5
// (feature-delta.md § Wave: DEVOPS / Observability stack;
// docs/feature/ledger-core/design/2026-08-27-ops-5-observability-design-decisions.md
// § Decision 1). This is the ONLY file in the package that constructs a
// prometheus.Registry or calls MustRegister — single-registration-site
// discipline, so the seven declared series stay the whole story a reviewer
// has to read, by inspection, without a second file to cross-reference.
//
// Metrics wraps the registry and exposes only narrow, named observation
// methods. A caller is never handed a raw prometheus.Counter/Histogram/Gauge
// — that would let a call site increment the wrong series, or increment a
// series with the wrong labels, silently. The seven methods below are the
// only way anything outside this file can move a number.
package http

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PostingOutcome is the `result` label ledgerops_postings_total carries — the
// three shapes a posting attempt can settle into (DDD-8): accepted for the
// first time, refused, or answered again from the record. This is the
// adapter-layer vocabulary ObservePosting's caller already has in hand at
// its single decision site (postTransferHandler); it is deliberately its own
// type here rather than importing the acceptance suite's PostingResult
// (tests/acceptance/ledgercore/domain_types.go) — that type belongs to the
// test package and documents Gherkin phrasing, not the adapter's own
// decision. The two vocabularies are label-string compatible by construction
// (posted | rejected | replayed) so nothing drifts in practice.
type PostingOutcome string

const (
	PostingPosted   PostingOutcome = "posted"
	PostingRejected PostingOutcome = "rejected"
	PostingReplayed PostingOutcome = "replayed"
)

// Metrics is the adapter's Prometheus collector set. The registry is
// unexported; every collector inside it is unexported. Nothing outside this
// file can reach a collector directly.
type Metrics struct {
	registry *prometheus.Registry

	postingsTotal            *prometheus.CounterVec
	postingDurationSeconds   prometheus.Histogram
	insufficientFundsTotal   prometheus.Counter
	idempotentReplaysTotal   prometheus.Counter
	trialBalanceImbalance    prometheus.Gauge
	trialBalanceScanDuration prometheus.Histogram
	driftedAccounts          prometheus.Gauge

	// transferStateNonterminalCount and transferStateOldestNextAttemptAge
	// are inter-tenant-transfer's own two operational-visibility gauges
	// (ADR-015 Amendment 2, wave-decisions.md § Key Decisions item 4):
	// caller-set (SetTransferStateBacklog below), the same pattern
	// trialBalanceImbalance/driftedAccounts already use above, rather than a
	// GaugeFunc computed on scrape — the coordinator, not this package,
	// knows how to read transfer_state, and this file's own single-
	// registration-site discipline (package doc) still holds either way.
	transferStateNonterminalCount     prometheus.Gauge
	transferStateOldestNextAttemptAge prometheus.Gauge
}

// NewMetrics constructs the registry and registers all seven declared
// series, exactly once. This is the single registration site: no other
// function in this package calls MustRegister.
func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()

	m := &Metrics{
		registry: registry,
		postingsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ledgerops_postings_total",
			Help: "Total number of transfer postings, labelled by result.",
		}, []string{"result"}),
		postingDurationSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "ledgerops_posting_duration_seconds",
			Help: "Time taken to answer a transfer posting request.",
		}),
		insufficientFundsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ledgerops_insufficient_funds_rejections_total",
			Help: "Total number of transfers refused for insufficient funds.",
		}),
		idempotentReplaysTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "ledgerops_idempotent_replays_total",
			Help: "Total number of transfers answered from a prior idempotent record.",
		}),
		trialBalanceImbalance: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ledgerops_trial_balance_imbalance_minor",
			Help: "Signed trial-balance imbalance, in minor units, from the most recent verification scan.",
		}),
		trialBalanceScanDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "ledgerops_trial_balance_scan_duration_seconds",
			Help: "Time taken to complete a trial-balance verification scan.",
		}),
		driftedAccounts: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ledgerops_drift_accounts",
			Help: "Number of accounts whose stored balance disagreed with their entries in the most recent verification scan.",
		}),
		transferStateNonterminalCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ledgerops_transfer_state_nonterminal_count",
			Help: "Number of cross-tenant transfers currently in a non-terminal status (pending or retrying).",
		}),
		transferStateOldestNextAttemptAge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ledgerops_transfer_state_oldest_next_attempt_age_seconds",
			Help: "Age, in seconds, of the oldest due-but-unclaimed cross-tenant transfer retry.",
		}),
	}

	registry.MustRegister(
		m.postingsTotal,
		m.postingDurationSeconds,
		m.insufficientFundsTotal,
		m.idempotentReplaysTotal,
		m.trialBalanceImbalance,
		m.trialBalanceScanDuration,
		m.driftedAccounts,
		m.transferStateNonterminalCount,
		m.transferStateOldestNextAttemptAge,
	)

	// A CounterVec exposes no series at all until a label combination has
	// been observed at least once -- an unauthenticated scrape taken before
	// any transfer ever posted would otherwise find ledgerops_postings_total
	// simply absent from the exposition (OPS-5 step 01-02). Pre-declaring
	// all three outcomes at zero makes the series visible from the first
	// scrape, matching the other six declared series, which are always
	// present because they are not label-vectors.
	for _, outcome := range []PostingOutcome{PostingPosted, PostingRejected, PostingReplayed} {
		m.postingsTotal.WithLabelValues(string(outcome))
	}

	return m
}

// Handler exposes the registry over the Prometheus text-exposition format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// ObservePosting increments ledgerops_postings_total{result=...} AND
// observes ledgerops_posting_duration_seconds in one call, at the single
// site the caller already decided the outcome — one call site, one method,
// no risk of the two series drifting apart or double-counting.
func (m *Metrics) ObservePosting(result PostingOutcome, elapsed time.Duration) {
	m.postingsTotal.WithLabelValues(string(result)).Inc()
	m.postingDurationSeconds.Observe(elapsed.Seconds())
}

// ObserveInsufficientFundsRejection increments
// ledgerops_insufficient_funds_rejections_total.
func (m *Metrics) ObserveInsufficientFundsRejection() {
	m.insufficientFundsTotal.Inc()
}

// ObserveIdempotentReplay increments ledgerops_idempotent_replays_total.
func (m *Metrics) ObserveIdempotentReplay() {
	m.idempotentReplaysTotal.Inc()
}

// SetTrialBalanceImbalance sets ledgerops_trial_balance_imbalance_minor to
// the signed imbalance, in minor units, the caller already computed for the
// wire response.
func (m *Metrics) SetTrialBalanceImbalance(minor int64) {
	m.trialBalanceImbalance.Set(float64(minor))
}

// ObserveTrialBalanceScanDuration observes
// ledgerops_trial_balance_scan_duration_seconds.
func (m *Metrics) ObserveTrialBalanceScanDuration(elapsed time.Duration) {
	m.trialBalanceScanDuration.Observe(elapsed.Seconds())
}

// SetDriftedAccounts sets ledgerops_drift_accounts to the number of accounts
// the caller already found drifted for the wire response.
func (m *Metrics) SetDriftedAccounts(n int) {
	m.driftedAccounts.Set(float64(n))
}

// SetTransferStateBacklog sets both inter-tenant-transfer backlog gauges
// together, at the single call site that already knows both facts from one
// scan of transfer_state (ADR-015 Amendment 2) — mirroring ObservePosting's
// own "one call, two series" shape above rather than risking the two
// numbers drifting apart across two separate calls. oldestNextAttemptAge is
// the caller's own already-computed time.Since(oldest next_attempt_at); a
// backlog of zero non-terminal transfers has no "oldest" to report, so the
// caller passes 0 in that case.
func (m *Metrics) SetTransferStateBacklog(nonterminalCount int, oldestNextAttemptAge time.Duration) {
	m.transferStateNonterminalCount.Set(float64(nonterminalCount))
	m.transferStateOldestNextAttemptAge.Set(oldestNextAttemptAge.Seconds())
}
