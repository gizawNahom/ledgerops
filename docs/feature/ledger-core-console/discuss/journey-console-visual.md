# Journey (Visual) — Verify the books, from the console

Comprehensive Phase 2 redo (corrected Decision 3 = "Comprehensive", superseding
the prior lightweight formalization pass). Starting input: `verify-the-books.yaml`
S1-S4 (real prior evidence, not discarded). This pass goes deeper: it
interrogates mental-model assumptions the prior pass took as given, elaborates
error paths beyond S1/S2's original two, and adds states the JSON-contract-only
view never had to consider (loading, staleness, partial failure mid-investigation)
because a curl command doesn't have a "loading" state — a browser page does.

---

## Trigger: two entry variants, not one

The prior pass assumed a single entry emotional state ("seeking reassurance").
Interrogating the mental model surfaces two distinct real triggers for P2:

```
Trigger A — Routine                  Trigger B — Reactive
(most common)                        (rarer, higher stakes)
"start of day, habitual check"       "something already smells wrong"
Feels: calm, dutiful                 Feels: anxious, urgent
                                      (a customer complained, or the
                                       operator's own DB access turned
                                       up something odd)
```

Both variants converge on the same S1 action (open the console) but start
from a different emotional baseline. This matters for S2: a YES verdict is
*relief* under Trigger A (confirms the routine) but *vindication/skepticism*
under Trigger B (the operator may not fully trust a clean answer if they
already suspect a problem — worth a design note for DESIGN, not a new story:
the verdict sentence alone may not be enough to fully resolve Trigger-B
anxiety; DDR-2 keeps this out of scope for automated testing, but it's a real
UX nuance downstream should be aware of).

---

## Full flow (ASCII, both triggers converge at S1)

```
[Trigger A: routine]  \
                        > [S1: Open console] -> [S1a: Loading] -> [S2: Verdict]
[Trigger B: reactive] /
  Feels: calm|anxious      Feels: brief          Feels: brief       Feels: relief (YES)
                            suspense               suspense           or alarm (NO)
                                                    ("is this          Sees: "Books
                                                    thing even          balance: YES/NO"
                                                    working?")          as first sentence

  [S2: Verdict = NO] -> [S3: Drift table] -> [S3a: click account_id]
    Feels: alarm            Feels: focused          Feels: tense anticipation
                             concern (dip)            ("about to see the
                                                        smoking gun")
                                  |
                                  v
                          [S4a: entries loading] -> [S4: Entries + running balance]
                            Feels: tense              Feels: understanding (PEAK)
                            anticipation
                                  |
                                  v (failure branch, NEW this pass)
                          [S4-fail: entries fetch fails]
                            Feels: WORST dip of the journey —
                            "so close to the answer and it broke"
```

---

## TUI Mockups

### S1a — Loading (NEW: prior pass had no state between "open" and "verdict")

```
+-- Console ---------------------------------------------------------+
|                                                                     |
|   Checking the books...                                            |
|                                                                     |
+---------------------------------------------------------------------+
```
Why this matters: a curl command either returns or hangs — there is no
in-between state to design. A browser page has one, and if it's left
undesigned it defaults to a blank page, which reads as "broken" before the
operator ever sees an answer. This directly threatens KPI 1 (operator trusts
the console for 100% of routine checks) — a tool that looks broken on first
load erodes exactly the trust it exists to build.

### S2 — Verdict, now with a freshness label (NEW artifact: `${fetched_at}`)

```
+-- Console ---------------------------------------------------------+
|                                                                     |
|   Books balance: YES                                               |
|   as of 09:14:02 (moments ago)          <-- ${fetched_at}          |
|                                                                     |
+---------------------------------------------------------------------+
```
Why this matters: interrogating the "stale-data" question this pass was
asked to consider — the console has no auto-refresh (US-4 explicitly rules
out polling as unproven complexity for a single-operator tool). If the
operator leaves a tab open across a lunch break, or bookmarks the page and
returns to it hours later without reloading, an unlabeled verdict could be
mistaken for current. `${fetched_at}` is a client-observed timestamp (see
shared-artifacts-registry.md) — no backend change required.

### S3 — Drift table (unchanged in substance from the lightweight pass)

```
+-- Console ---------------------------------------------------------+
|   Books balance: NO                                                |
|   as of 09:14:02 (moments ago)                                     |
|                                                                     |
|   account_id      stored     computed    delta                    |
|   alice-demo      100.00     105.00      5.00      [view entries] |
|                                                                     |
+---------------------------------------------------------------------+
```

