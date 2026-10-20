// Command smoke pulls a published image by digest, migrates a fresh Postgres,
// starts the image and requires /health 200 (fresh leg), then upgrades a
// frozen pre-feature database (upgrade leg). RED scaffold (DISTILL).
//
// SCAFFOLD: true
//
// Contract the acceptance suite drives: flags --image <repo@sha256:...>,
// --postgres-image <ref>; exit 0 on success; non-zero naming the failing
// step; summary line "smoke_pull_to_healthy_seconds=<n> arch=<arch>".
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "__SCAFFOLD__: smoke not yet implemented -- RED scaffold")
	os.Exit(70)
}
