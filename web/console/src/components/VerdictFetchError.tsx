// VerdictFetchError -- when the console's own verdict fetch fails, the
// operator is told immediately and shown the fallback path (US-4).
// Pure-function render -- no fetch, no storage.

export interface VerdictFetchErrorProps {
  fallbackPath: string;
}

export function VerdictFetchError({ fallbackPath }: VerdictFetchErrorProps): JSX.Element {
  return (
    <p role="alert">
      Could not load the verdict. Check the books directly via {fallbackPath}.
    </p>
  );
}
