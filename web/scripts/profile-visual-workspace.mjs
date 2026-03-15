import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { JSDOM } from 'jsdom';
import React from 'react';
import { flushSync } from 'react-dom';
import { createRoot } from 'react-dom/client';
import { createServer } from 'vite';

const ITERATIONS = 15;
const WARMUP_ITERATIONS = 3;
const SPAN_COUNTS = [200, 400, 800, 1200];
const SEARCH_TIMEOUT_MS = 2_000;
const EMPTY_SET = new Set();
const EMPTY_MAP = new Map();
const OUTPUT_PATH = path.resolve(
  process.cwd(),
  '../openspec/changes/add-visual-execution-workspace/artifacts/visual-workspace-profile.json'
);

installDomGlobals();

const vite = await createServer({
  root: process.cwd(),
  appType: 'custom',
  configFile: path.resolve(process.cwd(), 'vite.config.ts'),
  logLevel: 'error',
  server: {
    middlewareMode: true,
  },
});

try {
  const [
    { ExecutionWaterfall },
    { TreeRail },
    { useTreeRailState },
    { buildSpanTree, collectExpandableSpanIds, deriveVisibleRows },
  ] = await Promise.all([
    vite.ssrLoadModule('/src/components/ExecutionWaterfall.tsx'),
    vite.ssrLoadModule('/src/components/TreeRail.tsx'),
    vite.ssrLoadModule('/src/hooks/useTreeRailState.ts'),
    vite.ssrLoadModule('/src/utils/spanTree.ts'),
  ]);

  const results = {};

  for (const spanCount of SPAN_COUNTS) {
    const spans = createSyntheticSpans(spanCount);
    const spanTree = buildSpanTree(spans);
    const expandableSpanIds = collectExpandableSpanIds(spanTree);
    const expandedSpanIds = new Set(expandableSpanIds);
    const visibleRows = deriveVisibleRows(spanTree, expandedSpanIds);
    const sharedData = {
      spans,
      spanTree,
      spanIndex: new Map(spans.map((span) => [span.span_id, span])),
      expandableSpanIds,
      visibleRows,
      traceStartedAt: spans[0]?.started_at,
      traceEndedAt: spans.at(-1)?.ended_at,
    };

    results[spanCount] = {
      expandAll: await benchmarkOperation(
        `expand all (${spanCount})`,
        () => measureExpandAll(TreeRail, sharedData)
      ),
      collapseAll: await benchmarkOperation(
        `collapse all (${spanCount})`,
        () => measureCollapseAll(TreeRail, sharedData)
      ),
      waterfallRender: await benchmarkOperation(
        `waterfall render (${spanCount})`,
        () => measureWaterfallRender(ExecutionWaterfall, sharedData)
      ),
      searchInputCommit: await benchmarkOperation(
        `search input commit (${spanCount})`,
        () => measureSearch(useTreeRailState, sharedData, 'commit')
      ),
      searchDeferredSettle: await benchmarkOperation(
        `search deferred settle (${spanCount})`,
        () => measureSearch(useTreeRailState, sharedData, 'settle')
      ),
    };
  }

  const artifact = {
    generatedAt: new Date().toISOString(),
    methodology: {
      environment: 'jsdom via Vite SSR module loader',
      iterations: ITERATIONS,
      warmupIterations: WARMUP_ITERATIONS,
      spanCounts: SPAN_COUNTS,
      note:
        'Measurements are conservative development-mode timings on the local machine. They capture React render/commit wall time in the benchmark harness.',
    },
    machine: {
      platform: `${os.platform()} ${os.release()}`,
      cpu: os.cpus()[0]?.model ?? 'unknown',
      memoryGb: Number((os.totalmem() / 1024 / 1024 / 1024).toFixed(1)),
      node: process.version,
    },
    results,
  };
  const conclusions = {
    expandCollapseWithinOneFrameUnder500:
      results[200].expandAll.p95Ms < 16 &&
      results[200].collapseAll.p95Ms < 16 &&
      results[400].expandAll.p95Ms < 16 &&
      results[400].collapseAll.p95Ms < 16,
    waterfallRenderUnder100Ms:
      SPAN_COUNTS.every((count) => results[count].waterfallRender.p95Ms < 100),
    searchInputNotBlockedBeyondDeferredCycle:
      SPAN_COUNTS.every((count) => results[count].searchInputCommit.p95Ms < 16),
    expandAllRevealThreshold: 700,
    thresholdFinalized: false,
    thresholdRationale: '',
  };
  const budgetsValidated =
    conclusions.expandCollapseWithinOneFrameUnder500 &&
    conclusions.waterfallRenderUnder100Ms &&
    conclusions.searchInputNotBlockedBeyondDeferredCycle;

  conclusions.thresholdFinalized = budgetsValidated;
  conclusions.thresholdRationale = budgetsValidated
    ? 'Benchmarks at 200, 400, 800, and 1200 spans stayed within the Phase 9 budgets. The 700-row reveal guard remains a conservative cutoff that warns before very large expansions without prompting on smaller traces.'
    : 'The current 700-row guard remains a conservative cutoff below the 800-span tier, but this benchmark run did not validate the Phase 9 frame/render budgets. Keep tasks 5.13 and 5.14 open until browser-side profiling and/or optimizations bring the numbers within target.';
  artifact.conclusions = conclusions;

  await fs.mkdir(path.dirname(OUTPUT_PATH), { recursive: true });
  await fs.writeFile(OUTPUT_PATH, `${JSON.stringify(artifact, null, 2)}\n`);

  printSummary(artifact);
} finally {
  await vite.close();
}

