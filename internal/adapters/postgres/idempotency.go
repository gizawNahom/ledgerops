package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"ledgerops/internal/app/ports"
)

// idempotencyStore is the real ports.IdempotencyStore, scoped to one unit of
// work's transaction. The claim and the posting it guards commit together —
// same transaction, same fate — so there is no window in which a
// transaction is recorded without its key, or a key without its transaction
// (ADR-005).
type idempotencyStore struct {
	tx pgx.Tx
}

var _ ports.IdempotencyStore = idempotencyStore{}

// Claim records the key, the request fingerprint, and the transaction id it
// produced. The primary key on `key` (migration 1) is what makes the
// uniqueness guarantee (I7) hold under concurrency — not this code, the
// constraint.
//
// ON CONFLICT (key) DO NOTHING rather than a bare INSERT: two callers racing
// to claim the same key both reach this statement inside their own,
// still-uncommitted unit of work. PostgreSQL resolves the race by making the
// later INSERT wait for the earlier one's transaction to finish before
// deciding whether it conflicts — so by the time this call returns zero rows
// affected, the winner's row is guaranteed committed and visible, not merely
// inserted (I7). DO NOTHING also means the loser's own transaction is never
// put into PostgreSQL's aborted-transaction state the way a bare INSERT's
// unique-violation would, which matters because the loser's caller still
// needs this same *pgx.Tx usable long enough to roll back cleanly — see
// ErrIdempotencyKeyClaimConflict.
func (s idempotencyStore) Claim(ctx context.Context, key, fingerprint, transactionID string) (ports.Claim, error) {
	tag, err := s.tx.Exec(ctx,
		`INSERT INTO idempotency_keys (key, fingerprint, transaction_id) VALUES ($1, $2, $3)
		 ON CONFLICT (key) DO NOTHING`,
		key, fingerprint, transactionID)
	if err != nil {
		return ports.Claim{}, fmt.Errorf("claiming idempotency key %q: %w", key, err)
	}
	if tag.RowsAffected() == 0 {
		return ports.Claim{}, ports.ErrIdempotencyKeyClaimConflict
	}
	return ports.Claim{Key: key, Fingerprint: fingerprint, TransactionID: transactionID}, nil
}

// Lookup reads a previously stored claim, if one exists.
func (s idempotencyStore) Lookup(ctx context.Context, key string) (ports.Claim, bool, error) {
	var fingerprint, transactionID string
	row := s.tx.QueryRow(ctx,
		`SELECT fingerprint, transaction_id FROM idempotency_keys WHERE key = $1`, key)
	if err := row.Scan(&fingerprint, &transactionID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ports.Claim{}, false, nil
		}
		return ports.Claim{}, false, fmt.Errorf("looking up idempotency key %q: %w", key, err)
	}
	return ports.Claim{Key: key, Fingerprint: fingerprint, TransactionID: transactionID}, true, nil
}
