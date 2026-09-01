// k6 load/stress/soak/tier-sweep/success profiles across the full
// driving-port surface (OPS-12, feature-delta.md, amended 2026-08-31 --
// supersedes the 2026-08-29 POST /transfers-only script).
//
// Two-actor model traced to docs/product/journeys/post-a-transfer.yaml (P1,
// integrating developer) and verify-the-books.yaml (P2, platform operator).
// See feature-delta.md § "Load, stress, soak, and success testing" for
// full rationale on the endpoint mix, account skew, amount buckets,
// idempotency-replay/insufficient-funds slices, and think-time.
//
// PROFILE env var selects the run shape (default "load"):
//   load        -- short ramp, sustained steady stage, ramp-down (ramping-vus).
//   stress      -- ramps beyond what `load` sustains (ramping-vus).
//   soak        -- longest duration, moderate sustained load (ramping-vus).
//   tier-sweep  -- escalating ramping-arrival-rate, to find the saturation
//                  throughput of whichever hardware tier the compose stack
//                  is currently constrained to (tier limits are applied at
//                  the compose/workflow level, not in this script -- see
//                  load-test.yml). Informational; not gated.
//   success     -- fixed-rate ramping-arrival-rate at SUCCESS_TARGET_RPS,
//                  the one profile with abortOnFail: true (a real,
//                  enforced SLO -- feature-delta.md § "Success scenario").
import http from "k6/http";
import { check, sleep } from "k6";

const BASE_URL = "http://localhost:8080";
const AUTH_HEADER = { Authorization: "Bearer demo-operator-key" };
const JSON_HEADERS = { ...AUTH_HEADER, "Content-Type": "application/json" };

// Deliberate 422 (insufficient_funds) and idempotency-replay responses are
// expected, not failures -- excluded here so http_req_failed reflects real
// errors only. Blanket across the whole script (see feature-delta.md's
// stated tradeoff: a genuine unexpected 422 elsewhere would be masked).
http.setResponseCallback(http.expectedStatuses(200, 201, 422));

// Live-traffic account pool -- disjoint from the populated-at-scale seed
// data (seed-0000..seed-0249, load-test.yml's seed step). Backfilled
// history and live traffic are deliberately not the same accounts.
const HOT_ACCOUNTS = ["k6-hot-00", "k6-hot-01", "k6-hot-02", "k6-hot-03"];
const COLD_ACCOUNTS = Array.from(
  { length: 16 },
  (_, i) => `k6-cold-${String(i).padStart(2, "0")}`,
);
const ALL_ACCOUNTS = [...HOT_ACCOUNTS, ...COLD_ACCOUNTS];

const RAMPING_VUS_STAGES = {
  load: [
    { duration: "15s", target: 10 }, // ramp-up
    { duration: "1m", target: 10 }, // steady
    { duration: "15s", target: 0 }, // ramp-down
  ],
  stress: [
    { duration: "15s", target: 10 },
    { duration: "30s", target: 40 }, // beyond load's steady 10 VUs
    { duration: "1m", target: 40 },
    { duration: "15s", target: 0 },
  ],
  soak: [
    { duration: "15s", target: 8 }, // moderate, sustainable load
    { duration: "10m", target: 8 }, // longest sustain of the three
    { duration: "15s", target: 0 },
  ],
};

// Escalating rate (req/s), further and finer-grained than `stress`, to find
// each tier's saturation point. Read the per-stage plateau back from
// Prometheus (feature-delta.md § "Per-tier measurement") rather than from
// k6's own end-of-run summary.
const TIER_SWEEP_STAGES = [
  { duration: "30s", target: 5 },
  { duration: "30s", target: 10 },
  { duration: "30s", target: 20 },
  { duration: "30s", target: 40 },
  { duration: "30s", target: 80 },
  { duration: "30s", target: 160 },
  { duration: "30s", target: 320 },
];

// Placeholder until the Tier 4 tier-sweep has been run once and its
// measured plateau recorded (feature-delta.md § "Success scenario" -- the
// 579 rps business target is deliberately NOT this number; see that
// section for why the two are decoupled).
const SUCCESS_TARGET_RPS = Number(__ENV.SUCCESS_TARGET_RPS) || 10;