async function benchmarkOperation(name, runSample) {
  for (let iteration = 0; iteration < WARMUP_ITERATIONS; iteration += 1) {
    await runSample();
  }

  const samples = [];
  for (let iteration = 0; iteration < ITERATIONS; iteration += 1) {
    const durationMs = await runSample();
    samples.push(durationMs);
  }

  const summary = summarize(samples);
  console.log(
    `${name.padEnd(28)} mean=${summary.meanMs.toFixed(2)}ms p95=${summary.p95Ms.toFixed(2)}ms max=${summary.maxMs.toFixed(2)}ms`
  );
  return summary;
}

async function measureExpandAll(TreeRail, sharedData) {
  let profileDurationMs = 0;
  const { cleanup, container } = mount(
    React.createElement(
      React.Profiler,
      {
        id: 'tree-rail-expand',
        onRender(_id, phase, actualDuration) {
          if (phase === 'update') {
            profileDurationMs = actualDuration;
          }
        },
      },
      React.createElement(TreeRailHarness, {
        TreeRail,
        sharedData,
        initiallyExpanded: false,
      })
    )
  );

  await nextTick();
  const button = findButton(container, 'Expand all');
  flushSync(() => {
    button.click();
  });
  await waitFor(() => profileDurationMs > 0, SEARCH_TIMEOUT_MS);

  const visibleRowCount = getVisibleRowCount(container);
  cleanup();

  if (visibleRowCount !== sharedData.visibleRows.length) {
    throw new Error(
      `Expand all benchmark expected ${sharedData.visibleRows.length} visible rows, got ${visibleRowCount}`
    );
  }

  return profileDurationMs;
}

async function measureCollapseAll(TreeRail, sharedData) {
  let profileDurationMs = 0;
  const { cleanup, container } = mount(
    React.createElement(
      React.Profiler,
      {
        id: 'tree-rail-collapse',
        onRender(_id, phase, actualDuration) {
          if (phase === 'update') {
            profileDurationMs = actualDuration;
          }
        },
      },
      React.createElement(TreeRailHarness, {
        TreeRail,
        sharedData,
        initiallyExpanded: true,
      })
    )
  );

  await nextTick();
  const button = findButton(container, 'Collapse all');
  flushSync(() => {
    button.click();
  });
  await waitFor(() => profileDurationMs > 0, SEARCH_TIMEOUT_MS);

  const visibleRowCount = getVisibleRowCount(container);
  cleanup();

  if (visibleRowCount !== 1) {
    throw new Error(
      `Collapse all benchmark expected 1 visible root row, got ${visibleRowCount}`
    );
  }

  return profileDurationMs;
}

