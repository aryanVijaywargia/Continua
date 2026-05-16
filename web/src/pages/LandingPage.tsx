import {
  ArrowRight,
  Check,
  ChevronRight,
  Copy,
  Github,
  Moon,
  Sun,
  Terminal,
} from 'lucide-react';
import { useEffect, useRef, useState, type CSSProperties, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { useRuntimeAuth } from '../auth/runtime';
import { useMediaQuery } from '../hooks/useMediaQuery';
import { useTheme } from '../hooks/useTheme';

const GITHUB_REPO_URL = 'https://github.com/aryanVijaywargia/Continua';
const DOCS_URL = 'https://www.continua.in/docs';
const GITHUB_LICENSE_URL = `${GITHUB_REPO_URL}/blob/main/LICENSE`;
const API_REFERENCE_URL = `${DOCS_URL}/api-reference`;
const PYTHON_SDK_DOCS_URL = `${DOCS_URL}/sdk/python/overview`;
const ARCHITECTURE_DOCS_URL = `${DOCS_URL}/concepts/overview`;
const RUN_LOCALLY_DOCS_URL = `${DOCS_URL}/guides/installation`;

type SpanTone = 'ok' | 'fail' | 'run';
const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)';

const HERO_SPANS = [
  { name: 'resilient_agent', kind: 'TRACE', depth: 0, start: 0, dur: 100, tone: 'run' },
  { name: 'plan_research', kind: 'LLM', depth: 1, start: 2, dur: 18, tone: 'ok' },
  { name: 'fetch_corpus', kind: 'TOOL', depth: 1, start: 20, dur: 11, tone: 'fail' },
  { name: 'fetch_corpus', kind: 'TOOL', depth: 1, start: 31, dur: 9, tone: 'ok', retry: true },
  { name: 'classify', kind: 'LLM', depth: 1, start: 40, dur: 24, tone: 'run' },
  { name: 'rank', kind: 'CHAIN', depth: 2, start: 43, dur: 14, tone: 'ok' },
  { name: 'tool.search', kind: 'TOOL', depth: 3, start: 45, dur: 7, tone: 'ok' },
  { name: 'tool.dedupe', kind: 'TOOL', depth: 3, start: 52, dur: 3, tone: 'ok' },
  { name: 'summarize', kind: 'LLM', depth: 2, start: 57, dur: 7, tone: 'ok' },
  { name: 'finalize', kind: 'CHAIN', depth: 1, start: 64, dur: 30, tone: 'run' },
  { name: 'render_output', kind: 'TOOL', depth: 2, start: 68, dur: 16, tone: 'ok' },
  { name: 'persist', kind: 'TOOL', depth: 2, start: 84, dur: 8, tone: 'ok' },
] as const;

const AFTER_SPANS = [
  { name: 'resilient_agent', kind: 'TRACE', depth: 0, start: 0, dur: 100, tone: 'ok' },
  { name: 'plan_research', kind: 'LLM', depth: 1, start: 2, dur: 18, tone: 'ok' },
  { name: 'fetch_corpus', kind: 'TOOL', depth: 1, start: 20, dur: 11, tone: 'fail' },
  { name: 'fetch_corpus', kind: 'TOOL', depth: 1, start: 31, dur: 9, tone: 'ok', retry: true },
  { name: 'classify', kind: 'LLM', depth: 1, start: 41, dur: 24, tone: 'ok' },
  { name: 'rank', kind: 'CHAIN', depth: 2, start: 44, dur: 14, tone: 'ok' },
  { name: 'finalize', kind: 'CHAIN', depth: 1, start: 65, dur: 32, tone: 'ok' },
] as const;

const LOG_LINES = [
  ['gray', '[2026-05-16 14:22:18.041] INFO  agent:run starting'],
  ['gray', '[2026-05-16 14:22:18.044] DEBUG llm.openai calling gpt-4 model="gpt-4"'],
  ['gray', '[2026-05-16 14:22:18.987] DEBUG llm.openai response 200 tokens_out=100'],
  ['gray', '[2026-05-16 14:22:18.990] INFO  tool.fetch calling fetch_data q="docs"'],
  ['gray', '[2026-05-16 14:22:19.001] DEBUG http GET https://api.corpus.local/v1/...'],
  ['red', '[2026-05-16 14:22:24.012] ERROR TimeoutError: read timed out (5.0s)'],
  ['red', 'Traceback (most recent call last):'],
  ['red', '  File "agent.py", line 42, in run'],
  ['red', '    data = fetch_data(query)'],
  ['red', '  File "tools/fetch.py", line 18, in fetch_data'],
  ['red', '    return requests.get(url, timeout=5).json()'],
  ['gray', '[2026-05-16 14:22:24.013] WARN  retrying fetch_data attempt=2/3'],
  ['gray', '[2026-05-16 14:22:25.107] DEBUG http 200 OK size=18KB'],
  ['gray', '[2026-05-16 14:22:25.108] INFO  tool.fetch ok rows=124'],
  ['gray', '[2026-05-16 14:22:26.860] INFO  agent:done duration_ms=8819'],
] as const;

const TICKER_ITEMS = [
  ['trc_3a91c1', 'research_agent', '4.2s', 'ok'],
  ['trc_8f2a91', 'resilient_agent', '6.8s', 'retry'],
  ['trc_1d0432', 'code_review', '8.9s', 'ok'],
  ['trc_b2e1c7', 'planner_agent', '2.1s', 'ok'],
  ['trc_99af0c', 'data_ingest', '0.8s', 'fail'],
  ['trc_4c7e1b', 'summarizer', '1.4s', 'ok'],
  ['trc_e7d211', 'classifier_v2', '3.6s', 'ok'],
  ['trc_223de4', 'query_planner', '5.2s', 'ok'],
  ['trc_5a8b12', 'rag_pipeline', '12.4s', 'ok'],
  ['trc_c91f0a', 'tool_executor', '0.9s', 'retry'],
] as const;

const LANDING_SECTION_IDS = ['product', 'how', 'sdk', 'open-source'] as const;

const CODE_TABS = [
  {
    label: 'Basic agent',
    file: 'agent.py',
    rows: [
      ['k', 'from '], ['m', 'continua'], ['k', ' import '], ['_', 'Continua, span, trace\n\n'],
      ['t', 'client'], ['_', ' = Continua.init(\n    api_key='], ['s', '"<project-api-key>"'], ['_', ',\n    endpoint='], ['s', '"http://localhost:8080"'], ['_', ',\n)\n\n'],
      ['d', '@trace'], ['_', '(name='], ['s', '"research_agent"'], ['_', ')\n'],
      ['k', 'def '], ['fn', 'run'], ['_', '(query):\n    '],
      ['k', 'with '], ['_', 'span('], ['s', '"plan"'], ['_', ', kind='], ['s', '"llm"'], ['_', ') '], ['k', 'as '], ['_', 's:\n        s.set_input({'], ['s', '"query"'], ['_', ': query})\n        result = call_llm(query)\n        s.set_llm_response('], ['s', '"gpt-4"'], ['_', ', query, result)\n\n    '],
      ['k', 'return '], ['_', '{'], ['s', '"answer"'], ['_', ': result}'],
    ],
  },
  {
    label: 'With retries',
    file: 'resilient.py',
    rows: [
      ['c', '# Re-ingestion is deduped by span_id\n'],
      ['k', 'from '], ['m', 'continua'], ['k', ' import '], ['_', 'span, trace\n\n'],
      ['d', '@trace'], ['_', '(name='], ['s', '"resilient_agent"'], ['_', ')\n'],
      ['k', 'def '], ['fn', 'run'], ['_', '(task_id):\n    '],
      ['k', 'for '], ['_', 'attempt '], ['k', 'in '], ['_', 'range('], ['n', '1'], ['_', ', '], ['n', '4'], ['_', '):\n        '],
      ['k', 'with '], ['_', 'span('], ['s', '"fetch"'], ['_', ', kind='], ['s', '"tool"'], ['_', ') '], ['k', 'as '], ['_', 's:\n            '],
      ['k', 'try'], ['_', ':\n                '], ['k', 'return '], ['_', 'fetch_data(task_id)\n            '],
      ['k', 'except '], ['_', 'TimeoutError '], ['k', 'as '], ['_', 'exc:\n                s.exception(exc, payload={'], ['s', '"attempt"'], ['_', ': attempt})\n                '],
      ['k', 'if '], ['_', 'attempt == '], ['n', '3'], ['_', ': '], ['k', 'raise'],
    ],
  },
  {
    label: 'Sessions',
    file: 'review.py',
    rows: [
      ['c', '# Group nested runs under a session\n'],
      ['k', 'from '], ['m', 'continua'], ['k', ' import '], ['_', 'session, span\n\n'],
      ['k', 'with '], ['_', 'session('], ['s', '"demo-review"'], ['_', ', user_id='], ['s', '"u_123"'], ['_', ') '], ['k', 'as '], ['_', 's:\n    s.set_metadata({'], ['s', '"app"'], ['_', ': '], ['s', '"docs"'], ['_', '})\n\n    '],
      ['k', 'with '], ['_', 'span('], ['s', '"parse"'], ['_', ', kind='], ['s', '"tool"'], ['_', '):\n        parsed = parse_code(code)\n\n    '],
      ['k', 'with '], ['_', 'span('], ['s', '"review"'], ['_', ', kind='], ['s', '"llm"'], ['_', '):\n        review = call_llm({'], ['s', '"parsed"'], ['_', ': parsed})\n        '],
      ['k', 'return '], ['_', '{'], ['s', '"review"'], ['_', ': review}'],
    ],
  },
] as const;