function arrivalRateScenario(name, stages) {
  return {
    [name]: {
      executor: "ramping-arrival-rate",
      startRate: 1,
      timeUnit: "1s",
      preAllocatedVUs: 50,
      maxVUs: 400,
      stages,
    },
  };
}

const profile = __ENV.PROFILE || "load";

let resolvedOptions;
if (profile === "tier-sweep") {
  resolvedOptions = {
    scenarios: arrivalRateScenario("tier_sweep", TIER_SWEEP_STAGES),
    thresholds: {
      http_req_duration: [{ threshold: "p(95)<500", abortOnFail: false }],
      http_req_failed: [{ threshold: "rate<0.01", abortOnFail: false }],
    },
  };
} else if (profile === "fixed-rate") {
  // Single short hold at one requested rate (RATE env, req/s; DURATION env,
  // default 30s). Not meant to be run by hand -- tests/load/analysis/
  // run_usl_sweep.py invokes this repeatedly, once per rate step per tier,
  // so it can read one clean k6 summary per step (metrics.http_reqs.rate,
  // metrics.http_req_duration["p(95)"], metrics.http_req_failed.value --
  // verified against a real k6 v2.2.0 run) and decide in code whether
  // throughput has plateaued, instead of a human reading a continuous
  // `tier-sweep` ramp off a dashboard.
  const rate = Number(__ENV.RATE) || 10;
  const duration = __ENV.DURATION || "30s";
  resolvedOptions = {
    scenarios: arrivalRateScenario("fixed_rate", [{ duration, target: rate }]),
    thresholds: {
      http_req_duration: [{ threshold: "p(95)<500", abortOnFail: false }],
      http_req_failed: [{ threshold: "rate<0.01", abortOnFail: false }],
    },
  };
} else if (profile === "success") {
  resolvedOptions = {
    scenarios: arrivalRateScenario("success", [
      { duration: "5m", target: SUCCESS_TARGET_RPS },
    ]),
    thresholds: {
      http_req_duration: [{ threshold: "p(95)<500", abortOnFail: true }],
      http_req_failed: [{ threshold: "rate<0.01", abortOnFail: true }],
    },
  };
} else {
  resolvedOptions = {
    stages: RAMPING_VUS_STAGES[profile] || RAMPING_VUS_STAGES.load,
    thresholds: {
      http_req_duration: [{ threshold: "p(95)<500", abortOnFail: false }],
      http_req_failed: [{ threshold: "rate<0.01", abortOnFail: false }],
    },
  };
}

export const options = resolvedOptions;

function createAccount(accountId, type) {
  http.post(
    `${BASE_URL}/accounts`,
    JSON.stringify({ account_id: accountId, type }),
    { headers: JSON_HEADERS },
  );
}

export function setup() {
  createAccount("treasury", "system");
  for (const acct of ALL_ACCOUNTS) {
    createAccount(acct, "wallet");
  }
  // Fund every account generously up front, so the insufficient-funds slice
  // below is the only path that can produce a 422 -- ordinary traffic must
  // not accidentally starve an account mid-run.
  for (const acct of ALL_ACCOUNTS) {
    http.post(
      `${BASE_URL}/transfers`,
      JSON.stringify({ from: "treasury", to: acct, amount: "1000000.00" }),
      { headers: { ...JSON_HEADERS, "Idempotency-Key": `fund-${acct}` } },
    );
  }
}

// 80/20 skew: the 4 hot accounts (20% of the pool) receive 80% of traffic.
function pickAccount() {
  return Math.random() < 0.8
    ? HOT_ACCOUNTS[Math.floor(Math.random() * HOT_ACCOUNTS.length)]
    : COLD_ACCOUNTS[Math.floor(Math.random() * COLD_ACCOUNTS.length)];
}

function pickCounterparty(exclude) {
  let acct;
  do {
    acct = pickAccount();
  } while (acct === exclude);
  return acct;
}

// Three amount buckets, matching feature-delta.md § "Actor model and
// endpoint mix" -- reused (not reinvented) for the populated-at-scale seed
// data in load-test.yml's seed step.
function pickAmountMinor() {
  const r = Math.random();
  if (r < 0.7) return 1 + Math.floor(Math.random() * 1000); // $0.01-$10.00
  if (r < 0.95) return 1000 + Math.floor(Math.random() * 49000); // $10-$500
  return 50000 + Math.floor(Math.random() * 450000); // $500-$5000
}

