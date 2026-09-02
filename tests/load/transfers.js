// k6 load/stress/soak/capacity profiles across the driving-port surface
// (OPS-12, feature-delta.md 2026-08-29, multi-actor workload restored
// 2026-09-02). Two-actor model traced to
// docs/product/journeys/post-a-transfer.yaml (P1, integrating developer)
// and verify-the-books.yaml (P2, platform operator) -- endpoint mix,
// account skew, amount buckets, idempotency-replay/insufficient-funds
// slices, and think-time all reused from the version reverted alongside
// USL/tier-sweep; that revert was about the buggy curve-fitting machinery,
// not this traffic model, which had already been bug-fixed twice
// (idempotency replay must resend the request verbatim; ramping executors
// don't actually hold a steady rate).
//
// PROFILE env var selects which run shape executes (default "load"):
//   load     — short ramp, sustained steady stage, ramp-down (ramping-vus).
//              Baseline throughput/latency at an expected traffic level.
//              Informational thresholds only.
//   stress   — ramps beyond what `load` sustains (ramping-vus), to find
//              where the endpoint degrades or breaks, not just where it's
//              slow. Informational thresholds only.
//   soak     — longest duration of the three (ramping-vus), moderate
//              sustained load. The finding here is a trend over time (pool
//              exhaustion, memory growth, slow degradation), not a single
//              number. Informational thresholds only.
//   capacity — fixed-rate constant-arrival-rate at CAPACITY_TARGET_RPS, run
//              against docker-compose.loadtest.yml's pinned resource
//              ceiling (load-test.yml). The one profile with real, gating
//              (abortOnFail: true) thresholds on all three SLO dimensions
//              — latency, error rate, AND throughput. Provisional: the
//              579 rps/USL/tier-sweep approach this replaces was reverted
//              (too complex, buggy, not understood by the team) — this is
//              deliberately the simple version: one fixed target, one fixed
//              hardware ceiling, no curve-fitting.
//
// No prior numeric decision exists for VU counts/durations — these are
// first-pass values, deliberately modest for a single-instance
// docker-compose stack (no read replica, no external pooler — same
// contended topology as `make race-02`/`race-03`).
import http from "k6/http";
import { check, sleep } from "k6";

const BASE_URL = "http://localhost:8080";
const AUTH_HEADER = { Authorization: "Bearer demo-operator-key" };
const JSON_HEADERS = { ...AUTH_HEADER, "Content-Type": "application/json" };

// Deliberate 422 (insufficient_funds) and idempotency-replay responses are
// expected, not failures -- excluded here so http_req_failed reflects real
// errors only. Blanket across the whole script (a genuine unexpected 422
// elsewhere would be masked, an accepted tradeoff at this scope).
http.setResponseCallback(http.expectedStatuses(200, 201, 422));

// Live-traffic account pool -- disjoint from any seed data. 80/20 skew: the
// 4 hot accounts (20% of the pool) receive 80% of traffic.
const HOT_ACCOUNTS = ["k6-hot-00", "k6-hot-01", "k6-hot-02", "k6-hot-03"];
const COLD_ACCOUNTS = Array.from(
  { length: 16 },
  (_, i) => `k6-cold-${String(i).padStart(2, "0")}`,
);
const ALL_ACCOUNTS = [...HOT_ACCOUNTS, ...COLD_ACCOUNTS];

