// ConsoleApp -- root orchestrator. Holds page-level state (authenticated? /
// loading / verdict / error), decides which component to render, sequences
// the key-gate before any data fetch (US-1..US-4 shell, DESIGN SA-D4).
import { useEffect, useState } from "react";
import type { KeyStorage } from "../keyStorage";
import { FetchFailedError, type ApiClient } from "../apiClient";
import type { DriftRow, EntryRow, FetchPhase, VerdictStatus } from "../testing/domainTypes";
import { FALLBACK_HEALTH_PATH } from "../testing/domainTypes";
import { ApiKeyPrompt } from "./ApiKeyPrompt";
import { VerdictBanner } from "./VerdictBanner";
import { DriftTable } from "./DriftTable";
import { EntryTrace } from "./EntryTrace";
import { VerdictFetchError } from "./VerdictFetchError";

export interface ConsoleAppProps {
  keyStorage: KeyStorage;
  apiClient: ApiClient;
}

// EntryTraceSelection -- the account most recently clicked in DriftTable and
// the fetch-in-progress/complete state for its entries. Absent when no
// account has been selected yet (illegal to render EntryTrace without one).
type EntryTraceSelection = {
  accountId: string;
  phase: FetchPhase;
  entries: EntryRow[] | null;
};

// ConsoleState -- the key-gate / verdict / error-rejected state machine
// (see ConsoleApp.test.tsx header comment for the full transition diagram).
// Each state carries exactly the data available in that state -- illegal
// combinations (e.g. a verdict without a fetchedAt) are unrepresentable.
type ConsoleState =
  | { kind: "no-key" }
  | { kind: "checking" }
  | {
      kind: "showing-verdict";
      verdict: VerdictStatus;
      drifted: DriftRow[];
      fetchedAt: Date;
      selection: EntryTraceSelection | null;
    }
  | { kind: "key-rejected" }
  | { kind: "fetch-failed" };

function initialState(keyStorage: KeyStorage): ConsoleState {
  return keyStorage.get() ? { kind: "checking" } : { kind: "no-key" };
}

export function ConsoleApp({ keyStorage, apiClient }: ConsoleAppProps): JSX.Element {
  const [state, setState] = useState<ConsoleState>(() => initialState(keyStorage));

  useEffect(() => {
    if (state.kind !== "checking") {
      return;
    }
    let cancelled = false;
    apiClient
      .fetchVerdict()
      .then((response) => {
        if (cancelled) return;
        setState({
          kind: "showing-verdict",
          verdict: response.verdict,
          drifted: response.drifted,
          fetchedAt: new Date(),
          selection: null,
        });
      })
      .catch((error) => {
        if (cancelled) return;
        if (error instanceof FetchFailedError) {
          setState({ kind: "fetch-failed" });
          return;
        }
        // AuthRejectedError, and any other unexpected rejection, re-shows
        // the key prompt (unchanged default from step 01-03).
        setState({ kind: "key-rejected" });
      });
    return () => {
      cancelled = true;
    };
  }, [state.kind, apiClient]);

  useEffect(() => {
    if (state.kind !== "showing-verdict" || state.selection?.phase !== "loading") {
      return;
    }
    const { accountId } = state.selection;
    let cancelled = false;
    apiClient
      .fetchEntries(accountId)
      .then((entries) => {
        if (cancelled) return;
        setState((current) =>
          current.kind === "showing-verdict" && current.selection?.accountId === accountId
            ? { ...current, selection: { accountId, phase: "loaded", entries } }
            : current
        );
      })
      .catch(() => {
        if (cancelled) return;
        setState((current) =>
          current.kind === "showing-verdict" && current.selection?.accountId === accountId
            ? { ...current, selection: { accountId, phase: "error", entries: null } }
            : current
        );
      });
    return () => {
      cancelled = true;
    };
  }, [state, apiClient]);

  function retryWithKey(pastedKey: string): void {
    keyStorage.set(pastedKey);
    setState({ kind: "checking" });
  }

  function selectAccount(accountId: string): void {
    setState((current) =>
      current.kind === "showing-verdict"
        ? { ...current, selection: { accountId, phase: "loading", entries: null } }
        : current
    );
  }

  switch (state.kind) {
    case "no-key":
      return <ApiKeyPrompt rejected={false} onSubmit={retryWithKey} />;
    case "key-rejected":
      return <ApiKeyPrompt rejected={true} onSubmit={retryWithKey} />;
    case "fetch-failed":
      return <VerdictFetchError fallbackPath={FALLBACK_HEALTH_PATH} />;
    case "checking":
      return (
        <div className="mx-auto max-w-3xl px-6 py-10">
          <VerdictBanner loading={true} verdict={null} fetchedAt={null} />
        </div>
      );
    case "showing-verdict":
      return (
        <div className="mx-auto flex max-w-3xl flex-col gap-7 px-6 py-10">
          <VerdictBanner loading={false} verdict={state.verdict} fetchedAt={state.fetchedAt} />
          <div>
            <div className="mb-3 flex items-baseline justify-between">
              <h2 className="m-0 text-xs font-semibold uppercase tracking-wide text-ink-faint">
                Drift
              </h2>
              <span className="font-mono text-xs text-ink-faint">
                {state.drifted.length} account{state.drifted.length === 1 ? "" : "s"}
              </span>
            </div>
            {state.drifted.length > 0 ? (
              <DriftTable drifted={state.drifted} onSelectAccount={selectAccount} />
            ) : (
              <p className="m-0 rounded-lg border border-dashed border-line-strong px-5 py-7 text-center text-sm text-ink-faint">
                No drifted accounts — every ledger balances against its computed total.
              </p>
            )}
          </div>
          {state.selection && (
            <div>
              <h2 className="m-0 mb-3 text-xs font-semibold uppercase tracking-wide text-ink-faint">
                Entry trace
              </h2>
              <EntryTrace
                accountId={state.selection.accountId}
                phase={state.selection.phase}
                entries={state.selection.entries}
              />
            </div>
          )}
        </div>
      );
  }
}
