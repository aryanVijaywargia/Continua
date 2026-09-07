import {
  Activity,
  ArrowUpRight,
  Check,
  ChevronDown,
  ChevronRight,
  CircleAlert,
  Code2,
  GitBranch,
  Github,
  Layers,
  Moon,
  RotateCcw,
  Sun,
  Terminal,
} from 'lucide-react';
import {
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from 'react';
import { Link } from 'react-router-dom';
import { useRuntimeAuth } from '../auth/runtime';
import { useTheme } from '../hooks/useTheme';
import { CopyButton } from '../components/CopyButton';
import repoStats from '../data/repo-stats.json';
import '../styles/landing.css';

const GITHUB_REPO_URL = 'https://github.com/aryanVijaywargia/Continua';
const DOCS_URL = 'https://www.continua.in/docs';
const GITHUB_LICENSE_URL = `${GITHUB_REPO_URL}/blob/main/LICENSE`;
const API_REFERENCE_URL = `${DOCS_URL}/api-reference`;
const PYTHON_SDK_DOCS_URL = `${DOCS_URL}/sdk/python/overview`;
const ARCHITECTURE_DOCS_URL = `${DOCS_URL}/concepts/overview`;
const RUN_LOCALLY_DOCS_URL = `${DOCS_URL}/guides/installation`;

const CODE_TABS = [
  {
    label: 'Basic agent',
    file: 'agent.py',
    rows: [
      ['k', 'from '],
      ['m', 'continua'],
      ['k', ' import '],
      ['_', 'Continua, span, trace\n\n'],
      ['_', 'Continua.init(api_key='],
      ['s', '"<project-api-key>"'],
      ['_', ', endpoint='],
      ['s', '"http://localhost:8080"'],
      ['_', ')\n\n'],
      ['c', '# call_llm is supplied by your application\n'],
      ['d', '@trace'],
      ['_', '(name='],
      ['s', '"research_agent"'],
      ['_', ')\n'],
      ['k', 'def '],
      ['fn', 'run'],
      ['_', '(query):\n    '],
      ['k', 'with '],
      ['_', 'span('],
      ['s', '"plan"'],
      ['_', ', kind='],
      ['s', '"llm"'],
      ['_', ') '],
      ['k', 'as '],
      ['_', 's:\n        s.set_input({'],
      ['s', '"query"'],
      [
        '_',
        ': query})\n        result = call_llm(query)\n        s.set_llm_response(',
      ],
      ['s', '"gpt-4"'],
      ['_', ', query, result)\n\n    '],
      ['k', 'return '],
      ['_', '{'],
      ['s', '"answer"'],
      ['_', ': result}'],
    ],
  },
  {
    label: 'With retries',
    file: 'resilient.py',
    rows: [
      ['c', '# fetch_data is supplied by your application\n'],
      ['k', 'from '],
      ['m', 'continua'],
      ['k', ' import '],
      ['_', 'span, trace\n\n'],
      ['d', '@trace'],
      ['_', '(name='],
      ['s', '"resilient_agent"'],
      ['_', ')\n'],
      ['k', 'def '],
      ['fn', 'run'],
      ['_', '(task_id):\n    '],
      ['k', 'for '],
      ['_', 'attempt '],
      ['k', 'in '],
      ['_', 'range('],
      ['n', '1'],
      ['_', ', '],
      ['n', '4'],
      ['_', '):\n        '],
      ['k', 'with '],
      ['_', 'span('],
      ['s', '"fetch"'],
      ['_', ', kind='],
      ['s', '"tool"'],
      ['_', ') '],
      ['k', 'as '],
      ['_', 's:\n            '],
      ['k', 'try'],
      ['_', ':\n                '],
      ['k', 'return '],
      ['_', 'fetch_data(task_id)\n            '],
      ['k', 'except '],
      ['_', 'TimeoutError '],
      ['k', 'as '],
      ['_', 'exc:\n                s.exception(exc, payload={'],
      ['s', '"attempt"'],
      ['_', ': attempt})\n                '],
      ['k', 'if '],
      ['_', 'attempt == '],
      ['n', '3'],
      ['_', ': '],
      ['k', 'raise'],
    ],
  },
  {
    label: 'Sessions',
    file: 'review.py',
    rows: [
      ['c', '# parse_code and call_llm belong to your application\n'],
      ['k', 'from '],
      ['m', 'continua'],
      ['k', ' import '],
      ['_', 'session, span\n\n'],
      ['k', 'def '],
      ['fn', 'review'],
      ['_', '(code):\n    '],
      ['k', 'with '],
      ['_', 'session('],
      ['s', '"demo-review"'],
      ['_', ', user_id='],
      ['s', '"u_123"'],
      ['_', ') '],
      ['k', 'as '],
      ['_', 's:\n        s.set_metadata({'],
      ['s', '"app"'],
      ['_', ': '],
      ['s', '"docs"'],
      ['_', '})\n\n        '],
      ['k', 'with '],
      ['_', 'span('],
      ['s', '"parse"'],
      ['_', ', kind='],
      ['s', '"tool"'],
      ['_', '):\n            parsed = parse_code(code)\n\n        '],
      ['k', 'with '],
      ['_', 'span('],
      ['s', '"review"'],
      ['_', ', kind='],
      ['s', '"llm"'],
      ['_', '):\n            review = call_llm({'],
      ['s', '"parsed"'],
      ['_', ': parsed})\n        '],
      ['k', 'return '],
      ['_', '{'],
      ['s', '"review"'],
      ['_', ': review}'],
    ],
  },
] as const;

const LANDING_SECTION_IDS = [
  'observability',
  'sdk',
  'engine',
  'open-source',
] as const;
const INSTALL_COMMAND =
  'git clone https://github.com/aryanVijaywargia/Continua.git\ncd Continua\nmake demo';
const SAMPLE_SPANS = [
  {
    name: 'research_agent',
    kind: 'Workflow',
    start: 0,
    width: 100,
    duration: '4.20s',
    status: 'Completed',
    input: '{ "query": "How do agents recover?" }',
    output: '{ "answer": "Resume from saved history." }',
  },
  {
    name: 'plan_research',
    kind: 'LLM',
    start: 3,
    width: 19,
    duration: '798ms',
    status: 'Completed',
    input: '{ "task": "Build a research plan" }',
    output: '{ "steps": ["retrieve", "summarize"] }',
  },
  {
    name: 'fetch_sources',
    kind: 'Tool',
    start: 24,
    width: 31,
    duration: '1.30s',
    status: 'Failed',
    input: '{ "query": "agent recovery", "limit": 5 }',
    output: '{ "error": "TimeoutError", "attempt": 1 }',
  },
  {
    name: 'fetch_sources',
    kind: 'Retry',
    start: 56,
    width: 15,
    duration: '630ms',
    status: 'Completed',
    input: '{ "query": "agent recovery", "limit": 5 }',
    output: '{ "documents": 5, "attempt": 2 }',
  },
  {
    name: 'summarize',
    kind: 'LLM',
    start: 73,
    width: 20,
    duration: '840ms',
    status: 'Completed',
    input: '{ "documents": 5, "format": "summary" }',
    output: '{ "answer": "Resume from saved history." }',
  },
  {
    name: 'save_answer',
    kind: 'Tool',
    start: 94,
    width: 6,
    duration: '252ms',
    status: 'Completed',
    input: '{ "session_id": "research-42" }',
    output: '{ "saved": true }',
  },
] as const;

export function LandingPage() {
  const auth = useRuntimeAuth();
  const { resolvedTheme, toggleTheme } = useTheme();
  const available = auth.console_available !== false;
  const demo = auth.public_demo_enabled === true;
  const label = !available
    ? 'Run Locally'
    : demo
      ? 'Open Demo'
      : 'Open Console';
  useLandingSectionHashSync();

  return (
    <div className="continua-landing" id="top">
      <a href="#landing-main" className="cl-skip">
        Skip to content
      </a>
      <header className="cl-header cl-wrap">
        <a className="cl-brand" href="#top">
          <Logo />
          <span>Continua</span>
        </a>
        <nav aria-label="Landing sections" className="cl-nav">
          <a href="#observability">Observability</a>
          <a href="#engine">Engine</a>
          <a href="#sdk">SDK</a>
          <ExternalLink href={DOCS_URL}>Docs</ExternalLink>
        </nav>
        <div className="cl-nav-actions">
          <button
            className="cl-icon-button"
            aria-label="Toggle theme"
            onClick={toggleTheme}
          >
            {resolvedTheme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
          </button>
          <ConsoleLink
            available={available}
            label={label}
            className="cl-button cl-button-small"
          />
        </div>
      </header>
      <main id="landing-main">
        <section className="cl-hero cl-wrap" aria-labelledby="hero-title">
          <div className="cl-hero-copy">
            <a className="cl-preview-note" href="#engine">
              <span className="cl-status-dot" /> Open source. Built for agent
              builders.
              <ChevronRight size={14} />
            </a>
            <h1 id="hero-title">Know what your agent actually did.</h1>
            <p className="cl-lead">
              Every call, retry, and unexpected turn.
              <br className="cl-desktop-break" /> Trace your agents, inspect
              their state, and find out where things went wrong.
            </p>
            <div className="cl-actions">
              <ConsoleLink available={available} label={label} />
              <ExternalLink
                href={RUN_LOCALLY_DOCS_URL}
                className="cl-text-link"
              >
                <Terminal size={16} /> Run locally
              </ExternalLink>
            </div>
            <p className="cl-hero-footnote">
              <Check size={14} /> Self-hosted. MIT licensed. Your data stays
              yours.
            </p>
            {demo ? (
              <p className="cl-hosting-note">
                This hosted debugger uses seeded sample traces only. Run locally
                to inspect your own traces and sessions.
              </p>
            ) : !available ? (
              <p className="cl-hosting-note">
                Run Continua locally to inspect your own traces and sessions.
              </p>
            ) : null}
          </div>
          <TracePreview />
        </section>
        <div className="cl-facts cl-wrap">
          <span>Built to make agent runs understandable.</span>
          <div>
            <span>
              <GitBranch size={16} /> Trace the execution
            </span>
            <span>
              <Code2 size={16} /> Inspect the payload
            </span>
            <span>
              <Layers size={16} /> Follow the session
            </span>
          </div>
        </div>

        <section id="observability" className="cl-section cl-wrap">
          <div className="cl-section-intro">
            <h2>See the failure, the input, and the retry.</h2>
            <p>
              Start with the request that timed out. Then read the data it saw,
              the exception it recorded, and the retry that completed.
            </p>
          </div>
          <div className="cl-feature-grid">
            <article className="cl-feature">
              <div
                className="cl-payload-visual"
                aria-label="Example span payload"
              >
                <div className="cl-mini-heading">
                  <Code2 size={16} /> fetch_sources <span>Input → Output</span>
                </div>
                <div className="cl-payload-line">
                  <span>query</span>
                  <code>"agent recovery"</code>
                </div>
                <div className="cl-payload-line">
                  <span>attempt</span>
                  <code>1</code>
                </div>
                <div className="cl-error-message">
                  <CircleAlert size={16} />
                  <div>
                    <strong>TimeoutError</strong>
                    <span>Source request exceeded 1,300ms.</span>
                  </div>
                </div>
                <div className="cl-retry-note">
                  <RotateCcw size={14} /> Next attempt completed in 630ms.
                </div>
              </div>
              <h3>The failed request has a record.</h3>
              <p>
                Open a span to inspect inputs, outputs, exceptions, and state
                changes. Follow a failed call through to its next attempt.
              </p>
            </article>
            <article className="cl-feature">
              <div
                className="cl-session-visual"
                aria-label="Example session with three related traces"
              >
                <div className="cl-mini-heading">
                  <Layers size={16} /> research-session-42 <span>3 traces</span>
                </div>
                {[
                  ['Find sources', '4.20s', '6 spans'],
                  ['Refine the answer', '2.18s', '4 spans'],
                  ['Add citations', '1.06s', '3 spans'],
                ].map(([name, duration, count]) => (
                  <div className="cl-session-row" key={name}>
                    <span className="cl-session-node">
                      <Check size={12} />
                    </span>
                    <div>
                      <strong>{name}</strong>
                      <span>{count}</span>
                    </div>
                    <code>{duration}</code>
                  </div>
                ))}
              </div>
              <h3>Keep related runs in one session.</h3>
              <p>
                Group related runs into sessions. Move between traces and
                compare executions with their shared context in view.
              </p>
            </article>
          </div>
        </section>

        <section id="sdk" className="cl-sdk-section">
          <div className="cl-wrap cl-split">
            <div className="cl-section-copy">
              <span className="cl-section-marker">
                <Terminal size={18} /> Python SDK
              </span>
              <h2>Trace the call you need to explain.</h2>
              <p>
                Put a trace around the agent entry point and spans around the
                work inside it. Continua records the inputs, outputs, and errors.
              </p>
              <p className="cl-small-copy">
                The Python SDK batches spans, polls async ingest, and includes
                helpers for traces, spans, and sessions.
              </p>
              <ExternalLink href={PYTHON_SDK_DOCS_URL} className="cl-text-link">
                Read the SDK guide <ArrowUpRight size={16} />
              </ExternalLink>
            </div>
            <SdkExample />
          </div>
        </section>

        <section id="engine" className="cl-section cl-wrap cl-split">
          <div
            className="cl-engine-visual"
            aria-label="Workflow recovery example"
          >
            <div className="cl-mini-heading">
              <GitBranch size={17} /> research_workflow{' '}
              <span className="cl-badge">Preview</span>
            </div>
            <ol className="cl-event-list">
              <li>
                <Check size={15} />
                <div>
                  <strong>Sources fetched</strong>
                  <span>Activity result saved</span>
                </div>
                <code>10:42:01</code>
              </li>
              <li className="cl-event-interrupted">
                <CircleAlert size={15} />
                <div>
                  <strong>Worker disconnected</strong>
                  <span>Execution history retained</span>
                </div>
                <code>10:42:03</code>
              </li>
              <li>
                <RotateCcw size={15} />
                <div>
                  <strong>Workflow resumed</strong>
                  <span>Completed activity restored from history</span>
                </div>
                <code>10:42:08</code>
              </li>
              <li>
                <Check size={15} />
                <div>
                  <strong>Summary completed</strong>
                  <span>Continued with the next activity</span>
                </div>
                <code>10:42:09</code>
              </li>
            </ol>
            <div className="cl-engine-caption">
              The process stopped. The work carried on.
            </div>
          </div>
          <div className="cl-section-copy">
            <span className="cl-section-marker">
              <Activity size={18} /> Durable engine{' '}
              <span className="cl-badge">Preview</span>
            </span>
            <h2>Resume work after the worker stops.</h2>
            <p>
              Run Go-defined workflows that resume from persisted history after
              a process restart. Inspect their execution in the same debugger.
            </p>
            <ul className="cl-capabilities">
              <li>Activities + retries</li>
              <li>Timers + signals</li>
              <li>Child workflows</li>
              <li>Continue-as-new</li>
            </ul>
            <ExternalLink href={ARCHITECTURE_DOCS_URL} className="cl-text-link">
              Explore the architecture <ArrowUpRight size={16} />
            </ExternalLink>
            <p className="cl-small-copy cl-engine-boundary">
              The engine is a working preview. Workflow authoring is Go-only;
              the built-in runtime uses a fixed demo project.
            </p>
          </div>
        </section>

        <section id="open-source" className="cl-open-source">
          <div className="cl-wrap">
            <Github size={30} strokeWidth={1.5} />
            <h2>Run the debugger with your own data.</h2>
            <p>
              Run Continua on your own stack. Read the code, follow the
              development, and help build what comes next.
            </p>
            <div className="cl-actions">
              <ExternalLink href={GITHUB_REPO_URL} className="cl-button">
                <Github size={17} /> Star on GitHub
              </ExternalLink>
              <ExternalLink
                href={RUN_LOCALLY_DOCS_URL}
                className="cl-text-link"
              >
                Get started locally <ArrowUpRight size={16} />
              </ExternalLink>
            </div>
            <div className="cl-install">
              <div>
                <Terminal size={15} />
                <span>Start the local demo</span>
                <CopyButton
                  value={INSTALL_COMMAND}
                  aria-label="Copy local setup commands"
                />
              </div>
              <pre>
                <code>{INSTALL_COMMAND}</code>
              </pre>
            </div>
            <p className="cl-repo-note">
              {repoStats.commitTotal} commits
              {repoStats.firstCommitAt
                ? ` · since ${new Date(repoStats.firstCommitAt).toLocaleDateString('en-US', { month: 'short', year: 'numeric', timeZone: 'UTC' })}`
                : ''}
              <span>MIT licensed</span>
              <span>Alpha</span>
            </p>
          </div>
        </section>
      </main>
      <footer className="cl-footer cl-wrap">
        <div>
          <a href="#top" className="cl-brand">
            <Logo />
            <span>Continua</span>
          </a>
          <p>Open-source observability for AI agents.</p>
        </div>
        <nav aria-label="Footer">
          <a href="#open-source">Open source</a>
          <ExternalLink href={DOCS_URL}>Docs</ExternalLink>
          <ExternalLink href={API_REFERENCE_URL}>API reference</ExternalLink>
          <ExternalLink href={GITHUB_LICENSE_URL}>License</ExternalLink>
          {available ? (
            <Link to="/dashboard">Console</Link>
          ) : (
            <ExternalLink href={RUN_LOCALLY_DOCS_URL}>Console</ExternalLink>
          )}
        </nav>
        <a className="cl-back-top" href="#top">
          Back to top ↑
        </a>
      </footer>
    </div>
  );
}

function Logo() {
  return (
    <svg
      width="28"
      height="28"
      viewBox="0 0 32 32"
      fill="none"
      aria-hidden="true"
    >
      <path
        d="M24 6H13a10 10 0 0 0 0 20h11M24 12H13a4 4 0 0 0 0 8h11"
        stroke="currentColor"
        strokeWidth="3.2"
        strokeLinecap="round"
      />
      <circle cx="24" cy="6" r="2.2" fill="currentColor" />
      <circle cx="24" cy="20" r="2.2" fill="currentColor" />
    </svg>
  );
}

function ConsoleLink({
  available,
  label,
  className = 'cl-button',
}: {
  available: boolean;
  label: string;
  className?: string;
}) {
  return available ? (
    <Link to="/dashboard" className={className}>
      {label}
      <ArrowUpRight size={16} />
    </Link>
  ) : (
    <ExternalLink href={RUN_LOCALLY_DOCS_URL} className={className}>
      {label}
      <ArrowUpRight size={16} />
    </ExternalLink>
  );
}

function TracePreview() {
  const [selected, setSelected] = useState(2);
  const [view, setView] = useState('Payload');
  const views = ['Payload', 'Timeline', 'State'];
  const tabRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const span = SAMPLE_SPANS[selected];
  return (
    <div className="cl-trace-stage">
      <div className="cl-trace-window">
        <div className="cl-window-header">
          <div>
            <Logo />
            <span>research_agent</span>
            <ChevronDown size={13} />
          </div>
          <span className="cl-sample-label">Sample trace</span>
        </div>
        <div className="cl-trace-summary">
          <div>
            <span className="cl-complete">
              <Check size={12} /> Completed
            </span>
            <span>1 recovered error</span>
          </div>
          <code>4.20s</code>
        </div>
        <div className="cl-waterfall" aria-label="Interactive example trace">
          <div className="cl-waterfall-scale">
            <span>Execution</span>
            <div>
              <span>0s</span>
              <span>2s</span>
              <span>4.2s</span>
            </div>
          </div>
          {SAMPLE_SPANS.map((item, index) => (
            <button
              key={`${item.name}-${index}`}
              type="button"
              className={`cl-span-row ${item.status === 'Failed' ? 'cl-span-failed' : ''} ${item.kind === 'Retry' ? 'cl-span-retry' : ''}`}
              aria-pressed={selected === index}
              aria-label={`Inspect ${item.name}${item.kind === 'Retry' ? ' retry' : ''}`}
              onClick={() => setSelected(index)}
            >
              <span
                className={`cl-span-name ${index > 0 ? 'cl-span-child' : ''}`}
              >
                {index === 0 ? (
                  <GitBranch size={13} />
                ) : item.kind === 'Retry' ? (
                  <RotateCcw size={12} />
                ) : (
                  <span className="cl-tree-elbow" />
                )}
                <span>{item.name}</span>
              </span>
              <span className="cl-span-track">
                <span
                  className="cl-span-bar"
                  style={{ left: `${item.start}%`, width: `${item.width}%` }}
                />
                <span className="cl-span-duration">{item.duration}</span>
              </span>
            </button>
          ))}
        </div>
        <div className="cl-inspector">
          <div className="cl-inspector-title">
            <span>
              {span.status === 'Failed' ? (
                <CircleAlert size={15} />
              ) : (
                <Check size={15} />
              )}
              <strong>{span.name}</strong>
              <span className="cl-kind">{span.kind}</span>
            </span>
            <code>{span.duration}</code>
          </div>
          <div
            className="cl-preview-tabs"
            role="tablist"
            aria-label="Sample span inspection"
          >
            {views.map((item, index) => (
              <button
                key={item}
                ref={(el) => {
                  tabRefs.current[index] = el;
                }}
                id={`preview-tab-${item}`}
                role="tab"
                aria-selected={view === item}
                aria-controls={`preview-panel-${item}`}
                tabIndex={view === item ? 0 : -1}
                onClick={() => setView(item)}
                onKeyDown={(event) => {
                  let next = index;
                  if (event.key === 'ArrowRight')
                    next = (index + 1) % views.length;
                  else if (event.key === 'ArrowLeft')
                    next = (index + views.length - 1) % views.length;
                  else if (event.key === 'Home') next = 0;
                  else if (event.key === 'End') next = views.length - 1;
                  else return;
                  event.preventDefault();
                  setView(views[next]);
                  tabRefs.current[next]?.focus();
                }}
              >
                {item}
              </button>
            ))}
          </div>
          <div
            className="cl-preview-panel"
            role="tabpanel"
            id={`preview-panel-${view}`}
            aria-labelledby={`preview-tab-${view}`}
            tabIndex={0}
          >
            {view === 'Payload' ? (
              <>
                <div>
                  <span>Input</span>
                  <code>{span.input}</code>
                </div>
                <div
                  className={span.status === 'Failed' ? 'cl-output-error' : ''}
                >
                  <span>
                    {span.status === 'Failed' ? 'Exception' : 'Output'}
                  </span>
                  <code>{span.output}</code>
                </div>
              </>
            ) : view === 'Timeline' ? (
              <>
                <div>
                  <span>Started</span>
                  <code>
                    {((span.start / 100) * 4.2).toFixed(3)}s after trace start
                  </code>
                </div>
                <div>
                  <span>{span.status}</span>
                  <code>
                    {span.duration} elapsed
                    {span.status === 'Failed' ? ' · retry follows' : ''}
                  </code>
                </div>
              </>
            ) : (
              <>
                <div>
                  <span>Status</span>
                  <code>{span.status.toLowerCase()}</code>
                </div>
                <div>
                  <span>Attempt</span>
                  <code>
                    {span.kind === 'Retry' ? '2 · recovered' : '1'}
                    {span.status === 'Failed' ? ' · exception recorded' : ''}
                  </code>
                </div>
              </>
            )}
          </div>
        </div>
        <div className="cl-window-footer">
          <span>
            <span className="cl-status-dot" /> All 6 spans captured
          </span>
          <span>research-session-42</span>
        </div>
      </div>
      <div className="cl-trace-caption">
        <span className="cl-caption-line" />
        <span>A timeout, a retry, and the whole story.</span>
      </div>
      <p className="cl-interaction-hint">
        Select a span to take a closer look.
      </p>
    </div>
  );
}

function SdkExample() {
  const [active, setActive] = useState(0);
  const refs = useRef<(HTMLButtonElement | null)[]>([]);
  return (
    <div className="cl-code-window">
      <div
        className="cl-code-tabs"
        role="tablist"
        aria-label="Python SDK examples"
      >
        {CODE_TABS.map((tab, index) => (
          <button
            ref={(el) => {
              refs.current[index] = el;
            }}
            type="button"
            key={tab.file}
            role="tab"
            id={`sdk-tab-${index}`}
            aria-selected={active === index}
            aria-controls={`sdk-panel-${index}`}
            tabIndex={active === index ? 0 : -1}
            onClick={() => setActive(index)}
            onKeyDown={(event) => {
              let next = index;
              if (event.key === 'ArrowRight')
                next = (index + 1) % CODE_TABS.length;
              else if (event.key === 'ArrowLeft')
                next = (index + CODE_TABS.length - 1) % CODE_TABS.length;
              else if (event.key === 'Home') next = 0;
              else if (event.key === 'End') next = CODE_TABS.length - 1;
              else return;
              event.preventDefault();
              setActive(next);
              refs.current[next]?.focus();
            }}
          >
            {tab.file}
          </button>
        ))}
      </div>
      <pre
        role="tabpanel"
        id={`sdk-panel-${active}`}
        aria-labelledby={`sdk-tab-${active}`}
        tabIndex={0}
      >
        <code>
          {CODE_TABS[active].rows.map(([tag, text], index) => (
            <span className={`cl-code-${tag}`} key={index}>
              {text}
            </span>
          ))}
        </code>
      </pre>
      <div className="cl-code-footer">
        <span>Python instrumentation pattern</span>
        <CopyButton
          value={CODE_TABS[active].rows.map(([, text]) => text).join('')}
          aria-label="Copy Python example"
        />
      </div>
    </div>
  );
}

function useLandingSectionHashSync() {
  useEffect(() => {
    if (typeof IntersectionObserver !== 'function') {
      return;
    }

    const sections = LANDING_SECTION_IDS.map((id) =>
      document.getElementById(id),
    ).filter((section): section is HTMLElement => Boolean(section));
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
        `${window.location.pathname}${window.location.search}#${sectionId}`,
      );
    };

    const observer = new IntersectionObserver(
      (entries) => {
        const visibleEntry = entries
          .filter((entry) => entry.isIntersecting)
          .sort(
            (left, right) => right.intersectionRatio - left.intersectionRatio,
          )[0];

        if (visibleEntry?.target.id) {
          updateHash(visibleEntry.target.id);
        }
      },
      {
        rootMargin: '-35% 0px -50% 0px',
        threshold: [0, 0.2, 0.5, 0.8],
      },
    );

    sections.forEach((section) => observer.observe(section));

    return () => observer.disconnect();
  }, []);
}

function isSameOriginHref(href: string): boolean {
  if (typeof window === 'undefined') return false;
  if (href.startsWith('/') || href.startsWith('#')) return true;
  try {
    return (
      new URL(href, window.location.href).origin === window.location.origin
    );
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