### S4 — Entry trace, divergence visible (unchanged in substance)

```
+-- alice-demo entries -----------------------------------------------+
|   date        counterparty     amount    running balance            |
|   08-20       treasury-demo    +100.00   100.00                     |
|   08-24       (tampered)       +5.00     105.00   <-- diverges here |
|                                                                     |
+---------------------------------------------------------------------+
```

### S4-fail — Entries fetch fails mid-investigation (NEW this pass)

```
+-- alice-demo entries -----------------------------------------------+
|                                                                     |
|   Could not load entries for alice-demo.                           |
|   Try again, or check GET /accounts/alice-demo/entries directly.   |
|                                                                     |
+---------------------------------------------------------------------+
```
Why this matters most of all: US-4 (the prior pass's only failure story)
handles the *first* fetch failing — a relatively low-stakes moment, because
the operator hasn't invested anything yet. This new failure mode is worse:
it strikes at the peak-tension moment (S3a/S4a), after the operator has
already been alarmed by a NO verdict and has narrowed to a specific
suspect account. A silent failure here (blank panel, stuck spinner) is the
cruelest possible place for the journey to break. This pass adds it as a
new UAT scenario to US-3 (see feature-delta.md) rather than a new slice —
same fetch, same story, same slice-03 scope; it is not a new bounded
context or a new risk category, just an error branch on an existing call.

---

## Emotional Arc (redrawn, comprehensive)

```
confidence
  9 |                                              *  S4 (peak: understanding)
  8 |     * S2-YES (relief)
  7 |
  6 |                        * S3 (focused concern)
  5 | * S1 (seeking reassurance / anxious per trigger)
  4 |
  3 |            * S2-NO (alarm, dip begins)
  2 |
  1 |                                    x  S4-fail (worst dip, NEW)
    +---------------------------------------------------------------
      Trigger  S1   S1a   S2      S3    S3a/S4a   S4      S4-fail(branch)
```

Shape: dip-and-recover (unchanged core shape from `verify-the-books.yaml`),
but this pass adds a second, deeper possible dip (S4-fail) on the
error branch, and softens the start of the arc into two variants
(routine calm vs. reactive anxious) rather than a single starting emotion.

---

## Error Paths (elaborated beyond verify-the-books.yaml's original two)

| Step | Failure | Response | Recovery | Status |
|---|---|---|---|---|
| S1 | Verdict fetch fails/times out | Explicit bounded-time error naming `GET /health/trial-balance` | Reload once API recovers | Existing (US-4) |
| S1 | Console page itself unreachable | n/a — no in-app fix possible | Operator falls back to API directly | Existing, explicitly out of story-reach (slice-04 brief) |
| S1a | Fetch is in flight | Neutral "Checking the books..." state, not blank | Resolves to S2 or S1-fail | **NEW this pass** — added to US-1 |
| S2 | Verdict is stale (page left open, no auto-refresh) | `${fetched_at}` label shows how long ago | Operator manually reloads if in doubt | **NEW this pass** — added to US-1 |
| S3a/S4a | Entries fetch fails after clicking a drifted account | Bounded-time error naming the direct endpoint as fallback, same pattern as US-4 | Operator retries or falls back to `GET /accounts/{id}/entries` directly | **NEW this pass** — added to US-3, not a new slice |

---

## Integration Checkpoints

1. Before S2 renders, `${verdict}` and `${drifted}` must come from the same
   fetch response (never two separate calls that could disagree).
2. Before S4 renders, `${account_id}` passed into the entries fetch must be
   the exact string from the `${drifted}` row clicked, not a re-derived or
   re-typed value.
3. `${fetched_at}` must never be sourced from the server response (it isn't
   present today) — client-local only, or the artifact silently duplicates
   and can drift from a future server-added field.

---

## Changed Assumptions (vs. the prior lightweight pass)

The prior `feature-delta.md` (D3 = lightweight) stated: "this pass formalizes
`verify-the-books.yaml` into stories, it does not re-research the journey."
That assumption is superseded here per the corrected Decision 3
("Comprehensive"). The original journey document
(`docs/product/journeys/verify-the-books.yaml`) is not wrong — its S1-S4
happy path and dip-and-recover arc hold up under deeper interrogation — but
it was written at the HTTP-contract level (a curl-shaped mental model) and
never had to consider states a real browser page introduces: loading,
staleness, and a second, harder-hitting failure branch on the entries fetch.
This pass adds those states without discarding or contradicting the
original.
