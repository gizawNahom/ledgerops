package ledgercore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"time"

	apphttp "ledgerops/internal/adapters/http"
	"ledgerops/internal/adapters/postgres"
	"ledgerops/tests/common/statedelta"
)

// Ledger is the acceptance suite's composition root — the single place that
// knows how a scenario reaches the system. Step definitions call its methods
// and nothing else; per Mandate-12 no business logic lives in a step body.
//
// It wires the PRODUCTION composition root (Tier A, per Mandate 10): the real
// chi router, the real API-key middleware, the real postgres adapter against a
// real PostgreSQL 16 from Testcontainers (OPS-11, WS strategy C). The only
// fakes are Clock and IDGenerator, which are function types (DDD-13) and exist
// solely so timestamps and transaction ids are assertable
// (docs/architecture/atdd-infrastructure-policy.md).
// The state machine this suite exercises (C2 of the AT completeness taxonomy,
// added 2026-08-19). Recorded here because a state model nobody wrote down is a
// state model nobody covered — writing it out is what surfaced the one illegal
// event with no scenario against it (opening an account that is already open),
// which is upstream-unspecified and is raised rather than invented.
//
//	STORE       no schema --migrate--> migrated --post--> populated
//	                                       ^                  |
//	                                       +--migrate again---+   (expand-only, D7)
//	                            populated --tamper as privileged--> drifted
//	                            drifted stays drifted: there is no repair path,
//	                            deliberately (slice 04 § OUT of scope)
//
//	ACCOUNT     unopened --open--> open
//	            illegal from unopened: transfer in, transfer out, trace  (refused)
//	            illegal from open:     open again                        (UNSPECIFIED)
//
//	KEY         unused --post accepted--> committed
//	            unused --post refused---> unused        (a refusal consumes nothing)
//	            committed --same request--> replayed  (committed, re-rendered)
//	            committed --different request--> conflict (committed, refused)
//	            illegal from unused: none — a key is only ever presented with a request
//	            absent key: refused outright (optional idempotency is unused idempotency)
//
// Every transition above has at least one scenario, and every state has at
// least one illegal-event scenario, except where marked UNSPECIFIED.

type Ledger struct {
	server *httptest.Server
	client *http.Client

	// Two DSNs, never one (OPS-10). The suite acts as the application role;
	// only the corruption and migration helpers use the privileged role.
	appDSN        string
	privilegedDSN string

	operatorKey string
	actingAs    Credentials

	clock       func() time.Time
	nextID      func() string
	fixedIDs    []string
	accountKind map[AccountName]AccountKind

	lastAnswer   Answer
	priorAnswer  Answer
	lastAnswers  []Answer
	lastRequest  Transfer
	lastReport   BooksReport
	otherReport  BooksReport
	lastTrace    []TraceRow
	priorTrace   []TraceRow
	lastRace     RaceReport
	lastTamper   error
	tamperedRow  int
	tamperedBy   Money
	migrationSQL []string
}

// --- lifecycle -------------------------------------------------------------

// StartAgainstEmptyStore brings up a real PostgreSQL 16 container, migrates the
// schema from zero as the privileged role, and serves the production router
// over a real socket as the application role.
func (l *Ledger) StartAgainstEmptyStore(ctx context.Context) error {
	appDSN, privilegedDSN, err := startPostgres(ctx)
	if err != nil {
		return fmt.Errorf("bringing up PostgreSQL 16: %w", err)
	}
	l.appDSN, l.privilegedDSN = appDSN, privilegedDSN

	if err := postgres.Migrate(ctx, privilegedDSN); err != nil {
		return fmt.Errorf("migrating from zero: %w", err)
	}
	return l.serve(ctx)
}

// StartWithoutSchema brings up the container and deliberately does not migrate,
// so a scenario can assert that the schema builds from nothing.
func (l *Ledger) StartWithoutSchema(ctx context.Context) error {
	appDSN, privilegedDSN, err := startPostgres(ctx)
	if err != nil {
		return fmt.Errorf("bringing up PostgreSQL 16: %w", err)
	}
	l.appDSN, l.privilegedDSN = appDSN, privilegedDSN
	return nil
}

