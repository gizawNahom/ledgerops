package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"ledgerops/internal/support"
)

// SCAFFOLD: true -- RED stubs created by DISTILL (Mandate 7); signatures per
// DDD-30..32 / ADR-018. All are pure over an env snapshot; the shell reads
// env and files. The acceptance suite drives them through the built binary
// (package main cannot be imported); unit tests in this package drive them
// directly.

// Env is an environment snapshot (name to value; absent = unset).
type Env map[string]string

// Command is the parsed subcommand.
type Command string

const (
	CommandServe   Command = "serve"
	CommandMigrate Command = "migrate"
	CommandVersion Command = "version"
)

// ParseArgs maps argv (without the program name) to a Command or a usage error.
func ParseArgs(args []string) (Command, error) {
	if len(args) == 0 {
		return CommandServe, nil
	}
	switch command := Command(args[0]); command {
	case CommandServe, CommandMigrate, CommandVersion:
		if len(args) > 1 {
			return "", fmt.Errorf("%s takes no arguments, got %q (configuration is by environment; migrate has no force option)", command, args[1])
		}
		return command, nil
	}
	return "", fmt.Errorf("unknown command %q: expected serve, migrate or version", args[0])
}

// ServeConfig is the validated serve configuration.
type ServeConfig struct {
	AppDSN        support.Secret
	OperatorKey   support.Secret
	DemoTenantKey support.Secret
	Addr          string
}

// LoadServeConfig returns the config plus every issue found (blank and
// whitespace-only count as unset).
func LoadServeConfig(env Env) (ServeConfig, []string) {
	support.Scaffold("LoadServeConfig")
	return ServeConfig{}, nil
}

// MigrateSettings is the validated migrate configuration.
type MigrateSettings struct {
	MigrateDSN  support.Secret
	AppPassword support.Secret
}

// ParseMigrateConfig validates migrate's env; readFile is the injected
// password-file reader (strips exactly one trailing newline).
func ParseMigrateConfig(env Env, readFile func(path string) ([]byte, error)) (MigrateSettings, []string) {
	var issues []string
	dsn := env[envMigrateDSN]
	if strings.TrimSpace(dsn) == "" {
		issues = append(issues, envMigrateDSN+" is not set")
	}
	password, passwordIssues := resolveAppPassword(env, readFile)
	issues = append(issues, passwordIssues...)
	if len(issues) > 0 {
		return MigrateSettings{}, issues
	}
	return MigrateSettings{MigrateDSN: support.NewSecret(dsn), AppPassword: password}, nil
}

// resolveAppPassword picks the one password source that is set: the inline
// variable, or the file whose single trailing line ending is dropped.
func resolveAppPassword(env Env, readFile func(path string) ([]byte, error)) (support.Secret, []string) {
	inline, inlineSet := env[envAppPassword]
	path, fileSet := env[envAppPasswordFile]
	switch {
	case inlineSet && fileSet:
		return support.Secret{}, []string{"set only one of " + envAppPassword + " and " + envAppPasswordFile}
	case fileSet:
		return readPasswordFile(path, readFile)
	case inlineSet:
		return nonBlankPassword(inline)
	}
	return support.Secret{}, []string{"set " + envAppPassword + " or " + envAppPasswordFile}
}

func readPasswordFile(path string, readFile func(path string) ([]byte, error)) (support.Secret, []string) {
	content, err := readFile(path)
	if err != nil {
		return support.Secret{}, []string{"ledgerops_app_password file not readable: " + err.Error()}
	}
	return nonBlankPassword(stripOneTrailingLineEnding(string(content)))
}

func stripOneTrailingLineEnding(text string) string {
	if withoutCRLF, found := strings.CutSuffix(text, "\r\n"); found {
		return withoutCRLF
	}
	return strings.TrimSuffix(text, "\n")
}

func nonBlankPassword(password string) (support.Secret, []string) {
	if strings.TrimSpace(password) == "" {
		return support.Secret{}, []string{"set " + envAppPassword + " or " + envAppPasswordFile + " to a non-blank password"}
	}
	return support.NewSecret(password), nil
}

// refuseIfMigrateDSNPresent checks presence only, never the value (ADR-018 item 4).
func refuseIfMigrateDSNPresent(lookupEnv func(string) (string, bool)) error {
	support.Scaffold("refuseIfMigrateDSNPresent")
	return nil
}

// StartupRefusal is a classified startup failure.
type StartupRefusal struct {
	Reason string
	Role   string
}

// sqlstateInvalidPassword is PostgreSQL's invalid_password (28P01).
const sqlstateInvalidPassword = "28P01"

// ClassifyStartupError maps (error, DSN user) to a named refusal (SQLSTATE
// 28P01). The reason names the role, never a password: a mismatch between the
// app DSN and the password migrate set is caught here, at start (DDD-32).
func ClassifyStartupError(err error, dsnUser string) StartupRefusal {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == sqlstateInvalidPassword {
		return StartupRefusal{
			Reason: "password authentication failed for role " + dsnUser +
				": LEDGEROPS_APP_DSN does not carry the password migrate set",
			Role: dsnUser,
		}
	}
	return StartupRefusal{Reason: err.Error(), Role: dsnUser}
}
