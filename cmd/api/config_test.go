package main

import (
	"errors"
	"testing"

	"pgregory.net/rapid"
)

func readsAs(content string) func(string) ([]byte, error) {
	return func(string) ([]byte, error) { return []byte(content), nil }
}

func TestParseArgsDefaultsToServeAndNamesTheSubcommand(t *testing.T) {
	cases := map[string]Command{"": CommandServe, "serve": CommandServe, "migrate": CommandMigrate, "version": CommandVersion}
	for arg, want := range cases {
		args := []string{}
		if arg != "" {
			args = append(args, arg)
		}
		if got, err := ParseArgs(args); err != nil || got != want {
			t.Errorf("ParseArgs(%v) = %q, %v; want %q", args, got, err, want)
		}
	}
	if _, err := ParseArgs([]string{"launch"}); err == nil {
		t.Error("an unknown subcommand must be a usage error")
	}
}

func TestMigrateConfigDropsExactlyOneTrailingLineEnding(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		password := rapid.StringMatching(`[A-Za-z0-9!#$%@]{8,24}`).Draw(t, "password")
		ending := rapid.SampledFrom([]string{"", "\n", "\r\n"}).Draw(t, "ending")
		env := Env{envMigrateDSN: "postgres://u@h/db", envAppPasswordFile: "/run/secrets/pw"}
		settings, issues := ParseMigrateConfig(env, readsAs(password+ending))
		if len(issues) != 0 || settings.AppPassword.Reveal() != password {
			t.Fatalf("got %q, %v; want %q", settings.AppPassword.Reveal(), issues, password)
		}
	})
}

func TestMigrateConfigKeepsAnInnerAndASecondLineEnding(t *testing.T) {
	env := Env{envMigrateDSN: "postgres://u@h/db", envAppPasswordFile: "/p"}
	settings, _ := ParseMigrateConfig(env, readsAs("pass word\n\n"))
	if settings.AppPassword.Reveal() != "pass word\n" {
		t.Fatalf("got %q", settings.AppPassword.Reveal())
	}
}

func TestMigrateConfigReadsTheInlinePasswordUntouched(t *testing.T) {
	env := Env{envMigrateDSN: "postgres://u@h/db", envAppPassword: " T7!kq@9x%Zm\n"}
	settings, issues := ParseMigrateConfig(env, nil)
	if len(issues) != 0 || settings.AppPassword.Reveal() != " T7!kq@9x%Zm\n" {
		t.Fatalf("got %q, %v", settings.AppPassword.Reveal(), issues)
	}
	if settings.MigrateDSN.Reveal() != "postgres://u@h/db" {
		t.Fatal("the privileged DSN must be carried through")
	}
}

func TestMigrateConfigReportsEveryIssueWithoutEchoingSecrets(t *testing.T) {
	unreadable := func(string) ([]byte, error) { return nil, errors.New("permission denied") }
	cases := map[string]Env{
		"no dsn":      {envAppPassword: "secret-pw-1"},
		"no password": {envMigrateDSN: "postgres://u:dsn-secret@h/db"},
		"blank":       {envMigrateDSN: "postgres://u:dsn-secret@h/db", envAppPassword: "  "},
		"both":        {envMigrateDSN: "postgres://u:dsn-secret@h/db", envAppPassword: "secret-pw-1", envAppPasswordFile: "/p"},
		"unreadable":  {envMigrateDSN: "postgres://u:dsn-secret@h/db", envAppPasswordFile: "/p"},
		"blank file":  {envMigrateDSN: "postgres://u:dsn-secret@h/db", envAppPasswordFile: "/p"},
	}
	for name, env := range cases {
		read := unreadable
		if name == "blank file" {
			read = readsAs("\n")
		}
		_, issues := ParseMigrateConfig(env, read)
		if len(issues) == 0 {
			t.Errorf("%s: expected an issue", name)
		}
		for _, issue := range issues {
			for _, secret := range []string{"dsn-secret", "secret-pw-1"} {
				if contains(issue, secret) {
					t.Errorf("%s: issue %q leaks %q", name, issue, secret)
				}
			}
		}
	}
}

func contains(text, part string) bool {
	for index := 0; index+len(part) <= len(text); index++ {
		if text[index:index+len(part)] == part {
			return true
		}
	}
	return false
}
