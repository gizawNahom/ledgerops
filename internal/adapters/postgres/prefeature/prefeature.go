// Package prefeature is the FROZEN pre-feature migration set: migrations
// 0001-0006 exactly as they shipped before `container-image-release` edited
// 0001 (DDD-27 / ADR-017: role created NOLOGIN, GRANT CONNECT via
// current_database()).
//
// It exists only as a test and smoke fixture. The upgrade scenarios (US-4,
// Rule B) and the release smoke's upgrade leg need a database built EXACTLY
// as the pre-feature demo stack built it: `ledgerops_app` created LOGIN with
// the literal password `app-secret`, and `GRANT CONNECT` on a database
// literally named `ledgerops`. A database migrated by the live (edited) set
// cannot stand in for that, so the old text is kept here byte for byte.
//
// Rules:
//   - The *.sql files in this directory are never edited. A checksum guard
//     (prefeature_test.go) pins every one of them.
//   - Nothing in cmd/..., internal/domain or internal/app may import this
//     package, so the frozen SQL (and its well-known password) can never reach
//     the serving binary. An import guard in prefeature_test.go enforces it.
//     Only scripts/smoke and the acceptance suites import it.
//
// Embedded FS root is ".": golang-migrate's iofs source takes (FS, ".").
package prefeature

import "embed"

// FS holds the frozen up and down migrations, 0001 through 0006.
//
//go:embed *.sql
var FS embed.FS

// Dir is the directory to hand to iofs.New alongside FS.
const Dir = "."
