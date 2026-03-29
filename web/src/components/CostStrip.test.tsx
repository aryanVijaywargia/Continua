import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { CostStrip } from './CostStrip';

const WINDOW = {
  startMs: Date.parse('2026-03-14T10:00:00.000Z'),
  endMs: Date.parse('2026-03-14T10:00:10.000Z'),
  durationMs: 10_000,
};

describe('CostStrip', () => {
  it('renders a step chart for a cost series', () => {
    render(
      <CostStrip
        window={WINDOW}
        series={{
          partial: false,
          points: [
            {
              anchorMs: Date.parse('2026-03-14T10:00:02.000Z'),
              cumulativeCostUsd: 0.02,
              incrementalCostUsd: 0.02,
            },
            {
              anchorMs: Date.parse('2026-03-14T10:00:06.000Z'),
              cumulativeCostUsd: 0.05,
              incrementalCostUsd: 0.03,
            },
          ],
          totalCostUsd: 0.05,
        }}
      />
    );

    expect(screen.getByText('Cumulative cost')).toBeInTheDocument();
    expect(screen.getByLabelText('Cumulative cost chart')).toBeInTheDocument();
    expect(screen.getByText('$0.05')).toBeInTheDocument();
  });

  it('renders the partial indicator for running traces', () => {
    render(
      <CostStrip
        window={WINDOW}
        series={{
          partial: true,
          points: [
            {
              anchorMs: Date.parse('2026-03-14T10:00:04.000Z'),
              cumulativeCostUsd: 0.02,
              incrementalCostUsd: 0.02,
            },
          ],
          totalCostUsd: 0.02,
        }}
      />
    );

    expect(screen.getByText('Partial')).toBeInTheDocument();
  });

  it('renders nothing when there is no visible cost series', () => {
    const { container } = render(<CostStrip window={WINDOW} series={null} />);

    expect(container.firstChild).toBeNull();
  });
});
