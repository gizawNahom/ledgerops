// Command api is the entrypoint: wiring, probing, and configuration, nothing
// else.
//
// The wiring reads only the application DSN (OPS-10). The migrate DSN is not
// read here and must never be: the service that can rewrite history is a
// service whose append-only guarantee is decoration.
//
// Wire, then probe, then use (DDD-20 / ADR-009): before the server accepts a
// single connection, probeStartup opens its own transaction as the
// application role and asserts two things — the role can read, and an UPDATE
// on entries is refused (OPS-10, enforced at the database level since
// migration 0). A probe failure is a structured refusal to start
// (health.startup.refused), never a panic: the failure mode is "this
// database is not the one we can safely serve from," which is exactly the
// kind of thing an operator reads from a log line, not a stack trace.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	apphttp "ledgerops/internal/adapters/http"
	"ledgerops/internal/adapters/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx := context.Background()
	appDSN := os.Getenv("LEDGEROPS_APP_DSN")

	if err := probeStartup(ctx, appDSN); err != nil {
		logger.Error("health.startup.refused", "err", err)
		os.Exit(1)
	}

	store, err := postgres.Open(ctx, appDSN)
	if err != nil {
		logger.Error("could not open the store as the application role", "err", err)
		os.Exit(1)
	}
	defer store.Close()

	handler := apphttp.NewRouter(apphttp.Deps{
		Store:       store,
		OperatorKey: os.Getenv("LEDGEROPS_OPERATOR_KEY"),
		Clock:       time.Now,
		IDGenerator: func() string { return "txn_" + uuid.NewString() },
		Metrics:     apphttp.NewMetrics(),
	})

	addr := os.Getenv("LEDGEROPS_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	logger.Info("ledgerops listening", "addr", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

// probeStartup asserts the app-role connection can open a transaction and
// read, and that an UPDATE on the entries table is refused. It opens its own
// connection independent of the store the router will later use — the probe
// must prove the database this binary is actually pointed at, not assume the
// migration set proves it (that is CI job 6's job, not this one's).
func probeStartup(ctx context.Context, appDSN string) error {
	poolConfig, err := pgxpool.ParseConfig(appDSN)
	if err != nil {
		return fmt.Errorf("parsing the probe DSN: %w", err)
	}

	conn, err := pgx.ConnectConfig(ctx, poolConfig.ConnConfig)
	if err != nil {
		return fmt.Errorf("opening a probe connection as the application role: %w", err)
	}
	defer conn.Close(ctx)

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("the application role could not open a transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT 1"); err != nil {
		return fmt.Errorf("the application role could not read: %w", err)
	}

	if _, err := tx.Exec(ctx, "UPDATE entries SET amount_minor = amount_minor WHERE false"); err == nil {
		return errors.New("the application role was able to UPDATE entries -- append-only guarantee (OPS-10) is not enforced")
	}

	return nil
}