export function LandingPage() {
  const runtimeAuth = useRuntimeAuth();
  const { resolvedTheme, toggleTheme } = useTheme();

  const isPublicDemo = runtimeAuth.public_demo_enabled === true;
  const isConsoleAvailable = runtimeAuth.console_available !== false;
  const consoleLabel = isPublicDemo ? 'Open Demo' : isConsoleAvailable ? 'Open Console' : 'Run Locally';

  useLandingSectionHashSync();

  return (
    <div className="min-h-screen overflow-x-hidden bg-[var(--c-app-bg)] text-[var(--c-text-primary)]">
      <StatusBanner isPublicDemo={isPublicDemo} />
      <Nav
        consoleLabel={consoleLabel}
        isConsoleAvailable={isConsoleAvailable}
        theme={resolvedTheme}
        toggleTheme={toggleTheme}
      />
      <main>
        <Hero
          consoleLabel={consoleLabel}
          isConsoleAvailable={isConsoleAvailable}
          isPublicDemo={isPublicDemo}
        />
        <TraceTicker />
        <StatsStrip />
        <Manifesto />
        <LogsVsTraces />
        <AnatomySection />
        <StackDiagram />
        <SdkSection isConsoleAvailable={isConsoleAvailable} />
        <OpenSourceSection />
        <CtaSection
          consoleLabel={consoleLabel}
          isConsoleAvailable={isConsoleAvailable}
          isPublicDemo={isPublicDemo}
        />
      </main>
      <Footer
        isConsoleAvailable={isConsoleAvailable}
      />
    </div>
  );
}

function useLandingSectionHashSync() {
  useEffect(() => {
    if (typeof IntersectionObserver !== 'function') {
      return;
    }

    const sections = LANDING_SECTION_IDS.map((id) => document.getElementById(id)).filter(
      (section): section is HTMLElement => Boolean(section)
    );
    if (sections.length === 0) {
      return;
    }

    let activeSectionId = window.location.hash.slice(1);
    const updateHash = (sectionId: string) => {
      if (activeSectionId === sectionId) {
        return;
      }

      activeSectionId = sectionId;
      window.history.replaceState(
        window.history.state,
        '',
        `${window.location.pathname}${window.location.search}#${sectionId}`
      );
    };

    const observer = new IntersectionObserver(
      (entries) => {
        const visibleEntry = entries
          .filter((entry) => entry.isIntersecting)
          .sort((left, right) => right.intersectionRatio - left.intersectionRatio)[0];

        if (visibleEntry?.target.id) {
          updateHash(visibleEntry.target.id);
        }
      },
      {
        rootMargin: '-35% 0px -50% 0px',
        threshold: [0, 0.2, 0.5, 0.8],
      }
    );

    sections.forEach((section) => observer.observe(section));

    return () => observer.disconnect();
  }, []);
}

function StatusBanner({ isPublicDemo }: { isPublicDemo: boolean }) {
  return (
    <div className="border-b bg-[var(--c-app-bg)]" style={{ borderColor: 'var(--c-border)' }}>
      <div className="mx-auto flex max-w-7xl items-center justify-between gap-4 px-6 py-1.5 text-[10.5px]">
        <div className="flex items-center gap-2 font-mono text-[var(--c-text-muted)]">
          <span className="h-1.5 w-1.5 rounded-full bg-[var(--c-green)] shadow-[0_0_0_3px_var(--c-green-faint)]" />
          <span>{isPublicDemo ? 'Public demo with seeded traces' : 'Observability live · engine preview'}</span>
          <span className="text-[var(--c-border-strong)]">·</span>
          <span>docs at continua.in/docs</span>
        </div>
        <div className="hidden items-center gap-4 sm:flex">
          <ExternalLink href={`${GITHUB_REPO_URL}/blob/main/CHANGELOG.md`} className="font-mono text-[var(--c-text-muted)] hover:text-[var(--c-text-primary)]">
            changelog
          </ExternalLink>
          <ExternalLink href={DOCS_URL} className="font-mono text-[var(--c-text-muted)] hover:text-[var(--c-text-primary)]">
            docs
          </ExternalLink>
        </div>
      </div>
    </div>
  );
}

function Nav({
  consoleLabel,
  isConsoleAvailable,
  theme,
  toggleTheme,
}: {
  consoleLabel: string;
  isConsoleAvailable: boolean;
  theme: 'light' | 'dark';
  toggleTheme: () => void;
}) {
  const compactLinks = [
    ['Product', '#product'],
    ['How', '#how'],
    ['SDK', '#sdk'],
    ['Open source', '#open-source'],
  ] as const;

  return (
    <header
      className="sticky top-0 z-40 border-b backdrop-blur"
      style={{
        background: 'color-mix(in srgb, var(--c-app-bg) 88%, transparent)',
        borderColor: 'var(--c-border)',
      }}
    >
      <div className="mx-auto flex min-h-13 max-w-7xl items-center justify-between gap-3 px-4 py-2.5 sm:px-6">
        <a href="#" className="flex shrink-0 items-center gap-2.5">
          <Logo />
          <span className="text-[14px] font-semibold tracking-tight">Continua</span>
        </a>
        <nav className="hidden items-center gap-7 text-[12.5px] font-medium text-[var(--c-text-secondary)] md:flex">
          <a href="#product" className="transition hover:text-[var(--c-text-primary)]">Product</a>
          <a href="#how" className="transition hover:text-[var(--c-text-primary)]">How it works</a>
          <a href="#sdk" className="transition hover:text-[var(--c-text-primary)]">SDK</a>
          <a href="#open-source" className="transition hover:text-[var(--c-text-primary)]">Open source</a>
        </nav>
        <div className="flex items-center gap-1.5">
          <button
            type="button"
            aria-label="Toggle theme"
            onClick={toggleTheme}
            className="flex h-7 w-7 items-center justify-center rounded-md border bg-[var(--c-surface)] text-[var(--c-text-secondary)] transition"
            style={{ borderColor: 'var(--c-border)' }}
          >
            {theme === 'dark' ? <Sun size={13} /> : <Moon size={13} />}
          </button>
          <ExternalLink
            href={DOCS_URL}
            className="hidden h-7 items-center gap-1.5 rounded-md border bg-[var(--c-surface)] px-2.5 text-[12px] font-medium text-[var(--c-text-primary)] sm:inline-flex"
            style={{ borderColor: 'var(--c-border)' }}
          >
            Docs
          </ExternalLink>
          <ExternalLink
            href={GITHUB_REPO_URL}
            className="hidden h-7 items-center gap-1.5 rounded-md border bg-[var(--c-surface)] px-2.5 text-[12px] font-medium text-[var(--c-text-primary)] sm:inline-flex"
            style={{ borderColor: 'var(--c-border)' }}
          >
            <Github size={13} />
            <span>GitHub</span>
          </ExternalLink>
          {isConsoleAvailable ? (
            <Link
              to="/dashboard"
              className="inline-flex h-7 items-center gap-1.5 rounded-md px-2.5 text-[12px] font-semibold"
              style={{ background: 'var(--c-text-primary)', color: 'var(--c-app-bg)' }}
            >
              {consoleLabel}
              <ArrowRight size={12} />
            </Link>
          ) : (
            <ExternalLink
              href={RUN_LOCALLY_DOCS_URL}
              className="inline-flex h-7 items-center gap-1.5 rounded-md px-2.5 text-[12px] font-semibold"
              style={{ background: 'var(--c-text-primary)', color: 'var(--c-app-bg)' }}
            >
              {consoleLabel}
              <ArrowRight size={12} />
            </ExternalLink>
          )}
        </div>
      </div>
      <nav
        aria-label="Landing sections"
        className="flex gap-1 overflow-x-auto border-t px-4 py-2 md:hidden"
        style={{ borderColor: 'var(--c-border)' }}
      >
        {compactLinks.map(([label, href]) => (
          <a
            key={href}
            href={href}
            className="shrink-0 rounded-md border bg-[var(--c-surface)] px-2.5 py-1.5 text-[11px] font-medium text-[var(--c-text-primary)]"
            style={{ borderColor: 'var(--c-border)' }}
          >
            {label}
          </a>
        ))}
        <ExternalLink
          href={DOCS_URL}
          className="shrink-0 rounded-md border bg-[var(--c-surface)] px-2.5 py-1.5 text-[11px] font-medium text-[var(--c-text-primary)]"
          style={{ borderColor: 'var(--c-border)' }}
        >
          Docs
        </ExternalLink>
      </nav>
    </header>
  );
}

