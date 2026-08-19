// Package postgres is the driven adapter. SQL stays visible — no ORM — because
// the locking is the part that matters and it must be readable (DDD-6).
//
// SCAFFOLD: true — created by DISTILL for Mandate 7 RED-readiness.
//
// Two functions here deliberately do NOT panic: Migrate and Open. The
// acceptance suite calls both while establishing a scenario's Given, and a
// panic there would fail the scenario in setup — classified BROKEN — instead of
// at its assertion. They no-op so that every scenario reaches its When, gets a
// 501 from the HTTP scaffold, and fails on its Then for the right reason.
// Everything a scenario's assertion actually depends on panics as usual.
package postgres

import (
	"context"
	"fmt"

	"ledgerops/internal/app/ports"
)

// Migrate applies the whole migration set as the privileged role. Migrations
// are expand-only: no migration may DELETE from or drop the entry table, and
// every schema change must leave the previous binary able to run against the
// new schema (brief.md § Deployment shape).
//
// SCAFFOLD no-op — see the package comment for why this one does not panic.
func Migrate(ctx context.Context, privilegedDSN string) error {
	return nil
}

// MigrateStep applies only the newest schema change, which is how a scenario
// exercises a migration running over history it may not rewrite.
//
// SCAFFOLD no-op — see the package comment.
func MigrateStep(ctx context.Context, privilegedDSN string) error {
	return nil
}

// MigrationStatements returns every migration's SQL by name, so a scenario can
// assert the expand-only rule across the whole set rather than only the newest.
func MigrationStatements() (map[string]string, error) {
	return map[string]string{}, nil
}

// Open connects as the application role. That role holds SELECT and INSERT on
// entries and has UPDATE and DELETE revoked (OPS-10) — the service never
// connects as the migrate role.
//
// SCAFFOLD no-op — see the package comment.
func Open(ctx context.Context, appDSN string) (ports.Store, error) {
	return scaffoldStore{}, nil
}

type scaffoldStore struct{}

func (scaffoldStore) Begin(ctx context.Context) (ports.UnitOfWork, error) {
	panic("postgres.Store.Begin not yet implemented -- RED scaffold")
}

func (scaffoldStore) Close() error { return nil }

// AttemptOutOfBandChange reaches past the driving ports to try to rewrite
// recorded history as the given role. It exists for exactly two reasons, both
// of which are acceptance criteria:
//
//   - as the application role, every attempt MUST be refused — that is what
//     makes D7 structural rather than discipline (OPS-10);
//   - as the privileged role, the alteration MUST succeed, because slice 04's
//     verdict only means something if a genuine drift can be produced.
//
// An error return means the store refused. A nil return means it did not.
func AttemptOutOfBandChange(ctx context.Context, dsn string, action string, args map[string]any) error {
	panic("postgres.AttemptOutOfBandChange not yet implemented -- RED scaffold")
}

// InterruptPostingMidWrite kills the process partway through a posting, so the
// chaos scenario can observe what survived. If this turns out to be awkward to
// write, the chaos demo — the thing that makes slice 01 worth anything — will
// not exist (slice-01 § Pre-slice SPIKE).
func InterruptPostingMidWrite(ctx context.Context, appDSN, from, to string, amountMinor int64, key string) error {
	panic("postgres.InterruptPostingMidWrite not yet implemented -- RED scaffold")
}

// CountEntryPairsForKey counts the entry pairs recorded under one idempotency
// key. Asserted alongside the distinct transaction count because the id check
// alone would miss a double write that happens to render the same id
// (kpi-contracts.yaml, KPI-3).
func CountEntryPairsForKey(ctx context.Context, appDSN, key string) (int, error) {
	return 0, fmt.Errorf("postgres.CountEntryPairsForKey not yet implemented -- RED scaffold")
}

// CountNegativeWalletObservations reports how many times a wallet balance was
// seen below zero during a contended run. Observed throughout the run, not
// sampled at the end: a run that dips negative and recovers has still broken
// I4's promise.
func CountNegativeWalletObservations(ctx context.Context, appDSN string) (int, error) {
	return 0, fmt.Errorf("postgres.CountNegativeWalletObservations not yet implemented -- RED scaffold")
}
