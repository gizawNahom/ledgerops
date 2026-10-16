// ConsoleApp -- decides what the operator sees first: the key prompt, or the
// verdict flow (Pre-requisite D10, US-1 shell). Layer: component, with
// apiClient/keyStorage as collaborators substituted by hand-rolled fakes
// (not real fetch/localStorage -- this file tests ConsoleApp's own
// orchestration logic, not its collaborators, which have their own test
// files above).
//
// State machine (AT-completeness C2 -- documented here per the mechanical
// checklist's C2a requirement):
//
//   [no-key] --submit(key)--> [checking] --verdict ok--> [showing-verdict]
//   [no-key]                                 \--401/error--> [error: key-rejected] --submit(key)--> [checking]
//   [showing-verdict] --401 on any later fetch--> [error: key-rejected]
//
// Illegal-event coverage (C2b): a 401 arriving while already in
// [showing-verdict] (a state that assumed the key was valid) is the
// illegal-event-from-that-state case exercised below -- the key was valid at
// the time the verdict fetch was attempted, so the app never entered
// [error: key-rejected] "normally"; the 401 arriving despite that hits the
// state from the wrong assumption, which is exactly what this scenario
// exercises.
import { describe, it, expect, vi } from "vitest";
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ConsoleApp } from "./ConsoleApp";
import type { KeyStorage } from "../keyStorage";
import type { ApiClient } from "../apiClient";
import { AuthRejectedError, FetchFailedError } from "../apiClient";
import { FALLBACK_HEALTH_PATH } from "../testing/domainTypes";
import type { DriftRow } from "../testing/domainTypes";

function fakeKeyStorage(initialKey: string | null): KeyStorage {
  let stored = initialKey;
  return {
    get: () => stored,
    set: (key: string) => {
      stored = key;
    },
    clear: () => {
      stored = null;
    },
  };
}

function fakeApiClient(): ApiClient {
  return {
    fetchVerdict: vi.fn().mockResolvedValue({ verdict: "YES", imbalanceMinor: 0, entryCount: 0, elapsedMs: 1, drifted: [] }),
    fetchEntries: vi.fn().mockResolvedValue([]),
  };
}

const driftedAccount: DriftRow = { tenantId: "tnt_legacy_seed", accountId: "acc-42", stored: 100, computed: 90, delta: -10 };

