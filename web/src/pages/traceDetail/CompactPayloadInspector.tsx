import { useState, type ReactNode } from 'react';
import { ChevronDown, ChevronRight } from 'lucide-react';

export function InspectorEmptyState({ children }: { children: ReactNode }) {
  return (
    <div className="rounded border border-dashed border-[var(--c-border)] bg-[var(--c-surface-muted)] px-4 py-6 text-sm text-[var(--c-text-muted)]">
      {children}
    </div>
  );
}

export function CompactPayloadInspector({
  depth = 0,
  isLast = true,
  name,
  value,
}: {
  depth?: number;
  isLast?: boolean;
  name?: string | number;
  value: unknown;
}) {
  const [open, setOpen] = useState(depth < 2);
  const isObject = value !== null && typeof value === 'object';

  if (!isObject) {
    return (
      <div className="flex font-mono text-xs leading-6">
        {name !== undefined ? (
          <>
            <span className="text-[var(--c-text-secondary)]">"{String(name)}"</span>
            <span className="text-[var(--c-text-muted)]">: </span>
          </>
        ) : null}
        <PayloadPrimitive value={value} />
        {!isLast ? <span className="text-[var(--c-text-muted)]">,</span> : null}
      </div>
    );
  }

  const isArray = Array.isArray(value);
  const entries = isArray
    ? (value as unknown[]).map((entry, index) => [index, entry] as const)
    : Object.entries(value as Record<string, unknown>);
  const openToken = isArray ? '[' : '{';
  const closeToken = isArray ? ']' : '}';

  return (
    <div className="font-mono text-xs leading-6">
      <div className="flex items-center gap-1">
        <button
          type="button"
          aria-label={`${open ? 'Collapse' : 'Expand'} ${name ?? 'payload'}`}
          className="flex h-4 w-4 items-center justify-center text-[var(--c-text-muted)] hover:text-[var(--c-text-primary)]"
          onClick={() => setOpen((current) => !current)}
        >
          {open ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
        </button>
        {name !== undefined ? (
          <>
            <span className="text-[var(--c-text-secondary)]">"{String(name)}"</span>
            <span className="text-[var(--c-text-muted)]">: </span>
          </>
        ) : null}
        <span className="text-[var(--c-text-secondary)]">{openToken}</span>
        {!open ? (
          <>
            <span className="mx-1 text-[var(--c-text-muted)]">
              {entries.length} item{entries.length === 1 ? '' : 's'}
            </span>
            <span className="text-[var(--c-text-secondary)]">{closeToken}</span>
            {!isLast ? <span className="text-[var(--c-text-muted)]">,</span> : null}
          </>
        ) : null}
      </div>
      {open ? (
        <>
          <div className="ml-[5px] border-l border-[var(--c-border-subtle)] pl-4">
            {entries.map(([entryName, entryValue], index) => (
              <CompactPayloadInspector
                key={String(entryName)}
                depth={depth + 1}
                isLast={index === entries.length - 1}
                name={isArray ? undefined : entryName}
                value={entryValue}
              />
            ))}
          </div>
          <div className="pl-1 text-[var(--c-text-secondary)]">
            {closeToken}
            {!isLast ? <span className="text-[var(--c-text-muted)]">,</span> : null}
          </div>
        </>
      ) : null}
    </div>
  );
}

function PayloadPrimitive({ value }: { value: unknown }) {
  if (value === null) {
    return <span className="text-[var(--c-text-muted)]">null</span>;
  }
  if (value === undefined) {
    return <span className="text-[var(--c-text-muted)]">undefined</span>;
  }
  if (typeof value === 'string') {
    return <span className="break-all text-[var(--c-amber-text)]">"{value}"</span>;
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return <span className="text-[var(--c-blue-text)]">{String(value)}</span>;
  }
  return <span className="text-[var(--c-text-primary)]">{String(value)}</span>;
}
