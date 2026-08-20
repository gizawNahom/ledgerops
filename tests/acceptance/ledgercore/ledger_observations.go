package ledgercore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"ledgerops/internal/adapters/postgres"
)

// NewLedger builds the composition root with the two fakes the infrastructure
// policy permits, and nothing else faked.
func NewLedger() *Ledger {
	fixed := []string{"txn_0001", "txn_0002", "txn_0003", "txn_0004", "txn_0005"}
	issued := 0
	return &Ledger{
		operatorKey: "test-operator-key",
		actingAs:    ApplicationRole,
		accountKind: map[AccountName]AccountKind{},
		fixedIDs:    fixed,
		clock:       func() time.Time { return time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC) },
		nextID: func() string {
			issued++
			return fmt.Sprintf("txn_%04d", issued)
		},
	}
}

// FixClockAt pins the injected clock. Function-typed port (DDD-13), so the fake
// is a literal — see the infrastructure policy for what it cannot model.
func (l *Ledger) FixClockAt(stamp string) error {
	at, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return fmt.Errorf("clock %q is not an RFC3339 instant: %w", stamp, err)
	}
	l.clock = func() time.Time { return at }
	return nil
}

// FixNextIdentifier pins the next transaction id the generator hands out.
func (l *Ledger) FixNextIdentifier(id string) {
	handed := false
	l.nextID = func() string {
		if !handed {
			handed = true
			return id
		}
		return id + "-next"
	}
}

// --- containers ------------------------------------------------------------

var containerMu sync.Mutex

// startPostgres brings up PostgreSQL 16 per test package via Testcontainers
// (OPS-11) and returns the application and privileged DSNs (OPS-10). A fresh
// instance per package satisfies the `contended` and `corrupted` preconditions
// structurally: nothing is inherited from a prior test and no pooler can sit in
// front of it.
func startPostgres(ctx context.Context) (appDSN string, privilegedDSN string, err error) {
	containerMu.Lock()
	defer containerMu.Unlock()

	container, err := tcpostgres.Run(ctx,
		"postgres:16",
		tcpostgres.WithDatabase("ledgerops"),
		tcpostgres.WithUsername("ledgerops_migrate"),
		tcpostgres.WithPassword("migrate-secret"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		return "", "", err
	}
	privilegedDSN, err = container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return "", "", err
	}
	parsed, err := url.Parse(privilegedDSN)
	if err != nil {
		return "", "", err
	}
	parsed.User = url.UserPassword("ledgerops_app", "app-secret")
	return parsed.String(), privilegedDSN, nil
}

// --- reads through the driving ports --------------------------------------

// readBalance is a Then-side verification read. It authenticates as the
// legitimate operator explicitly rather than inheriting l.actingAs, because a
// preceding When step may have deliberately poisoned actingAs to prove a
// refusal (e.g. "the caller presents no operator key") — the follow-on
// balance check must still be able to observe that nothing moved. This does
// not apply to readTrace/readBooks: those are also invoked as When-side
// actions (Trace, AskWhetherBooksBalance) where inheriting a bad actingAs is
// the scenario under test (see milestone-04/milestone-05 "unidentified
// caller" scenarios).
func (l *Ledger) readBalance(ctx context.Context, account AccountName) (Money, error) {
	answer, err := l.callAs(ctx, ApplicationRole, http.MethodGet, "/accounts/"+url.PathEscape(string(account)), nil, NoIdempotencyKey)
	if err != nil {
		return 0, err
	}
	var payload struct {
		Balance string `json:"balance"`
	}
	if err := json.Unmarshal([]byte(answer.Raw), &payload); err != nil {
		return 0, fmt.Errorf("reading the balance of %q: %w (answer was %s)", account, err, answer.Raw)
	}
	return ParseMoney(payload.Balance), nil
}

func (l *Ledger) readBooks(ctx context.Context, surface Surface) (BooksReport, error) {
	path := "/health/trial-balance"
	if surface == ConsoleSurface {
		path = "/console/verdict"
	}
	answer, err := l.call(ctx, http.MethodGet, path, nil, NoIdempotencyKey)
	if err != nil {
		return BooksReport{}, err
	}
	var payload struct {
		Verdict         string `json:"verdict"`
		ImbalanceMinor  int64  `json:"imbalance_minor"`
		EntryCount      int    `json:"entry_count"`
		ElapsedMillis   int    `json:"elapsed_ms"`
		VerdictPosition int    `json:"verdict_position"`
		FigurePosition  int    `json:"first_figure_position"`
		Drifted         []struct {
			AccountID string `json:"account_id"`
			Stored    string `json:"stored"`
			Computed  string `json:"computed"`
			Delta     string `json:"delta"`
		} `json:"drifted"`
	}
	if err := json.Unmarshal([]byte(answer.Raw), &payload); err != nil {
		return BooksReport{}, fmt.Errorf("reading the verdict from the %s surface: %w (answer was %s)", surface, err, answer.Raw)
	}
	report := BooksReport{
		Verdict:           Verdict(payload.Verdict),
		TrialBalance:      Money(payload.ImbalanceMinor),
		EntryCount:        payload.EntryCount,
		ElapsedMillis:     payload.ElapsedMillis,
		VerdictStatedAt:   payload.VerdictPosition,
		FirstFigureStated: payload.FigurePosition,
	}
	for _, row := range payload.Drifted {
		report.Drifted = append(report.Drifted, DriftRow{
			Account:  AccountName(row.AccountID),
			Stored:   ParseMoney(row.Stored),
			Computed: ParseMoney(row.Computed),
			Delta:    ParseMoney(row.Delta),
		})
	}
	return report, nil
}

