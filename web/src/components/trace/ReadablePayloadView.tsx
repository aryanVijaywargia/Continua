import { useMemo } from 'react';
import { toReadablePayload } from './traceSteps';

/**
 * Readable key/value view of a JSON payload. A plain string payload shows as
 * text. The raw JSON stays one toggle away in the host.
 */
export function ReadablePayloadView({ value }: { value: unknown }) {
  const readable = useMemo(() => toReadablePayload(value), [value]);

  if (readable.text !== null) {
    return (
      <p className="whitespace-pre-wrap break-words rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 py-2.5 text-[13px] leading-6 text-[var(--c-text-primary)]">
        {readable.text.length > 0 ? readable.text : 'Empty text'}
      </p>
    );
  }

  return (
    <div className="overflow-hidden rounded-md border border-[var(--c-border)] bg-[var(--c-surface)]">
      <dl className="divide-y divide-[var(--c-border-subtle)]">
        {readable.fields.map((field) => (
          <div
            key={field.key}
            className="grid grid-cols-[minmax(6rem,38%)_minmax(0,1fr)] gap-3 px-3 py-2 text-xs"
          >
            <dt className="min-w-0 break-words font-mono text-[var(--c-text-muted)]">{field.key}</dt>
            <dd
              className={`min-w-0 whitespace-pre-wrap break-words ${
                field.tone === 'literal'
                  ? 'font-mono text-[var(--c-accent-text)]'
                  : field.tone === 'muted'
                    ? 'italic text-[var(--c-text-muted)]'
                    : 'text-[var(--c-text-primary)]'
              }`}
            >
              {field.value}
            </dd>
          </div>
        ))}
      </dl>
      {readable.hiddenCount > 0 ? (
        <p className="border-t border-[var(--c-border-subtle)] px-3 py-2 text-xs text-[var(--c-text-muted)]">
          {readable.hiddenCount} more fields. Open the raw JSON to see all of them.
        </p>
      ) : null}
    </div>
  );
}
