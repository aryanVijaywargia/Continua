import { JsonViewer } from './JsonViewer';
import {
  formatInlineSemanticValue,
  isStructuredSemanticValue,
} from '../utils/eventSemantics';
import type { ExtractedStateChange } from '../utils/stateChanges';

interface StateDiffViewerProps {
  changes: ExtractedStateChange[];
}

const FALLBACK_NAMESPACE_LABEL = 'General';

export function StateDiffViewer({ changes }: StateDiffViewerProps) {
  const groups = groupStateChanges(changes);

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <div className="border-b border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-950/70">
        <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-slate-600 dark:text-slate-300">
          State
        </h2>
        <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
          Structured state transitions grouped by namespace.
        </p>
      </div>

      {changes.length === 0 ? (
        <div className="px-4 py-12 text-center text-sm text-slate-500 dark:text-slate-400">
          No structured state changes recorded for this trace.
        </div>
      ) : (
        <div className="min-h-0 flex-1 space-y-6 overflow-y-auto p-4">
          {groups.map(([namespace, namespaceChanges]) => (
            <div key={namespace}>
              <h3 className="text-xs font-semibold uppercase tracking-[0.18em] text-slate-500 dark:text-slate-400">
                {namespace}
              </h3>
              <div className="mt-3 space-y-3">
                {namespaceChanges.map((change) => (
                  <StateChangeCard key={change.event.id} change={change} />
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

function StateChangeCard({ change }: { change: ExtractedStateChange }) {
  const usesStructuredView =
    isStructuredSemanticValue(change.oldValue) ||
    isStructuredSemanticValue(change.newValue);

  return (
    <div className="rounded-xl border border-slate-200 bg-slate-50/60 p-4 dark:border-slate-800 dark:bg-slate-950/70">
      <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">{change.key}</div>
      <div className="mt-1 text-xs text-slate-500 dark:text-slate-400">
        {change.event.span_name ?? change.event.span_id ?? 'Trace'}
        {' · '}
        {formatStateChangeTime(change.event.timestamp)}
      </div>

      {usesStructuredView ? (
        <div className="mt-4 grid gap-4 lg:grid-cols-2">
          <StructuredValuePanel label="Old value" value={change.oldValue} />
          <StructuredValuePanel label="New value" value={change.newValue} />
        </div>
      ) : (
        <div className="mt-4 flex flex-wrap items-center gap-2 text-sm font-medium text-slate-900 dark:text-slate-100">
          <InlineValuePill>{formatInlineSemanticValue(change.oldValue)}</InlineValuePill>
          <span className="text-slate-400 dark:text-slate-500">→</span>
          <InlineValuePill tone="accent">
            {formatInlineSemanticValue(change.newValue)}
          </InlineValuePill>
        </div>
      )}
    </div>
  );
}

function StructuredValuePanel({
  label,
  value,
}: {
  label: string;
  value: unknown;
}) {
  return (
    <div>
      <div className="mb-2 text-xs font-semibold uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
        {label}
      </div>
      {isStructuredSemanticValue(value) ? (
        <JsonViewer data={value} className="max-h-64 overflow-y-auto" />
      ) : (
        <div className="rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-700 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-200">
          {formatInlineSemanticValue(value)}
        </div>
      )}
    </div>
  );
}

function InlineValuePill({
  children,
  tone = 'neutral',
}: {
  children: string;
  tone?: 'neutral' | 'accent';
}) {
  return (
    <span
      className={`rounded-full border px-2.5 py-1 text-xs font-medium ${
        tone === 'accent'
          ? 'border-blue-200 bg-blue-100 text-blue-800 dark:border-sky-500/40 dark:bg-sky-500/15 dark:text-sky-200'
          : 'border-slate-200 bg-white text-slate-700 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-200'
      }`}
    >
      {children}
    </span>
  );
}

function groupStateChanges(changes: ExtractedStateChange[]) {
  const grouped = new Map<string, ExtractedStateChange[]>();

  for (const change of changes) {
    const namespace = change.namespace ?? FALLBACK_NAMESPACE_LABEL;
    const current = grouped.get(namespace) ?? [];
    current.push(change);
    grouped.set(namespace, current);
  }

  return Array.from(grouped.entries());
}

function formatStateChangeTime(timestamp: string) {
  return new Date(timestamp).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}
