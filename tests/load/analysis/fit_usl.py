#!/usr/bin/env python3
"""Fit Gunther's Universal Scalability Law to hardware-tier measurements.

OPS-12 (feature-delta.md, § "Hardware-tier extrapolation", amended
2026-08-31). N here is a hardware tier (vCPU/mem allocation), not
concurrency -- each row in the input CSV is one tier's measured saturation
throughput from a `tier-sweep` k6 run (see tests/load/transfers.js and
.github/workflows/load-test.yml). Concurrency/arrival-rate is only the
mechanism used to find that plateau within a single tier; it is not the N
this script fits against.

Checked in, manually invoked. Not wired into any workflow -- capacity
planning here is an occasional exercise, not a recurring gate.

Method: Gunther's linearization, not a direct nonlinear fit to
    X(N) = lambda*N / (1 + alpha*(N-1) + beta*N*(N-1))
Rearranged into a straight line over the *relative capacity* C(N) = X(N)/X(1):
    y(N) = (N/C(N) - 1) / (N - 1) = alpha + beta*N      (N > 1)
alpha (contention) and beta (coherency) come out of an ordinary least-squares
fit of y against N; lambda is just X(1), the baseline tier's own throughput.

Usage:
    python3 fit_usl.py measurements.csv
    python3 fit_usl.py measurements.csv --target-rps 579

CSV columns (header required): tier,throughput_rps[,p95_latency_ms,source]
  - tier: N, the hardware tier index (must include tier 1 as baseline)
  - throughput_rps: X(N), that tier's measured saturation throughput
  - p95_latency_ms: optional, informational only, not used in the fit
  - source: optional, e.g. "ci" or "local"; local/non-CI rows are only
    flagged in the report, never silently dropped or silently weighted
    differently -- dropping data ad hoc would misrepresent what was
    actually measured.
"""
import argparse
import csv
import sys


def read_measurements(path):
    rows = []
    with open(path, newline="") as f:
        reader = csv.DictReader(f)
        for r in reader:
            rows.append(
                {
                    "tier": int(r["tier"]),
                    "throughput_rps": float(r["throughput_rps"]),
                    "p95_latency_ms": float(r["p95_latency_ms"])
                    if r.get("p95_latency_ms")
                    else None,
                    "source": r.get("source") or "unspecified",
                }
            )
    rows.sort(key=lambda r: r["tier"])
    return rows


def least_squares_line(xs, ys):
    """Ordinary least squares fit of y = a + b*x. Returns (a, b)."""
    m = len(xs)
    sum_x = sum(xs)
    sum_y = sum(ys)
    sum_xy = sum(x * y for x, y in zip(xs, ys))
    sum_xx = sum(x * x for x in xs)
    denom = m * sum_xx - sum_x * sum_x
    if denom == 0:
        raise ValueError(
            "degenerate fit: all tier values identical, or only one point supplied"
        )
    b = (m * sum_xy - sum_x * sum_y) / denom
    a = (sum_y - b * sum_x) / m
    return a, b


def fit_usl(rows):
    baseline = next((r for r in rows if r["tier"] == 1), None)
    if baseline is None:
        raise ValueError(
            "no tier=1 row found -- USL's linearization needs the N=1 baseline "
            "throughput (lambda = X(1)); cannot fit without it"
        )
    x1 = baseline["throughput_rps"]
    if x1 <= 0:
        raise ValueError("tier=1 throughput must be positive")

    points = [r for r in rows if r["tier"] > 1]
    if len(points) < 2:
        raise ValueError(
            f"need at least 2 tiers above the N=1 baseline to fit alpha and beta "
            f"(2 unknowns); got {len(points)}. With exactly 2 the fit has 0 "
            f"degrees of freedom -- it will run, but there is no way to judge "
            f"fit quality."
        )

    xs, ys = [], []
    for r in points:
        n = r["tier"]
        c_n = r["throughput_rps"] / x1
        if c_n <= 0:
            raise ValueError(f"tier={n} has non-positive relative capacity; check input")
        y = (n / c_n - 1) / (n - 1)
        xs.append(n)
        ys.append(y)

    alpha, beta = least_squares_line(xs, ys)
    lam = x1
    dof = len(points) - 2
    return {
        "lambda": lam,
        "alpha": alpha,
        "beta": beta,
        "n_points_above_baseline": len(points),
        "degrees_of_freedom": dof,
    }


def usl_throughput(n, lam, alpha, beta):
    denom = 1 + alpha * (n - 1) + beta * n * (n - 1)
    if denom <= 0:
        return float("inf")  # model breakdown region; not physically meaningful
    return lam * n / denom


