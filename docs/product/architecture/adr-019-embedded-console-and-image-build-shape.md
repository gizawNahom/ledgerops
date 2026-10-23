# ADR-019: Embed the console in the binary under a `console_embed` build tag; image build shape

**Status**: Accepted, 2026-10-17, revised in review cycle 1 (DDD-33, DDD-34)
**Owner**: nw-solution-architect (DESIGN, `container-image-release`)

## Context

`consoleDistDir = "web/console/dist"` is resolved against the working
directory, `dist/` is gitignored, and `mountConsole` silently registers nothing
when `index.html` is absent. Copying the binary alone yields a `/console` that
404s without error (US-5). `go:embed` of a gitignored directory breaks every
`go build`/`go vet`/`go test` that lacks the Node build, and `go:embed` cannot
reference a parent directory.

## Decision

1. **Assets seam is an `fs.FS`** (read-only capability) passed to the router.
   Two small files in the existing `internal/adapters/http` package: one under
   `//go:build console_embed` embedding `all:console_dist`, one under
   `!console_embed` returning no embedded FS. With no embedded FS the
   composition root falls back to `os.DirFS(consoleDistDir)` guarded by the
   existing probe, so dev, CI and `console_static_test` behave as today. The
   embedded directory `internal/adapters/http/console_dist/` is gitignored; the
   Dockerfile copies the built bundle there. (No new package: the embed files
   extend the HTTP adapter that already owns console serving.)
2. **Every image build passes `-tags console_embed`**: the single Dockerfile
   (dev compose and release alike) always does. A missing or empty
   `console_dist` fails at `go build` (embed pattern matches no files); a
   failed SPA build fails the Node stage first, so a failed console build never
   yields an image. A structural test asserts the Dockerfile's build command
   carries the tag.
3. **Startup refusal**: in a `console_embed` build, absence of `index.html` in
   the embedded FS (for example a directory holding only a placeholder file)
   refuses to start (wire, probe, use). The image therefore cannot serve
   without a console; the release smoke additionally asserts `/console` 200.
4. **Image build shape**: Node stage, Go stage on `$BUILDPLATFORM`
   cross-compiling with `GOOS/GOARCH` from `TARGET*` (no QEMU for compile),
   `-trimpath` and `-ldflags "-s -w -X main.version -X main.commit"` from build
   args (defaults `dev`, `unknown`), final stage `alpine` with CA certificates
   copied from the build stage so it contains no `RUN`, OCI labels from
   `docker/metadata-action`, new `.dockerignore`, BuildKit npm cache mount.
5. **Non-root with a numeric user**: `USER 65532:65532`, no `/etc/passwd`
   entry needed (the process uses no name lookups; Docker and Kubernetes
   accept numeric users). Documented trap: a secret file mounted `0400 root`
   is unreadable by `migrate`; Docker and Kubernetes defaults
   (`0444`/`0644`) work.
6. **Pinned bases (DDD-44)**: every `FROM` carries a digest
   (`golang`, `node`, `alpine`), values supplied by DEVOPS and bumped by
   dependabot/renovate.

## Alternatives considered

- **Keep the path convention, copy `dist` into `/app/web/console/dist`, set
  `WORKDIR`.** No Go change, but the silent skip remains: an overridden
  workdir or entrypoint loses the console and only the smoke test notices.
- **A new `consoledist` package.** Needless: the HTTP adapter already owns
  console serving and a tagged file pair does the same job with no new import
  edge.
- **`LEDGEROPS_CONSOLE_DIR` plus a "require console" flag.** Adds configuration
  surface to a problem embedding removes.
- **Stay root.** Ships a ledger holding platform funds as uid 0. Rejected.

## Consequences

- `make up` and the CI `demo` job now build the console. The KPI-5
  `demo_first_green_seconds` baseline will rise; DEVOPS re-baselines.
- Untagged developer builds still skip the console silently (existing dev
  behaviour); only image builds are guaranteed to carry it.

## Amendments from DEVOPS (2026-10-19; `devops/upstream-changes.md` items 3, 4, 5)

The text above is kept as decided; these amend it.
- **Runtime base (item 3, user-decided 2026-10-17):** decision 4's final stage is `alpine:3.24` (index digest `sha256:294b683c...c77e6`, amd64 and arm64 verified), not 3.20, which left support on 2026-04-01.
- **Node stage (item 4, user-decided 2026-10-17):** the console stage and the CI `console` job use `node:22-slim` (glibc), not Node 20. Reason: `npm ci` fails on musl (`EBADPLATFORM`; the Pact native package is glibc-only) and Pact 17 declares `node >=22`. Digests are DEVOPS's (feature-delta § Supply-chain pins).
- **Topology (item 5):** the stage structure is now a tested contract. The golang and node stages are `FROM --platform=$BUILDPLATFORM`; `GOOS`/`GOARCH` come from `TARGETOS`/`TARGETARCH`; no `RUN` follows the last `FROM`, so nothing executes on arm64 during build. A `workflow-guards` structural test asserts it, with the `-tags console_embed` and digest-per-`FROM` assertions.
