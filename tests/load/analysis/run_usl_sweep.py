#!/usr/bin/env python3
"""Fully automated hardware-tier sweep for the USL fit (OPS-12,
feature-delta.md § "Hardware-tier extrapolation", amended 2026-08-31).

Owns the entire measurement loop so a human never has to trigger four
separate workflow runs, read a Grafana dashboard by eye, or hand-fill a
CSV: for each tier 1-4, it resizes the running app+postgres containers to
that tier's CPU/mem cap, waits for the app to answer again, then steps
`transfers.js`'s `fixed-rate` profile through an escalating list of rates,
reading k6's own --summary-export JSON after each step to decide -- in
code, not by eye -- whether throughput has plateaued. One CSV row per tier
comes out the other end, then fit_usl.py runs automatically against it.

Assumes the compose stack (postgres, migrate, app, prometheus, grafana) is
already up, unconstrained, and already seeded to populated-at-scale before
this script starts -- see .github/workflows/load-test.yml's `usl-sweep`
profile, which does both before invoking this script. Postgres's data lives
in a named volume, so recreating the app/postgres containers between tiers
(to apply new resource limits) does not lose the seed data or require
reseeding per tier.

k6 summary JSON schema used here (verified against a real k6 v2.2.0 run,
2026-08-31 -- NOT assumed from docs, which vary across k6 versions):
    metrics.http_reqs.rate            -> achieved throughput, req/s
    metrics.http_req_duration["p(95)"] -> p95 latency, ms
    metrics.http_req_failed.value      -> error rate, 0-1 fraction
    metrics.dropped_iterations.count   -> ABSENT entirely when zero, not 0
If a future k6 upgrade changes this shape, this script will raise a
KeyError loudly rather than silently mis-measure -- see parse_k6_summary.
"""
import argparse
import json
import subprocess
import sys
import time
import urllib.error
import urllib.request

# Must stay in sync with the manual `tier` case statement in
# .github/workflows/load-test.yml -- duplicated there for the human-
# triggered single-tier `tier-sweep` debug path, defined here as the single
# source of truth for the fully-automated `usl-sweep` path. A drift between
# the two would only affect which CPU/mem numbers a *manual* single-tier
# debug run uses; it would not corrupt this script's own sweep.
TIERS = {
    1: {"app_cpus": "0.25", "app_mem": "512m", "db_cpus": "0.25", "db_mem": "1g"},
    2: {"app_cpus": "0.5", "app_mem": "1g", "db_cpus": "0.5", "db_mem": "2g"},
    3: {"app_cpus": "1.0", "app_mem": "2g", "db_cpus": "1.0", "db_mem": "3g"},
    4: {"app_cpus": "1.5", "app_mem": "3g", "db_cpus": "1.5", "db_mem": "4g"},
}

RATE_STEPS_RPS = [5, 10, 20, 40, 80, 160, 320]
STEP_DURATION = "30s"
GROWTH_PLATEAU_THRESHOLD = 0.05  # <5% throughput growth vs. previous step
ERROR_BREAKDOWN_THRESHOLD = 0.05  # 5% error rate -- a different, higher bar
# than the success-scenario's 1% SLO; this one only decides "has this rate
# step broken the system badly enough to stop escalating," not "did this
# tier pass its SLO."
WAIT_FOR_APP_TIMEOUT_S = 60
# First live run (2026-08-31) timed out here at 60s -- not because Postgres
# was slow (its log showed a clean shutdown and readiness within ~150ms
# even at Tier 1's 0.25 vCPU), but because resize_to_tier's docker compose
# command left `migrate` out of the up invocation, bypassing app's actual
# depends_on chain entirely and letting it race a not-yet-listening
# postgres. That race is now fixed in resize_to_tier itself (migrate is
# back in the command, restoring the same dependency gate the workflow's
# cold-start step already relies on). This timeout stays generous as a
# safety margin for genuine slow recovery under a throttled tier, not
# because that's the bug that was actually observed -- 240s costs nothing
# in the success case (wait_for_app returns as soon as the app answers).
WAIT_FOR_APP_AFTER_RESIZE_TIMEOUT_S = 240
OVERRIDE_FILE = "docker-compose.tier-override.yml"
COMPOSE_FILE = "docker-compose.yml"
BASE_URL = "http://localhost:8080"
AUTH_HEADER = "Bearer demo-operator-key"


def write_override(tier):
    t = TIERS[tier]
    content = (
        "services:\n"
        "  app:\n"
        f"    cpus: {t['app_cpus']}\n"
        f"    mem_limit: {t['app_mem']}\n"
        "  postgres:\n"
        f"    cpus: {t['db_cpus']}\n"
        f"    mem_limit: {t['db_mem']}\n"
    )
    with open(OVERRIDE_FILE, "w") as f:
        f.write(content)


