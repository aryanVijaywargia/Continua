import type { ReactNode } from 'react';
import type { Session, SessionNarrativeSummary } from '../../api/client';
import { CopyButton } from '../../components/CopyButton';
import { HonestyNote, NotAvailableCard } from '../../components/DataState';
import { Chip } from '../../components/DebuggerKit';
import { formatExactTime } from '../../utils/format';
import { shortId } from './sessionDisplay';

interface FeedbackItem {
  score?: string | number;
  text: string;
  trace?: string;
  user?: string;
  when?: string;
}

/**
 * Right-hand drawer on the session page. It holds identity, metadata, the
 * fields the dataset does not hold, and any recorded feedback.
 */
export function SessionDetailsDrawer({
  session,
  summary,
  usageUnverified,
}: {
  session: Session;
  summary?: SessionNarrativeSummary;
  usageUnverified: boolean;
}) {
  const metadataEntries = Object.entries(session.metadata ?? {});
  const feedbackItems = getFeedbackItems(session);
  const showNotAvailable = usageUnverified || !session.user_id;

  return (
    <aside
      aria-label="Session details"
      className="border-t border-[var(--c-border)] bg-[var(--c-surface)] lg:w-80 lg:shrink-0 lg:overflow-y-auto lg:border-l lg:border-t-0"
    >
      <div className="flex flex-col gap-5 px-5 py-4">
        <div className="flex items-baseline justify-between gap-2">
          <h2 className="text-[13px] font-semibold text-[var(--c-text-primary)]">Details</h2>
          <span className="text-[11px] text-[var(--c-text-muted)]">identity · metadata</span>
        </div>

        <dl className="flex flex-col">
          <DrawerRow label="Session ID">
            <span className="flex items-center gap-2">
              <span className="font-mono" title={session.id}>
                {shortId(session.id)}
              </span>
              <CopyButton
                aria-label="Copy session ID"
                value={session.id}
                idleLabel="Copy"
                successLabel="Copied"
                className="!rounded-md !border-[var(--c-border)] !bg-[var(--c-surface)] !px-1.5 !py-0 !text-[10.5px] !text-[var(--c-text-secondary)]"
              />
            </span>
          </DrawerRow>
          <DrawerRow label="External ID">
            <span className="break-all font-mono">{session.external_id}</span>
          </DrawerRow>
          <DrawerRow label="Name">
            {session.name ? session.name : <NotRecorded />}
          </DrawerRow>
          <DrawerRow label="User ID">
            {session.user_id ? (
              <span className="break-all font-mono">{session.user_id}</span>
            ) : (
              <NotRecorded />
            )}
          </DrawerRow>
          <DrawerRow label="Metadata">
            {metadataEntries.length === 0 ? (
              <span className="text-[var(--c-text-muted)]">Empty</span>
            ) : (
              <dl className="flex flex-col gap-1.5">
                {metadataEntries.map(([key, value]) => (
                  <div key={key} className="min-w-0">
                    <dt className="font-mono text-[11px] text-[var(--c-text-muted)]">{key}</dt>
                    <dd className="max-h-40 overflow-auto whitespace-pre-wrap break-words font-mono text-[11.5px] text-[var(--c-text-primary)]">
                      {typeof value === 'string' ? value : JSON.stringify(value, null, 2)}
                    </dd>
                  </div>
                ))}
              </dl>
            )}
          </DrawerRow>
          <DrawerRow label="Created">
            <span title={session.created_at}>{formatExactTime(session.created_at)}</span>
          </DrawerRow>
          {summary?.last_activity_at ? (
            <DrawerRow label="Last activity">
              <span title={summary.last_activity_at}>
                {formatExactTime(summary.last_activity_at)}
              </span>
            </DrawerRow>
          ) : null}
        </dl>

        {showNotAvailable ? (
          <section>
            <h3 className="mb-2 text-[10.5px] font-semibold uppercase tracking-[0.06em] text-[var(--c-text-muted)]">
              Not available
            </h3>
            <div className="flex flex-col gap-2">
              {usageUnverified ? (
                <NotAvailableCard kind="unverified" title="Usage telemetry">
                  Token and cost fields read 0 or are missing, which the source cannot prove is a
                  measured zero.
                </NotAvailableCard>
              ) : null}
              {!session.user_id ? (
                <NotAvailableCard title="User identity">
                  No user was attached when this session was ingested.
                </NotAvailableCard>
              ) : null}
            </div>
          </section>
        ) : null}

        {feedbackItems.length > 0 ? (
          <section>
            <h3 className="mb-2 text-[10.5px] font-semibold uppercase tracking-[0.06em] text-[var(--c-text-muted)]">
              Feedback
            </h3>
            <div className="flex flex-col gap-2">
              {feedbackItems.map((item, index) => (
                <div
                  key={`${item.text}-${index}`}
                  className="rounded-md border border-[var(--c-border)] bg-[var(--c-app-bg)] px-3 py-2.5"
                >
                  <div className="mb-1 flex flex-wrap items-center gap-2 text-[11px]">
                    {item.score != null ? (
                      <Chip tone={isPositiveFeedback(item.score) ? 'success' : 'error'}>
                        {String(item.score)}
                      </Chip>
                    ) : null}
                    <span className="font-mono text-[var(--c-text-secondary)]">
                      {item.user || session.user_id || 'anonymous'}
                    </span>
                    {item.when ? (
                      <span className="text-[var(--c-text-muted)]">{item.when}</span>
                    ) : null}
                  </div>
                  <p className="text-[12.5px] leading-5 text-[var(--c-text-primary)]">{item.text}</p>
                  {item.trace ? (
                    <p className="mt-1 truncate font-mono text-[11px] text-[var(--c-accent-text)]">
                      {item.trace}
                    </p>
                  ) : null}
                </div>
              ))}
            </div>
          </section>
        ) : (
          <HonestyNote kind="not-captured">
            No feedback has been recorded for this session, so no feedback view is shown.
          </HonestyNote>
        )}
      </div>
    </aside>
  );
}

