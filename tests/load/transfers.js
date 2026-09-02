// k6 load/stress/soak profiles for POST /transfers (OPS-12,
// feature-delta.md 2026-08-29). Scope is deliberately narrow: this is the
// double-entry posting hot path from delivered slices 01-03, not the full
// API surface.
//
// PROFILE env var selects which `stages` array runs (default "load"):
//   load   — short ramp, sustained steady stage, ramp-down. Baseline
//            throughput/latency at an expected traffic level.
//   stress — ramps beyond what `load` sustains, to find where the endpoint
//            degrades or breaks, not just where it's slow.
//   soak   — longest duration of the three, moderate sustained load. The
//            finding here is a trend over time (pool exhaustion, memory
//            growth, slow degradation), not a single number.
//
// No prior numeric decision exists for VU counts/durations — these are
// first-pass values, deliberately modest for a single-instance
// docker-compose stack (no read replica, no external pooler — same
// contended topology as `make race-02`/`race-03`).
import http from "k6/http";
import { check } from "k6";

const BASE_URL = "http://localhost:8080";
const AUTH_HEADER = { Authorization: "Bearer demo-operator-key" };
const JSON_HEADERS = { ...AUTH_HEADER, "Content-Type": "application/json" };

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

const profile = __ENV.PROFILE || "load";

export const options = {
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
  createAccount("load-source", "wallet");
  createAccount("load-sink", "wallet");
  http.post(
    `${BASE_URL}/transfers`,
    JSON.stringify({ from: "treasury", to: "load-source", amount: "1000000.00" }),
    { headers: { ...JSON_HEADERS, "Idempotency-Key": "load-test-fund" } },
  );
}

export default function () {
  const idempotencyKey = `load-${__VU}-${__ITER}`;
  const res = http.post(
    `${BASE_URL}/transfers`,
    JSON.stringify({ from: "load-source", to: "load-sink", amount: "0.01" }),
    {
      headers: { ...JSON_HEADERS, "Idempotency-Key": idempotencyKey },
    },
  );
  check(res, {
    "status is 200 or 201": (r) => r.status === 200 || r.status === 201,
  });
}
