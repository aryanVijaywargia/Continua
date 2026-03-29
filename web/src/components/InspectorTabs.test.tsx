import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { InspectorTabs } from './InspectorTabs';

describe('InspectorTabs', () => {
  it('shows the state badge only when the count is greater than zero', () => {
    const { rerender } = render(
      <InspectorTabs
        details={<div>Details</div>}
        reasoning={<div>Reasoning</div>}
        timeline={<div>Timeline</div>}
        state={<div>State</div>}
        stateCount={0}
      />
    );

    expect(screen.getByRole('button', { name: 'State' })).toHaveTextContent(/^State$/);

    rerender(
      <InspectorTabs
        details={<div>Details</div>}
        reasoning={<div>Reasoning</div>}
        timeline={<div>Timeline</div>}
        state={<div>State</div>}
        stateCount={3}
      />
    );

    expect(screen.getByRole('button', { name: 'State' })).toHaveTextContent('3');
  });
});