def project_tier_for_target(lam, alpha, beta, target_rps, max_n=10000, step=0.01):
    """Scan upward for the first N whose modeled throughput reaches target_rps.

    A linear scan, not an analytic inverse: X(N) is only unimodal when
    beta > 0 (real coherency overhead), and even then a closed-form inverse
    isn't worth the complexity for a once-in-a-while, manually-run script.
    """
    peak_n, peak_x = 1.0, usl_throughput(1.0, lam, alpha, beta)
    n = 1.0
    while n <= max_n:
        x = usl_throughput(n, lam, alpha, beta)
        if x > peak_x:
            peak_n, peak_x = n, x
        if x >= target_rps:
            return n, peak_n, peak_x
        n += step
    return None, peak_n, peak_x


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("csv_path", help="path to the tier-measurements CSV")
    parser.add_argument(
        "--target-rps",
        type=float,
        default=None,
        help="if given, project the tier needed to reach this throughput (e.g. 579 for the OPS-12 success-scenario business target)",
    )
    args = parser.parse_args()

    rows = read_measurements(args.csv_path)
    non_ci = [r for r in rows if r["source"] not in ("ci", "unspecified")]

    print(f"Loaded {len(rows)} tier measurement(s) from {args.csv_path}")
    for r in rows:
        lat = f", p95={r['p95_latency_ms']:.0f}ms" if r["p95_latency_ms"] is not None else ""
        print(f"  tier={r['tier']:>2}  X={r['throughput_rps']:>8.2f} rps{lat}  source={r['source']}")
    if non_ci:
        print(
            f"\nNOTE: {len(non_ci)} row(s) are not from the CI-hosted tier sweep "
            f"(source != 'ci'). Per feature-delta.md's supplementary-data-point "
            f"rule, these are included in the fit below but are NOT reproducible "
            f"by third parties and must be labeled as such in any report."
        )

    try:
        fit = fit_usl(rows)
    except ValueError as e:
        print(f"\nERROR: {e}", file=sys.stderr)
        sys.exit(1)

    print("\n--- USL fit (Gunther's linearization) ---")
    print(f"lambda (baseline throughput, X(1)) : {fit['lambda']:.4f} rps")
    print(f"alpha  (contention)                : {fit['alpha']:.6f}")
    print(f"beta   (coherency)                 : {fit['beta']:.6f}")
    print(f"points above N=1 baseline used     : {fit['n_points_above_baseline']}")
    print(f"degrees of freedom                 : {fit['degrees_of_freedom']}")

    if fit["degrees_of_freedom"] <= 1:
        print(
            "\nCAUTION: degrees of freedom <= 1. This is a thin basis for a "
            "2-parameter fit -- per feature-delta.md, treat this curve as "
            "DIRECTIONAL ONLY, not a confidence-bounded prediction. Do not "
            "present it to a technical evaluator without this caveat attached."
        )

    if fit["beta"] > 0:
        peak_n = ((1 - fit["alpha"]) / fit["beta"]) ** 0.5
        peak_x = usl_throughput(peak_n, fit["lambda"], fit["alpha"], fit["beta"])
        print(
            f"\nModel predicts a throughput PEAK at N*={peak_n:.2f} "
            f"(X(N*)={peak_x:.2f} rps) -- retrograde scalability beyond that "
            f"point (coherency overhead dominates). This is what beta>0 means."
        )
    else:
        print(
            "\nbeta <= 0 in this fit: no retrograde region predicted within the "
            "model as fit. With this few points, a non-positive beta is easily "
            "an artifact of measurement noise, not evidence that coherency "
            "overhead is truly absent -- do not report this as a finding on its "
            "own."
        )

    if args.target_rps is not None:
        n_needed, peak_n, peak_x = project_tier_for_target(
            fit["lambda"], fit["alpha"], fit["beta"], args.target_rps
        )
        print(f"\n--- Projection for target {args.target_rps:.0f} rps ---")
        if n_needed is None:
            print(
                f"NOT REACHABLE within the scanned range under this fit: modeled "
                f"peak throughput is {peak_x:.2f} rps at tier N*={peak_n:.2f}, "
                f"below the {args.target_rps:.0f} rps target."
            )
        else:
            print(
                f"Projected tier: N~={n_needed:.2f} "
                f"(UNVALIDATED beyond the measured range {rows[0]['tier']}-"
                f"{rows[-1]['tier']}; directional only, per the degrees-of-"
                f"freedom caution above)."
            )


if __name__ == "__main__":
    main()
