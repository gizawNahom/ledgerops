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
//
// An id absent from the ledger is silently omitted from the result rather
// than treated as an error: the pure core (domain.Post) is what decides
// UnknownAccount, from the gap between what was asked for and what the
// snapshots contain. Locking is an effect and has no opinion on domain rules.
func (r accountRepository) LockForUpdate(ctx context.Context, accountIDs []string) ([]domain.Account, error) {
	sorted := dedupedSorted(accountIDs)

	accounts := make([]domain.Account, 0, len(sorted))
	for _, id := range sorted {
		account, found, err := r.lockOne(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("locking account %q: %w", id, err)
		}
		if found {
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}

func (r accountRepository) lockOne(ctx context.Context, id string) (domain.Account, bool, error) {
	row := r.tx.QueryRow(ctx,
		`SELECT id, kind, balance_minor, currency FROM accounts WHERE id = $1 FOR UPDATE`, id)
	account, err := scanAccount(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, false, nil
	}
	if err != nil {
		return domain.Account{}, false, err
	}
	return account, true, nil
}

// ApplyDeltas writes each balance delta the pure core produced. The currency
// is asserted in the WHERE clause as a defence-in-depth check: a mismatch
// there means either a bug upstream or a row was renamed under the
// transaction, and either way the update must affect nothing rather than
// silently apply.
func (r accountRepository) ApplyDeltas(ctx context.Context, deltas []domain.BalanceDelta) error {
	for _, delta := range deltas {
		tag, err := r.tx.Exec(ctx,
			`UPDATE accounts SET balance_minor = balance_minor + $1 WHERE id = $2 AND currency = $3`,
			delta.Delta.MinorUnits(), delta.AccountID, delta.Delta.Currency())
		if err != nil {
			return fmt.Errorf("applying delta to account %q: %w", delta.AccountID, err)
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("applying delta to account %q: no matching row (wrong id or currency)", delta.AccountID)
		}
	}
	return nil
}

// Create persists a newly opened account.
func (r accountRepository) Create(ctx context.Context, account domain.Account) error {
	_, err := r.tx.Exec(ctx,
		`INSERT INTO accounts (id, kind, balance_minor, currency) VALUES ($1, $2, $3, $4)`,
		account.ID(), string(account.Kind()), account.Balance().MinorUnits(), account.Balance().Currency())
	if err != nil {
		return fmt.Errorf("creating account %q: %w", account.ID(), err)
	}
	return nil
}

// Get reads one account's stored state without locking it.
func (r accountRepository) Get(ctx context.Context, accountID string) (domain.Account, error) {
	row := r.tx.QueryRow(ctx,
		`SELECT id, kind, balance_minor, currency FROM accounts WHERE id = $1`, accountID)
	account, err := scanAccount(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Account{}, domain.NewUnknownAccount(accountID)
	}
	if err != nil {
		return domain.Account{}, fmt.Errorf("reading account %q: %w", accountID, err)
	}
	return account, nil
}

func scanAccount(row pgx.Row) (domain.Account, error) {
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
	account, err := domain.NewAccount(id, domain.AccountKind(kind), balance)
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
