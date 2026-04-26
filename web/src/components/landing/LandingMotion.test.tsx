import { render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { setMatchMediaMatches } from '../../test/matchMedia';
import { AnimatedCounter } from './AnimatedCounter';
import { CheckpointIllustration } from './CheckpointIllustration';
import { HeroIllustration } from './HeroIllustration';

describe('landing motion', () => {
  it('renders static illustrations when reduced motion is preferred', () => {
    setMatchMediaMatches(true);

    const heroView = render(<HeroIllustration />);
    expect(
      heroView.container.querySelectorAll(
        'animate, animateMotion, animateTransform'
      )
    ).toHaveLength(0);
    heroView.unmount();

    const checkpointView = render(<CheckpointIllustration />);
    expect(
      checkpointView.container.querySelectorAll(
        'animate, animateMotion, animateTransform'
      )
    ).toHaveLength(0);
  });

  it('shows the final counter value without running the count-up animation', async () => {
    setMatchMediaMatches(true);

    render(<AnimatedCounter end={12.4} decimals={1} suffix="k+" />);

    await waitFor(() => {
      expect(screen.getByText('12.4k+')).toBeInTheDocument();
    });
  });
});
