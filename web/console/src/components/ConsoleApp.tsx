// ConsoleApp -- root orchestrator. Holds page-level state (authenticated? /
// loading / verdict / error), decides which component to render, sequences
// the key-gate before any data fetch (US-1..US-4 shell, DESIGN SA-D4).
import { useEffect, useState } from "react";
import type { KeyStorage } from "../keyStorage";
import type { ApiClient } from "../apiClient";
import type { VerdictStatus } from "../testing/domainTypes";
import { ApiKeyPrompt } from "./ApiKeyPrompt";
import { VerdictBanner } from "./VerdictBanner";

export interface ConsoleAppProps {
  keyStorage: KeyStorage;
  apiClient: ApiClient;
}

// ConsoleState -- the key-gate / verdict / error-rejected state machine
// (see ConsoleApp.test.tsx header comment for the full transition diagram).
// Each state carries exactly the data available in that state -- illegal
// combinations (e.g. a verdict without a fetchedAt) are unrepresentable.
type ConsoleState =
  | { kind: "no-key" }
  | { kind: "checking" }
  | { kind: "showing-verdict"; verdict: VerdictStatus; fetchedAt: Date }
  | { kind: "key-rejected" };

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
        setState({ kind: "showing-verdict", verdict: response.verdict, fetchedAt: new Date() });
      })
      .catch(() => {
        if (cancelled) return;
        setState({ kind: "key-rejected" });
      });
    return () => {
      cancelled = true;
    };
  }, [state.kind, apiClient]);

  function retryWithKey(pastedKey: string): void {
    keyStorage.set(pastedKey);
    setState({ kind: "checking" });
  }

  switch (state.kind) {
    case "no-key":
      return <ApiKeyPrompt rejected={false} onSubmit={retryWithKey} />;
    case "key-rejected":
      return <ApiKeyPrompt rejected={true} onSubmit={retryWithKey} />;
    case "checking":
      return <VerdictBanner loading={true} verdict={null} fetchedAt={null} />;
    case "showing-verdict":
      return <VerdictBanner loading={false} verdict={state.verdict} fetchedAt={state.fetchedAt} />;
  }
}
