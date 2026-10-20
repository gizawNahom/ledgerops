package main

// The migrate command: the only file that reads the privileged DSN and the
// app-password settings (ADR-018). Parsing is pure (config.go); this file is
// the effect shell that snapshots the environment, reads the password file,
// and runs the schema files followed by the password step.

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"ledgerops/internal/adapters/postgres"
)

const (
	envMigrateDSN      = "LEDGEROPS_MIGRATE_DSN"
	envAppPassword     = "LEDGEROPS_APP_PASSWORD"
	envAppPasswordFile = "LEDGEROPS_APP_PASSWORD_FILE"
)

// Exit codes of the migrate command.
const (
	exitOK            = 0
	exitMigrateFailed = 1
	exitConfigInvalid = 2
)

// environmentSnapshot captures the process environment as a value.
func environmentSnapshot() Env {
	snapshot := Env{}
	for _, entry := range os.Environ() {
		if name, value, found := strings.Cut(entry, "="); found {
			snapshot[name] = value
		}
	}
	return snapshot
}

// runMigrate prepares the database: schema files to head, then the app role's
// password last. It prints a summary and never the password or a raw DSN.
func runMigrate(ctx context.Context, env Env, stdout, stderr io.Writer) int {
	settings, issues := ParseMigrateConfig(env, os.ReadFile)
	if len(issues) > 0 {
		fmt.Fprintf(stderr, "migrate: config invalid: %s\n", strings.Join(issues, "; "))
		return exitConfigInvalid
	}
	dsn := settings.MigrateDSN.Reveal()
	if err := postgres.Migrate(ctx, dsn, settings.AppPassword); err != nil {
		fmt.Fprintf(stderr, "migrate: %s\n", postgres.RedactDSN(err.Error()))
		return exitMigrateFailed
	}
	version, err := postgres.SchemaVersion(dsn)
	if err != nil {
		fmt.Fprintf(stderr, "migrate: %s\n", postgres.RedactDSN(err.Error()))
		return exitMigrateFailed
	}
	fmt.Fprintf(stdout, "migrate: schema at version %d\n", version)
	fmt.Fprintln(stdout, "migrate: app role password set")
	return exitOK
}
