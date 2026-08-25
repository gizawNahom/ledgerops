// ApiKeyPrompt -- the paste-in UI, gating first load when no key is stored,
// and re-shown with a rejection message after a 401 (Pre-requisite D10,
// ADR-010). The one effect (keyStorage.set) is delegated, not inlined. RED
// scaffold created by DISTILL per Mandate 7.
export const __SCAFFOLD__ = true;

export interface ApiKeyPromptProps {
  rejected: boolean;
  onSubmit: (pastedKey: string) => void;
}

export function ApiKeyPrompt(_props: ApiKeyPromptProps): JSX.Element {
  throw new Error("Not yet implemented -- RED scaffold");
}