def resize_to_tier(tier):
    write_override(tier)
    print(f"[tier {tier}] applying {TIERS[tier]}", flush=True)
    # `app` does not depend on `postgres` directly (docker-compose.yml) --
    # it depends on `migrate` with condition: service_completed_successfully,
    # and `migrate` is what actually waits on postgres's own healthcheck
    # (pg_isready, 20 retries). `--no-deps app postgres` (the first live-run
    # attempt, 2026-08-31) left `migrate` out of the command entirely, which
    # meant compose enforced NO readiness gate between app and postgres at
    # all -- app started racing a freshly-recreated postgres with zero
    # ordering guarantee, lost that race by ~130ms, and (both app and
    # migrate are `restart: "no"`) just stayed dead for the rest of the
    # wait window. Listing `migrate` here and dropping --no-deps restores
    # the same dependency chain the workflow's own cold-start step already
    # relies on -- migrate is idempotent against an up-to-date schema
    # (expand-only migrations), so re-running it on every tier resize is
    # safe and fast, not a correctness risk.
    subprocess.run(
        [
            "docker",
            "compose",
            "-f",
            COMPOSE_FILE,
            "-f",
            OVERRIDE_FILE,
            "up",
            "-d",
            "postgres",
            "migrate",
            "app",
        ],
        check=True,
    )


def wait_for_app(timeout_s=WAIT_FOR_APP_TIMEOUT_S):
    deadline = time.time() + timeout_s
    url = f"{BASE_URL}/accounts/__probe__"
    while time.time() < deadline:
        try:
            req = urllib.request.Request(url, headers={"Authorization": AUTH_HEADER})
            urllib.request.urlopen(req, timeout=3)
            return
        except urllib.error.HTTPError:
            # Any HTTP response at all (even 404/401) means the app answered.
            return
        except (urllib.error.URLError, OSError):
            time.sleep(1)

    # Unlike the workflow's own cold-start wait step, a post-resize wait has
    # no visibility into *why* it timed out -- was Postgres still replaying
    # WAL after a SIGKILL-forced restart under a throttled tier, or is the
    # app crash-looping while it waits for a DB that isn't ready yet? Dump
    # both, mirroring the pattern the original wait-for-app workflow step
    # already used, so a real CI failure is diagnosable from the job log
    # directly instead of a bare exception (this exact gap was hit on the
    # first live run, 2026-08-31 -- see feature-delta.md's amendment note
    # for what this timeout actually needs to cover under a throttled tier).
    print(f"\napp never answered within {timeout_s}s -- dumping container logs for diagnosis:", flush=True)
    subprocess.run(["docker", "compose", "-f", COMPOSE_FILE, "logs", "--tail=100", "app", "postgres"])
    raise RuntimeError(f"app never answered at {url} within {timeout_s}s")


def parse_k6_summary(path):
    with open(path) as f:
        d = json.load(f)
    m = d["metrics"]
    return {
        "throughput_rps": m["http_reqs"]["rate"],
        "p95_latency_ms": m["http_req_duration"]["p(95)"],
        "error_rate": m["http_req_failed"]["value"],
        "dropped_iterations": m.get("dropped_iterations", {}).get("count", 0),
    }


def run_fixed_rate_step(rate, summary_path):
    env = {"PROFILE": "fixed-rate", "RATE": str(rate), "DURATION": STEP_DURATION}
    import os

    full_env = {**os.environ, **env}
    print(f"    running fixed-rate step at {rate} rps...", flush=True)
    proc = subprocess.run(
        [
            "k6",
            "run",
            f"--summary-export={summary_path}",
            "tests/load/transfers.js",
        ],
        env=full_env,
        capture_output=True,
        text=True,
    )
    # k6 exits 99 specifically when a threshold it was tracking got
    # breached during the run (ExitCodeThresholdsHaveFailed) -- during a
    # sweep that is expected, informative data (the whole point is to push
    # load until something breaks), not a script failure, and the summary
    # file is still written normally either way. `check=True` here (first
    # live sweep run, 2026-09-01) treated that routine outcome as a fatal
    # exception and crashed on the very first rate step. Any OTHER
    # non-zero exit means the run itself didn't complete as expected (a
    # script error, a setup() failure, etc.) and the summary may not exist
    # or be trustworthy, so those still raise.
    if proc.returncode not in (0, 99):
        print(proc.stdout)
        print(proc.stderr, file=sys.stderr)
        raise RuntimeError(
            f"k6 exited {proc.returncode} at rate={rate}rps -- not a threshold "
            f"breach (that's 99), treating this as a genuine failure, not sweep data"
        )
    if proc.returncode == 99:
        print(
            f"    (k6 exit 99: a threshold was breached at {rate}rps -- expected "
            f"during a sweep, continuing)",
            flush=True,
        )
    return parse_k6_summary(summary_path)