func (l *Ledger) serve(ctx context.Context) error {
	store, err := postgres.Open(ctx, l.appDSN)
	if err != nil {
		return fmt.Errorf("opening the store as the application role: %w", err)
	}
	handler := apphttp.NewRouter(apphttp.Deps{
		Store:       store,
		OperatorKey: l.operatorKey,
		Clock:       l.clock,
		IDGenerator: l.nextID,
	})
	l.server = httptest.NewServer(handler)
	l.client = l.server.Client()
	return nil
}

// Restart stops the process and serves again against the same store, which is
// how the chaos scenario observes what survived an interrupted write.
func (l *Ledger) Restart(ctx context.Context) error {
	l.Stop()
	return l.serve(ctx)
}

// Stop releases the server. The container is released by the suite teardown.
func (l *Ledger) Stop() {
	if l.server != nil {
		l.server.Close()
		l.server = nil
	}
}

// --- the observable universe ----------------------------------------------

// Universe is the set of port-exposed observable names this suite reasons
// about. Every name here is readable through a driving port — none of them
// names an internal field, so a rename inside the domain cannot red a scenario
// (nw-distill § Example, good vs bad Universe).
func Universe(accounts ...AccountName) []string {
	names := []string{
		"ledger.entry_count",
		"ledger.trial_balance",
		"ledger.transaction_count",
		"ledger.drifted_accounts",
	}
	for _, account := range accounts {
		names = append(names, fmt.Sprintf("account.%s.balance", account))
		names = append(names, fmt.Sprintf("account.%s.entry_count", account))
	}
	sort.Strings(names)
	return names
}

// CaptureUniverse reads every observable in the universe through the driving
// ports, so a before/after pair can be handed to statedelta.AssertStateDelta.
func (l *Ledger) CaptureUniverse(ctx context.Context, accounts ...AccountName) (statedelta.Snapshot, error) {
	report, err := l.readBooks(ctx, HealthSurface)
	if err != nil {
		return nil, err
	}
	snapshot := statedelta.Snapshot{
		"ledger.entry_count":      report.EntryCount,
		"ledger.trial_balance":    report.TrialBalance,
		"ledger.transaction_count": report.EntryCount / 2,
		"ledger.drifted_accounts": len(report.Drifted),
	}
	for _, account := range accounts {
		balance, err := l.readBalance(ctx, account)
		if err != nil {
			return nil, err
		}
		rows, err := l.readTrace(ctx, account)
		if err != nil {
			return nil, err
		}
		snapshot[fmt.Sprintf("account.%s.balance", account)] = balance
		snapshot[fmt.Sprintf("account.%s.entry_count", account)] = len(rows)
	}
	return snapshot, nil
}

// --- driving-port calls ----------------------------------------------------

// OpenAccount opens an account of the given kind through the real front door.
func (l *Ledger) OpenAccount(ctx context.Context, name AccountName, kind AccountKind) error {
	l.accountKind[name] = kind
	body := map[string]any{"account_id": string(name), "type": string(kind)}
	answer, err := l.call(ctx, http.MethodPost, "/accounts", body, NoIdempotencyKey)
	l.lastAnswer = answer
	return err
}

// SubmitTransfer moves value between two accounts under a caller-owned key.
func (l *Ledger) SubmitTransfer(ctx context.Context, transfer Transfer) error {
	l.priorAnswer = l.lastAnswer
	l.lastRequest = transfer
	body := map[string]any{
		"from":   string(transfer.From),
		"to":     string(transfer.To),
		"amount": transfer.Amount.String(),
	}
	answer, err := l.call(ctx, http.MethodPost, "/transfers", body, transfer.Key)
	l.lastAnswer = answer
	return err
}

// RepeatLastRequest resubmits the previous transfer verbatim under the given
// key — the retry the whole of slice 03 exists for.
func (l *Ledger) RepeatLastRequest(ctx context.Context, key IdempotencyKey) error {
	return l.SubmitTransfer(ctx, Transfer{
		From:   l.lastRequest.From,
		To:     l.lastRequest.To,
		Amount: l.lastRequest.Amount,
		Key:    key,
	})
}

// RepeatLastRequestRewritten resubmits the previous transfer with its fields
// reordered and respaced. The fingerprint is computed over canonical JSON
// (ADR-005), so this must NOT read as a different request.
func (l *Ledger) RepeatLastRequestRewritten(ctx context.Context, key IdempotencyKey) error {
	l.priorAnswer = l.lastAnswer
	raw := fmt.Sprintf("{\n  \"amount\" :  %q ,\n\t\"to\":%q,\n \"from\" : %q\n}",
		l.lastRequest.Amount.String(), string(l.lastRequest.To), string(l.lastRequest.From))
	answer, err := l.callRaw(ctx, http.MethodPost, "/transfers", []byte(raw), key)
	l.lastAnswer = answer
	return err
}

