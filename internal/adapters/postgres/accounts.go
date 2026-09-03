package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

// accountRepository is the real ports.AccountRepository, scoped to one unit
// of work's transaction.
type accountRepository struct {
	tx pgx.Tx
}

var _ ports.AccountRepository = accountRepository{}

// LockForUpdate acquires a row lock on each touched account in ascending
// account-id order (DDD-6), one SELECT ... FOR UPDATE per id, issued in
// sorted order — deterministic regardless of the order the caller passed
// ids in, which is the concurrency-safety mechanism I4 depends on: two
// opposing-direction transfers between the same pair of accounts always
// request their locks in the same order, so they queue rather than deadlock.
// The ordering is scoped within the given tenant's own account set — narrower
// than before (I9 rescoping), but deterministic the same way.
//
// An id absent from the ledger, or bound to a different tenant, is silently
// omitted from the result rather than treated as an error: the pure core
// (domain.Post) is what decides UnknownAccount, from the gap between what
// was asked for and what the snapshots contain. Locking is an effect and has
// no opinion on domain rules.
func (r accountRepository) LockForUpdate(ctx context.Context, tenantID string, accountIDs []string) ([]domain.Account, error) {
	sorted := dedupedSorted(accountIDs)

	accounts := make([]domain.Account, 0, len(sorted))
	for _, id := range sorted {
		account, found, err := r.lockOne(ctx, tenantID, id)
		if err != nil {
			return nil, fmt.Errorf("locking account %q: %w", id, err)
		}
		if found {
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}

func (r accountRepository) lockOne(ctx context.Context, tenantID, id string) (domain.Account, bool, error) {
	row := r.tx.QueryRow(ctx,
		`SELECT id, kind, balance_minor, currency FROM accounts WHERE tenant_id = $1 AND id = $2 FOR UPDATE`, tenantID, id)
	account, err := scanAccount(row, tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, false, nil
	}
	if err != nil {
		return domain.Account{}, false, err
	}
	return account, true, nil
}

// ApplyDeltas writes each balance delta the pure core produced, scoped to the
// given tenant's own rows. The currency is asserted in the WHERE clause as a
// defence-in-depth check: a mismatch there means either a bug upstream or a
// row was renamed under the transaction, and either way the update must
// affect nothing rather than silently apply.
func (r accountRepository) ApplyDeltas(ctx context.Context, tenantID string, deltas []domain.BalanceDelta) error {
	for _, delta := range deltas {
		tag, err := r.tx.Exec(ctx,
			`UPDATE accounts SET balance_minor = balance_minor + $1 WHERE tenant_id = $2 AND id = $3 AND currency = $4`,
			delta.Delta.MinorUnits(), tenantID, delta.AccountID, delta.Delta.Currency())
		if err != nil {
			return fmt.Errorf("applying delta to account %q: %w", delta.AccountID, err)
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("applying delta to account %q: no matching row (wrong id, tenant, or currency)", delta.AccountID)
		}
	}
	return nil
}

// Create persists a newly opened account under the given tenant. Account-name
// uniqueness (I9/DDD-18) is enforced by the composite (tenant_id, id) primary
// key (migration 0003) — two different tenants may independently create an
// account with the same id.
func (r accountRepository) Create(ctx context.Context, tenantID string, account domain.Account) error {
	_, err := r.tx.Exec(ctx,
		`INSERT INTO accounts (tenant_id, id, kind, balance_minor, currency) VALUES ($1, $2, $3, $4, $5)`,
		tenantID, account.ID(), string(account.Kind()), account.Balance().MinorUnits(), account.Balance().Currency())
	if err != nil {
		return fmt.Errorf("creating account %q: %w", account.ID(), err)
	}
	return nil
}

// Get reads one account's stored state without locking it, scoped to the
// given tenant — a query for tenant A's account never returns tenant B's
// row even if their ids collide.
func (r accountRepository) Get(ctx context.Context, tenantID string, accountID string) (domain.Account, error) {
	row := r.tx.QueryRow(ctx,
		`SELECT id, kind, balance_minor, currency FROM accounts WHERE tenant_id = $1 AND id = $2`, tenantID, accountID)
	account, err := scanAccount(row, tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, domain.NewUnknownAccount(accountID)
	}
	if err != nil {
		return domain.Account{}, fmt.Errorf("reading account %q: %w", accountID, err)
	}
	return account, nil
}

// All enumerates every account belonging to the given tenant, ordered by id,
// without locking any of them — VerifyBooks' full scan is a read-only
// comparison against ComputedBalances, not a write path, so it takes no row
// locks (D9).
func (r accountRepository) All(ctx context.Context, tenantID string) ([]domain.Account, error) {
	rows, err := r.tx.Query(ctx,
		`SELECT id, kind, balance_minor, currency FROM accounts WHERE tenant_id = $1 ORDER BY id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("reading every account: %w", err)
	}
	defer rows.Close()

	var accounts []domain.Account
	for rows.Next() {
		account, err := scanAccount(rows, tenantID)
		if err != nil {
			return nil, fmt.Errorf("reading every account: %w", err)
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading every account: %w", err)
	}
	return accounts, nil
}

// scanAccount reconstructs one domain.Account from a row already scoped to
// tenantID by its caller's own WHERE clause — the column itself is not
// re-selected, since the caller already knows which tenant it asked for.
func scanAccount(row pgx.Row, tenantID string) (domain.Account, error) {
	var (
		id, kind, currency string
		balanceMinor       int64
	)
	if err := row.Scan(&id, &kind, &balanceMinor, &currency); err != nil {
		return domain.Account{}, err
	}
	balance, err := domain.NewMoney(balanceMinor, currency)
	if err != nil {
		return domain.Account{}, fmt.Errorf("stored balance for %q carries an unrecognised currency %q: %w", id, currency, err)
	}
	account, err := domain.NewAccount(tenantID, id, domain.AccountKind(kind), balance)
	if err != nil {
		return domain.Account{}, fmt.Errorf("stored row for %q violates domain invariants: %w", id, err)
	}
	return account, nil
}

// dedupedSorted returns ids in ascending order with duplicates removed. A
// transfer between the same account twice, or a caller passing the same id
// under two different calls, must still resolve to one lock request.
func dedupedSorted(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
