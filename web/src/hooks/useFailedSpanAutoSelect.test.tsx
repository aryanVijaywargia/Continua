import { renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useFailedSpanAutoSelect } from './useFailedSpanAutoSelect';

type Options = Parameters<typeof useFailedSpanAutoSelect>[0];

function baseOptions(overrides: Partial<Options> = {}): Options {
  // By default spanParam and selectedSpanId agree (a param that resolves).
  const spanParam = overrides.spanParam ?? null;
  return {
    traceId: 'trace-1',
    isReady: true,
    traceStatus: 'FAILED',
    spanParam,
    selectedSpanId: spanParam,
    primaryFailedSpanId: 'failed-span',
    setSpanParam: vi.fn(),
    ...overrides,
  };
}

describe('useFailedSpanAutoSelect', () => {
  it('selects the primary failed span once on a fresh failed trace', () => {
    const setSpanParam = vi.fn();
    renderHook(() =>
      useFailedSpanAutoSelect(baseOptions({ setSpanParam }))
    );
    expect(setSpanParam).toHaveBeenCalledTimes(1);
    expect(setSpanParam).toHaveBeenCalledWith('failed-span');
  });

  it('does nothing when a span is already selected via the URL', () => {
    const setSpanParam = vi.fn();
    renderHook(() =>
      useFailedSpanAutoSelect(baseOptions({ spanParam: 'root', setSpanParam }))
    );
    expect(setSpanParam).not.toHaveBeenCalled();
  });

  it('does not latch on an invalid span param; auto-opens once it is cleared', () => {
    // ?span=bad-id present but it does not resolve to a real span
    // (selectedSpanId null). The hook must wait for the cleared state rather
    // than latching on the phantom — otherwise auto-open is blocked forever.
    const setSpanParam = vi.fn();
    const { rerender } = renderHook(
      (props: Options) => useFailedSpanAutoSelect(props),
      {
        initialProps: baseOptions({
          spanParam: 'bad-id',
          selectedSpanId: null,
          setSpanParam,
        }),
      }
    );
    expect(setSpanParam).not.toHaveBeenCalled(); // did not latch on phantom

    // page clears the invalid param
    rerender(baseOptions({ spanParam: null, selectedSpanId: null, setSpanParam }));
    expect(setSpanParam).toHaveBeenCalledTimes(1);
    expect(setSpanParam).toHaveBeenCalledWith('failed-span');
  });

  it('does nothing for a non-failed trace', () => {
    const setSpanParam = vi.fn();
    renderHook(() =>
      useFailedSpanAutoSelect(
        baseOptions({ traceStatus: 'RUNNING', setSpanParam })
      )
    );
    expect(setSpanParam).not.toHaveBeenCalled();
  });

  it('does not re-open after the operator clears the selection in the same trace load', () => {
    const setSpanParam = vi.fn();
    const { rerender } = renderHook(
      (props: Options) => useFailedSpanAutoSelect(props),
      { initialProps: baseOptions({ setSpanParam }) }
    );
    expect(setSpanParam).toHaveBeenCalledTimes(1);

    // simulate the URL having been set, then the operator closing the panel
    rerender(baseOptions({ spanParam: 'failed-span', setSpanParam }));
    rerender(baseOptions({ spanParam: null, setSpanParam }));

    // latch already fired for this trace -> no second auto-open
    expect(setSpanParam).toHaveBeenCalledTimes(1);
  });

  it('auto-opens again when the trace changes (fresh load)', () => {
    const setSpanParam = vi.fn();
    const { rerender } = renderHook(
      (props: Options) => useFailedSpanAutoSelect(props),
      { initialProps: baseOptions({ setSpanParam }) }
    );
    rerender(baseOptions({ spanParam: null, setSpanParam })); // same trace, no re-fire
    expect(setSpanParam).toHaveBeenCalledTimes(1);

    rerender(
      baseOptions({ traceId: 'trace-2', spanParam: null, setSpanParam })
    );
    expect(setSpanParam).toHaveBeenCalledTimes(2);
    expect(setSpanParam).toHaveBeenLastCalledWith('failed-span');
  });

  it('does not auto-open after clearing when the trace opened with an explicit span', () => {
    // Failed trace deep-linked to an explicit span: the system must treat this
    // load as already handled, so clearing the panel must NOT trigger auto-open.
    const setSpanParam = vi.fn();
    const { rerender } = renderHook(
      (props: Options) => useFailedSpanAutoSelect(props),
      {
        initialProps: baseOptions({
          spanParam: 'root',
          selectedSpanId: 'root',
          setSpanParam,
        }),
      }
    );
    expect(setSpanParam).not.toHaveBeenCalled(); // explicit span present on load

    rerender(baseOptions({ spanParam: null, selectedSpanId: null, setSpanParam })); // operator clears
    expect(setSpanParam).not.toHaveBeenCalled(); // still no auto-open this load
  });

  it('does not latch on an explicit span before span data is ready', () => {
    // A pre-ready render with a span param must not mark the trace handled;
    // once ready with no selection, auto-open should still fire.
    const setSpanParam = vi.fn();
    const { rerender } = renderHook(
      (props: Options) => useFailedSpanAutoSelect(props),
      {
        initialProps: baseOptions({
          isReady: false,
          spanParam: 'root',
          setSpanParam,
        }),
      }
    );
    expect(setSpanParam).not.toHaveBeenCalled();

    rerender(baseOptions({ isReady: true, spanParam: null, setSpanParam }));
    expect(setSpanParam).toHaveBeenCalledTimes(1);
    expect(setSpanParam).toHaveBeenCalledWith('failed-span');
  });

  it('waits until span data is ready before acting', () => {
    const setSpanParam = vi.fn();
    const { rerender } = renderHook(
      (props: Options) => useFailedSpanAutoSelect(props),
      { initialProps: baseOptions({ isReady: false, setSpanParam }) }
    );
    expect(setSpanParam).not.toHaveBeenCalled();

    rerender(baseOptions({ isReady: true, setSpanParam }));
    expect(setSpanParam).toHaveBeenCalledTimes(1);
  });
});
