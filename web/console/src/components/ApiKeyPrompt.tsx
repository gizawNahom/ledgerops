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
    <form onSubmit={handleSubmit}>
      {rejected && <p>Key rejected — enter a valid operator API key</p>}
      <label htmlFor="operator-api-key">Operator API key</label>
      <input
        id="operator-api-key"
        type="password"
        value={pastedKey}
        onChange={(event) => setPastedKey(event.target.value)}
      />
      <button type="submit">Submit</button>
    </form>
  );
}