func (l *Ledger) readTrace(ctx context.Context, account AccountName) ([]TraceRow, error) {
	answer, err := l.call(ctx, http.MethodGet,
		"/accounts/"+url.PathEscape(string(account))+"/entries", nil, NoIdempotencyKey)
	if err != nil {
		return nil, err
	}
	if answer.Outcome == Refused {
		l.lastAnswer = answer
		return nil, nil
	}
	var payload struct {
		Entries []struct {
			TransactionID  string `json:"transaction_id"`
			Counterparty   string `json:"counterparty"`
			Amount         string `json:"amount"`
			RecordedAt     string `json:"recorded_at"`
			RunningBalance string `json:"running_balance"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(answer.Raw), &payload); err != nil {
		return nil, fmt.Errorf("tracing %q: %w (answer was %s)", account, err, answer.Raw)
	}
	rows := make([]TraceRow, 0, len(payload.Entries))
	for _, entry := range payload.Entries {
		rows = append(rows, TraceRow{
			TransactionID:  entry.TransactionID,
			Counterparty:   AccountName(entry.Counterparty),
			Amount:         ParseMoney(entry.Amount),
			RecordedAt:     entry.RecordedAt,
			RunningBalance: ParseMoney(entry.RunningBalance),
		})
	}
	return rows, nil
}

func decodeAnswer(status int, raw []byte) Answer {
	answer := Answer{Raw: string(raw)}
	var payload struct {
		TransactionID string `json:"transaction_id"`
		Status        string `json:"status"`
		Error         string `json:"error"`
		AccountID     string `json:"account_id"`
		Available     string `json:"available"`
		Requested     string `json:"requested"`
		Legs          []struct {
			Account string `json:"account"`
			Amount  string `json:"amount"`
		} `json:"legs"`
	}
	_ = json.Unmarshal(raw, &payload)

	switch {
	case status >= 400:
		answer.Outcome = Refused
		answer.Refusal = RefusalKind(payload.Error)
		answer.NamedAccount = AccountName(payload.AccountID)
		if payload.Available != "" {
			answer.Available = ParseMoney(payload.Available)
		}
		if payload.Requested != "" {
			answer.Requested = ParseMoney(payload.Requested)
		}
	case status == http.StatusOK:
		answer.Outcome = Replayed
	default:
		answer.Outcome = Accepted
	}
	answer.TransactionID = payload.TransactionID
	for _, leg := range payload.Legs {
		answer.Legs = append(answer.Legs, Leg{
			Account: AccountName(leg.Account),
			Amount:  ParseMoney(leg.Amount),
		})
	}
	return answer
}

// --- contended runs --------------------------------------------------------

// RaceSpenders fires n simultaneous transfers of the same amount and reports
// the denominators alongside the results. Both are asserted, because a harness
// that quietly ran ten iterations reports zero negatives and passes
// (kpi-contracts.yaml, KPI-2).
func (l *Ledger) RaceSpenders(ctx context.Context, n int, transfer Transfer, distinctKeys bool) error {
	report := RaceReport{}
	answers := make([]Answer, n)
	var wg sync.WaitGroup
	var mu sync.Mutex

	release := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := transfer.Key
			if distinctKeys {
				key = IdempotencyKey(fmt.Sprintf("%s-%d", transfer.Key, i))
			}
			<-release
			answer, err := l.call(ctx, http.MethodPost, "/transfers", map[string]any{
				"from":   string(transfer.From),
				"to":     string(transfer.To),
				"amount": transfer.Amount.String(),
			}, key)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				answers[i] = answer
			}
		}(i)
	}
	close(release)
	wg.Wait()

	distinct := map[string]bool{}
	for _, answer := range answers {
		report.Attempts++
		report.Answered++
		switch answer.Outcome {
		case Accepted, Replayed:
			report.Accepted++
			distinct[answer.TransactionID] = true
		case Refused:
			if answer.Refusal == InsufficientFunds {
				report.RefusedInsufficientFunds++
			}
			if answer.Refusal == "deadlock_detected" {
				report.Deadlocked++
			}
		}
	}
	report.DistinctTransactionIDs = len(distinct)
	pairs, err := postgres.CountEntryPairsForKey(ctx, l.appDSN, string(transfer.Key))
	if err == nil {
		report.EntryPairsStored = pairs
	}
	report.NegativeBalanceObservations, _ = postgres.CountNegativeWalletObservations(ctx, l.appDSN)
	l.lastRace = report
	l.lastAnswers = answers
	return nil
}

// RaceOpposingTransfers fires n movements each way between the same pair of
// accounts, which is the shape that deadlocks when lock ordering is wrong
// (DDD-6).
func (l *Ledger) RaceOpposingTransfers(ctx context.Context, n int, a, b AccountName, amount Money) error {
	if err := l.RaceSpenders(ctx, n, Transfer{From: a, To: b, Amount: amount, Key: "opposing-ab"}, true); err != nil {
		return err
	}
	forward := l.lastRace
	if err := l.RaceSpenders(ctx, n, Transfer{From: b, To: a, Amount: amount, Key: "opposing-ba"}, true); err != nil {
		return err
	}
	l.lastRace = RaceReport{
		Attempts:   forward.Attempts + l.lastRace.Attempts,
		Answered:   forward.Answered + l.lastRace.Answered,
		Accepted:   forward.Accepted + l.lastRace.Accepted,
		Deadlocked: forward.Deadlocked + l.lastRace.Deadlocked,
	}
	return nil
}