// AskWhetherBooksBalance reads the operator's verdict from one of the two
// surfaces that must agree.
func (l *Ledger) AskWhetherBooksBalance(ctx context.Context, surface Surface) error {
	report, err := l.readBooks(ctx, surface)
	if err != nil {
		return err
	}
	l.otherReport = l.lastReport
	l.lastReport = report
	return nil
}

// Trace reads an account's ordered history with its running balance.
func (l *Ledger) Trace(ctx context.Context, account AccountName) error {
	rows, err := l.readTrace(ctx, account)
	if err != nil {
		return err
	}
	l.priorTrace = l.lastTrace
	l.lastTrace = rows
	return nil
}

// FollowDriftListing takes the route the operator actually takes: from the
// drift listing on the verdict into that account's entries.
func (l *Ledger) FollowDriftListing(ctx context.Context, account AccountName) error {
	if err := l.requireDrifted(account); err != nil {
		return err
	}
	return l.Trace(ctx, account)
}

func (l *Ledger) requireDrifted(account AccountName) error {
	for _, row := range l.lastReport.Drifted {
		if row.Account == account {
			return nil
		}
	}
	return fmt.Errorf("the drift listing does not offer %q, so the operator has nothing to follow", account)
}

// --- what the suite may never do for real ---------------------------------

// ActAs selects the credentials subsequent calls present.
func (l *Ledger) ActAs(credentials Credentials) {
	l.actingAs = credentials
}

// AttemptTamper tries to rewrite recorded history as the given role. It is the
// only place the suite reaches past the driving ports, and only because D7's
// guarantee is precisely that this must fail for the application role.
func (l *Ledger) AttemptTamper(ctx context.Context, as Credentials, action TamperAction) error {
	l.lastTamper = postgres.AttemptOutOfBandChange(ctx, l.dsnFor(as), string(action), nil)
	return nil
}

// CorruptEntry alters one recorded entry as the privileged role, which is the
// only role that can — that is what makes the slice-04 demo honest.
func (l *Ledger) CorruptEntry(ctx context.Context, account AccountName, ordinal int, by Money) error {
	l.tamperedRow, l.tamperedBy = ordinal, by
	return postgres.AttemptOutOfBandChange(ctx, l.privilegedDSN, string(AlterEntry), map[string]any{
		"account_id": string(account),
		"ordinal":    ordinal,
		"delta":      int64(by),
	})
}

// CorruptStoredBalance alters one stored balance as the privileged role.
func (l *Ledger) CorruptStoredBalance(ctx context.Context, account AccountName, by Money) error {
	l.tamperedBy = by
	return postgres.AttemptOutOfBandChange(ctx, l.privilegedDSN, string(AlterStoredBalance), map[string]any{
		"account_id": string(account),
		"delta":      int64(by),
	})
}

func (l *Ledger) dsnFor(as Credentials) string {
	if as == PrivilegedRole {
		return l.privilegedDSN
	}
	return l.appDSN
}

// --- transport -------------------------------------------------------------

func (l *Ledger) call(ctx context.Context, method, path string, body any, key IdempotencyKey) (Answer, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return Answer{}, err
	}
	return l.callRaw(ctx, method, path, encoded, key)
}

func (l *Ledger) callRaw(ctx context.Context, method, path string, body []byte, key IdempotencyKey) (Answer, error) {
	request, err := http.NewRequestWithContext(ctx, method, l.server.URL+path, bytes.NewReader(body))
	if err != nil {
		return Answer{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	l.authenticate(request)
	if key != NoIdempotencyKey && key != "" {
		request.Header.Set("Idempotency-Key", string(key))
	}

	response, err := l.client.Do(request)
	if err != nil {
		return Answer{}, err
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return Answer{}, err
	}
	return decodeAnswer(response.StatusCode, raw), nil
}

func (l *Ledger) authenticate(request *http.Request) {
	switch l.actingAs {
	case NoKey:
		return
	case UnissuedKey:
		request.Header.Set("Authorization", "Bearer never-issued-key")
	default:
		request.Header.Set("Authorization", "Bearer "+l.operatorKey)
	}
}
