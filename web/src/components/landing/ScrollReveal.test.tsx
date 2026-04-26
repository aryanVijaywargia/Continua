import { act, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { setMatchMediaMatches } from '../../test/matchMedia';
import { ScrollReveal } from './ScrollReveal';

let observerCallback: IntersectionObserverCallback | null = null;
const originalIntersectionObserver = globalThis.IntersectionObserver;

class MockIntersectionObserver implements IntersectionObserver {
  readonly root = null;
  readonly rootMargin = '0px';
  readonly thresholds = [0.12];

  constructor(callback: IntersectionObserverCallback) {
    observerCallback = callback;
  }

  disconnect() {}

  observe() {}

  takeRecords() {
    return [];
  }

  unobserve() {}
}

describe('ScrollReveal', () => {
  beforeEach(() => {
    observerCallback = null;
    Object.defineProperty(window, 'IntersectionObserver', {
      configurable: true,
      writable: true,
      value: MockIntersectionObserver,
    });
    Object.defineProperty(globalThis, 'IntersectionObserver', {
      configurable: true,
      writable: true,
      value: MockIntersectionObserver,
    });
  });

  afterEach(() => {
    Object.defineProperty(window, 'IntersectionObserver', {
      configurable: true,
      writable: true,
      value: originalIntersectionObserver,
    });
    Object.defineProperty(globalThis, 'IntersectionObserver', {
      configurable: true,
      writable: true,
      value: originalIntersectionObserver,
    });
  });

  it('keeps hidden content out of the accessibility tree until revealed', async () => {
    setMatchMediaMatches(false);

    const { container } = render(
      <ScrollReveal>
        <a href="#details">Hidden content</a>
      </ScrollReveal>
    );

    const wrapper = container.querySelector('.scroll-reveal');
    expect(wrapper).toHaveAttribute('aria-hidden', 'true');
    expect(wrapper).toHaveAttribute('inert');
    expect(
      screen.queryByRole('link', { name: 'Hidden content' })
    ).not.toBeInTheDocument();

    await act(async () => {
      observerCallback?.(
        [
          {
            isIntersecting: true,
            target: wrapper,
          } as IntersectionObserverEntry,
        ],
        {} as IntersectionObserver
      );
    });

    await waitFor(() => {
      expect(screen.getByRole('link', { name: 'Hidden content' })).toBeInTheDocument();
    });
    expect(wrapper).not.toHaveAttribute('aria-hidden');
    expect(wrapper).not.toHaveAttribute('inert');
  });

  it('reveals immediately when reduced motion is preferred', () => {
    setMatchMediaMatches(true);

    const { container } = render(
      <ScrollReveal>
        <button type="button">Visible immediately</button>
      </ScrollReveal>
    );

    expect(container.querySelector('.scroll-reveal')).not.toHaveAttribute(
      'aria-hidden'
    );
    expect(
      screen.getByRole('button', { name: 'Visible immediately' })
    ).toBeInTheDocument();
  });
});
