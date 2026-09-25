import type { ReactNode } from 'react';
import { AlertCircle, Check, CircleDashed, Clock3, Scissors } from 'lucide-react';

/**
 * The data-honesty ladder. Every page labels a value with one of these
 * instead of inventing its own phrasing:
 * - recorded: the API returned a measured value.
 * - derived: computed from recorded values (for example end - start).
 * - unverified: a field exists, but a default cannot prove a measured value.
 * - not-captured: the dataset does not hold this data at all.
 * - unavailable: the capability is off; retrying cannot help.
 */
export type DataHonesty =
  | 'recorded'
  | 'derived'
  | 'unverified'
  | 'not-captured'
  | 'unavailable';

const HONESTY_LABELS: Record<DataHonesty, string> = {
  recorded: 'recorded',
  derived: 'derived',
  unverified: 'unverified',
  'not-captured': 'not captured',
  unavailable: 'unavailable',
};

const HONESTY_TEXT_CLASSES: Record<DataHonesty, string> = {
  recorded: 'text-[var(--c-green-text)]',
  derived: 'text-[var(--c-amber-text)]',
  unverified: 'text-[var(--c-amber-text)]',
  'not-captured': 'text-[var(--c-amber-text)]',
  unavailable: 'text-[var(--c-text-muted)]',
};

/** Small dashed tag that marks a computed value, e.g. a duration from timestamps. */
export function DerivedTag({
  className = '',
  label = 'derived',
  title = 'Computed from recorded start and end timestamps.',
}: {
  className?: string;
  label?: string;
  title?: string;
}) {
  return (
    <span
      title={title}
      className={`inline-flex h-4 items-center rounded-[3px] border border-dashed border-[var(--c-border-strong)] px-1 font-sans text-[10px] font-medium leading-none text-[var(--c-amber-text)] ${className}`}
    >
      {label}
    </span>
  );
}

/** Icon that matches a rung on the ladder. */
export function HonestyIcon({ kind, className = 'h-4 w-4' }: { kind: DataHonesty; className?: string }) {
  if (kind === 'recorded') {
    return (
      <span
        aria-hidden="true"
        className={`inline-flex shrink-0 items-center justify-center rounded-full bg-[var(--c-green-faint)] text-[var(--c-green-text)] ${className}`}
      >
        <Check className="h-2.5 w-2.5" strokeWidth={3} />
      </span>
    );
  }
  if (kind === 'derived' || kind === 'unavailable') {
    return (
      <CircleDashed
        aria-hidden="true"
        className={`shrink-0 text-[var(--c-text-muted)] ${className}`}
      />
    );
  }
  return (
    <AlertCircle aria-hidden="true" className={`shrink-0 text-[var(--c-amber)] ${className}`} />
  );
}

/** One line that pairs a subject with its ladder state, e.g. in a coverage card. */
export function HonestyRow({
  kind,
  label,
  value,
}: {
  kind: DataHonesty;
  label: ReactNode;
  value?: ReactNode;
}) {
  return (
    <li className="flex items-center gap-2.5 py-1.5 text-[13px] text-[var(--c-text-primary)]">
      <HonestyIcon kind={kind} />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      <span className={`shrink-0 text-xs font-medium ${HONESTY_TEXT_CLASSES[kind]}`}>
        {value ?? HONESTY_LABELS[kind]}
      </span>
    </li>
  );
}

/** Amber pill that names a value the dataset cannot prove, e.g. "Usage not verified". */
export function UnverifiedPill({
  children,
  title,
}: {
  children: ReactNode;
  title?: string;
}) {
  return (
    <span
      title={title}
      className="inline-flex h-6 items-center gap-1.5 whitespace-nowrap rounded-full border border-[var(--c-amber-border)] bg-[var(--c-amber-faint)] px-2.5 text-xs font-medium text-[var(--c-amber-text)]"
    >
      <AlertCircle aria-hidden="true" className="h-3.5 w-3.5" />
      {children}
    </span>
  );
}

export function ReadOnlyBadge({ className = '' }: { className?: string }) {
  return (
    <span
      title="This console reads recorded data. It cannot change runs or traces."
      className={`inline-flex h-6 items-center whitespace-nowrap rounded border border-[var(--c-border)] bg-[var(--c-surface-muted)] px-2 font-mono text-[10.5px] font-semibold uppercase tracking-[0.06em] text-[var(--c-text-secondary)] ${className}`}
    >
      Read-only
    </span>
  );
}

/** Amber card for a capability or field that the dataset does not hold. */
export function NotAvailableCard({
  children,
  kind = 'not-captured',
  title,
}: {
  children: ReactNode;
  kind?: Extract<DataHonesty, 'unverified' | 'not-captured' | 'unavailable'>;
  title: ReactNode;
}) {
  const dashed = kind === 'unavailable';
  return (
    <div
      className={`rounded-md border px-3 py-2.5 ${
        dashed
          ? 'border-dashed border-[var(--c-border-strong)] bg-transparent'
          : 'border-[var(--c-amber-border)] bg-[var(--c-amber-faint)]'
      }`}
    >
      <div className="flex items-center gap-2 text-[13px] font-semibold text-[var(--c-text-primary)]">
        <HonestyIcon kind={kind} className="h-3.5 w-3.5" />
        {title}
      </div>
      <div className="mt-1 text-xs leading-5 text-[var(--c-text-secondary)]">{children}</div>
    </div>
  );
}

/** Dashed explanatory note, used under tables and in empty sections. */
export function HonestyNote({
  children,
  className = '',
  kind = 'derived',
}: {
  children: ReactNode;
  className?: string;
  kind?: DataHonesty;
}) {
  return (
    <p
      className={`flex items-start gap-2 text-xs leading-5 text-[var(--c-text-secondary)] ${className}`}
    >
      <HonestyIcon kind={kind} className="mt-0.5 h-3.5 w-3.5" />
      <span>{children}</span>
    </p>
  );
}

export function StaleNotice({ age, children }: { age: ReactNode; children?: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-xs font-medium text-[var(--c-amber-text)]">
      <Clock3 aria-hidden="true" className="h-3.5 w-3.5" />
      stale · {age}
      {children}
    </span>
  );
}

export function TruncatedNotice({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-xs font-medium text-[var(--c-amber-text)]">
      <Scissors aria-hidden="true" className="h-3.5 w-3.5" />
      {children}
    </span>
  );
}