function minorToDecimal(minor) {
  const abs = Math.abs(minor);
  return `${Math.floor(abs / 100)}.${String(abs % 100).padStart(2, "0")}`;
}

// Bounded per-VU cache of successfully-posted idempotency keys, so the
// replay slice below has something real to replay. Bounded to avoid
// unbounded per-VU memory growth over `soak`'s longer duration.
const replayKeys = [];

function postTransfer() {
  const from = pickAccount();
  const to = pickCounterparty(from);
  const roll = Math.random();

  // ~2% idempotency-replay slice (I7, ADR-005): reuse a prior key, expect
  // the identical transaction_id back, not a fresh posting.
  if (roll < 0.02 && replayKeys.length > 0) {
    const key = replayKeys[Math.floor(Math.random() * replayKeys.length)];
    const res = http.post(
      `${BASE_URL}/transfers`,
      JSON.stringify({ from, to, amount: minorToDecimal(pickAmountMinor()) }),
      { headers: { ...JSON_HEADERS, "Idempotency-Key": key } },
    );
    check(res, {
      "replay: 200 or 201": (r) => r.status === 200 || r.status === 201,
    });
    return;
  }

  // ~1.5% deliberate insufficient-funds slice, on top of the 2% replay
  // slice above (roughly 2%-3.5% of rolls).
  const forceInsufficientFunds = roll >= 0.02 && roll < 0.035;
  const amountMinor = forceInsufficientFunds
    ? 999999999999 // implausibly large, exceeds any funded/seeded balance
    : pickAmountMinor();
  const key = `k6-${__VU}-${__ITER}-${Date.now()}`;

  const res = http.post(
    `${BASE_URL}/transfers`,
    JSON.stringify({ from, to, amount: minorToDecimal(amountMinor) }),
    { headers: { ...JSON_HEADERS, "Idempotency-Key": key } },
  );

  if (forceInsufficientFunds) {
    check(res, { "insufficient_funds: 422": (r) => r.status === 422 });
    return;
  }

  check(res, {
    "status is 200 or 201": (r) => r.status === 200 || r.status === 201,
  });
  if (res.status === 200 || res.status === 201) {
    replayKeys.push(key);
    if (replayKeys.length > 500) replayKeys.shift();
  }
}

function getBalance() {
  const res = http.get(`${BASE_URL}/accounts/${pickAccount()}`, {
    headers: AUTH_HEADER,
  });
  check(res, { "balance: 200": (r) => r.status === 200 });
}

function getEntries() {
  const res = http.get(`${BASE_URL}/accounts/${pickAccount()}/entries`, {
    headers: AUTH_HEADER,
  });
  check(res, { "entries: 200": (r) => r.status === 200 });
}

function getVerdict() {
  const res = http.get(`${BASE_URL}/console/verdict`, {
    headers: AUTH_HEADER,
  });
  check(res, { "verdict: 200": (r) => r.status === 200 });
}

let accountSeq = 0;
function openAccount() {
  accountSeq += 1;
  const id = `k6-new-${__VU}-${__ITER}-${accountSeq}`;
  const res = http.post(
    `${BASE_URL}/accounts`,
    JSON.stringify({ account_id: id, type: "wallet" }),
    { headers: JSON_HEADERS },
  );
  // 409 (account_already_exists, DDD-18) is accepted alongside 201: two VUs
  // landing on the same __VU/__ITER/accountSeq combination is vanishingly
  // unlikely but not impossible; the point of this slice is onboarding
  // traffic, not exercising DDD-18, which already has its own coverage.
  check(res, {
    "open account: 201 or 409": (r) => r.status === 201 || r.status === 409,
  });
}

function defaultIteration() {
  const r = Math.random();
  if (r < 0.6) {
    postTransfer();
    sleep(1 + Math.random() * 2); // P1, 1-3s
  } else if (r < 0.75) {
    getBalance();
    sleep(1 + Math.random() * 2); // P1, 1-3s
  } else if (r < 0.85) {
    getEntries();
    sleep(5 + Math.random() * 10); // P2, 5-15s
  } else if (r < 0.95) {
    getVerdict();
    sleep(5 + Math.random() * 10); // P2, 5-15s
  } else {
    openAccount();
    sleep(1 + Math.random() * 2); // P1, 1-3s
  }
}

export default function () {
  defaultIteration();
}
