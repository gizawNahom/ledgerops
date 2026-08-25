// console_static.go serves the built console SPA bundle (web/console/dist,
// Vite's build.outDir) at GET /console and its static assets underneath --
// the DEVOPS-owned infra wiring the DESIGN wave's "Build-output wiring" open
// item left undecided. Resolved (2026-08-25, user decision) as a
// DEVOPS/infra concern: a minimal static-file-serving route is
// infrastructure, like the health endpoint, not console feature business
// logic -- DDR-2's "no backend changes" constraint scoped the four console
// user stories' behavior, not the infra wiring required to serve the SPA at
// all. See feature-delta.md § Wave: DEVOPS / Build-output wiring.
//
// Deliberately outside the operator-API-key middleware group NewRouter
// builds (router.go): the HTML shell and its JS/CSS assets are the SPA's own
// delivery mechanism, not a ledger operation. The operator key is still
// checked exactly where it always was -- on GET /console/verdict and the
// other JSON endpoints the SPA calls once loaded -- never on the static
// shell that fetches them.
package http

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
)

// consoleDistDir is Vite's build.outDir for web/console, relative to the
// process's working directory -- the same convention cmd/api already uses
// for its own configuration (LEDGEROPS_APP_DSN, LEDGEROPS_ADDR read as-is,
// no path resolution against the binary's own location).
const consoleDistDir = "web/console/dist"

// mountConsole registers GET /console (the SPA shell) and GET /console/*
// (its built assets, e.g. /console/assets/index-XXXX.js) against distDir --
// but only if distDir/index.html exists at call time.
//
// web/console/ does not exist until ledger-core-console's DELIVER wave
// lands (confirmed absent as of this DEVOPS pass, same as the `console` CI
// job's own probe in .github/workflows/ci.yml). Mounting is therefore
// probe-guarded the same way: if the built bundle is not there, no /console
// route is registered at all, and the path 404s through chi's ordinary
// unmatched-route handling -- absence is a no-op, not a startup failure or a
// build break, exactly the posture the CI job already established for the
// same "doesn't exist yet" problem.
func mountConsole(router chi.Router, distDir string) {
	indexPath := filepath.Join(distDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return
	}

	fileServer := http.StripPrefix("/console", http.FileServer(http.Dir(distDir)))

	router.Get("/console", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, indexPath)
	})
	router.Get("/console/*", fileServer.ServeHTTP)
}
