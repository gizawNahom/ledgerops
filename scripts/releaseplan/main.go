// Command releaseplan prints the ReleasePlan for a pushed tag as key=value
// lines (job outputs). Thin main over internal/release. RED scaffold (DISTILL).
//
// SCAFFOLD: true
//
// Contract the acceptance suite drives (milestone-06, milestone-07):
//
//	releaseplan --tag vX.Y.Z[-pre] --existing-tags a,b,c [--image-repo REPO --commit SHA]
//
// stdout: version=, tags= (space separated, promotion order), prerelease=,
// move_latest=, highest_stable=, resumed= . A tag that is not a strict version
// exits non-zero saying "not a version tag". With --image-repo and --commit, a
// resumed tag whose image's org.opencontainers.image.revision differs from
// --commit exits non-zero naming both commits (user-decided 2026-10-19).
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "__SCAFFOLD__: releaseplan not yet implemented -- RED scaffold")
	os.Exit(70)
}
