// VerdictFetchError -- when the console's own verdict fetch fails, the
// operator is told immediately and shown the fallback path (US-4).
// Pure-function render. RED scaffold created by DISTILL per Mandate 7.
export const __SCAFFOLD__ = true;

export interface VerdictFetchErrorProps {
  fallbackPath: string;
}

export function VerdictFetchError(_props: VerdictFetchErrorProps): JSX.Element {
  throw new Error("Not yet implemented -- RED scaffold");
}
