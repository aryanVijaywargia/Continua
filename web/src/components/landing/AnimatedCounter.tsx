import { useEffect, useRef, useState } from 'react';
import { useMediaQuery } from '../../hooks/useMediaQuery';

interface AnimatedCounterProps {
  /** Target number to count up to (e.g. 12.4, 250, 4800) */
  end: number;
  /** Suffix appended after the number (e.g. "k+", "+") */
  suffix?: string;
  /** Number of decimal places to display */
  decimals?: number;
  /** Duration of the counting animation in ms */
  duration?: number;
}

/**
 * Counter that animates from 0 → end when it scrolls into view.
 * Uses ease-out cubic easing for a satisfying deceleration.
 * Starts counting when 50% of the element is visible.
 */
export function AnimatedCounter({
  end,
  suffix = '',
  decimals = 0,
  duration = 1800,
}: AnimatedCounterProps) {
  const ref = useRef<HTMLSpanElement>(null);
  const prefersReducedMotion = useMediaQuery(
    '(prefers-reduced-motion: reduce)'
  );
  const [value, setValue] = useState(0);
  const [started, setStarted] = useState(false);

  useEffect(() => {
    if (prefersReducedMotion) {
      setValue(Number(end.toFixed(decimals)));
      setStarted(true);
      return;
    }

    const el = ref.current;
    if (!el) return;
    if (typeof IntersectionObserver !== 'function') {
      setStarted(true);
      return;
    }

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setStarted(true);
          observer.unobserve(el);
        }
      },
      { threshold: 0.5 },
    );

    observer.observe(el);
    return () => observer.disconnect();
  }, [decimals, end, prefersReducedMotion]);

  useEffect(() => {
    if (!started || prefersReducedMotion) {
      return;
    }

    const startTime = performance.now();
    let animationFrameId = 0;

    function tick(now: number) {
      const elapsed = now - startTime;
      const progress = Math.min(elapsed / duration, 1);
      // Ease-out cubic: decelerates smoothly
      const eased = 1 - Math.pow(1 - progress, 3);
      setValue(Number((end * eased).toFixed(decimals)));
      if (progress < 1) {
        animationFrameId = requestAnimationFrame(tick);
      }
    }

    animationFrameId = requestAnimationFrame(tick);

    return () => {
      window.cancelAnimationFrame(animationFrameId);
    };
  }, [started, end, duration, decimals, prefersReducedMotion]);

  const displayValue = prefersReducedMotion || started ? value : 0;

  return (
    <span ref={ref}>
      {displayValue}
      {suffix}
    </span>
  );
}