function DrawerRow({ children, label }: { children: ReactNode; label: string }) {
  return (
    <div className="grid grid-cols-[92px_minmax(0,1fr)] items-baseline gap-3 border-b border-[var(--c-border-subtle)] py-2 text-[12px]">
      <dt className="text-[var(--c-text-muted)]">{label}</dt>
      <dd className="min-w-0 text-[var(--c-text-primary)]">{children}</dd>
    </div>
  );
}

function NotRecorded() {
  return <span className="font-medium text-[var(--c-amber-text)]">Not recorded</span>;
}

function getFeedbackItems(session: Session): FeedbackItem[] {
  const metadata = session.metadata;
  if (!metadata) {
    return [];
  }

  const rawFeedback =
    metadata.feedback ?? metadata.feedback_items ?? metadata.ratings ?? metadata.rating;

  if (Array.isArray(rawFeedback)) {
    return rawFeedback
      .map(normalizeFeedbackItem)
      .filter((item): item is FeedbackItem => item !== null);
  }

  const normalizedSingle = normalizeFeedbackItem(rawFeedback);
  if (normalizedSingle) {
    return [normalizedSingle];
  }

  if (typeof metadata.feedback_text === 'string') {
    return [
      {
        score:
          typeof metadata.feedback_score === 'string' ||
          typeof metadata.feedback_score === 'number'
            ? metadata.feedback_score
            : undefined,
        text: metadata.feedback_text,
      },
    ];
  }

  return [];
}

function normalizeFeedbackItem(value: unknown): FeedbackItem | null {
  if (typeof value === 'string') {
    return { text: value };
  }

  if (!value || typeof value !== 'object') {
    return null;
  }

  const record = value as Record<string, unknown>;
  const rawText = record.text ?? record.comment ?? record.message ?? record.feedback;
  if (typeof rawText !== 'string' || rawText.trim() === '') {
    return null;
  }

  return {
    score:
      typeof record.score === 'string' || typeof record.score === 'number'
        ? record.score
        : typeof record.rating === 'string' || typeof record.rating === 'number'
          ? record.rating
          : undefined,
    text: rawText,
    trace: typeof record.trace === 'string' ? record.trace : undefined,
    user: typeof record.user === 'string' ? record.user : undefined,
    when: typeof record.when === 'string' ? record.when : undefined,
  };
}

function isPositiveFeedback(score: string | number): boolean {
  if (typeof score === 'number') {
    return score >= 0;
  }
  return !/negative|bad|fail|thumbs\s*down|down/i.test(score);
}
