# ledgerops site — page outline (draft 2)

Built from `brandscript.md`, revised after an independent review. One reader:
the engineer evaluating ledgers. Each page has one job. Copy comes after this
is agreed.

## Sitemap

| Page | Job | Reader's question |
|---|---|---|
| **Home** | Make the problem and the fix clear, and send them to Run it | "Is this for me, and is it serious?" |
| **Quickstart** | Get a first transfer posted; holds the API reference | "Can I run it?" |
| **Guarantees** | How it works, then each guarantee as a mechanism | "Can I trust it?" |
| **Compare** | Say when not to choose ledgerops | "Does it fit my stack?" |
| **Status, limits and operating** | What isn't there yet, and what running it takes | "What will bite me?" |

Header nav: Quickstart · Guarantees · Compare · Status · GitHub. One button in
the header and on Home: **Run it**. Five pages; the API route table lives on
Quickstart as "Reference" until an OpenAPI spec exists.

## Home

1. **Hero.** "A double-entry ledger for your wallet or credits system." Sub
   line. Button: Run it. Text links: Read the guarantees, Compare.
2. **The two calls.** The 422 overdraft refusal and the trial-balance verdict
   as real request and response, with the console drift screenshot beside the
   trial-balance one.
3. **Problem.** "A balance column works until it doesn't." Three stakes:
   double charge on retry, negative balance under load, retrofitting.
4. **Guarantees at a glance.** Six one-line mechanisms, each linking to
   Guarantees.
5. **Honest status.** Short "not yet" list (one currency, two-leg only, no
   holds or webhooks, not run in production, the trial-balance check is the
   service's own), linking to the full list.
6. **Closing call.** Run it, once.

The three-step plan (run, move value, check) lives on Quickstart, not Home.

## Quickstart

1. Prerequisites: Docker, curl.
2. **Build from source:** `make up`, `make demo-01`, then the retry, overdraw,
   balance and trial-balance commands. Notes: first build is slow and starts
   Prometheus and Grafana too; `demo-04` fails on a second run
   (`tenant_already_exists`); `make down` deletes the data volume. Demo keys
   are local only.
3. **Pull the image (release candidate):** written only after someone has run
   it on a clean machine. It must say whether it needs an external PostgreSQL
   and role setup, whether it ships the console, and include a signature
   verify command.
4. "From nothing" walkthrough: link to the tenant-from-scratch guide, noting
   it needs `make up` first.
5. **Reference:** route table and credentials.

## Guarantees

1. **How it works.** The architecture diagram, the pure domain core, the
   app and migrate database roles, stored balances. A short security model:
   two key types, the operator key refused on the account and transfer-posting routes,
   unauthenticated `/metrics`.
2. One section per guarantee, each with the claim, the mechanism, **Check
   it**, and the ADR link. Where a `make` target exists it is named; where it
   doesn't, the section says "run in CI" and links the `go test` suite or
   `ci.yml`. No invented commands.

| Guarantee | Check it | ADR |
|---|---|---|
| Conservation | property tests (`go test ./internal/domain/... ./internal/app/...`) | ADR-001, 007 |
| No overdrafts under concurrency | `make race-02`, `make demo-02` | ADR-002 |
| Append-only history | acceptance suite, run in CI | ADR-004 |
| Safe retries | `make demo-03`, `make race-03`, `make chaos-01` | ADR-005 |
| Tenant isolation | multitenancy acceptance suite, run in CI | ADR-011, 012, 013 |
| Cross-tenant transfers need both sides | acceptance suite, run in CI | ADR-014, 016 |

Closing note on what the check does not do: the service runs its own
trial-balance check, and independent verification is on the roadmap.
(ADR numbers are from the reviewer's reading of the ADR list; confirm before
copy. Confirm which acceptance feature files cover append-only and
multitenancy.)

## Compare

The README's comparison table (TigerBeetle, pgledger, Formance, Blnk) and the
"not an accounting package, payments processor or hosted service" lines.
Tone: fair, "choose them if".

## Status, limits and operating

- The full not-yet list.
- The hot-account performance result (p95 923 ms vs 500 ms target, row locks
  the cause).
- **Operating it:** run behind a TLS-terminating proxy; metrics on :9090 and
  Grafana on :3000; expand-only migrations, so rolling back code needs no
  schema rollback; single binary; not tested with more than one app
  instance; password rotation needs a restart.
- The four-step roadmap.

## Source-of-truth fixes (done in the README)

- Tagline "can prove it did" is now "lets you check that it did".
- "Proof" section is now "Check it yourself"; "on-demand proof" is now
  "on-demand check".
- The image line now says a signed release-candidate image is published.
- Not changed: "timing targets are reported rather than enforced". CI emits
  the demo timing as a notice and never asserts the 300 s limit, so the line
  is true. `kpi-contracts.yaml` says `fails_build: true`; that disagreement is
  for the owning agent.

## Open items before copy

- Confirm the image steps so Quickstart's second path can be written.
- Decide whether the site gets its own domain or stays at the GitHub Pages
  address.
- Confirm ADR numbers and the acceptance suites behind each guarantee.
