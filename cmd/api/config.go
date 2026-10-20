package main

import "ledgerops/internal/support"

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
	support.Scaffold("ParseArgs")
	return CommandServe, nil
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
	support.Scaffold("ParseMigrateConfig")
	return MigrateSettings{}, nil
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

// ClassifyStartupError maps (error, DSN user) to a named refusal (SQLSTATE 28P01).
func ClassifyStartupError(err error, dsnUser string) StartupRefusal {
	support.Scaffold("ClassifyStartupError")
	return StartupRefusal{}
}
