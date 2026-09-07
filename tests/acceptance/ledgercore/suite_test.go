package ledgercore

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
)

var options = godog.Options{
	Output: colors.Colored(os.Stdout),
	Format: "pretty",
	// One-at-a-time delivery: DISTILL authors every scenario as a RED scaffold
	// tagged @pending (ADR-025). DELIVER unskips exactly one at a time by
	// removing its @pending tag — it does not re-author the scenario.
	//
	// The walking skeleton carries no @pending tag: it is the first thing
	// DELIVER makes green, and nothing else is worth reading while it is red.
	Tags: "~@pending",
}

func init() {
	godog.BindCommandLineFlags("godog.", &options)
}

func TestMain(m *testing.M) {
	flag.Parse()
	options.Paths = flag.Args()
	if len(options.Paths) == 0 {
		options.Paths = []string{"."}
	}

	// One escape hatch, used by the DISTILL red-classification gate and by CI:
	// LEDGEROPS_AT_TAGS widens or narrows the selection without editing this
	// file. Running it with an empty value selects every scenario including the
	// @pending ones, which is how an undefined step is caught before it can
	// reach DELIVER as a BROKEN scenario rather than a red one — godog prints a
	// definition location beside every bound step, even a skipped one, and
	// counts the rest as undefined.
	if tags, ok := os.LookupEnv("LEDGEROPS_AT_TAGS"); ok {
		options.Tags = tags
	}

	status := godog.TestSuite{
		Name:                 "ledger-core",
		ScenarioInitializer:  InitializeScenario,
		TestSuiteInitializer: InitializeTestSuite,
		Options:              &options,
	}.Run()

	if suiteStatus := m.Run(); suiteStatus > status {
		status = suiteStatus
	}
	os.Exit(status)
}

func InitializeTestSuite(ctx *godog.TestSuiteContext) {
	ctx.BeforeSuite(func() {
		// Nothing global: containers are per-scenario so that `contended` and
		// `corrupted` inherit no state from a prior test (OPS-11). A shared
		// container would make corrupt-04 pass on leftover drift. Each
		// scenario's own After hook terminates the ones it started.
	})
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	ledger := NewLedger()
	RegisterSteps(ctx, ledger)

	ctx.After(func(c context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		ledger.Close()
		return c, nil
	})
}
