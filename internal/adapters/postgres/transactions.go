package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

// transactionRepository is the real ports.TransactionRepository, scoped to
// one unit of work's transaction. It offers no update and no delete on
// entries — the append-only guarantee (D7) is enforced twice already, by the
// trigger and by the revoked grant (migration 0); this repository simply
// never attempts either verb.
type transactionRepository struct {
	tx pgx.Tx
}

var _ ports.TransactionRepository = transactionRepository{}

// Append writes the transaction row and every entry the pure core produced,
// inside the caller's transaction, scoped to the given tenant. Both the
// transaction and its entries land together or not at all — there is no
// partial posting.
func (r transactionRepository) Append(ctx context.Context, tenantID string, posting domain.Posting) error {
	if _, err := r.tx.Exec(ctx,
		`INSERT INTO transactions (tenant_id, id, recorded_at) VALUES ($1, $2, $3)`,
		tenantID, posting.Transaction.ID(), posting.Transaction.RecordedAt()); err != nil {
		return fmt.Errorf("appending transaction %q: %w", posting.Transaction.ID(), err)
	}

	for _, entry := range posting.Entries {
		if _, err := r.tx.Exec(ctx,
			`INSERT INTO entries (tenant_id, transaction_id, account_id, counterparty_id, amount_minor, currency, recorded_at, sequence)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			tenantID, posting.Transaction.ID(), entry.AccountID(), entry.Counterparty(),
			entry.Amount().MinorUnits(), entry.Amount().Currency(), entry.RecordedAt(), entry.Sequence()); err != nil {
			return fmt.Errorf("appending entry for account %q on transaction %q: %w",
				entry.AccountID(), posting.Transaction.ID(), err)
		}
	}
	return nil
}

// Get reconstructs one transaction and its entries, scoped to the given
// tenant. Deltas are not persisted separately (ADR-003 stores the balance,
// not the derivation), so a reconstructed Posting carries no Deltas —
// nothing this step's use cases need replays it that way yet.
func (r transactionRepository) Get(ctx context.Context, tenantID string, transactionID string) (domain.Posting, error) {
	var recordedAt time.Time
	row := r.tx.QueryRow(ctx, `SELECT recorded_at FROM transactions WHERE tenant_id = $1 AND id = $2`, tenantID, transactionID)
	if err := row.Scan(&recordedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Posting{}, fmt.Errorf("transaction %q not found", transactionID)
		}
		return domain.Posting{}, fmt.Errorf("reading transaction %q: %w", transactionID, err)
	}
	transaction, err := domain.NewTransaction(transactionID, recordedAt)
	if err != nil {
		return domain.Posting{}, err
	}

	entries, err := r.entriesForTransaction(ctx, tenantID, transactionID)
	if err != nil {
		return domain.Posting{}, err
	}

	return domain.Posting{Transaction: transaction, Entries: entries}, nil
}

func (r transactionRepository) entriesForTransaction(ctx context.Context, tenantID, transactionID string) ([]domain.Entry, error) {
	rows, err := r.tx.Query(ctx,
		`SELECT account_id, counterparty_id, amount_minor, currency, recorded_at, sequence
		 FROM entries WHERE tenant_id = $1 AND transaction_id = $2 ORDER BY sequence`, tenantID, transactionID)
	if err != nil {
		return nil, fmt.Errorf("reading entries for transaction %q: %w", transactionID, err)
	}
	defer rows.Close()
	return scanEntries(rows, transactionID)
}

// EntriesFor reads one account's ordered history — by recorded instant then
// by sequence, so two entries sharing a clock tick still settle in a
// deterministic order (US-5).
//
// scope decides whether the query is narrowed to one tenant's own entries
// (tenant_key callers, new) or left unscoped across every tenant (the
// existing OperatorKey/console call path, which must return byte-identical
// results to before this step — brief.md § Multitenancy Contract-shape
// table: this is the pure-function, return-only contract shape).
func (r transactionRepository) EntriesFor(ctx context.Context, scope ports.TenantScope, accountID string) ([]domain.Entry, error) {
	tenantID, scoped := scope.Resolve()
	var (
		rows pgx.Rows
		err  error
	)
	if scoped {
		rows, err = r.tx.Query(ctx,
			`SELECT transaction_id, counterparty_id, amount_minor, currency, recorded_at, sequence
			 FROM entries WHERE tenant_id = $1 AND account_id = $2 ORDER BY recorded_at, sequence`, tenantID, accountID)
	} else {
		rows, err = r.tx.Query(ctx,
			`SELECT transaction_id, counterparty_id, amount_minor, currency, recorded_at, sequence
			 FROM entries WHERE account_id = $1 ORDER BY recorded_at, sequence`, accountID)
	}
	if err != nil {
		return nil, fmt.Errorf("reading entries for account %q: %w", accountID, err)
	}
	defer rows.Close()
	return scanAccountEntries(rows, accountID)
}

// TrialBalance sums every entry, per currency, and reports the count
// alongside it (KPI-3's denominator needs both). scope narrows the sum to
// one tenant's own entries, or leaves it platform-wide when Unscoped() —
// the existing GET /health/trial-balance/GET /console/verdict contract,
// preserved byte-identical for the unscoped path.
func (r transactionRepository) TrialBalance(ctx context.Context, scope ports.TenantScope) (domain.Money, int, error) {
	var (
		sumMinor   int64
		count      int
		currency   string
		hasEntries bool
	)
	tenantID, scoped := scope.Resolve()
	var (
		rows pgx.Rows
		err  error
	)
	if scoped {
		rows, err = r.tx.Query(ctx, `SELECT currency, SUM(amount_minor), COUNT(*) FROM entries WHERE tenant_id = $1 GROUP BY currency`, tenantID)
	} else {
		rows, err = r.tx.Query(ctx, `SELECT currency, SUM(amount_minor), COUNT(*) FROM entries GROUP BY currency`)
	}
	if err != nil {
		return domain.Money{}, 0, fmt.Errorf("computing the trial balance: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var rowCurrency string
		var rowSum int64
		var rowCount int
		if err := rows.Scan(&rowCurrency, &rowSum, &rowCount); err != nil {
			return domain.Money{}, 0, fmt.Errorf("reading a trial-balance row: %w", err)
		}
		if !hasEntries {
			currency, sumMinor, hasEntries = rowCurrency, rowSum, true
		} else {
			sumMinor += rowSum
		}
		count += rowCount
	}
	if err := rows.Err(); err != nil {
		return domain.Money{}, 0, fmt.Errorf("computing the trial balance: %w", err)
	}
	if !hasEntries {
		currency = "USD"
	}
	total, err := domain.NewMoney(sumMinor, currency)
	if err != nil {
		return domain.Money{}, 0, err
	}
	return total, count, nil
}

// ComputedBalances derives every account's balance from its entries, grouped
// by account, for the operator's verdict to compare against stored balances
// (I3). scope narrows the derivation to one tenant, or leaves it
// platform-wide when Unscoped() — same contract-shape rationale as
// TrialBalance.
func (r transactionRepository) ComputedBalances(ctx context.Context, scope ports.TenantScope) (map[string]domain.Money, error) {
	tenantID, scoped := scope.Resolve()
	var (
		rows pgx.Rows
		err  error
	)
	if scoped {
		rows, err = r.tx.Query(ctx,
			`SELECT account_id, currency, SUM(amount_minor) FROM entries WHERE tenant_id = $1 GROUP BY account_id, currency`, tenantID)
	} else {
		rows, err = r.tx.Query(ctx,
			`SELECT account_id, currency, SUM(amount_minor) FROM entries GROUP BY account_id, currency`)
	}
	if err != nil {
		return nil, fmt.Errorf("computing balances from entries: %w", err)
	}
	defer rows.Close()

	balances := make(map[string]domain.Money)
	for rows.Next() {
		var accountID, currency string
		var sumMinor int64
		if err := rows.Scan(&accountID, &currency, &sumMinor); err != nil {
			return nil, fmt.Errorf("reading a computed-balance row: %w", err)
		}
		balance, err := domain.NewMoney(sumMinor, currency)
		if err != nil {
			return nil, err
		}
		balances[accountID] = balance
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("computing balances from entries: %w", err)
	}
	return balances, nil
}

// entryFromRow builds one domain.Entry from the six columns both scanEntries
// and scanAccountEntries read off `entries`. The two queries fix a different
// column (transaction_id vs account_id) and scan the other, but the
// construction from there — Money, then Entry — was identical in both; this
// is the one place that construction happens.
func entryFromRow(transactionID, accountID, counterparty, currency string, amountMinor, sequence int64, recordedAt time.Time) (domain.Entry, error) {
	amount, err := domain.NewMoney(amountMinor, currency)
	if err != nil {
		return domain.Entry{}, err
	}
	return domain.NewEntry(transactionID, accountID, counterparty, amount, recordedAt, sequence)
}

func scanEntries(rows pgx.Rows, transactionID string) ([]domain.Entry, error) {
	var entries []domain.Entry
	for rows.Next() {
		var (
			accountID, counterparty, currency string
			amountMinor, sequence             int64
			recordedAt                        time.Time
		)
		if err := rows.Scan(&accountID, &counterparty, &amountMinor, &currency, &recordedAt, &sequence); err != nil {
			return nil, fmt.Errorf("reading an entry for transaction %q: %w", transactionID, err)
		}
		entry, err := entryFromRow(transactionID, accountID, counterparty, currency, amountMinor, sequence, recordedAt)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading entries for transaction %q: %w", transactionID, err)
	}
	return entries, nil
}

func scanAccountEntries(rows pgx.Rows, accountID string) ([]domain.Entry, error) {
	var entries []domain.Entry
	for rows.Next() {
		var (
			transactionID, counterparty, currency string
			amountMinor, sequence                 int64
			recordedAt                            time.Time
		)
		if err := rows.Scan(&transactionID, &counterparty, &amountMinor, &currency, &recordedAt, &sequence); err != nil {
			return nil, fmt.Errorf("reading an entry for account %q: %w", accountID, err)
		}
		entry, err := entryFromRow(transactionID, accountID, counterparty, currency, amountMinor, sequence, recordedAt)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading entries for account %q: %w", accountID, err)
	}
	return entries, nil
}
