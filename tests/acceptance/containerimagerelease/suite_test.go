package containerimagerelease

import (
	"context"
	"flag"
	"os"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
)

// Mirrors tests/acceptance/multitenancy/suite_test.go (ADR-025 one-at-a-time
// RED-scaffold delivery): scenarios tagged @pending are excluded by default;
// LEDGEROPS_AT_TAGS overrides the filter (for example "@pending" to run the
// RED gate, or "@US-1a" to run one story). @requires_external scenarios skip
// themselves unless LEDGEROPS_EXTERNAL is set.
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
		Name:                 "containerimagerelease",
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
	ctx.AfterSuite(func() { CleanupBuilds() })
}

func InitializeScenario(ctx *godog.ScenarioContext) {
	world := NewWorld()
	RegisterSteps(ctx, world)

	ctx.Before(func(c context.Context, sc *godog.Scenario) (context.Context, error) {
		var tags []string
		for _, t := range sc.Tags {
			tags = append(tags, t.Name)
		}
		world.Configure(tags)
		return c, nil
	})
	ctx.After(func(c context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		world.Close()
		return c, nil
	})
}