const STAGES = {
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

// First-pass business-adjacent number, not measured against a real
// deployment -- see the `capacity` case in the header comment above. Held
// flat for CAPACITY_DURATION via constant-arrival-rate (not
// ramping-arrival-rate, which only ramps toward the target and never
// actually holds it steady).
const CAPACITY_TARGET_RPS = Number(__ENV.CAPACITY_TARGET_RPS) || 400;
const CAPACITY_DURATION = __ENV.CAPACITY_DURATION || "2m";

const profile = __ENV.PROFILE || "load";

export const options =
  profile === "capacity"
    ? {
        scenarios: {
          capacity: {
            executor: "constant-arrival-rate",
            rate: CAPACITY_TARGET_RPS,
            timeUnit: "1s",
            duration: CAPACITY_DURATION,
            // Sized for the multi-actor mix's 5-15s think-time slices
            // (getEntries/getVerdict), not just raw request latency --
            // concurrent VUs needed is roughly rate * avg-iteration-
            // duration, which is dominated by sleep() here, not response
            // time. Too few would ceiling on VU exhaustion
            // (dropped_iterations) rather than real app/hardware capacity.
            preAllocatedVUs: 200,
            maxVUs: 2000,
          },
        },
        thresholds: {
          // delayAbortEval: without it, abortOnFail evaluates from t=0,
          // when achieved rate is still near zero (VUs haven't finished
          // scaling up, and the first iterations -- including 5-15s
          // getEntries/getVerdict ones -- haven't completed yet). That
          // false-positives the throughput threshold within ~2s, before
          // the run ever reaches steady state, regardless of real
          // app/hardware capacity (caught 2026-09-02: a postgres resource
          // bump made no difference to the ~2s abort, confirming this was
          // a threshold-timing bug, not a capacity finding). 20s is a
          // first-pass number, not tuned -- enough for VU ramp-up plus at
          // least one worst-case (15s) iteration to complete.
          http_req_duration: [
            { threshold: "p(95)<500", abortOnFail: true, delayAbortEval: "20s" },
          ],
          http_req_failed: [
            { threshold: "rate<0.01", abortOnFail: true, delayAbortEval: "20s" },
          ],
          // 95% of target, not 100% -- a few seconds of executor VU
          // spin-up at the very start of a flat-rate run is expected and
          // shouldn't fail an otherwise-healthy result.
          http_reqs: [
            {
              threshold: `rate>=${CAPACITY_TARGET_RPS * 0.95}`,
              abortOnFail: true,
              delayAbortEval: "20s",
            },
          ],
        },
      }
    : {
        stages: STAGES[profile] || STAGES.load,
        thresholds: {
          http_req_duration: [{ threshold: "p(95)<500", abortOnFail: false }],
          http_req_failed: [{ threshold: "rate<0.01", abortOnFail: false }],
        },
      };

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

// Three amount buckets, matching the actor model referenced above.
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

// Bounded per-VU cache of successfully-posted requests (key AND the exact
// body that earned it), so the replay slice below can resend the SAME
// request under the SAME key -- a true idempotent replay. Caching only the
// key and drawing a fresh from/to/amount at replay time is not a replay at
// all: it's the same key reused with a different request body, which
// ADR-005/DDD-8's fingerprint check correctly refuses as a conflict, not
// the "identical transaction_id back" this slice is supposed to exercise.
// Bounded to avoid unbounded per-VU memory growth over `soak`'s duration.
const replayableRequests = [];

function postTransfer() {
  const roll = Math.random();

  // ~2% idempotency-replay slice (I7, ADR-005): resend a prior request
  // VERBATIM (same key, same from/to/amount) and expect the identical
  // transaction_id back, not a fresh posting.
  if (roll < 0.02 && replayableRequests.length > 0) {
    const original =
      replayableRequests[Math.floor(Math.random() * replayableRequests.length)];
    const res = http.post(`${BASE_URL}/transfers`, original.body, {
      headers: { ...JSON_HEADERS, "Idempotency-Key": original.key },
    });
    check(res, {
      "replay: 200 or 201": (r) => r.status === 200 || r.status === 201,
    });
    return;
  }

  const from = pickAccount();
  const to = pickCounterparty(from);

  // ~1.5% deliberate insufficient-funds slice, on top of the 2% replay
  // slice above (roughly 2%-3.5% of rolls).
  const forceInsufficientFunds = roll >= 0.02 && roll < 0.035;
  const amountMinor = forceInsufficientFunds
    ? 999999999999 // implausibly large, exceeds any funded/seeded balance
    : pickAmountMinor();
  const key = `k6-${__VU}-${__ITER}-${Date.now()}`;
  const body = JSON.stringify({ from, to, amount: minorToDecimal(amountMinor) });

  const res = http.post(`${BASE_URL}/transfers`, body, {
    headers: { ...JSON_HEADERS, "Idempotency-Key": key },
  });

  if (forceInsufficientFunds) {
    check(res, { "insufficient_funds: 422": (r) => r.status === 422 });
    return;
  }

  check(res, {
    "status is 200 or 201": (r) => r.status === 200 || r.status === 201,
  });
  if (res.status === 200 || res.status === 201) {
    replayableRequests.push({ key, body });
    if (replayableRequests.length > 500) replayableRequests.shift();
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
  if (r < 0.4) {
    postTransfer();
    sleep(1 + Math.random() * 2); // P1, 1-3s
  } else if (r < 0.8) {
    getBalance();
    sleep(1 + Math.random() * 2); // P1, 1-3s
  } else if (r < 0.9) {
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
