package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"ledgerops/internal/app/ports"
	"ledgerops/internal/domain"
)

// transferStateRepository is the real ports.TransferStateRepository, scoped
// to one unit of work's transaction. Unlike every other repository in this
// package, transfer_state is coordinator process state, not a ledger fact
// (ADR-015) — hence the UPDATE grant migration 02-01 documents — but it
// still shares this unit of work's *pgx.Tx exactly like the other six
// repositories, which is what makes ClaimOne's read-then-extend atomic.
type transferStateRepository struct {
	tx pgx.Tx
}

var _ ports.TransferStateRepository = transferStateRepository{}

// Create inserts the initial row exactly as handed — next_attempt_at must
// already be set by the caller (ADR-015 Amendment); this adapter performs
// no defaulting of its own.
func (r transferStateRepository) Create(ctx context.Context, state ports.TransferState) error {
	_, err := r.tx.Exec(ctx,
		`INSERT INTO transfer_state
		   (transfer_id, tenant_id, idempotency_key, status, leg1_status, leg2_status, leg3_status,
		    leg1_attempts, leg2_attempts, leg3_attempts, next_attempt_at, reason,
		    counterparty_tenant_id, target_account_id, amount_minor, currency)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		state.TransferID, state.TenantID, state.IdempotencyKey, state.Status,
		state.Leg1Status, state.Leg2Status, state.Leg3Status,
		state.Leg1Attempts, state.Leg2Attempts, state.Leg3Attempts,
		state.NextAttemptAt, toNullableReason(state.Reason),
		toNullableString(state.CounterpartyTenantID), toNullableString(state.TargetAccountID),
		state.Amount.MinorUnits(), toNullableString(state.Amount.Currency()))
	if err != nil {
		return fmt.Errorf("creating transfer state %q: %w", state.TransferID, err)
	}
	return nil
}

// Get is the read-only driving surface for GetTransfer (a later step). It
// takes no lock and offers no companion write on this type — Core
// Principle 12's read/write split lives at the port-signature level
// (TransferStateRepository declares Get and UpdateStatus/ClaimOne as
// separate methods; a caller wired only to Get cannot reach either write).
// An absent transfer_id answers (zero value, false, nil), the expected
// shape of "no".
func (r transferStateRepository) Get(ctx context.Context, transferID string) (ports.TransferState, bool, error) {
	row := r.tx.QueryRow(ctx,
		`SELECT transfer_id, tenant_id, idempotency_key, status, leg1_status, leg2_status, leg3_status,
		        leg1_attempts, leg2_attempts, leg3_attempts, next_attempt_at, reason,
		        counterparty_tenant_id, target_account_id, amount_minor, currency
		 FROM transfer_state WHERE transfer_id = $1`, transferID)

	state, err := scanTransferState(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.TransferState{}, false, nil
	}
	if err != nil {
		return ports.TransferState{}, false, fmt.Errorf("reading transfer state %q: %w", transferID, err)
	}
	return state, true, nil
}

// UpdateStatus transitions the named transfer's top-level status (and
// records or clears its terminal-state reason) in place — the coordinator's
// own lifecycle write, distinct from ClaimOne's lease extension. Naming a
// transfer_id absent from the table is an infrastructure error: no caller
// today updates a transfer this repository did not itself Create first.
func (r transferStateRepository) UpdateStatus(ctx context.Context, transferID string, status string, reason string) error {
	tag, err := r.tx.Exec(ctx,
		`UPDATE transfer_state SET status = $1, reason = $2 WHERE transfer_id = $3`,
		status, toNullableReason(reason), transferID)
	if err != nil {
		return fmt.Errorf("updating status for transfer %q: %w", transferID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("updating status for transfer %q: no matching row", transferID)
	}
	return nil
}

// AdvanceAfterLegOutcome is attemptLeg's own "second write" (brief.md §
// Retry and reversal mechanics), overwriting whatever ClaimOne's lease left
// next_attempt_at holding — status and next_attempt_at move together, in
// one UPDATE, since they describe the same attempt outcome.
func (r transferStateRepository) AdvanceAfterLegOutcome(ctx context.Context, transferID string, status string, nextAttemptAt time.Time, reason string) error {
	tag, err := r.tx.Exec(ctx,
		`UPDATE transfer_state SET status = $1, next_attempt_at = $2, reason = $3 WHERE transfer_id = $4`,
		status, nextAttemptAt, toNullableReason(reason), transferID)
	if err != nil {
		return fmt.Errorf("advancing transfer %q after a leg outcome: %w", transferID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("advancing transfer %q after a leg outcome: no matching row", transferID)
	}
	return nil
}

// ClaimOne is the ONLY claim-granting call (ADR-015 Amendment 2). It locks
// the named row with SELECT ... FOR UPDATE — the same row-locking idiom
// AccountRepository.lockOne already uses (accounts.go) — inside this unit
// of work's own transaction, checks eligibility (status pending/retrying
// and next_attempt_at already due), and only if eligible extends
// next_attempt_at by leaseDuration before returning ok=true.
//
// The concurrent-claim race this closes: two callers racing ClaimOne on the
// identical transfer_id each open their own unit of work (their own *pgx.Tx).
// The first caller's SELECT ... FOR UPDATE acquires the row lock; the
// second's identical SELECT blocks until the first either commits or rolls
// back. Once the first has committed its lease extension, the second's
// SELECT unblocks and reads the now-extended (future) next_attempt_at —
// eligibility fails, and it returns (zero value, false, nil), never an
// error. A losing claim race is a normal outcome, not a failure.
func (r transferStateRepository) ClaimOne(ctx context.Context, transferID string, leaseDuration time.Duration) (ports.TransferState, bool, error) {
	row := r.tx.QueryRow(ctx,
		`SELECT transfer_id, tenant_id, idempotency_key, status, leg1_status, leg2_status, leg3_status,
		        leg1_attempts, leg2_attempts, leg3_attempts, next_attempt_at, reason,
		        counterparty_tenant_id, target_account_id, amount_minor, currency
		 FROM transfer_state WHERE transfer_id = $1 FOR UPDATE`, transferID)

	state, err := scanTransferState(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.TransferState{}, false, nil
	}
	if err != nil {
		return ports.TransferState{}, false, fmt.Errorf("locking transfer %q to claim it: %w", transferID, err)
	}

	if !transferStateIsDue(state) {
		return ports.TransferState{}, false, nil
	}

	extendedNextAttempt := time.Now().UTC().Add(leaseDuration)
	if _, err := r.tx.Exec(ctx,
		`UPDATE transfer_state SET next_attempt_at = $1 WHERE transfer_id = $2`,
		extendedNextAttempt, transferID); err != nil {
		return ports.TransferState{}, false, fmt.Errorf("extending the claim lease for transfer %q: %w", transferID, err)
	}

	state.NextAttemptAt = extendedNextAttempt
	return state, true, nil
}

// transferStateIsDue reports whether a locked row is eligible for
// ClaimOne — status IN ('pending', 'retrying') and next_attempt_at has
// already passed. Named apart from ClaimOne's own body so the predicate
// reads as one thing, matching this project's "small composable functions"
// convention (dedupedSorted in accounts.go is the same shape one file
// over).
func transferStateIsDue(state ports.TransferState) bool {
	dueStatus := state.Status == "pending" || state.Status == "retrying"
	return dueStatus && !state.NextAttemptAt.After(time.Now().UTC())
}

// ClaimDue is read-only discovery, never a claim and never a write — that
// split (ADR-015 Amendment 2, "ClaimDue split and batch-capped") is what
// keeps the whole scheme race-free: both the inline goroutine and the
// ticker's dispatch loop discover candidates here, then converge on
// ClaimOne's own atomicity regardless of how a row was found. now is
// caller-supplied (not server-side now()) so a scenario can inject a
// fixed clock deterministically.
func (r transferStateRepository) ClaimDue(ctx context.Context, now time.Time, batchLimit int) ([]string, error) {
	rows, err := r.tx.Query(ctx,
		`SELECT transfer_id FROM transfer_state
		 WHERE status IN ('pending', 'retrying') AND next_attempt_at <= $1
		 ORDER BY next_attempt_at ASC
		 LIMIT $2`,
		now, batchLimit)
	if err != nil {
		return nil, fmt.Errorf("discovering due transfers: %w", err)
	}
	defer rows.Close()

	var transferIDs []string
	for rows.Next() {
		var transferID string
		if err := rows.Scan(&transferID); err != nil {
			return nil, fmt.Errorf("discovering due transfers: %w", err)
		}
		transferIDs = append(transferIDs, transferID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("discovering due transfers: %w", err)
	}
	return transferIDs, nil
}

// scanTransferState is the shared column-scan behind Get and ClaimOne —
// both select the identical twelve columns in the identical order, so the
// scan itself is factored out rather than repeated (mirrors
// scanTenantLinkRow's own reuse shape in tenant_links.go). reason is
// nullable (migration 02-01): absent while a transfer is still in flight.
func scanTransferState(row pgx.Row) (ports.TransferState, error) {
	var (
		state                ports.TransferState
		reason               sql.NullString
		counterpartyTenantID sql.NullString
		targetAccountID      sql.NullString
		amountMinor          sql.NullInt64
		currency             sql.NullString
	)
	err := row.Scan(
		&state.TransferID, &state.TenantID, &state.IdempotencyKey, &state.Status,
		&state.Leg1Status, &state.Leg2Status, &state.Leg3Status,
		&state.Leg1Attempts, &state.Leg2Attempts, &state.Leg3Attempts,
		&state.NextAttemptAt, &reason,
		&counterpartyTenantID, &targetAccountID, &amountMinor, &currency)
	if err != nil {
		return ports.TransferState{}, err
	}
	state.Reason = reason.String
	state.CounterpartyTenantID = counterpartyTenantID.String
	state.TargetAccountID = targetAccountID.String
	if currency.Valid {
		amount, err := domain.NewMoney(amountMinor.Int64, currency.String)
		if err != nil {
			return ports.TransferState{}, fmt.Errorf("stored transfer amount carries an unrecognised currency %q: %w", currency.String, err)
		}
		state.Amount = amount
	}
	return state, nil
}

// toNullableReason maps the port's plain string ("" while in flight) onto
// the nullable text column migration 02-01 declares.
func toNullableReason(reason string) any {
	return toNullableString(reason)
}

// toNullableString maps any port-level "" (not-yet-known) string field onto
// SQL NULL rather than an empty-string literal. This matters beyond mere
// column hygiene for counterparty_tenant_id specifically: migration 0006
// gives it a REFERENCES tenants (tenant_id) foreign key, and an empty
// string is a non-NULL value the FK constraint would check against (and
// reject, since no tenant is ever named "") — only NULL is exempt from FK
// validation. Every TransferState this coordinator itself ever creates
// populates these fields, but callers/fixtures built before migration 0006
// added them (e.g. transfer_state_test.go's pre-existing fixture) leave
// them at Go's zero value, which must still round-trip as "unknown", not
// fail the write outright.
func toNullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
