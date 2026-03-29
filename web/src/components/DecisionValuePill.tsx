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
          ? 'border-blue-200 bg-white text-blue-800 dark:border-sky-500/40 dark:bg-slate-950 dark:text-sky-200'
          : 'border-slate-200 bg-white text-slate-700 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-200'
      }`}
    >
      {children}
    </span>
  );
}
