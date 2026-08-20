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
// constraint. Conflict handling (replay vs. key-reuse-with-different-content)
// is a later step's job (04-xx); this step surfaces the constraint violation
// as an error and goes no further.
func (s idempotencyStore) Claim(ctx context.Context, key, fingerprint, transactionID string) (ports.Claim, error) {
	if _, err := s.tx.Exec(ctx,
		`INSERT INTO idempotency_keys (key, fingerprint, transaction_id) VALUES ($1, $2, $3)`,
		key, fingerprint, transactionID); err != nil {
		return ports.Claim{}, fmt.Errorf("claiming idempotency key %q: %w", key, err)
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
