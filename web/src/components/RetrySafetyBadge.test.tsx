import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { RetrySafetyBadge } from './RetrySafetyBadge';

describe('RetrySafetyBadge', () => {
  it.each([
    ['retryable', 'Retryable', 'emerald'],
    ['unsafe', 'Unsafe', 'red'],
    ['unknown', 'Unknown', 'amber'],
  ] as const)('renders %s with semantic color classes', (classification, label, color) => {
    render(
      <RetrySafetyBadge
        classification={classification}
        variant="compact"
      />
    );

    const badge = screen.getByText(label);
    expect(badge).toHaveClass(`bg-${color}-100`);
    expect(badge).toHaveClass(`text-${color}-800`);
    expect(badge).toHaveClass(`dark:bg-${color}-500/15`);
    expect(badge).toHaveClass(`dark:text-${color}-200`);
    expect(badge.className).not.toContain('#');
  });

  it('supports compact and full variants', () => {
    const { rerender } = render(
      <RetrySafetyBadge classification="retryable" variant="compact" />
    );

    expect(screen.getByText('Retryable')).toHaveClass('text-[10px]');

    rerender(<RetrySafetyBadge classification="retryable" variant="full" />);
    expect(screen.getByText('Retryable')).toHaveClass('text-[11px]');
  });

  it('passes through aria-labels', () => {
    render(
      <RetrySafetyBadge
        classification="unsafe"
        variant="full"
        aria-label="Retry safety advisory: unsafe. Inferred from effect metadata."
      />
    );

    expect(
      screen.getByLabelText(
        'Retry safety advisory: unsafe. Inferred from effect metadata.'
      )
    ).toBeInTheDocument();
  });
});