describe("ConsoleApp -- the console decides what the operator sees first", () => {
  it("shows the key-entry form when no operator key has ever been stored", () => {
    render(<ConsoleApp keyStorage={fakeKeyStorage(null)} apiClient={fakeApiClient()} />);
    expect(screen.getByLabelText(/operator api key/i)).toBeInTheDocument();
  });

  it("goes straight to the verdict flow when a stored key is already present", async () => {
    render(<ConsoleApp keyStorage={fakeKeyStorage("a-stored-key")} apiClient={fakeApiClient()} />);
    expect(await screen.findByText(/books balance/i)).toBeInTheDocument();
  });

  it("@error re-shows the key form with a 'key rejected' message after a 401, instead of the blank first-load form", async () => {
    const rejectingClient: ApiClient = {
      fetchVerdict: vi.fn().mockRejectedValue(new Error("unidentified_caller")),
      fetchEntries: vi.fn(),
    };
    render(<ConsoleApp keyStorage={fakeKeyStorage("a-key-the-api-will-reject")} apiClient={rejectingClient} />);
    expect(await screen.findByText(/key rejected/i)).toBeInTheDocument();
  });

  it("renders the drift table below the verdict when the verdict response carries drifted accounts", async () => {
    const client: ApiClient = {
      fetchVerdict: vi.fn().mockResolvedValue({
        verdict: "NO",
        imbalanceMinor: 10,
        entryCount: 2,
        elapsedMs: 1,
        drifted: [driftedAccount],
      }),
      fetchEntries: vi.fn().mockResolvedValue([]),
    };
    render(<ConsoleApp keyStorage={fakeKeyStorage("a-stored-key")} apiClient={client} />);

    expect(await screen.findByText(/books balance/i)).toBeInTheDocument();
    expect(await screen.findByText(driftedAccount.accountId)).toBeInTheDocument();
  });

  it("fetches and renders the entry trace for the account clicked in the drift table", async () => {
    const user = userEvent.setup();
    const client: ApiClient = {
      fetchVerdict: vi.fn().mockResolvedValue({
        verdict: "NO",
        imbalanceMinor: 10,
        entryCount: 2,
        elapsedMs: 1,
        drifted: [driftedAccount],
      }),
      fetchEntries: vi.fn().mockResolvedValue([
        { amount: 5, counterparty: "vendor-x", recordedAt: "2026-08-01", runningBalance: 5 },
      ]),
    };
    render(<ConsoleApp keyStorage={fakeKeyStorage("a-stored-key")} apiClient={client} />);

    const row = await screen.findByText(driftedAccount.accountId);
    await user.click(row);

    expect(client.fetchEntries).toHaveBeenCalledWith(driftedAccount.accountId, driftedAccount.tenantId);
    expect(await screen.findByText(/vendor-x/i)).toBeInTheDocument();
  });

  it("renders the verdict-fetch-error fallback (not the key prompt) when fetchVerdict rejects with a FetchFailedError", async () => {
    const failingClient: ApiClient = {
      fetchVerdict: vi.fn().mockRejectedValue(new FetchFailedError("network down")),
      fetchEntries: vi.fn(),
    };
    render(<ConsoleApp keyStorage={fakeKeyStorage("a-stored-key")} apiClient={failingClient} />);

    expect(await screen.findByText(new RegExp(FALLBACK_HEALTH_PATH.replace("/", "\\/"), "i"))).toBeInTheDocument();
    expect(screen.queryByText(/key rejected/i)).not.toBeInTheDocument();
  });

  it("still shows the rejected key prompt (unchanged) when fetchVerdict rejects with an AuthRejectedError", async () => {
    const rejectingClient: ApiClient = {
      fetchVerdict: vi.fn().mockRejectedValue(new AuthRejectedError()),
      fetchEntries: vi.fn(),
    };
    render(<ConsoleApp keyStorage={fakeKeyStorage("a-key-the-api-will-reject")} apiClient={rejectingClient} />);

    expect(await screen.findByText(/key rejected/i)).toBeInTheDocument();
  });

  // --- fix-unscoped-entries-tenant-merge: two tenants each hold "treasury" and
  // both drifted. The drill-down must fetch the selected tenant's entries and
  // render exactly what the server served -- no client-side filtering, no
  // client-side recomputation of running balances (US-3). --------------------

  const acmeTreasury: DriftRow = { tenantId: "tnt_acme", accountId: "treasury", stored: -100, computed: -105, delta: -5 };
  const beaconTreasury: DriftRow = { tenantId: "tnt_beacon", accountId: "treasury", stored: -40, computed: -47, delta: -7 };
  const acmeTrail = [
    { amount: -100, counterparty: "vendor-acme-1", recordedAt: "2026-08-01", runningBalance: -100 },
    { amount: -25, counterparty: "vendor-acme-2", recordedAt: "2026-08-02", runningBalance: -125 },
  ];
  const beaconTrail = [{ amount: -40, counterparty: "vendor-beacon", recordedAt: "2026-08-01", runningBalance: -40 }];

  function twoTenantTreasuryClient(fetchEntries: ApiClient["fetchEntries"]): ApiClient {
    return {
      fetchVerdict: vi.fn().mockResolvedValue({
        verdict: "NO",
        imbalanceMinor: 12,
        entryCount: 3,
        elapsedMs: 1,
        drifted: [acmeTreasury, beaconTreasury],
      }),
      fetchEntries,
    };
  }

  it("drilling into one tenant's drifted \"treasury\" fetches that tenant's entries and shows them as served", async () => {
    const user = userEvent.setup();
    const fetchEntries = vi.fn(async (_accountId: string, tenantId?: string) =>
      tenantId === "tnt_beacon" ? beaconTrail : acmeTrail
    );
    render(<ConsoleApp keyStorage={fakeKeyStorage("a-stored-key")} apiClient={twoTenantTreasuryClient(fetchEntries)} />);

    await user.click(await screen.findByText("tnt_beacon"));

    expect(await screen.findByText(/vendor-beacon/i)).toBeInTheDocument();
    expect(fetchEntries).toHaveBeenCalledTimes(1);
    expect(fetchEntries).toHaveBeenCalledWith("treasury", "tnt_beacon");
    expect(screen.queryByText(/vendor-acme/i)).not.toBeInTheDocument();
    expect(screen.getAllByText("-40").length).toBeGreaterThan(0);
    expect(screen.queryByText("-125")).not.toBeInTheDocument();
  });

  it("moving from one tenant's \"treasury\" to its namesake shows only the namesake's entries, even if the first answer arrives late", async () => {
    const user = userEvent.setup();
    let releaseAcme: (rows: typeof acmeTrail) => void = () => undefined;
    const fetchEntries = vi.fn((_accountId: string, tenantId?: string) =>
      tenantId === "tnt_acme"
        ? new Promise<typeof acmeTrail>((resolve) => {
            releaseAcme = resolve;
          })
        : Promise.resolve(beaconTrail)
    );
    render(<ConsoleApp keyStorage={fakeKeyStorage("a-stored-key")} apiClient={twoTenantTreasuryClient(fetchEntries)} />);

    await user.click(await screen.findByText("tnt_acme"));
    await user.click(await screen.findByText("tnt_beacon"));
    expect(await screen.findByText(/vendor-beacon/i)).toBeInTheDocument();

    await act(async () => {
      releaseAcme(acmeTrail);
    });

    expect(screen.getByText(/vendor-beacon/i)).toBeInTheDocument();
    expect(screen.queryByText(/vendor-acme/i)).not.toBeInTheDocument();
    expect(fetchEntries.mock.calls).toEqual([
      ["treasury", "tnt_acme"],
      ["treasury", "tnt_beacon"],
    ]);
  });
});
