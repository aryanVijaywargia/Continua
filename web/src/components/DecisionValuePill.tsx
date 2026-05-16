import type { ReactNode } from 'react';

interface DecisionValuePillProps {
  children: ReactNode;
  tone?: 'neutral' | 'accent';
}

export function DecisionValuePill({
  children,
  tone = 'neutral',
}: DecisionValuePillProps) {
  return (
    <span
      className={`rounded-full border px-2.5 py-1 text-xs font-medium ${
        tone === 'accent'
          ? 'border-[var(--c-accent-border)] bg-[var(--c-accent-faint)] text-[var(--c-accent-text)]'
          : 'border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-secondary)]'
      }`}
    >
      {children}
    </span>
  );
}
