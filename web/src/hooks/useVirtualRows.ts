import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type RefObject,
  type UIEvent,
} from 'react';

interface UseVirtualRowsOptions<T> {
  defaultViewportHeight?: number;
  estimatedRowHeight: number;
  overscan?: number;
  rows: T[];
}

interface VirtualRow<T> {
  index: number;
  row: T;
}

interface UseVirtualRowsResult<T> {
  containerRef: RefObject<HTMLDivElement>;
  onScroll: (event: UIEvent<HTMLDivElement>) => void;
  paddingBottom: number;
  paddingTop: number;
  scrollToIndex: (index: number) => void;
  virtualRows: VirtualRow<T>[];
}

const DEFAULT_OVERSCAN = 8;
const DEFAULT_VIEWPORT_HEIGHT = 720;

export function useVirtualRows<T>({
  defaultViewportHeight = DEFAULT_VIEWPORT_HEIGHT,
  estimatedRowHeight,
  overscan = DEFAULT_OVERSCAN,
  rows,
}: UseVirtualRowsOptions<T>): UseVirtualRowsResult<T> {
  const containerRef = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(defaultViewportHeight);

  const measureViewport = useCallback(() => {
    const nextViewportHeight =
      containerRef.current?.clientHeight || defaultViewportHeight;

    setViewportHeight((currentViewportHeight) =>
      currentViewportHeight === nextViewportHeight
        ? currentViewportHeight
        : nextViewportHeight
    );
  }, [defaultViewportHeight]);

  useEffect(() => {
    measureViewport();

    const container = containerRef.current;
    if (!container) {
      return;
    }

    const resizeObserver =
      typeof ResizeObserver === 'undefined'
        ? null
        : new ResizeObserver(() => {
            measureViewport();
          });

    resizeObserver?.observe(container);
    window.addEventListener('resize', measureViewport);

    return () => {
      resizeObserver?.disconnect();
      window.removeEventListener('resize', measureViewport);
    };
  }, [measureViewport]);

  const onScroll = useCallback((event: UIEvent<HTMLDivElement>) => {
    const nextScrollTop = event.currentTarget.scrollTop;
    setScrollTop((currentScrollTop) =>
      currentScrollTop === nextScrollTop ? currentScrollTop : nextScrollTop
    );
  }, []);

  const { endIndex, startIndex } = useMemo(() => {
    const visibleRowCount = Math.max(
      1,
      Math.ceil(viewportHeight / estimatedRowHeight)
    );
    const rawStartIndex = Math.floor(scrollTop / estimatedRowHeight);
    const nextStartIndex = Math.max(0, rawStartIndex - overscan);
    const nextEndIndex = Math.min(
      rows.length,
      rawStartIndex + visibleRowCount + overscan
    );

    return {
      endIndex: nextEndIndex,
      startIndex: nextStartIndex,
    };
  }, [estimatedRowHeight, overscan, rows.length, scrollTop, viewportHeight]);

  const virtualRows = useMemo(
    () =>
      rows.slice(startIndex, endIndex).map((row, offset) => ({
        index: startIndex + offset,
        row,
      })),
    [endIndex, rows, startIndex]
  );

  const scrollToIndex = useCallback(
    (index: number) => {
      const container = containerRef.current;
      if (!container) {
        return;
      }

      const nextViewportHeight = container.clientHeight || defaultViewportHeight;
      const currentTop = container.scrollTop;
      const currentBottom = currentTop + nextViewportHeight;
      const targetTop = index * estimatedRowHeight;
      const targetBottom = targetTop + estimatedRowHeight;
      let nextScrollTop = currentTop;

      if (targetTop < currentTop) {
        nextScrollTop = Math.max(targetTop - overscan * estimatedRowHeight, 0);
      } else if (targetBottom > currentBottom) {
        nextScrollTop = Math.max(
          targetBottom - nextViewportHeight + overscan * estimatedRowHeight,
          0
        );
      }

      if (nextScrollTop === currentTop) {
        return;
      }

      container.scrollTop = nextScrollTop;
      setScrollTop(nextScrollTop);
    },
    [defaultViewportHeight, estimatedRowHeight, overscan]
  );

  return {
    containerRef,
    onScroll,
    paddingBottom: Math.max(0, (rows.length - endIndex) * estimatedRowHeight),
    paddingTop: startIndex * estimatedRowHeight,
    scrollToIndex,
    virtualRows,
  };
}
