// ApiKeyPrompt -- the paste-in UI, gating first load when no key is stored,
// and re-shown with a rejection message after a 401 (Pre-requisite D10,
// ADR-010). The one effect (keyStorage.set) is delegated, not inlined --
// this component receives an onSubmit callback and never touches keyStorage
// directly, keeping it a pure-function render.
import { useState } from "react";

export interface ApiKeyPromptProps {
  rejected: boolean;
  onSubmit: (pastedKey: string) => void;
}

export function ApiKeyPrompt({ rejected, onSubmit }: ApiKeyPromptProps): JSX.Element {
  const [pastedKey, setPastedKey] = useState("");

  function handleSubmit(event: React.FormEvent): void {
    event.preventDefault();
    onSubmit(pastedKey);
  }

  return (
    <div className="flex justify-center px-6 py-16">
      <form
        onSubmit={handleSubmit}
        className="w-full max-w-md rounded-xl border border-line bg-white p-8 shadow-card"
      >
        <p className="m-0 mb-1.5 font-mono text-xs uppercase tracking-wider text-ink-faint">
          Operator access
        </p>
        <h1 className="m-0 mb-2 text-xl font-semibold text-ink text-balance">
          Enter your operator API key
        </h1>
        <p className="m-0 mb-7 text-sm leading-relaxed text-ink-muted">
          The console reads live trial-balance data from ledger-core. Your key is stored only in
          this browser and sent with every request.
        </p>

        {rejected && (
          <p className="mb-5 flex items-start gap-2.5 rounded-lg border border-bad-border bg-bad-soft px-3.5 py-3 text-sm leading-relaxed text-bad">
            Key rejected — enter a valid operator API key
          </p>
        )}

        <label
          htmlFor="operator-api-key"
          className="mb-2 block text-xs font-medium text-ink-muted"
        >
          Operator API key
        </label>
        <input
          id="operator-api-key"
          type="password"
          value={pastedKey}
          onChange={(event) => setPastedKey(event.target.value)}
          placeholder="Paste key"
          className="w-full rounded-lg border border-line-strong px-3.5 py-3 font-mono text-sm tracking-wide text-ink outline-none focus:border-accent focus:ring-4 focus:ring-accent/20"
        />
        <p className="mt-2 text-xs text-ink-faint">Issued by ledger-core operations onboarding.</p>

        <button
          type="submit"
          className="mt-5 rounded-lg bg-accent px-5 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-accent-strong"
        >
          Submit
        </button>
      </form>
    </div>
  );
}
