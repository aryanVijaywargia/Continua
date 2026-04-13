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
          ? 'border-[var(--continua-accent)] bg-[var(--continua-accent-faint)] text-[var(--continua-accent)]'
          : 'border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)]'
      }`}
    >
      {children}
    </span>
  );
}