async function measureWaterfallRender(ExecutionWaterfall, sharedData) {
  let profileDurationMs = 0;
  const { cleanup } = mount(
    React.createElement(
      React.Profiler,
      {
        id: 'waterfall-render',
        onRender(_id, phase, actualDuration) {
          if (phase === 'mount') {
            profileDurationMs = actualDuration;
          }
        },
      },
      React.createElement(ExecutionWaterfall, {
        events: [],
        rows: sharedData.visibleRows,
        selectedSpanId: null,
        onSelectSpanAndShowDetails: () => {},
        revealTarget: null,
        revealVersion: 0,
        spans: sharedData.spans,
        traceStartedAt: sharedData.traceStartedAt,
        traceEndedAt: sharedData.traceEndedAt,
      })
    )
  );

  await waitFor(() => profileDurationMs > 0, SEARCH_TIMEOUT_MS);
  cleanup();
  return profileDurationMs;
}

async function measureSearch(useTreeRailState, sharedData, mode) {
  let latestState = null;
  const { cleanup } = mount(
    React.createElement(SearchHarness, {
      useTreeRailState,
      sharedData,
      onStateChange(nextState) {
        latestState = nextState;
      },
    })
  );

  await waitFor(
    () => typeof latestState?.setSearchQueryInput === 'function',
    SEARCH_TIMEOUT_MS
  );

  const query = sharedData.spans.at(-1).name;
  const startedAt = performance.now();
  flushSync(() => {
    latestState.setSearchQueryInput(query);
  });
  const commitDurationMs = performance.now() - startedAt;

  if (mode === 'commit') {
    cleanup();
    return commitDurationMs;
  }

  await waitFor(() => latestState?.matchedSpanIds?.size === 1, SEARCH_TIMEOUT_MS);
  const totalDurationMs = performance.now() - startedAt;
  cleanup();
  return totalDurationMs;
}

function TreeRailHarness({ TreeRail, sharedData, initiallyExpanded }) {
  const [expandedSpanIds, setExpandedSpanIds] = React.useState(
    () => new Set(initiallyExpanded ? sharedData.expandableSpanIds : [])
  );
  const [visibleRows, setVisibleRows] = React.useState(() =>
    deriveVisibleRowsSafe(sharedData, expandedSpanIds)
  );

  return React.createElement(
    'div',
    null,
    React.createElement(TreeRail, {
      expandableSpanIds: sharedData.expandableSpanIds,
      expandedSpanIds,
      failedSpanIds: EMPTY_SET,
      inlineErrorPreviews: EMPTY_MAP,
      onSelectSpan: () => {},
      onToggleExpand: (spanId) => {
        setExpandedSpanIds((currentExpandedSpanIds) => {
          const nextExpandedSpanIds = new Set(currentExpandedSpanIds);
          if (nextExpandedSpanIds.has(spanId)) {
            nextExpandedSpanIds.delete(spanId);
          } else {
            nextExpandedSpanIds.add(spanId);
          }
          return nextExpandedSpanIds;
        });
      },
      onVisibleRowsChange: setVisibleRows,
      primaryAncestorPath: EMPTY_SET,
      revealKey: 0,
      revealPath: EMPTY_SET,
      selectedSpanId: null,
      setExpandedSpanIds,
      spanIndex: sharedData.spanIndex,
      spanTree: sharedData.spanTree,
      spans: sharedData.spans,
    }),
    React.createElement('div', { 'data-visible-row-count': String(visibleRows.length) })
  );
}