function Hero({
  consoleLabel,
  isConsoleAvailable,
  isPublicDemo,
}: {
  consoleLabel: string;
  isConsoleAvailable: boolean;
  isPublicDemo: boolean;
}) {
  return (
    <section className="relative overflow-hidden">
      <div
        className="landing-hero-grid absolute inset-0 -z-10"
        style={{ maskImage: 'linear-gradient(to bottom, black 0%, transparent 90%)' }}
      />
      <div className="mx-auto max-w-7xl px-6 pb-20 pt-16 sm:pt-20">
        <div className="grid items-center gap-12 lg:grid-cols-[minmax(0,1fr)_580px]">
          <div className="min-w-0 max-w-2xl">
            <SpanEyebrow idx={0} kind="TRACE" name="introducing_continua" dur="alpha" status="ok" />
            <h1
              aria-label="Your agent's black box. Opened."
              className="landing-display mt-6 text-[52px] font-bold tracking-[-0.035em] text-[var(--c-text-primary)] sm:text-[68px] lg:text-[76px]"
            >
              <span className="block">Your agent's black box.</span>
              <span
                className="inline-block rounded-[2px] bg-[var(--c-accent)] px-[0.2em] pb-[0.08em] pt-[0.04em] text-white"
                style={{ transform: 'rotate(-0.6deg) translateY(2px)' }}
              >
                Opened.
              </span>
            </h1>
            <p className="mt-6 max-w-xl text-[15.5px] leading-relaxed text-[var(--c-text-secondary)]">
              Open-source observability for AI agents. Capture every retry, payload, and state
              transition as it happens — and inspect them from a self-hosted console that runs in
              one binary.
            </p>
            {isPublicDemo ? (
              <p className="mt-3 max-w-xl text-[13px] leading-6 text-[var(--c-text-muted)]">
                This hosted debugger uses seeded sample traces only. Run locally to inspect your
                own traces and sessions.
              </p>
            ) : !isConsoleAvailable ? (
              <p className="mt-3 max-w-xl text-[13px] leading-6 text-[var(--c-text-muted)]">
                This hosted Pages deployment is static. Run locally to inspect your own traces
                and sessions.
              </p>
            ) : null}
            <div className="mt-7 flex flex-wrap items-center gap-2">
              {isConsoleAvailable ? (
                <Link
                  to="/dashboard"
                  className="inline-flex h-9 items-center gap-2 rounded-md px-3.5 text-[13px] font-semibold"
                  style={{ background: 'var(--c-text-primary)', color: 'var(--c-app-bg)' }}
                >
                  {consoleLabel} <ArrowRight size={14} />
                </Link>
              ) : (
                <ExternalLink
                  href={RUN_LOCALLY_DOCS_URL}
                  className="inline-flex h-9 items-center gap-2 rounded-md px-3.5 text-[13px] font-semibold"
                  style={{ background: 'var(--c-text-primary)', color: 'var(--c-app-bg)' }}
                >
                  {consoleLabel} <ArrowRight size={14} />
                </ExternalLink>
              )}
              <ExternalLink
                href={RUN_LOCALLY_DOCS_URL}
                className="inline-flex h-9 items-center gap-2 rounded-md border bg-[var(--c-surface)] px-3.5 text-[13px] font-medium text-[var(--c-text-primary)]"
                style={{ borderColor: 'var(--c-border)' }}
              >
                <Terminal size={13} />
                Run locally
              </ExternalLink>
            </div>
            <InstallSnippet />
          </div>
          <div className="relative min-w-0">
            <div
              className="absolute -inset-6 -z-10 rounded-3xl"
              style={{ background: 'radial-gradient(60% 50% at 50% 30%, var(--c-accent-faint), transparent 70%)' }}
            />
            <Reveal>
              <HeroWaterfall />
            </Reveal>
            <div className="mt-3 flex items-center justify-between gap-3 font-mono text-[10px] text-[var(--c-text-muted)]">
              <span>fig.01 · resilient_agent · live</span>
              <span className="text-right">retries deduped by ingest key</span>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

function InstallSnippet() {
  const [copied, setCopied] = useState(false);
  const cmd = 'git clone https://github.com/aryanVijaywargia/Continua.git\ncd Continua\nmake demo';

  return (
    <div className="mt-7 w-full max-w-md overflow-hidden rounded-md border bg-[var(--c-surface)]" style={{ borderColor: 'var(--c-border)' }}>
      <div className="flex items-center justify-between border-b px-3 py-1.5" style={{ borderColor: 'var(--c-border)' }}>
        <div className="font-mono text-[9.5px] font-semibold uppercase tracking-[0.12em] text-[var(--c-text-muted)]">
          local demo
        </div>
        <button
          type="button"
          onClick={() => {
            void navigator.clipboard?.writeText(cmd);
            setCopied(true);
            window.setTimeout(() => setCopied(false), 1500);
          }}
          className="inline-flex h-5 items-center gap-1 rounded px-1 text-[10px]"
          style={{ color: copied ? 'var(--c-green-text)' : 'var(--c-text-muted)' }}
        >
          {copied ? <Check size={10} /> : <Copy size={10} />}
          {copied ? 'Copied' : 'Copy'}
        </button>
      </div>
      <pre className="overflow-x-auto px-3 py-2 font-mono text-[12px] leading-[1.7] text-[var(--c-text-primary)]">
        <span className="text-[var(--c-text-muted)]">$</span> git clone https://github.com/aryanVijaywargia/Continua.git{'\n'}
        <span className="text-[var(--c-text-muted)]">$</span> cd Continua{'\n'}
        <span className="text-[var(--c-text-muted)]">$</span> make demo{'\n'}
        <span className="text-[var(--c-green-text)]">✓</span> <span className="text-[var(--c-text-secondary)]">seeded console ready at</span> <span className="text-[var(--c-accent-text)]">localhost:8080</span>
      </pre>
    </div>
  );
}

function HeroWaterfall() {
  const [t, setT] = useState(0);
  const prefersReducedMotion = useMediaQuery(REDUCED_MOTION_QUERY);

  useEffect(() => {
    if (prefersReducedMotion) {
      setT(72);
      return;
    }
    let raf = 0;
    let last = performance.now();
    const loop = (now: number) => {
      const dt = Math.min(now - last, 60);
      last = now;
      setT((prev) => (prev + dt * 0.012) % 130);
      raf = requestAnimationFrame(loop);
    };
    raf = requestAnimationFrame(loop);
    return () => cancelAnimationFrame(raf);
  }, [prefersReducedMotion]);

  const playhead = Math.min(100, t);
  const elapsedStr = `${Math.min((t / 100) * 4.2, 4.2).toFixed(2)}s`;

  return (
    <div className="relative w-full min-w-0 overflow-hidden rounded-xl border bg-[var(--c-surface)] shadow-[0_24px_48px_-20px_rgba(15,23,42,0.18)]" style={{ borderColor: 'var(--c-border)' }}>
      <div className="flex items-center justify-between border-b bg-[var(--c-sidebar-bg)] px-3 py-2" style={{ borderColor: 'var(--c-border)' }}>
        <div className="flex min-w-0 items-center gap-2.5">
          <span className="font-mono text-[10px] font-semibold tracking-[0.06em] text-[var(--c-text-primary)]">trc_8f2a91c4</span>
          <span className="text-[var(--c-text-muted)]">·</span>
          <span className="truncate text-[11px] text-[var(--c-text-secondary)]">resilient_agent</span>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <span className="font-mono text-[10px] tabular-nums text-[var(--c-text-muted)]">{elapsedStr}</span>
          <span
            className="inline-flex items-center gap-1 rounded-[3px] border px-1 py-px text-[9.5px] font-medium"
            style={{
              background: playhead >= 100 ? 'var(--c-green-faint)' : 'var(--c-accent-faint)',
              borderColor: playhead >= 100 ? 'var(--c-green-border)' : 'var(--c-accent-border)',
              color: playhead >= 100 ? 'var(--c-green-text)' : 'var(--c-accent-text)',
            }}
          >
            <span className="h-1.5 w-1.5 rounded-full" style={{ background: playhead >= 100 ? 'var(--c-green)' : 'var(--c-accent)' }} />
            <span className="font-mono uppercase tracking-[0.06em]">{playhead >= 100 ? 'Completed' : 'Running'}</span>
          </span>
        </div>
      </div>
      <div className="relative border-b bg-[var(--c-app-bg)] px-3" style={{ borderColor: 'var(--c-border)' }}>
        <div className="grid h-5" style={{ gridTemplateColumns: '128px minmax(0, 1fr)' }}>
          <div />
          <div className="relative min-w-0 overflow-hidden">
            {[0, 1, 2, 3, 4].map((s) => (
              <span
                key={s}
                className="absolute top-1.5 font-mono text-[9px] tabular-nums text-[var(--c-text-muted)]"
                style={{ left: `${Math.min(100, (s / 4.2) * 100)}%`, transform: 'translateX(-50%)' }}
              >
                {s}.0s
              </span>
            ))}
          </div>
        </div>
      </div>
      <div className="relative">
        {HERO_SPANS.map((span) => (
          <WaterfallBar key={`${span.name}-${span.start}`} {...span} playhead={playhead} />
        ))}
        <div
          aria-hidden="true"
          className="pointer-events-none absolute bottom-0 top-0 w-px bg-[var(--c-accent)] transition-opacity"
          style={{
            left: `calc(138px + ${playhead}% * (100% - 138px) / 100)`,
            opacity: playhead < 100 ? 0.6 : 0,
          }}
        >
          <div className="absolute -left-[3px] -top-1 h-1.5 w-1.5 rounded-full bg-[var(--c-accent)]" />
        </div>
      </div>
      <div className="grid grid-cols-4 border-t font-mono text-[10px]" style={{ borderColor: 'var(--c-border)' }}>
        <Kpi label="Spans" value={HERO_SPANS.length} />
        <Kpi label="Tokens" value="3.1k" border />
        <Kpi label="Cost" value="$0.04" border />
        <Kpi label="Retries" value="1" border tone="amber" />
      </div>
    </div>
  );
}

function WaterfallBar({
  name,
  kind,
  depth,
  start,
  dur,
  tone,
  retry,
  playhead,
}: {
  name: string;
  kind: string;
  depth: number;
  start: number;
  dur: number;
  tone: SpanTone;
  retry?: boolean;
  playhead: number;
}) {
  const colors = toneColors(tone);
  const fillWidth = Math.max(0, Math.max(start, Math.min(start + dur, playhead)) - start);
  const isRunning = playhead >= start && playhead < start + dur;

  return (
    <div
      className="grid items-center gap-2 px-3"
      style={{
        gridTemplateColumns: '128px 1fr',
        borderBottom: '1px solid var(--c-border-subtle)',
        background: isRunning ? 'var(--c-row-hover-bg)' : 'transparent',
        height: 22,
      }}
    >
      <div className="flex min-w-0 items-center gap-1.5">
        <ChevronRight size={9} className="shrink-0 text-[var(--c-text-muted)]" style={{ opacity: depth === 0 ? 0 : 0.8, transform: 'rotate(90deg)' }} />
        <span className="w-8 shrink-0 font-mono text-[9px] uppercase tracking-[0.04em] text-[var(--c-text-muted)]">{kind}</span>
        <span
          className="truncate font-mono text-[11px]"
          style={{
            color: tone === 'fail' ? 'var(--c-red-text)' : 'var(--c-text-primary)',
            paddingLeft: depth * 8,
            fontWeight: depth === 0 ? 600 : 500,
          }}
        >
          {name}
          {retry ? <span className="ml-1 text-[9.5px] text-[var(--c-amber-text)]">↻</span> : null}
        </span>
      </div>
      <div className="relative h-full">
        <div
          className="absolute top-1/2 rounded-[3px]"
          style={{
            left: `${start}%`,
            width: `${dur}%`,
            height: 9,
            background: colors.bg,
            border: `1px solid ${colors.border}`,
            transform: 'translateY(-50%)',
          }}
        />
        <div
          className="absolute top-1/2 rounded-[3px] transition-[width]"
          style={{
            left: `${start}%`,
            width: `${fillWidth}%`,
            height: 9,
            background: colors.fill,
            opacity: isRunning ? 0.92 : 1,
            transform: 'translateY(-50%)',
          }}
        />
      </div>
    </div>
  );
}

function TraceTicker() {
  return (
    <div className="overflow-hidden border-y bg-[var(--c-surface-muted)]" style={{ borderColor: 'var(--c-border)', contain: 'layout paint' }}>
      <div className="flex items-stretch">
        <div className="flex shrink-0 items-center gap-1.5 border-r px-4 font-mono text-[9.5px] font-semibold uppercase tracking-[0.12em] text-[var(--c-text-muted)]" style={{ borderColor: 'var(--c-border)' }}>
          <span className="h-1.5 w-1.5 rounded-full bg-[var(--c-green)] shadow-[0_0_0_3px_var(--c-green-faint)]" />
          Sample traces
        </div>
        <div className="relative h-9 min-w-0 flex-1 overflow-hidden">
          <div className="landing-marquee-track absolute left-0 top-0 flex w-max items-center gap-6 whitespace-nowrap py-2.5">
            {[...TICKER_ITEMS, ...TICKER_ITEMS].map(([id, name, dur, kind], i) => {
              const d = tickerTone(kind);
              return (
                <span key={`${id}-${i}`} className="inline-flex items-center gap-2 font-mono text-[11px]">
                  <span className="h-1.5 w-1.5 rounded-full" style={{ background: d.dot }} />
                  <span className="text-[var(--c-text-muted)]">{id}</span>
                  <span className="text-[var(--c-text-primary)]">{name}</span>
                  <span className="text-[var(--c-text-muted)]">·</span>
                  <span className="tabular-nums text-[var(--c-text-secondary)]">{dur}</span>
                  <span className="text-[9.5px] uppercase tracking-[0.08em]" style={{ color: d.text }}>{d.label}</span>
                  <span className="text-[var(--c-text-muted)]">·</span>
                </span>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
}

function StatsStrip() {
  const [ref, inView] = useInView<HTMLDivElement>();
  const items = [
    { value: 'REST', label: 'Ingest API', hint: 'Authenticated trace, session, span, and event writes', pct: 88 },
    { value: 'SQLC', label: 'Postgres store', hint: 'Typed queries backed by platform migrations', pct: 99 },
    { value: 'River', label: 'Async jobs', hint: 'Background ingest, rollups, and payload cleanup', pct: 72 },
    { value: 'Preview', label: 'Engine surface', hint: 'Control APIs and contracts for durable execution work', pct: 56, zero: true },
  ];

  return (
    <section ref={ref} className="border-y bg-[var(--c-surface-muted)]" style={{ borderColor: 'var(--c-border)' }}>
      <div className="mx-auto grid max-w-7xl grid-cols-2 gap-px md:grid-cols-4">
        {items.map((it, i) => (
          <div
            key={it.label}
            className="relative overflow-hidden bg-[var(--c-app-bg)] px-6 py-7"
            style={{ borderLeft: i > 0 ? '1px solid var(--c-border)' : undefined }}
          >
            <div className="font-mono text-[36px] font-semibold tracking-[-0.02em] text-[var(--c-text-primary)] sm:text-[44px]">
              {it.value}
            </div>
            <div className="mt-1 font-mono text-[10px] font-semibold uppercase tracking-[0.12em] text-[var(--c-accent-text)]">
              {it.label}
            </div>
            <div className="mt-1.5 text-[11.5px] leading-5 text-[var(--c-text-muted)]">{it.hint}</div>
            <div
              className="absolute bottom-0 left-0 h-[2px] transition-[width] duration-1000"
              style={{
                width: inView ? `${it.pct}%` : '0%',
                background: it.zero ? 'var(--c-green)' : 'var(--c-accent)',
              }}
            />
          </div>
        ))}
      </div>
    </section>
  );
}

function Manifesto() {
  return (
    <section className="border-t" style={{ borderColor: 'var(--c-border)' }}>
      <div className="mx-auto max-w-5xl px-6 py-28 sm:py-32">
        <div className="flex items-start gap-5 sm:gap-7">
          <span className="select-none pt-3 font-mono text-[10px] font-semibold uppercase tracking-[0.16em] text-[var(--c-text-muted)]">¶ 02</span>
          <div className="flex-1">
            <h2 className="landing-display text-[36px] font-semibold tracking-[-0.03em] text-[var(--c-text-primary)] sm:text-[52px]">
              Spans are first-class.
              <br />
              <span className="text-[var(--c-text-muted)]">Logs are aftermath.</span>
            </h2>
            <div className="mt-7 grid gap-x-10 gap-y-3 text-[14.5px] leading-7 text-[var(--c-text-secondary)] sm:grid-cols-2">
              <p>
                Most teams ship agents to production and then reconstruct the run from{' '}
                <code className="mx-1 rounded-[3px] border bg-[var(--c-surface)] px-1 font-mono text-[12.5px]" style={{ borderColor: 'var(--c-border)' }}>
                  stdout
                </code>
                and stack traces.
              </p>
              <p>
                Continua flips the model: retries, payloads, and state transitions are captured
                as they happen, then made queryable from the console.
              </p>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

function LogsVsTraces() {
  return (
    <section id="product" className="border-t" style={{ borderColor: 'var(--c-border)' }}>
      <div className="mx-auto max-w-7xl px-6 py-24">
        <SectionHeader
          eyebrow={<SpanEyebrow idx={1} kind="SECTION" name="before / after" status="ok" />}
          title={<>Stop scrolling logs.<br /><span className="text-[var(--c-text-muted)]">Read the trace.</span></>}
          copy="The same agent run, two ways. On the left, a legacy stdout dump. On the right, the same execution captured as a structured Continua trace."
        />
        <Reveal>
          <div className="grid grid-cols-1 gap-px overflow-hidden rounded-xl border md:grid-cols-2" style={{ borderColor: 'var(--c-border)', background: 'var(--c-border)' }}>
            <BeforeTerminal />
            <AfterTrace />
          </div>
        </Reveal>
      </div>
    </section>
  );
}

function BeforeTerminal() {
  const ref = useRef<HTMLDivElement>(null);
  const prefersReducedMotion = useMediaQuery(REDUCED_MOTION_QUERY);

  useEffect(() => {
    if (prefersReducedMotion) return;
    const el = ref.current;
    if (!el) return;
    let scroll = 0;
    const id = window.setInterval(() => {
      scroll = (scroll + 0.4) % el.scrollHeight;
      el.scrollTop = scroll;
    }, 60);
    return () => window.clearInterval(id);
  }, [prefersReducedMotion]);

  return (
    <div className="relative bg-[#0a0b0f]">
      <div className="flex items-center justify-between gap-3 border-b border-white/10 px-4 py-2.5">
        <div className="flex items-center gap-2">
          <span className="font-mono text-[10px] font-semibold uppercase tracking-[0.1em] text-rose-400">stdout</span>
          <span className="text-[10px] text-zinc-500">$ python agent.py</span>
        </div>
        <span className="inline-flex items-center gap-1 rounded-[3px] border border-rose-300/30 bg-rose-400/10 px-1.5 py-0.5 text-[9.5px] font-medium text-rose-300">
          <span className="h-1 w-1 rounded-full bg-rose-400" />
          root cause unclear
        </span>
      </div>
      <div ref={ref} className="overflow-hidden px-4 py-3 font-mono text-[10.5px] leading-[1.7] text-zinc-400" style={{ height: 340 }}>
        {[...LOG_LINES, ...LOG_LINES, ...LOG_LINES].map(([tone, line], i) => (
          <div key={`${line}-${i}`} className={tone === 'red' ? 'text-rose-300' : 'text-slate-400'} style={{ whiteSpace: 'pre' }}>
            {line}
          </div>
        ))}
      </div>
      <div className="pointer-events-none absolute inset-x-0 bottom-0 h-12 bg-gradient-to-b from-transparent to-[#0a0b0f]" />
    </div>
  );
}

function AfterTrace() {
  return (
    <div className="bg-[var(--c-app-bg)]">
      <div className="flex items-center justify-between gap-3 border-b px-4 py-2.5" style={{ borderColor: 'var(--c-border)' }}>
        <div className="flex items-center gap-2">
          <span className="font-mono text-[10px] font-semibold uppercase tracking-[0.1em] text-[var(--c-accent-text)]">trace</span>
          <span className="font-mono text-[10px] text-[var(--c-text-muted)]">trc_8f2a91c4</span>
        </div>
        <span className="inline-flex items-center gap-1 rounded-[3px] border bg-[var(--c-green-faint)] px-1.5 py-0.5 text-[9.5px] font-medium text-[var(--c-green-text)]" style={{ borderColor: 'var(--c-green-border)' }}>
          <span className="h-1 w-1 rounded-full bg-[var(--c-green)]" />
          retry recovered
        </span>
      </div>
      <div className="py-2" style={{ height: 340 }}>
        {AFTER_SPANS.map((span, i) => {
          const colors = toneColors(span.tone);
          return (
            <div
              key={`${span.name}-${span.start}`}
              className="grid items-center gap-2 px-4"
              style={{
                gridTemplateColumns: 'minmax(116px,152px) 1fr 44px',
                height: 30,
                borderBottom: '1px solid var(--c-border-subtle)',
                background: i === 3 ? 'var(--c-row-hover-bg)' : 'transparent',
              }}
            >
              <div className="flex min-w-0 items-center gap-1.5">
                <span className="w-8 shrink-0 font-mono text-[9px] uppercase tracking-wide text-[var(--c-text-muted)]">{span.kind}</span>
                <span
                  className="truncate font-mono text-[11px]"
                  style={{
                    color: span.tone === 'fail' ? 'var(--c-red-text)' : 'var(--c-text-primary)',
                    paddingLeft: span.depth * 8,
                    fontWeight: span.depth === 0 ? 600 : 500,
                  }}
                >
                  {span.name}
                  {'retry' in span && span.retry ? <span className="ml-1 text-[var(--c-amber-text)]">↻</span> : null}
                </span>
              </div>
              <div className="relative h-2">
                <div className="absolute top-1/2 rounded-[2px]" style={{ left: `${span.start}%`, width: `${span.dur}%`, height: 8, background: colors.bg, border: `1px solid ${colors.border}`, transform: 'translateY(-50%)' }} />
                <div className="absolute top-1/2 rounded-[2px]" style={{ left: `${span.start}%`, width: `${span.dur}%`, height: 8, background: colors.fill, transform: 'translateY(-50%)' }} />
              </div>
              <span className="text-right font-mono text-[10px] tabular-nums text-[var(--c-text-muted)]">{Math.round(span.dur * 88)}ms</span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function AnatomySection() {
  const cards = [
    { eyebrow: 'Span tree', title: 'Drill any depth', copy: 'Nested chains, parallel calls, retried tools, and the whole call graph inline.', visual: <SpanTreeVisual /> },
    { eyebrow: 'Payload inspector', title: 'See every byte', copy: 'Inputs, outputs, errors, and JSON payloads stay attached to the span.', visual: <PayloadVisual /> },
    { eyebrow: 'Failure summary', title: 'Get to root cause', copy: 'The first failing span, retry chain, and stack are already wired together.', visual: <FailureVisual /> },
  ];

  return (
    <section id="how" className="border-t" style={{ borderColor: 'var(--c-border)' }}>
      <div className="mx-auto max-w-7xl px-6 py-24">
        <SectionHeader
          eyebrow={<SpanEyebrow idx={4} kind="SECTION" name="anatomy" status="ok" />}
          title={<>Everything an operator needs,<br /><span className="text-[var(--c-text-muted)]">on one screen.</span></>}
        />
        <div className="grid gap-6 lg:grid-cols-3">
          {cards.map((card) => (
            <Reveal key={card.title}>
              <div className="overflow-hidden rounded-lg border bg-[var(--c-surface)]" style={{ borderColor: 'var(--c-border)' }}>
                <div className="border-b bg-[var(--c-surface-muted)]" style={{ borderColor: 'var(--c-border)' }}>{card.visual}</div>
                <div className="p-5">
                  <div className="font-mono text-[9.5px] font-semibold uppercase tracking-[0.12em] text-[var(--c-accent-text)]">{card.eyebrow}</div>
                  <h3 className="mt-2 text-[15px] font-semibold tracking-[-0.01em] text-[var(--c-text-primary)]">{card.title}</h3>
                  <p className="mt-1.5 text-[12.5px] leading-5 text-[var(--c-text-secondary)]">{card.copy}</p>
                </div>
              </div>
            </Reveal>
          ))}
        </div>
      </div>
    </section>
  );
}

function StackDiagram() {
  return (
    <section className="border-t" style={{ borderColor: 'var(--c-border)' }}>
      <div className="mx-auto max-w-7xl px-6 py-24">
        <div className="grid gap-12 md:grid-cols-2">
          <div>
            <SpanEyebrow idx={3} kind="SECTION" name="what's in the box" status="ok" />
            <h2 className="landing-display mt-4 text-[32px] font-semibold tracking-[-0.025em] text-[var(--c-text-primary)] sm:text-[40px]">
              One binary,
              <br />
              one Postgres.
            </h2>
            <p className="mt-4 max-w-md text-[14px] leading-6 text-[var(--c-text-secondary)]">
              The stack is a Go service, a Postgres database, and a React console served from the
              same binary. River queues handle async ingest, rollups, and cleanup.
            </p>
            <ul className="mt-6 space-y-2">
              {[
                ['Go service', 'Fx-wired modules under internal/'],
                ['Postgres', 'Idempotent ingest, sqlc-typed queries'],
                ['River', 'Async jobs: rollups, cleanup, dependencies'],
                ['React console', 'Embedded SPA served from /'],
                ['SDKs', 'Python real, TypeScript stub'],
              ].map(([k, v]) => (
                <li key={k} className="flex items-baseline gap-3 text-[12.5px]">
                  <span className="w-[110px] shrink-0 font-mono font-medium text-[var(--c-text-primary)]">{k}</span>
                  <span className="text-[var(--c-text-secondary)]">{v}</span>
                </li>
              ))}
            </ul>
          </div>
          <Reveal>
            <div className="relative rounded-xl border bg-[var(--c-surface-muted)] p-6" style={{ borderColor: 'var(--c-border)' }}>
              <Layer label="Your code" sub="agent.py · @trace · span()" tone="muted" />
              <Connector label="import" />
              <Layer label="Python SDK" sub="continua · batching · helpers" tone="accent" />
              <Connector label="POST /v1/ingest · HTTPS" />
              <Layer label="Continua service · Go" sub="auth · ingest · mappers" tone="primary" split={[['Postgres', 'sqlc + migrations'], ['River', 'async workers']]} />
              <Connector label="engine preview surface" />
              <Layer label="Durable engine · planned runtime" sub="control APIs · pending work · history" tone="muted" />
              <Connector label="GET /api/*" />
              <Layer label="Operator console · React" sub="traces · sessions · settings" tone="accent" />
            </div>
          </Reveal>
        </div>
      </div>
    </section>
  );
}

function SdkSection({ isConsoleAvailable }: { isConsoleAvailable: boolean }) {
  const [active, setActive] = useState(0);
  const panelId = `sdk-tabpanel-${CODE_TABS[active].file.replace('.', '-')}`;

  const selectAdjacentTab = (direction: -1 | 1) => {
    setActive((current) => (current + direction + CODE_TABS.length) % CODE_TABS.length);
  };

  return (
    <section id="sdk" className="border-t" style={{ borderColor: 'var(--c-border)' }}>
      <div className="mx-auto max-w-7xl px-6 py-24">
        <SectionHeader
          eyebrow={<SpanEyebrow idx={5} kind="SECTION" name="sdk" status="ok" />}
          title={<>Two decorators<br /><span className="text-[var(--c-text-muted)]">and you're traced.</span></>}
          copy="The Python SDK batches spans, polls async ingest, and ships helpers for traces, spans, and sessions."
        />
        <Reveal>
          <div className="overflow-hidden rounded-xl border bg-[var(--c-surface)]" style={{ borderColor: 'var(--c-border)' }}>
            <div className="flex items-center justify-between border-b bg-[var(--c-surface-muted)] px-1" style={{ borderColor: 'var(--c-border)' }}>
              <div
                role="tablist"
                aria-label="Python SDK examples"
                className="flex overflow-x-auto"
                onKeyDown={(event) => {
                  if (event.key === 'ArrowRight') {
                    event.preventDefault();
                    selectAdjacentTab(1);
                  }
                  if (event.key === 'ArrowLeft') {
                    event.preventDefault();
                    selectAdjacentTab(-1);
                  }
                }}
              >
                {CODE_TABS.map((tab, i) => (
                  <button
                    key={tab.file}
                    type="button"
                    role="tab"
                    id={`sdk-tab-${tab.file.replace('.', '-')}`}
                    aria-controls={`sdk-tabpanel-${tab.file.replace('.', '-')}`}
                    aria-selected={active === i}
                    tabIndex={active === i ? 0 : -1}
                    onClick={() => setActive(i)}
                    className="inline-flex shrink-0 items-center gap-1.5 border-b-2 px-3 py-2.5 font-mono text-[11px] font-medium transition"
                    style={{
                      borderColor: active === i ? 'var(--c-accent)' : 'transparent',
                      color: active === i ? 'var(--c-text-primary)' : 'var(--c-text-muted)',
                      background: active === i ? 'var(--c-surface)' : 'transparent',
                    }}
                  >
                    <Terminal size={11} />
                    {tab.file}
                  </button>
                ))}
              </div>
              <span className="hidden px-3 font-mono text-[9.5px] uppercase tracking-[0.12em] text-[var(--c-text-muted)] sm:block">Python · 3.11</span>
            </div>
            <div className="grid lg:grid-cols-[1fr_360px]">
              <pre
                role="tabpanel"
                id={panelId}
                aria-labelledby={`sdk-tab-${CODE_TABS[active].file.replace('.', '-')}`}
                className="overflow-x-auto px-5 py-5 font-mono text-[12.5px] leading-[1.75] text-[var(--c-text-primary)]"
              >
                {CODE_TABS[active].rows.map(([tag, text], i) => (
                  <span key={`${tag}-${i}`} style={{ color: codeColor(tag) }}>{text}</span>
                ))}
              </pre>
              <aside className="border-t bg-[var(--c-surface-muted)] lg:border-l lg:border-t-0" style={{ borderColor: 'var(--c-border)' }}>
                <div className="px-4 pb-2 pt-4">
                  <div className="font-mono text-[9.5px] font-semibold uppercase tracking-[0.12em] text-[var(--c-text-muted)]">Renders to console</div>
                  <div className="mt-1 text-[11px] text-[var(--c-text-secondary)]">What the operator sees after this code runs.</div>
                </div>
                <CodeOutputPanel variant={active} isConsoleAvailable={isConsoleAvailable} />
              </aside>
            </div>
          </div>
        </Reveal>
      </div>
    </section>
  );
}

function OpenSourceSection() {
  return (
    <section id="open-source" className="border-t" style={{ borderColor: 'var(--c-border)' }}>
      <div className="mx-auto max-w-7xl px-6 py-24">
        <div className="grid gap-12 md:grid-cols-[1fr_360px]">
          <div>
            <SpanEyebrow idx={6} kind="SECTION" name="open_source" status="ok" />
            <h2 className="landing-display mt-4 text-[32px] font-semibold tracking-[-0.025em] text-[var(--c-text-primary)] sm:text-[44px]">
              Built in public.
              <br />
              <span className="text-[var(--c-text-muted)]">Run it on your laptop.</span>
            </h2>
            <p className="mt-4 max-w-lg text-[14px] leading-6 text-[var(--c-text-secondary)]">
              Continua is MIT licensed. The Go service, React console, Python SDK, migrations,
              and engine preview contracts are open. Self-host with one command, and keep traces
              inside your network.
            </p>
            <div className="mt-6 flex flex-wrap gap-2">
              <ExternalLink
                href={GITHUB_REPO_URL}
                className="inline-flex h-9 items-center gap-2 whitespace-nowrap rounded-md px-3.5 text-[13px] font-semibold"
                style={{ background: 'var(--c-text-primary)', color: 'var(--c-app-bg)' }}
              >
                <Github size={14} />
                View on GitHub
              </ExternalLink>
              <ExternalLink
                href={RUN_LOCALLY_DOCS_URL}
                className="inline-flex h-9 items-center gap-2 rounded-md border bg-[var(--c-surface)] px-3.5 text-[13px] font-medium text-[var(--c-text-primary)]"
                style={{ borderColor: 'var(--c-border)' }}
              >
                <Terminal size={13} />
                Run locally
              </ExternalLink>
            </div>
          </div>
          <RepoCard />
        </div>
      </div>
    </section>
  );
}

function CtaSection({
  consoleLabel,
  isConsoleAvailable,
  isPublicDemo,
}: {
  consoleLabel: string;
  isConsoleAvailable: boolean;
  isPublicDemo: boolean;
}) {
  return (
    <section className="border-t" style={{ borderColor: 'var(--c-border)' }}>
      <div className="mx-auto max-w-7xl px-6 py-24">
        <div className="relative overflow-hidden rounded-xl px-6 py-14 text-center sm:px-10 sm:py-20" style={{ background: 'var(--c-text-primary)', color: 'var(--c-app-bg)' }}>
          <div className="landing-cta-grid absolute inset-0 opacity-30" />
          <div className="relative">
            <div className="inline-flex items-center gap-2 rounded-[4px] border border-white/20 bg-white/5 px-2 py-1 font-mono text-[10px] text-white/70">
              <span className="h-1.5 w-1.5 rounded-full bg-[#10b981]" />
              {isPublicDemo ? 'demo_ready' : 'ready_to_trace'}
            </div>
            <h2 className="landing-display mx-auto mt-5 max-w-3xl text-[40px] font-semibold tracking-[-0.025em] sm:text-[56px]">
              Trace your first agent
              <br />
              in <span className="text-[#79b4f5]">under sixty seconds</span>.
            </h2>
            <div className="mt-7 flex flex-wrap justify-center gap-2">
              {isConsoleAvailable ? (
                <Link to="/dashboard" className="inline-flex h-10 items-center gap-2 rounded-md px-4 text-[14px] font-semibold" style={{ background: 'var(--c-app-bg)', color: 'var(--c-text-primary)' }}>
                  {consoleLabel} <ArrowRight size={14} />
                </Link>
              ) : (
                <ExternalLink href={RUN_LOCALLY_DOCS_URL} className="inline-flex h-10 items-center gap-2 rounded-md px-4 text-[14px] font-semibold" style={{ background: 'var(--c-app-bg)', color: 'var(--c-text-primary)' }}>
                  {consoleLabel} <ArrowRight size={14} />
                </ExternalLink>
              )}
              <ExternalLink href={GITHUB_REPO_URL} className="inline-flex h-10 items-center gap-2 rounded-md border border-white/20 bg-white/5 px-4 text-[14px] font-medium text-white/90">
                <Github size={14} />
                Star on GitHub
              </ExternalLink>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

function Footer({
  isConsoleAvailable,
}: {
  isConsoleAvailable: boolean;
}) {
  const columns = [
    { label: 'Product', links: [['Console', '/dashboard'], ['Traces', '/traces'], ['Sessions', '/sessions'], ['Changelog', `${GITHUB_REPO_URL}/blob/main/CHANGELOG.md`]] },
    { label: 'Develop', links: [['Docs', DOCS_URL], ['API reference', API_REFERENCE_URL], ['Python SDK', PYTHON_SDK_DOCS_URL], ['Run locally', RUN_LOCALLY_DOCS_URL]] },
    { label: 'Open source', links: [['GitHub', GITHUB_REPO_URL], ['License', GITHUB_LICENSE_URL], ['Contributing', `${GITHUB_REPO_URL}/blob/main/CONTRIBUTING.md`], ['Architecture', ARCHITECTURE_DOCS_URL]] },
  ];

  return (
    <footer className="border-t" style={{ borderColor: 'var(--c-border)' }}>
      <div className="mx-auto max-w-7xl px-6 py-14">
        <div className="grid gap-10 md:grid-cols-[1.4fr_1fr_1fr_1fr]">
          <div>
            <div className="flex items-center gap-2.5">
              <Logo />
              <span className="text-[14px] font-semibold tracking-tight">Continua</span>
            </div>
            <p className="mt-4 max-w-xs text-[12px] leading-5 text-[var(--c-text-muted)]">AI agent observability today, durable execution tomorrow. MIT licensed.</p>
            <div className="mt-5 font-mono text-[10px] text-[var(--c-text-muted)]">© 2026 · alpha</div>
          </div>
          {columns.map((col) => (
            <div key={col.label}>
              <div className="font-mono text-[9.5px] font-semibold uppercase tracking-[0.12em] text-[var(--c-text-muted)]">{col.label}</div>
              <ul className="mt-3 space-y-2">
                {col.links.map(([label, href]) => (
                  <li key={label}>
                    {href.startsWith('http') ? (
                      <ExternalLink href={href} className="text-[12.5px] text-[var(--c-text-secondary)] transition hover:text-[var(--c-text-primary)]">{label}</ExternalLink>
                    ) : !isConsoleAvailable ? (
                      <ExternalLink href={RUN_LOCALLY_DOCS_URL} className="text-[12.5px] text-[var(--c-text-secondary)] transition hover:text-[var(--c-text-primary)]">{label}</ExternalLink>
                    ) : (
                      <Link to={href} className="text-[12.5px] text-[var(--c-text-secondary)] transition hover:text-[var(--c-text-primary)]">{label}</Link>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
        <div className="mt-12 flex flex-col items-start justify-between gap-3 border-t pt-6 text-[11px] text-[var(--c-text-muted)] sm:flex-row sm:items-center" style={{ borderColor: 'var(--c-border)' }}>
          <span className="inline-flex items-center gap-2 font-mono">
            <span className="h-1.5 w-1.5 rounded-full bg-[var(--c-green)]" />
            All systems operational
          </span>
          <span className="font-mono">trc_landing · 2026-05-16</span>
        </div>
      </div>
    </footer>
  );
}

function SpanEyebrow({
  idx,
  name,
  kind,
  status = 'ok',
  dur,
}: {
  idx: number;
  name: string;
  kind: string;
  status?: SpanTone;
  dur?: string;
}) {
  const c = status === 'fail'
    ? { dot: 'var(--c-red)', text: 'var(--c-red-text)', label: 'failed' }
    : status === 'run'
      ? { dot: 'var(--c-accent)', text: 'var(--c-accent-text)', label: 'running' }
      : { dot: 'var(--c-green)', text: 'var(--c-green-text)', label: 'ok' };

  return (
    <div className="inline-flex items-center gap-2 rounded-[4px] border bg-[var(--c-surface)] px-2 py-1 font-mono text-[10px] text-[var(--c-text-muted)]" style={{ borderColor: 'var(--c-border)' }}>
      <span className="tabular-nums">[{String(idx).padStart(2, '0')}]</span>
      <span>·</span>
      <span className="uppercase tracking-[0.04em] text-[var(--c-text-secondary)]">{kind}</span>
      <span className="font-semibold text-[var(--c-text-primary)]">{name}</span>
      {dur ? <><span>·</span><span className="tabular-nums">{dur}</span></> : null}
      <span>·</span>
      <span className="inline-flex items-center gap-1" style={{ color: c.text }}>
        <span className="h-1 w-1 rounded-full" style={{ background: c.dot }} />
        {c.label}
      </span>
    </div>
  );
}

function SectionHeader({
  eyebrow,
  title,
  copy,
}: {
  eyebrow: ReactNode;
  title: ReactNode;
  copy?: string;
}) {
  return (
    <div className="mb-10 max-w-3xl">
      {eyebrow}
      <h2 className="landing-display mt-4 text-[32px] font-semibold tracking-[-0.025em] text-[var(--c-text-primary)] sm:text-[44px]">{title}</h2>
      {copy ? <p className="mt-4 max-w-2xl text-[14px] leading-6 text-[var(--c-text-secondary)]">{copy}</p> : null}
    </div>
  );
}

function Logo() {
  return (
    <img src="/logo.svg" alt="" className="h-6 w-6 shrink-0" />
  );
}

function Reveal({ children, delay = 0 }: { children: ReactNode; delay?: number }) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (typeof IntersectionObserver !== 'function') {
      el.classList.add('in');
      return;
    }
    const observer = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting) {
          window.setTimeout(() => entry.target.classList.add('in'), delay);
          observer.unobserve(entry.target);
        }
      });
    }, { threshold: 0.08 });
    observer.observe(el);
    return () => observer.disconnect();
  }, [delay]);

  return <div ref={ref} className="landing-reveal">{children}</div>;
}

function useInView<T extends HTMLElement>() {
  const ref = useRef<T>(null);
  const [inView, setInView] = useState(false);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    if (typeof IntersectionObserver !== 'function') {
      setInView(true);
      return;
    }
    const observer = new IntersectionObserver(([entry]) => {
      if (entry?.isIntersecting) {
        setInView(true);
        observer.unobserve(el);
      }
    }, { threshold: 0.2 });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);
  return [ref, inView] as const;
}

function Kpi({ label, value, border, tone }: { label: string; value: ReactNode; border?: boolean; tone?: 'amber' }) {
  return (
    <div className="px-3 py-2" style={{ borderLeft: border ? '1px solid var(--c-border)' : undefined }}>
      <div className="text-[9px] font-semibold uppercase tracking-[0.1em] text-[var(--c-text-muted)]">{label}</div>
      <div className="mt-0.5 font-mono text-[12.5px] font-semibold tabular-nums" style={{ color: tone === 'amber' ? 'var(--c-amber-text)' : 'var(--c-text-primary)' }}>{value}</div>
    </div>
  );
}

function SpanTreeVisual() {
  const rows = [
    ['agent.run', 'TRACE', 0, '#10b981'],
    ['plan', 'LLM', 1, '#10b981'],
    ['tools', 'CHAIN', 1, '#10b981'],
    ['search', 'TOOL', 2, '#10b981'],
    ['fetch', 'TOOL', 2, '#ef4444'],
    ['fetch ↻', 'TOOL', 2, '#10b981'],
    ['compose', 'LLM', 1, '#3b82f6'],
  ] as const;
  return (
    <div className="px-3 py-3" style={{ height: 180 }}>
      {rows.map(([name, kind, depth, dot]) => (
        <div key={`${name}-${depth}`} className="flex items-center gap-1.5 py-1 text-[10.5px]" style={{ paddingLeft: depth * 10 }}>
          <ChevronRight size={9} className="text-[var(--c-text-muted)]" style={{ transform: depth < 2 ? 'rotate(90deg)' : 'none', opacity: depth < 2 ? 0.8 : 0 }} />
          <span className="h-1.5 w-1.5 shrink-0 rounded-full" style={{ background: dot }} />
          <span className="w-[28px] shrink-0 font-mono text-[8.5px] uppercase tracking-wide text-[var(--c-text-muted)]">{kind}</span>
          <span className="truncate font-mono" style={{ color: dot === '#ef4444' ? 'var(--c-red-text)' : 'var(--c-text-primary)' }}>{name}</span>
        </div>
      ))}
    </div>
  );
}

function PayloadVisual() {
  return (
    <div className="px-3 py-3" style={{ height: 180 }}>
      <div className="font-mono text-[10px] leading-[1.7] text-[var(--c-text-secondary)]">
        <div>{'{'}</div>
        <div className="pl-3"><span className="text-[var(--c-amber-text)]">"model"</span>: <span className="text-[var(--c-green-text)]">"gpt-4"</span>,</div>
        <div className="pl-3"><span className="text-[var(--c-amber-text)]">"prompt"</span>: <span className="text-[var(--c-green-text)]">"summarize the corpus..."</span>,</div>
        <div className="pl-3"><span className="text-[var(--c-amber-text)]">"tokens"</span>: {'{'}</div>
        <div className="pl-6"><span className="text-[var(--c-amber-text)]">"in"</span>: <span className="text-[var(--c-accent-text)]">50</span>,</div>
        <div className="pl-6"><span className="text-[var(--c-amber-text)]">"out"</span>: <span className="text-[var(--c-accent-text)]">312</span></div>
        <div className="pl-3">{'}'},</div>
        <div className="pl-3"><span className="text-[var(--c-amber-text)]">"latency_ms"</span>: <span className="text-[var(--c-accent-text)]">1843</span></div>
        <div>{'}'}</div>
      </div>
    </div>
  );
}

function FailureVisual() {
  return (
    <div className="px-3 py-3" style={{ height: 180 }}>
      <div className="mb-2 inline-flex items-center gap-1.5 rounded-[3px] border bg-[var(--c-red-faint)] px-1.5 py-0.5 text-[9.5px] font-medium text-[var(--c-red-text)]" style={{ borderColor: 'var(--c-red-border)' }}>
        <span className="h-1 w-1 rounded-full bg-[var(--c-red)]" />
        TimeoutError · attempt 1
      </div>
      <div className="mt-2 rounded-md border bg-[var(--c-app-bg)] p-2 font-mono text-[10px]" style={{ borderColor: 'var(--c-border)' }}>
        <div className="text-[var(--c-red-text)]">requests.exceptions.TimeoutError</div>
        <div className="text-[var(--c-text-muted)]">at tools/fetch.py:18 in fetch_data</div>
        <div className="mt-2 text-[var(--c-text-secondary)]">→ <span className="text-[var(--c-green-text)]">retry succeeded</span> at 14:22:25</div>
      </div>
      <div className="mt-3 flex items-center justify-between font-mono text-[9.5px] text-[var(--c-text-muted)]">
        <span>span_id · spn_4a7c</span>
        <span className="text-[var(--c-accent-text)]">open span →</span>
      </div>
    </div>
  );
}

function Layer({
  label,
  sub,
  tone,
  split,
}: {
  label: string;
  sub: string;
  tone: 'muted' | 'accent' | 'primary';
  split?: readonly (readonly [string, string])[];
}) {
  const styles = {
    muted: { bg: 'var(--c-surface)', border: 'var(--c-border)', label: 'var(--c-text-primary)' },
    accent: { bg: 'var(--c-accent-faint)', border: 'var(--c-accent-border)', label: 'var(--c-accent-text)' },
    primary: { bg: 'var(--c-text-primary)', border: 'var(--c-text-primary)', label: 'var(--c-app-bg)' },
  }[tone];

  return (
    <div className="rounded-md border" style={{ background: styles.bg, borderColor: styles.border, color: styles.label }}>
      <div className="px-3 py-2.5">
        <div className="font-mono text-[11.5px] font-semibold">{label}</div>
        <div className="mt-0.5 font-mono text-[10px] opacity-70">{sub}</div>
      </div>
      {split ? (
        <div className="grid grid-cols-2 gap-px border-t border-white/10 bg-white/10">
          {split.map(([splitLabel, splitSub]) => (
            <div key={splitLabel} className="px-3 py-2" style={{ background: styles.bg }}>
              <div className="font-mono text-[10.5px] font-semibold">{splitLabel}</div>
              <div className="mt-0.5 font-mono text-[9.5px] opacity-70">{splitSub}</div>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function Connector({ label }: { label: string }) {
  return (
    <div className="my-1.5 flex items-center justify-center gap-2">
      <span className="block h-2 w-px bg-[var(--c-border-strong)]" />
      <span className="font-mono text-[9px] uppercase tracking-[0.12em] text-[var(--c-text-muted)]">{label}</span>
      <span className="block h-2 w-px bg-[var(--c-border-strong)]" />
    </div>
  );
}

function CodeOutputPanel({
  variant,
  isConsoleAvailable,
}: {
  variant: number;
  isConsoleAvailable: boolean;
}) {
  const variants = [
    [['research_agent', 0, '#10b981', 'TRACE'], ['plan', 1, '#10b981', 'LLM']],
    [['resilient_agent', 0, '#10b981', 'TRACE'], ['fetch', 1, '#ef4444', 'TOOL'], ['fetch ↻', 1, '#f59e0b', 'TOOL'], ['fetch ↻', 1, '#10b981', 'TOOL']],
    [['demo-review', 0, '#3b82f6', 'SESSION'], ['parse', 1, '#10b981', 'TOOL'], ['review', 1, '#10b981', 'LLM']],
  ] as const;
  return (
    <div className="px-4 pb-5">
      <div className="rounded-md border bg-[var(--c-app-bg)]" style={{ borderColor: 'var(--c-border)' }}>
        {variants[variant].map(([name, depth, dot, kind], i, arr) => (
          <div key={`${name}-${i}`} className="flex items-center gap-1.5 px-2.5 py-1 text-[10.5px]" style={{ paddingLeft: 10 + depth * 10, borderBottom: i < arr.length - 1 ? '1px solid var(--c-border-subtle)' : undefined }}>
            <span className="h-1.5 w-1.5 shrink-0 rounded-full" style={{ background: dot }} />
            <span className="w-11 shrink-0 font-mono text-[8.5px] uppercase tracking-wide text-[var(--c-text-muted)]">{kind}</span>
            <span className="truncate font-mono" style={{ color: dot === '#ef4444' ? 'var(--c-red-text)' : 'var(--c-text-primary)' }}>{name}</span>
          </div>
        ))}
      </div>
      <div className="mt-3 flex items-center justify-between text-[10px] text-[var(--c-text-muted)]">
        <span className="font-mono">→ /traces</span>
        {isConsoleAvailable ? (
          <Link to="/dashboard" className="font-mono text-[var(--c-accent-text)]">open console ↗</Link>
        ) : (
          <ExternalLink href={RUN_LOCALLY_DOCS_URL} className="font-mono text-[var(--c-accent-text)]">run locally ↗</ExternalLink>
        )}
      </div>
    </div>
  );
}

import repoStatsJson from '../data/repo-stats.json';

interface RepoStatsWeek {
  weekStart: string;
  total: number;
  days: number[];
}

interface RepoStatsFile {
  generatedAt: string;
  branch: string | null;
  commitTotal: number;
  firstCommitAt: string | null;
  weeks: RepoStatsWeek[];
}

interface RepoActivity {
  weeks: RepoStatsWeek[];
  commitTotal: number;
  sinceLabel: string;
  monthLabels: string[];
}

const MAX_HEATMAP_WEEKS = 52;
const MONTH_NAMES = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

function buildRepoActivity(): RepoActivity {
  const stats = repoStatsJson as RepoStatsFile;
  const raw = Array.isArray(stats.weeks) ? stats.weeks : [];

  // Drop leading zero-commit weeks so the heatmap starts on the first real week of activity.
  let start = 0;
  while (start < raw.length && raw[start].total === 0) start += 1;
  let trimmed = raw.slice(start);

  // Cap window so very old repos don't blow out the row width.
  if (trimmed.length > MAX_HEATMAP_WEEKS) {
    trimmed = trimmed.slice(trimmed.length - MAX_HEATMAP_WEEKS);
  }

  const commitTotal = trimmed.reduce((acc, w) => acc + (w.total ?? 0), 0);

  let sinceLabel = '—';
  const monthLabels: string[] = [];
  if (trimmed.length > 0) {
    const first = new Date(`${trimmed[0].weekStart}T00:00:00Z`);
    sinceLabel = `${MONTH_NAMES[first.getUTCMonth()]} ${first.getUTCFullYear()}`;

    let prevMonth = -1;
    let prevYear = -1;
    for (const w of trimmed) {
      const d = new Date(`${w.weekStart}T00:00:00Z`);
      const m = d.getUTCMonth();
      const y = d.getUTCFullYear();
      if (m !== prevMonth || y !== prevYear) {
        monthLabels.push(MONTH_NAMES[m]);
        prevMonth = m;
        prevYear = y;
      }
    }
  }

  return { weeks: trimmed, commitTotal, sinceLabel, monthLabels };
}

const REPO_ACTIVITY: RepoActivity = buildRepoActivity();

function commitLevel(count: number): 0 | 1 | 2 | 3 | 4 {
  if (count <= 0) return 0;
  if (count <= 2) return 1;
  if (count <= 5) return 2;
  if (count <= 10) return 3;
  return 4;
}

function CommitHeatmap({ activity }: { activity: RepoActivity }) {
  return (
    <div className="border-b px-4 py-3" style={{ borderColor: 'var(--c-border)' }}>
      <div className="flex items-center justify-between">
        <div className="font-mono text-[10px] font-semibold uppercase tracking-[0.12em] text-[var(--c-text-muted)]">
          Commit activity
        </div>
        <span className="font-mono text-[10px] text-[var(--c-text-muted)]">
          <span className="text-[var(--c-text-primary)]">{activity.commitTotal}</span> commits · since {activity.sinceLabel}
        </span>
      </div>
      <div className="mt-2.5 flex gap-[3px]" aria-hidden="true">
        {activity.weeks.map((week, wi) => (
          <div key={`${week.weekStart}-${wi}`} className="flex flex-col gap-[3px]">
            {week.days.map((c, di) => {
              const level = commitLevel(c);
              const bg =
                level === 0
                  ? 'var(--c-app-bg)'
                  : level === 1
                    ? 'color-mix(in srgb, var(--c-accent) 25%, transparent)'
                    : level === 2
                      ? 'color-mix(in srgb, var(--c-accent) 50%, transparent)'
                      : level === 3
                        ? 'color-mix(in srgb, var(--c-accent) 75%, transparent)'
                        : 'var(--c-accent)';
              return (
                <div
                  key={di}
                  className="h-[9px] w-[9px] rounded-[2px] border"
                  style={{ background: bg, borderColor: 'var(--c-border)' }}
                />
              );
            })}
          </div>
        ))}
      </div>
      <div className="mt-2 flex items-center justify-between text-[9.5px] font-mono text-[var(--c-text-muted)]">
        <div className="flex gap-2">
          {activity.monthLabels.map((m, i) => (
            <span key={`${m}-${i}`}>{m}</span>
          ))}
        </div>
        <div className="flex items-center gap-1">
          <span>less</span>
          {[0, 1, 2, 3, 4].map((lvl) => (
            <span
              key={lvl}
              className="inline-block h-[8px] w-[8px] rounded-[2px] border"
              style={{
                borderColor: 'var(--c-border)',
                background:
                  lvl === 0
                    ? 'var(--c-app-bg)'
                    : lvl === 1
                      ? 'color-mix(in srgb, var(--c-accent) 25%, transparent)'
                      : lvl === 2
                        ? 'color-mix(in srgb, var(--c-accent) 50%, transparent)'
                        : lvl === 3
                          ? 'color-mix(in srgb, var(--c-accent) 75%, transparent)'
                          : 'var(--c-accent)',
              }}
            />
          ))}
          <span>more</span>
        </div>
      </div>
    </div>
  );
}

function RepoCard() {
  const activity = REPO_ACTIVITY;
  const repoFacts = [
    ['License', 'MIT'],
    ['Release', 'Alpha'],
    ['Commits', String(activity.commitTotal)],
  ] as const;

  return (
    <Reveal>
      <div className="rounded-xl border bg-[var(--c-surface)]" style={{ borderColor: 'var(--c-border)' }}>
        <a
          href={GITHUB_REPO_URL}
          target="_blank"
          rel="noreferrer"
          className="flex items-center gap-2 border-b px-4 py-3 transition hover:bg-[var(--c-app-bg)]"
          style={{ borderColor: 'var(--c-border)' }}
        >
          <Github size={14} className="text-[var(--c-text-secondary)]" />
          <span className="font-mono text-[12px] text-[var(--c-text-primary)]">aryanVijaywargia/Continua</span>
        </a>
        <CommitHeatmap activity={activity} />
        <div className="border-b px-4 py-3" style={{ borderColor: 'var(--c-border)' }}>
          <div className="flex items-center justify-between">
            <div className="font-mono text-[10px] font-semibold uppercase tracking-[0.12em] text-[var(--c-text-muted)]">Current surface</div>
            <ExternalLink href={DOCS_URL} className="font-mono text-[10px] text-[var(--c-accent-text)]">docs ↗</ExternalLink>
          </div>
          <div className="mt-3 grid gap-2">
            {[
              ['Observability', 'REST ingest, Postgres persistence, async River jobs, trace/session read APIs'],
              ['Debugger console', 'Embedded React operator workspace for traces, sessions, payloads, and comparisons'],
              ['Durable engine', 'Preview control surface and contracts; workflow runtime is still on the roadmap'],
            ].map(([label, value]) => (
              <div key={label} className="rounded-md border bg-[var(--c-app-bg)] px-3 py-2" style={{ borderColor: 'var(--c-border)' }}>
                <div className="font-mono text-[10.5px] font-semibold text-[var(--c-text-primary)]">{label}</div>
                <div className="mt-0.5 text-[11px] leading-4 text-[var(--c-text-secondary)]">{value}</div>
              </div>
            ))}
          </div>
        </div>
        <div className="px-4 py-3">
          <div className="font-mono text-[10px] font-semibold uppercase tracking-[0.12em] text-[var(--c-text-muted)]">Repository facts</div>
          <div className="mt-2.5 space-y-1.5">
            {[
              ['Runtime', 'Go server + Postgres + River workers'],
              ['Frontend', 'Vite React console embedded in the Go binary'],
              ['SDK', 'Python SDK is functional; TypeScript package is an early stub'],
              ['Docs', 'Hosted at continua.in/docs'],
            ].map(([label, value]) => (
              <div key={label} className="flex items-center justify-between gap-2 text-[11px]">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="shrink-0 font-mono text-[var(--c-accent-text)]">{label}</span>
                  <span className="truncate text-[var(--c-text-secondary)]">{value}</span>
                </div>
              </div>
            ))}
          </div>
        </div>
        <div className="grid grid-cols-3 gap-px border-t bg-[var(--c-border)]" style={{ borderColor: 'var(--c-border)' }}>
          {repoFacts.map(([label, value]) => (
            <div key={label} className="bg-[var(--c-surface)] px-3 py-2.5">
              <div className="font-mono text-[9px] font-semibold uppercase tracking-[0.1em] text-[var(--c-text-muted)]">{label}</div>
              <div className="mt-0.5 font-mono text-[14px] font-semibold tabular-nums text-[var(--c-text-primary)]">{value}</div>
            </div>
          ))}
        </div>
      </div>
    </Reveal>
  );
}

function isSameOriginHref(href: string): boolean {
  if (typeof window === 'undefined') return false;
  if (href.startsWith('/') || href.startsWith('#')) return true;
  try {
    return new URL(href, window.location.href).origin === window.location.origin;
  } catch {
    return false;
  }
}

function ExternalLink({
  href,
  children,
  className,
  style,
}: {
  href: string;
  children: ReactNode;
  className?: string;
  style?: CSSProperties;
}) {
  const sameOrigin = isSameOriginHref(href);
  return (
    <a
      href={href}
      {...(sameOrigin ? {} : { target: '_blank', rel: 'noreferrer' })}
      className={className}
      style={style}
    >
      {children}
    </a>
  );
}

function toneColors(tone: SpanTone) {
  if (tone === 'fail') {
    return { fill: '#ef4444', bg: 'rgba(239, 68, 68, 0.16)', border: 'rgba(239, 68, 68, 0.35)' };
  }
  if (tone === 'run') {
    return { fill: '#3b82f6', bg: 'rgba(59, 130, 246, 0.18)', border: 'rgba(59, 130, 246, 0.35)' };
  }
  return { fill: '#10b981', bg: 'rgba(16, 185, 129, 0.16)', border: 'rgba(16, 185, 129, 0.35)' };
}

function tickerTone(kind: string) {
  if (kind === 'fail') return { dot: 'var(--c-red)', text: 'var(--c-red-text)', label: 'failed' };
  if (kind === 'retry') return { dot: 'var(--c-amber)', text: 'var(--c-amber-text)', label: 'retried' };
  return { dot: 'var(--c-green)', text: 'var(--c-green-text)', label: 'completed' };
}

function codeColor(tag: string) {
  switch (tag) {
    case 'k':
    case 'n':
      return 'var(--c-accent-text)';
    case 'd':
      return 'var(--c-amber-text)';
    case 's':
      return 'var(--c-green-text)';
    case 'c':
      return 'var(--c-text-muted)';
    case 'fn':
      return '#a855f7';
    default:
      return 'var(--c-text-primary)';
  }
}
