package release

// Layer 1 properties of the release classifier (Mandate 9: property-based
// machinery belongs at the unit layer, not in the acceptance suite). Created
// by DISTILL as RED tests: they fail against the scaffold. They are skipped
// unless LEDGEROPS_RUN_PENDING is set so `go test ./...` stays green until
// DELIVER slice 03 enables them (delete the skip then).

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func pending(t *testing.T) {
	if os.Getenv("LEDGEROPS_RUN_PENDING") == "" {
		t.Skip("pending: DELIVER slice 03 implements Classify; set LEDGEROPS_RUN_PENDING=1 to see the RED run")
	}
}

func genVersion(t *rapid.T, label string) (string, int, int) {
	major := rapid.IntRange(0, 3).Draw(t, label+"-major")
	minor := rapid.IntRange(0, 3).Draw(t, label+"-minor")
	patch := rapid.IntRange(0, 6).Draw(t, label+"-patch")
	return fmt.Sprintf("%d.%d.%d", major, minor, patch), major, minor
}

func genStableSet(t *rapid.T) []string {
	n := rapid.IntRange(0, 6).Draw(t, "count")
	seen := map[string]bool{}
	var out []string
	for i := 0; i < n; i++ {
		v, _, _ := genVersion(t, fmt.Sprint("existing", i))
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func genNoise(t *rapid.T) []string {
	pool := []string{"latest", "0.2", "0.3", "main", "sha256-ab12cd34ef56.sig", "sha256-ab12cd34ef56.att", "sha256-0f0f0f.sbom", "nightly-2026-10-17"}
	return rapid.SliceOf(rapid.SampledFrom(pool)).Draw(t, "noise")
}

func cmp3(a, b string) int {
	var x, y [3]int
	fmt.Sscanf(a, "%d.%d.%d", &x[0], &x[1], &x[2])
	fmt.Sscanf(b, "%d.%d.%d", &y[0], &y[1], &y[2])
	for i := 0; i < 3; i++ {
		if x[i] != y[i] {
			if x[i] < y[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func TestLatestMovesOnlyForTheHighestStable(t *testing.T) {
	pending(t)
	rapid.Check(t, func(t *rapid.T) {
		existing := genStableSet(t)
		v, _, _ := genVersion(t, "new")
		plan, err := Classify("v"+v, append(existing, genNoise(t)...))
		if err != nil {
			t.Fatalf("a strict tag was rejected: %v", err)
		}
		higher := true
		for _, e := range existing {
			if e != v && cmp3(e, v) > 0 {
				higher = false
			}
		}
		if plan.MoveLatest != higher {
			t.Fatalf("version %s over %v: latest moved=%v, expected %v", v, existing, plan.MoveLatest, higher)
		}
	})
}

func TestPrereleaseNeverMovesLatestOrMinor(t *testing.T) {
	pending(t)
	rapid.Check(t, func(t *rapid.T) {
		existing := genStableSet(t)
		v, _, _ := genVersion(t, "new")
		rc := rapid.IntRange(1, 9).Draw(t, "rc")
		plan, err := Classify(fmt.Sprintf("v%s-rc.%d", v, rc), existing)
		if err != nil {
			t.Fatalf("a strict prerelease tag was rejected: %v", err)
		}
		if plan.MoveLatest || len(plan.Tags) != 1 || !plan.Prerelease {
			t.Fatalf("prerelease planned %+v", plan)
		}
	})
}

func TestNonVersionTagsNeverChangeThePlan(t *testing.T) {
	pending(t)
	rapid.Check(t, func(t *rapid.T) {
		existing := genStableSet(t)
		v, _, _ := genVersion(t, "new")
		clean, err1 := Classify("v"+v, existing)
		noisy, err2 := Classify("v"+v, append(append([]string{}, existing...), genNoise(t)...))
		if (err1 == nil) != (err2 == nil) || !reflect.DeepEqual(clean, noisy) {
			t.Fatalf("noise changed the plan: %+v vs %+v", clean, noisy)
		}
	})
}

func TestMinorTagNeverMovesToALowerPatch(t *testing.T) {
	pending(t)
	rapid.Check(t, func(t *rapid.T) {
		existing := genStableSet(t)
		v, major, minor := genVersion(t, "new")
		plan, err := Classify("v"+v, existing)
		if err != nil {
			t.Fatalf("rejected: %v", err)
		}
		line := fmt.Sprintf("%d.%d", major, minor)
		for _, tag := range plan.Tags {
			if tag != line {
				continue
			}
			for _, e := range existing {
				if strings.HasPrefix(e, line+".") && cmp3(e, v) > 0 {
					t.Fatalf("minor tag %s would move back from %s to %s", line, e, v)
				}
			}
		}
	})
}

func TestRerunPlansTheSameTagsAndResumes(t *testing.T) {
	pending(t)
	rapid.Check(t, func(t *rapid.T) {
		existing := genStableSet(t)
		v, _, _ := genVersion(t, "new")
		first, err := Classify("v"+v, existing)
		if err != nil {
			t.Fatalf("rejected: %v", err)
		}
		again, err := Classify("v"+v, append(append([]string{}, existing...), v))
		if err != nil || !reflect.DeepEqual(first.Tags, again.Tags) || !again.Resumed {
			t.Fatalf("re-running %s: tags %v then %v (resumed=%v, err=%v)", v, first.Tags, again.Tags, again.Resumed, err)
		}
	})
}
