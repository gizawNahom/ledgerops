// VerdictFetchError -- when the console's own verdict fetch fails, the
// operator is told immediately and shown the fallback path (US-4).
// Pure-function render -- no fetch, no storage.

export interface VerdictFetchErrorProps {
  fallbackPath: string;
}

export function VerdictFetchError({ fallbackPath }: VerdictFetchErrorProps): JSX.Element {
  return (
    <div className="flex justify-center px-6 py-16">
      <div className="w-full max-w-md rounded-xl border border-bad-border bg-white p-8 text-center shadow-card">
        <div className="mx-auto mb-4 flex h-11 w-11 items-center justify-center rounded-full border border-bad-border bg-bad-soft text-bad">
          <svg width="20" height="20" viewBox="0 0 16 16" fill="none" aria-hidden="true">
            <circle cx="8" cy="8" r="7" stroke="currentColor" strokeWidth="1.4" />
            <path d="M8 4.5v4.2M8 11.2v.1" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" />
          </svg>
        </div>
        <p role="alert" className="m-0 text-sm leading-relaxed text-ink-muted">
          Could not load the verdict. Check the books directly via {fallbackPath}.
        </p>
      </div>
    </div>
  );
}
