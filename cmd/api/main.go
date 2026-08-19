// Command api is the entrypoint: wiring and configuration, nothing else.
//
// SCAFFOLD: true — created by DISTILL for Mandate 7 RED-readiness.
//
// The wiring reads only the application DSN (OPS-10). The migrate DSN is not
// read here and must never be: the service that can rewrite history is a
// service whose append-only guarantee is decoration.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"

	apphttp "ledgerops/internal/adapters/http"
	"ledgerops/internal/adapters/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx := context.Background()
	store, err := postgres.Open(ctx, os.Getenv("LEDGEROPS_APP_DSN"))
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
