import { useMemo, useState } from 'react';
import { Copy as CopyIcon, Download, Search } from 'lucide-react';
import { downloadTextFile } from '../../utils/downloadText';
import {
  LOG_LEVELS,
  buildLogLines,
  formatLogLinesAsText,
  type LogLevel,
  type LogLine,
} from '../../utils/traceLogs';
import { InspectorEmptyState } from './CompactPayloadInspector';
import { useTraceDetailWorkspace } from './traceDetailWorkspaceContext';

/** Explicit log events with level filters, search, tail, copy, and download. */
export function TraceLogsSection() {
  const { events, selectSpanAndShowDetails, spanIndex } = useTraceDetailWorkspace();
  const [search, setSearch] = useState('');
  const [activeLevels, setActiveLevels] = useState<Record<LogLevel, boolean>>({
    info: true,
    warn: true,
    error: true,
    debug: false,
  });
  const [tail, setTail] = useState(true);
  const [copied, setCopied] = useState(false);

  const lines = useMemo<LogLine[]>(() => buildLogLines(events), [events]);

  const filtered = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return lines.filter((line) => {
      if (!activeLevels[line.level]) return false;
      if (!needle) return true;
      return (
        line.message.toLowerCase().includes(needle) ||
        line.source.toLowerCase().includes(needle)
      );
    });
  }, [activeLevels, lines, search]);

  const levelCount = useMemo(() => {
    const out: Record<LogLevel, number> = { info: 0, warn: 0, error: 0, debug: 0 };
    lines.forEach((line) => {
      out[line.level] += 1;
    });
    return out;
  }, [lines]);

  const levelTextColor: Record<LogLevel, string> = {
    info: 'var(--c-text-muted)',
    warn: 'var(--c-amber-text)',
    error: 'var(--c-red-text)',
    debug: 'var(--c-text-muted)',
  };

  const handleDownload = () => {
    if (filtered.length === 0) return;
    downloadTextFile('trace-logs.log', formatLogLinesAsText(filtered));
  };

  const handleCopy = async () => {
    if (filtered.length === 0) return;
    try {
      await navigator.clipboard.writeText(formatLogLinesAsText(filtered));
      setCopied(true);
      setTimeout(() => setCopied(false), 1400);
    } catch {
      // ignore — clipboard might not be available
    }
  };

  if (lines.length === 0) {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center p-6">
        <InspectorEmptyState>No explicit log events recorded for this trace.</InspectorEmptyState>
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-wrap items-center gap-3 border-b border-[var(--c-border-subtle)] px-4 py-2.5">
        <div className="relative w-full max-w-[320px] sm:w-auto sm:flex-1">
          <Search className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--c-text-muted)]" />
          <input
            aria-label="Filter logs"
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Filter logs…"
            className="h-7 w-full rounded border border-[var(--c-border)] bg-[var(--c-surface)] py-1 pl-7 pr-2 text-xs text-[var(--c-text-primary)] outline-none focus:border-[var(--c-border-strong)]"
          />
        </div>

        <div className="flex flex-wrap gap-1">
          {LOG_LEVELS.map((level) => {
            const active = activeLevels[level];
            return (
              <button
                key={level}
                type="button"
                onClick={() =>
                  setActiveLevels((prev) => ({ ...prev, [level]: !prev[level] }))
                }
                aria-pressed={active}
                className={`inline-flex items-center gap-1.5 rounded border px-2 py-1 font-mono text-[10.5px] font-semibold uppercase tracking-[0.04em] transition ${
                  active
                    ? 'border-[var(--c-border-strong)] bg-[var(--c-surface)]'
                    : 'border-[var(--c-border)] bg-[var(--c-surface-muted)] opacity-60'
                }`}
                style={{ color: levelTextColor[level] }}
              >
                {level}
                <span className="text-[10px] text-[var(--c-text-muted)]">
                  {levelCount[level]}
                </span>
              </button>
            );
          })}
        </div>

        <div className="ml-auto flex items-center gap-3">
          <label className="inline-flex cursor-pointer items-center gap-1.5 text-[11.5px] text-[var(--c-text-secondary)]">
            <input
              type="checkbox"
              checked={tail}
              onChange={(event) => setTail(event.target.checked)}
              className="h-3 w-3 cursor-pointer accent-[var(--c-accent)]"
            />
            Tail
          </label>
          <button
            type="button"
            onClick={handleDownload}
            disabled={filtered.length === 0}
            className="inline-flex items-center gap-1.5 rounded border border-[var(--c-border)] bg-[var(--c-surface)] px-2 py-1 text-[11.5px] font-medium text-[var(--c-text-secondary)] transition hover:text-[var(--c-text-primary)] disabled:opacity-50"
          >
            <Download className="h-3.5 w-3.5" />
            Download
          </button>
          <button
            type="button"
            onClick={handleCopy}
            disabled={filtered.length === 0}
            className="inline-flex items-center gap-1.5 rounded border border-[var(--c-border)] bg-[var(--c-surface)] px-2 py-1 text-[11.5px] font-medium text-[var(--c-text-secondary)] transition hover:text-[var(--c-text-primary)] disabled:opacity-50"
          >
            <CopyIcon className="h-3.5 w-3.5" />
            {copied ? 'Copied' : 'Copy'}
          </button>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto bg-[var(--c-app-bg)] font-mono text-xs leading-[1.55]">
        {filtered.length === 0 ? (
          <div className="p-6 text-[var(--c-text-muted)]">No log lines match the current filters.</div>
        ) : (
          filtered.map((line, index) => {
            const canSelect = Boolean(line.spanId && spanIndex.has(line.spanId));
            const rowBg =
              line.level === 'error'
                ? 'rgba(239,68,68,0.05)'
                : line.level === 'warn'
                  ? 'rgba(245,158,11,0.04)'
                  : 'transparent';
            const railColor =
              line.level === 'error'
                ? 'var(--c-red)'
                : line.level === 'warn'
                  ? 'var(--c-amber)'
                  : 'transparent';
            return (
              <div
                key={line.id}
                className="grid items-baseline gap-3 py-1 pr-4"
                style={{
                  gridTemplateColumns: '50px 130px 60px 200px minmax(0, 1fr)',
                  background: rowBg,
                  borderLeft: `2px solid ${railColor}`,
                }}
              >
                <span className="select-none pl-3 text-right text-[var(--c-text-muted)]">
                  {index + 1}
                </span>
                <span className="text-[var(--c-text-muted)]">{line.hms}</span>
                <span
                  className="text-[10.5px] font-semibold uppercase"
                  style={{ color: levelTextColor[line.level] }}
                >
                  {line.level}
                </span>
                {canSelect ? (
                  <button
                    type="button"
                    onClick={() => selectSpanAndShowDetails(line.spanId!)}
                    className="truncate text-left text-[var(--c-text-secondary)] hover:text-[var(--c-accent-text)]"
                  >
                    {line.source}
                  </button>
                ) : (
                  <span className="truncate text-[var(--c-text-secondary)]">{line.source}</span>
                )}
                <span className="whitespace-pre-wrap text-[var(--c-text-primary)]">
                  {line.message}
                </span>
              </div>
            );
          })
        )}
        {tail ? (
          <div
            className="grid gap-3 py-1 pr-4"
            style={{ gridTemplateColumns: '50px minmax(0, 1fr)' }}
          >
            <span className="pl-3 text-right text-[var(--c-text-muted)]">—</span>
            <span className="inline-flex items-center gap-2 italic text-[var(--c-text-muted)]">
              <span
                className="inline-block h-1.5 w-1.5 rounded-full bg-[var(--c-blue)]"
                style={{ animation: 'continua-pulse 1.6s ease-out infinite' }}
              />
              Tailing — stream open
            </span>
          </div>
        ) : null}
      </div>
    </div>
  );
}
