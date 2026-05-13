import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { setMatchMediaMatches } from '../../test/matchMedia';
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
});