function SearchHarness({ useTreeRailState, sharedData, onStateChange }) {
  const [expandedSpanIds, setExpandedSpanIds] = React.useState(
    () => new Set(sharedData.expandableSpanIds)
  );
  const state = useTreeRailState({
    expandableSpanIds: sharedData.expandableSpanIds,
    expandedSpanIds,
    inlineErrorPreviews: EMPTY_MAP,
    setExpandedSpanIds,
    spanIndex: sharedData.spanIndex,
    spanTree: sharedData.spanTree,
    spans: sharedData.spans,
  });

  React.useEffect(() => {
    onStateChange(state);
  }, [onStateChange, state]);

  return null;
}

function mount(element) {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);

  flushSync(() => {
    root.render(element);
  });

  return {
    container,
    cleanup() {
      flushSync(() => {
        root.unmount();
      });
      container.remove();
    },
  };
}

function findButton(container, label) {
  const button = Array.from(container.querySelectorAll('button')).find(
    (candidate) => candidate.textContent?.trim() === label
  );

  if (!(button instanceof window.HTMLButtonElement)) {
    throw new Error(`Unable to find button "${label}"`);
  }

  return button;
}

function getVisibleRowCount(container) {
  const count = container
    .querySelector('[data-visible-row-count]')
    ?.getAttribute('data-visible-row-count');

  if (!count) {
    throw new Error('Unable to find visible row count marker');
  }

  return Number(count);
}

async function waitFor(predicate, timeoutMs) {
  const timeoutAt = performance.now() + timeoutMs;

  while (performance.now() < timeoutAt) {
    if (predicate()) {
      return;
    }

    await nextTick();
  }

  throw new Error(`Timed out after ${timeoutMs}ms waiting for benchmark condition`);
}

