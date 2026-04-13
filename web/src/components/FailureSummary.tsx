import type { FailureSummary as FailureSummaryData } from '../utils/failureAnalysis';
import { formatTimestamp } from '../utils/format';
import {
  getAccessibleSummary,
  getReasonExplanation,
  type RetrySafetyAssessment,
} from '../utils/retrySafety';
import { RetrySafetyBadge } from './RetrySafetyBadge';
import { SpanBreadcrumb } from './SpanBreadcrumb';

interface FailureSummaryProps {
  summary: FailureSummaryData;
  onJumpToPrimaryFailedSpan: (spanId: string) => void;
  traceRetrySafety?: RetrySafetyAssessment | null;
}

export function FailureSummary({
  summary,
  onJumpToPrimaryFailedSpan,
  traceRetrySafety = null,
}: FailureSummaryProps) {
  const primaryFailedSpan = summary.primaryFailedSpan;
  const decisiveSpanDiffers =
    Boolean(traceRetrySafety?.decisiveSpanId) &&
    traceRetrySafety?.decisiveSpanId !== primaryFailedSpan?.span_id;
  const decisiveSpanLabel =
    traceRetrySafety?.decisiveSpanName ?? traceRetrySafety?.decisiveSpanId ?? 'decisive span';

  return (
    <section className="overflow-hidden rounded-[1rem] border border-[var(--continua-error-border)] bg-[var(--continua-surface-elevated)] shadow-sm">
      <div className="border-b border-red-200 bg-red-50 px-4 py-3">
        <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-red-700">
          Failure Summary
        </h2>
        <p className="mt-1 text-sm text-red-700/80">
          Failure-first guidance for this trace.
        </p>
      </div>

      <div className="space-y-4 p-4">
        {primaryFailedSpan ? (
          <>
            <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h3 className="text-base font-semibold text-[var(--continua-text-primary)]">
                    {primaryFailedSpan.name}
                  </h3>
                  <span className="rounded-full bg-[var(--continua-text-primary)] px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.16em] text-[var(--continua-surface-elevated)]">
                    {primaryFailedSpan.kind}
                  </span>
                </div>

                {summary.errorPreview ? (
                  <p className="mt-2 text-sm text-[var(--continua-text-secondary)]">{summary.errorPreview}</p>
                ) : (
                  <p className="mt-2 text-sm text-[var(--continua-text-muted)]">
                    No inline error preview was available for the primary failed span.
                  </p>
                )}
              </div>

              <button
                type="button"
                className="inline-flex shrink-0 items-center justify-center rounded-full bg-red-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-200"
                aria-label={`Jump to failed span ${primaryFailedSpan.name}`}
                onClick={() => onJumpToPrimaryFailedSpan(primaryFailedSpan.span_id)}
              >
                Jump to failed span
              </button>
            </div>

            <div className="grid gap-3 md:grid-cols-3">
              <SummaryStat
                label="Failed spans"
                value={String(summary.failedSpanCount)}
              />
              <SummaryStat
                label="Error events"
                value={String(summary.errorEventCount)}
              />
              <SummaryStat
                label="Failure timestamp"
                value={formatTimestamp(summary.failureTimestamp)}
              />
            </div>

            {traceRetrySafety ? (
              <RetrySafetyPanel
                assessment={traceRetrySafety}
                decisiveSpanDiffers={decisiveSpanDiffers}
                decisiveSpanLabel={decisiveSpanLabel}
                onJumpToSpan={onJumpToPrimaryFailedSpan}
              />
            ) : null}

            <div>
              <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
                Failure path
              </div>
              <SpanBreadcrumb
                path={summary.breadcrumbPath}
                ariaLabel="Failure path"
              />
            </div>
          </>
        ) : (
          <>
            <p className="text-sm text-[var(--continua-text-secondary)]">
              This trace is marked as failed, but no failed span could be identified
              from the current span data.
            </p>
            <div className="grid gap-3 md:grid-cols-2">
              <SummaryStat
                label="Failed spans"
                value={String(summary.failedSpanCount)}
              />
              <SummaryStat
                label="Error events"
                value={String(summary.errorEventCount)}
              />
            </div>
            {traceRetrySafety ? (
              <RetrySafetyPanel
                assessment={traceRetrySafety}
                decisiveSpanDiffers={decisiveSpanDiffers}
                decisiveSpanLabel={decisiveSpanLabel}
                onJumpToSpan={onJumpToPrimaryFailedSpan}
              />
            ) : null}
          </>
        )}
      </div>
    </section>
  );
}

function SummaryStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] p-3">
      <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
        {label}
      </div>
      <div className="mt-2 text-sm font-medium text-[var(--continua-text-primary)]">{value}</div>
    </div>
  );
}

function RetrySafetyPanel({
  assessment,
  decisiveSpanDiffers,
  decisiveSpanLabel,
  onJumpToSpan,
}: {
  assessment: RetrySafetyAssessment;
  decisiveSpanDiffers: boolean;
  decisiveSpanLabel: string;
  onJumpToSpan: (spanId: string) => void;
}) {
  return (
    <div className="rounded-lg border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] p-4">
      <div className="flex flex-wrap items-center gap-3">
        <div className="text-[11px] font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
          Trace retry safety
        </div>
        <RetrySafetyBadge
          classification={assessment.classification}
          variant="full"
          aria-label={getAccessibleSummary(assessment.classification)}
        />
      </div>
      <p className="mt-3 text-sm text-[var(--continua-text-secondary)]">
        Advisory only. Retry safety is inferred from recorded effect metadata.
      </p>
      <p className="mt-2 text-sm text-[var(--continua-text-secondary)]">
        {getReasonExplanation(assessment.reason)}
      </p>
      {decisiveSpanDiffers && assessment.decisiveSpanId ? (
        <div className="mt-3 flex flex-wrap items-center gap-3">
          <p className="text-sm text-[var(--continua-text-secondary)]">
            Determined by failed span <span className="font-medium">{decisiveSpanLabel}</span>.
          </p>
          <button
            type="button"
            className="inline-flex items-center justify-center rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-1.5 text-sm font-medium text-[var(--continua-text-secondary)] transition hover:bg-[var(--continua-surface-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent-faint)]"
            aria-label={`Jump to decisive span ${decisiveSpanLabel}`}
            onClick={() => onJumpToSpan(assessment.decisiveSpanId!)}
          >
            Jump to decisive span
          </button>
        </div>
      ) : null}
    </div>
  );
}
