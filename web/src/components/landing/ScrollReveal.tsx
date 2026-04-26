import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useMediaQuery } from '../../hooks/useMediaQuery';

interface ScrollRevealProps {
  children: ReactNode;
  className?: string;
  /** Extra delay in ms before the reveal transition triggers */
  delay?: number;
}

/**
 * Wraps children in a div that fades-in + translates-up when it enters
 * the viewport. Once revealed, it stays visible permanently.
 * Requires the `.scroll-reveal` / `.scroll-reveal.visible` classes in globals.css.
 */
export function ScrollReveal({ children, className = '', delay = 0 }: ScrollRevealProps) {
  const ref = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState(false);
  const prefersReducedMotion = useMediaQuery(
    '(prefers-reduced-motion: reduce)'
  );
  const isRevealed = visible || prefersReducedMotion;

  useEffect(() => {
    if (prefersReducedMotion) {
      setVisible(true);
      return;
    }

    const el = ref.current;
    if (!el) return;
    if (typeof IntersectionObserver !== 'function') {
      setVisible(true);
      return;
    }

    // Reveal when 12% of the element is visible, with a slight bottom offset
    // so items reveal before they're fully in the viewport
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setVisible(true);
          observer.unobserve(el);
        }
      },
      { threshold: 0.12, rootMargin: '0px 0px -40px 0px' },
    );

    observer.observe(el);
    return () => observer.disconnect();
  }, [prefersReducedMotion]);

  const hiddenInteractionProps = !isRevealed
    ? ({
        'aria-hidden': true,
        inert: '',
      } as Record<string, string | boolean>)
    : {};

  return (
    <div
      ref={ref}
      className={`scroll-reveal${isRevealed ? ' visible' : ''} ${className}`}
      style={delay > 0 ? { transitionDelay: `${delay}ms` } : undefined}
      {...hiddenInteractionProps}
    >
      {children}
    </div>
  );
}
