// Package release is the pure release classifier (DDD-36, ADR-020): from a
// pushed tag and the tags already present in the registry it produces a
// ReleasePlan; the workflow only executes the plan.
//
// SCAFFOLD: true -- RED stubs created by DISTILL (Mandate 7).
package release

import "ledgerops/internal/support"

// ReleasePlan is what release.yml executes.
type ReleasePlan struct {
	// Version is the tag without the leading "v", e.g. "0.2.0-rc.1".
	Version string
	// Tags are the image tags to promote, in promotion order: exact version,
	// then minor ("0.2"), then "latest"; prereleases carry the exact tag only.
	Tags []string
	// Prerelease is true when the version has a prerelease suffix.
	Prerelease bool
	// MoveLatest is true only for a stable version higher than every existing
	// stable version.
	MoveLatest bool
	// HighestStable is the highest existing stable version, "" on a first release.
	HighestStable string
	// Resumed is true when the exact version tag already exists (DDD-45).
	Resumed bool
}

// ErrNotAVersionTag is returned for a tag that is not strict vMAJOR.MINOR.PATCH[-pre].
var ErrNotAVersionTag error

// Classify maps (pushed tag, every tag currently in the registry package) to a
// plan. existingTags may contain noise (sha256-<hex>.sig, .att, "latest",
// minor tags); only strict semver versions count (DDD-50). A nil or empty
// slice is a first release.
func Classify(tag string, existingTags []string) (ReleasePlan, error) {
	support.Scaffold("release.Classify")
	return ReleasePlan{}, nil
}
