package intertenanttransfer

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
)

// Mirrors tests/acceptance/multitenancy/suite_test.go exactly (ADR-025
// one-at-a-time RED-scaffold delivery). See that file's comments for the
// rationale; not re-explained here to avoid the same prose drifting apart
// across three copies.
var options = godog.Options{
	Output: colors.Colored(os.Stdout),
	Format: "pretty",
	Tags:   "~@pending",
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

	if tags, ok := os.LookupEnv("LEDGEROPS_AT_TAGS"); ok {
		options.Tags = tags
	}

	status := godog.TestSuite{
		Name:                 "intertenanttransfer",
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
		// Nothing global: containers are per-scenario (OPS-11 precedent).
	})
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	world := NewWorld()
	RegisterSteps(ctx, world)

	ctx.After(func(c context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		world.Stop()
		return c, nil
	})
}