function nextTick() {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

function summarize(samples) {
  const sorted = [...samples].sort((left, right) => left - right);
  const sum = sorted.reduce((total, sample) => total + sample, 0);

  return {
    iterations: samples.length,
    meanMs: Number((sum / sorted.length).toFixed(2)),
    p95Ms: Number(percentile(sorted, 0.95).toFixed(2)),
    minMs: Number(sorted[0].toFixed(2)),
    maxMs: Number(sorted.at(-1).toFixed(2)),
  };
}

function percentile(sortedValues, ratio) {
  const index = Math.min(
    sortedValues.length - 1,
    Math.max(0, Math.ceil(sortedValues.length * ratio) - 1)
  );
  return sortedValues[index];
}

function printSummary(artifact) {
  console.log('\nSummary');
  console.log(`Artifact written to ${OUTPUT_PATH}`);
  console.log(
    `Expand/collapse <500 spans within one frame: ${artifact.conclusions.expandCollapseWithinOneFrameUnder500 ? 'yes' : 'no'}`
  );
  console.log(
    `Waterfall render under 100ms: ${artifact.conclusions.waterfallRenderUnder100Ms ? 'yes' : 'no'}`
  );
  console.log(
    `Search input commits under one frame: ${artifact.conclusions.searchInputNotBlockedBeyondDeferredCycle ? 'yes' : 'no'}`
  );
  console.log(
    `Reveal threshold: ${artifact.conclusions.expandAllRevealThreshold} (${artifact.conclusions.thresholdFinalized ? 'finalized' : 'provisional'})`
  );
}

function createSyntheticSpans(count) {
  const spans = [];
  const startMs = Date.parse('2026-03-14T10:00:00.000Z');
  const branchingFactor = 4;

  for (let index = 0; index < count; index += 1) {
    const spanId = index === 0 ? 'root' : `span-${index}`;
    const parentIndex =
      index === 0 ? null : Math.floor((index - 1) / branchingFactor);
    const offsetMs = index * 5;
    const durationMs = 40 + (index % 11) * 3;
    const name = index === count - 1 ? `needle-${count}` : `Span ${index}`;

    spans.push({
      id: `uuid-${spanId}`,
      trace_id: 'benchmark-trace',
      span_id: spanId,
      parent_span_id: parentIndex === null ? undefined : spans[parentIndex].span_id,
      name,
      kind: index % 5 === 0 ? 'LLM' : index % 3 === 0 ? 'TOOL' : 'CHAIN',
      status: index % 13 === 0 ? 'FAILED' : 'COMPLETED',
      started_at: new Date(startMs + offsetMs).toISOString(),
      ended_at: new Date(startMs + offsetMs + durationMs).toISOString(),
      latency_ms: durationMs,
      tokens_in: 32 + (index % 17),
      tokens_out: 18 + (index % 11),
      cost_usd: Number((0.001 + index / 10_000).toFixed(4)),
      error_message: index % 13 === 0 ? `Synthetic failure ${index}` : undefined,
      model: index % 5 === 0 ? 'gpt-4.1' : undefined,
      provider: index % 5 === 0 ? 'openai' : undefined,
      input: undefined,
      output: undefined,
      metadata: undefined,
    });
  }

  return spans;
}

function deriveVisibleRowsSafe(sharedData, expandedSpanIds) {
  let rowCount = 0;

  const visit = (nodes, depth) => {
    for (const node of nodes) {
      rowCount += 1;
      if (node.children.length > 0 && expandedSpanIds.has(node.span.span_id)) {
        visit(node.children, depth + 1);
      }
    }
  };

  visit(sharedData.spanTree, 0);
  return Array.from({ length: rowCount });
}

function installDomGlobals() {
  const dom = new JSDOM('<!doctype html><html><body></body></html>', {
    pretendToBeVisual: true,
    url: 'http://localhost',
  });

  const { window } = dom;

  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    writable: true,
    value: window,
  });
  Object.defineProperty(globalThis, 'document', {
    configurable: true,
    writable: true,
    value: window.document,
  });
  Object.defineProperty(globalThis, 'navigator', {
    configurable: true,
    writable: true,
    value: window.navigator,
  });
  Object.defineProperty(globalThis, 'HTMLElement', {
    configurable: true,
    writable: true,
    value: window.HTMLElement,
  });
  Object.defineProperty(globalThis, 'HTMLInputElement', {
    configurable: true,
    writable: true,
    value: window.HTMLInputElement,
  });
  Object.defineProperty(globalThis, 'HTMLButtonElement', {
    configurable: true,
    writable: true,
    value: window.HTMLButtonElement,
  });
  Object.defineProperty(globalThis, 'MouseEvent', {
    configurable: true,
    writable: true,
    value: window.MouseEvent,
  });
  Object.defineProperty(globalThis, 'Event', {
    configurable: true,
    writable: true,
    value: window.Event,
  });
  Object.defineProperty(globalThis, 'Node', {
    configurable: true,
    writable: true,
    value: window.Node,
  });
  Object.defineProperty(globalThis, 'getComputedStyle', {
    configurable: true,
    writable: true,
    value: window.getComputedStyle.bind(window),
  });
  Object.defineProperty(globalThis, 'requestAnimationFrame', {
    configurable: true,
    writable: true,
    value: (callback) => setTimeout(() => callback(performance.now()), 0),
  });
  Object.defineProperty(globalThis, 'cancelAnimationFrame', {
    configurable: true,
    writable: true,
    value: (handle) => clearTimeout(handle),
  });

  Object.defineProperty(window.HTMLElement.prototype, 'scrollIntoView', {
    configurable: true,
    value() {},
  });

  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    writable: true,
    value: () => ({
      matches: true,
      media: '',
      onchange: null,
      addEventListener() {},
      removeEventListener() {},
      addListener() {},
      removeListener() {},
      dispatchEvent() {
        return true;
      },
    }),
  });

  class ResizeObserverMock {
    observe() {}

    unobserve() {}

    disconnect() {}
  }

  Object.defineProperty(globalThis, 'ResizeObserver', {
    configurable: true,
    writable: true,
    value: ResizeObserverMock,
  });
  Object.defineProperty(window, 'ResizeObserver', {
    configurable: true,
    writable: true,
    value: ResizeObserverMock,
  });
  window.confirm = () => true;
  globalThis.confirm = window.confirm;
}