def sweep_one_tier(tier):
    resize_to_tier(tier)
    wait_for_app(timeout_s=WAIT_FOR_APP_AFTER_RESIZE_TIMEOUT_S)

    prev = None
    for i, rate in enumerate(RATE_STEPS_RPS):
        summary_path = f"k6-usl-tier{tier}-step{i}.json"
        result = run_fixed_rate_step(rate, summary_path)
        print(
            f"    -> requested={rate}rps achieved={result['throughput_rps']:.2f}rps "
            f"p95={result['p95_latency_ms']:.0f}ms err={result['error_rate']:.4f} "
            f"dropped={result['dropped_iterations']}",
            flush=True,
        )

        if result["error_rate"] > ERROR_BREAKDOWN_THRESHOLD:
            if prev is not None:
                print(
                    f"    error rate {result['error_rate']:.2%} exceeds breakdown "
                    f"threshold at {rate}rps -- using previous step ({prev['throughput_rps']:.2f}rps) as the plateau",
                    flush=True,
                )
                return prev
            print(
                f"    error rate {result['error_rate']:.2%} exceeds breakdown "
                f"threshold on the FIRST step ({rate}rps) -- this tier cannot "
                f"even sustain the lowest tested rate; recording it anyway so "
                f"the gap is visible, not silently dropped",
                flush=True,
            )
            return result

        if prev is not None and prev["throughput_rps"] > 0:
            growth = (result["throughput_rps"] - prev["throughput_rps"]) / prev["throughput_rps"]
            if growth < GROWTH_PLATEAU_THRESHOLD:
                print(
                    f"    throughput growth {growth:.2%} below the "
                    f"{GROWTH_PLATEAU_THRESHOLD:.0%} plateau threshold -- "
                    f"treating {rate}rps as this tier's plateau",
                    flush=True,
                )
                if result["dropped_iterations"] > 0:
                    print(
                        f"    WARNING: {result['dropped_iterations']} dropped "
                        f"iteration(s) at the plateau step -- k6 itself may have "
                        f"run out of headroom on this shared runner rather than "
                        f"the target system saturating. Treat this plateau with "
                        f"extra caution (feature-delta.md's documented risk).",
                        flush=True,
                    )
                return result

        prev = result

    print(
        f"    reached the top of the tested rate range ({RATE_STEPS_RPS[-1]}rps) "
        f"without detecting a plateau -- this tier's true capacity may exceed "
        f"what was tested. Recording the last step, but this is a known gap, "
        f"not a confident measurement.",
        flush=True,
    )
    return prev


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--out-csv", default="tier-measurements.csv")
    parser.add_argument("--target-rps", type=float, default=579.0)
    parser.add_argument(
        "--fit-script",
        default="tests/load/analysis/fit_usl.py",
        help="path to fit_usl.py, run automatically once all tiers are measured",
    )
    parser.add_argument(
        "--summary-file",
        default=None,
        help="if given (e.g. $GITHUB_STEP_SUMMARY), the fit_usl.py report is appended here too",
    )
    args = parser.parse_args()

    rows = []
    for tier in sorted(TIERS):
        print(f"\n=== Tier {tier} ===", flush=True)
        plateau = sweep_one_tier(tier)
        rows.append(
            {
                "tier": tier,
                "throughput_rps": plateau["throughput_rps"],
                "p95_latency_ms": plateau["p95_latency_ms"],
                "source": "ci",
            }
        )

    with open(args.out_csv, "w") as f:
        f.write("tier,throughput_rps,p95_latency_ms,source\n")
        for r in rows:
            f.write(f"{r['tier']},{r['throughput_rps']:.4f},{r['p95_latency_ms']:.2f},{r['source']}\n")
    print(f"\nWrote {len(rows)} tier measurement(s) to {args.out_csv}", flush=True)

    fit_result = subprocess.run(
        [sys.executable, args.fit_script, args.out_csv, "--target-rps", str(args.target_rps)],
        capture_output=True,
        text=True,
    )
    print(fit_result.stdout)
    if fit_result.stderr:
        print(fit_result.stderr, file=sys.stderr)

    if args.summary_file:
        with open(args.summary_file, "a") as f:
            f.write("## USL sweep results\n\n```\n")
            f.write(fit_result.stdout)
            f.write("\n```\n")


if __name__ == "__main__":
    main()
