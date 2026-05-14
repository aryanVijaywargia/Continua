import { useState } from 'react';
import { Link } from 'react-router-dom';
import { CheckpointIllustration } from '../components/landing/CheckpointIllustration';
import { HeroIllustration } from '../components/landing/HeroIllustration';
import { ScrollReveal } from '../components/landing/ScrollReveal';
import { useRuntimeAuth } from '../auth/runtime';
import { useMediaQuery } from '../hooks/useMediaQuery';

/* ── Code tab content ────────────────────────────────────────────── */

interface CodeTab {
  label: string;
  content: JSX.Element;
}

const CODE_TABS: CodeTab[] = [
  {
    label: 'Basic Agent',
    content: (
      <pre className="font-mono text-sm leading-relaxed text-on-surface md:text-lg">
        <span className="text-primary-container">from</span> continua{' '}
        <span className="text-primary-container">import</span> Continua, span, trace{'\n'}
        {'\n'}
        client = Continua.init({'\n'}
        {'    '}api_key=<span className="text-outline">&quot;default&quot;</span>,{'\n'}
        {'    '}endpoint=<span className="text-outline">&quot;http://localhost:8080&quot;</span>,{'\n'}
        {'    '}ingest_mode=<span className="text-outline">&quot;sync&quot;</span>,{'\n'}
        ){'\n'}
        {'\n'}
        <span className="text-primary-container">@trace</span>(name=<span className="text-outline">&quot;research_agent&quot;</span>){'\n'}
        <span className="text-primary-container">def</span>{' '}
        <span className="text-tertiary">run_research_agent</span>(query: str):{'\n'}
        {'    '}<span className="text-primary-container">with</span> span(<span className="text-outline">&quot;plan_research&quot;</span>, kind=<span className="text-outline">&quot;llm&quot;</span>) <span className="text-primary-container">as</span> s:
        {'\n'}
        {'        '}s.set_input({'{'}<span className="text-outline">&quot;query&quot;</span>: query{'}'}){'\n'}
        {'        '}response = call_llm(query){'\n'}
        {'        '}s.set_llm_response({'\n'}
        {'            '}<span className="text-outline">&quot;gpt-4&quot;</span>, query, response,{'\n'}
        {'            '}tokens_in=50, tokens_out=100, provider=<span className="text-outline">&quot;openai&quot;</span>,{'\n'}
        {'        '}){'\n'}
        {'\n'}
        {'    '}<span className="text-primary-container">return</span> {'{'}<span className="text-outline">&quot;answer&quot;</span>: response{'}'}{'\n'}
        {'\n'}
        result = run_research_agent(<span className="text-outline">&quot;debug retry behavior&quot;</span>){'\n'}
        client.flush()
      </pre>
    ),
  },
  {
    label: 'With Retries',
    content: (
      <pre className="font-mono text-sm leading-relaxed text-on-surface md:text-lg">
        <span className="text-primary-container">from</span> continua{' '}
        <span className="text-primary-container">import</span> Continua, span, trace{'\n'}
        {'\n'}
        client = Continua.init({'\n'}
        {'    '}api_key=<span className="text-outline">&quot;default&quot;</span>,{'\n'}
        {'    '}endpoint=<span className="text-outline">&quot;http://localhost:8080&quot;</span>,{'\n'}
        {'    '}ingest_mode=<span className="text-outline">&quot;sync&quot;</span>,{'\n'}
        {'    '}max_retries=3,{'\n'}
        ){'\n'}
        {'\n'}
        <span className="text-primary-container">@trace</span>(name=<span className="text-outline">&quot;resilient_agent&quot;</span>, tags=[<span className="text-outline">&quot;demo&quot;</span>]){'\n'}
        <span className="text-primary-container">def</span>{' '}
        <span className="text-tertiary">run_resilient_agent</span>(task_id: str):{'\n'}
        {'    '}<span className="text-primary-container">for</span> attempt <span className="text-primary-container">in</span> range(1, 4):{'\n'}
        {'        '}<span className="text-primary-container">with</span> span(<span className="text-outline">&quot;fetch_data&quot;</span>, kind=<span className="text-outline">&quot;tool&quot;</span>) <span className="text-primary-container">as</span> s:
        {'\n'}
        {'            '}s.set_input({'{'}<span className="text-outline">&quot;task_id&quot;</span>: task_id, <span className="text-outline">&quot;attempt&quot;</span>: attempt{'}'}){'\n'}
        {'            '}<span className="text-primary-container">try</span>:{'\n'}
        {'                '}result = fetch_external_data(task_id){'\n'}
        {'                '}s.set_tool_call(<span className="text-outline">&quot;fetch_external_data&quot;</span>, {'{'}<span className="text-outline">&quot;task_id&quot;</span>: task_id{'}'}, result){'\n'}
        {'                '}<span className="text-primary-container">return</span> result{'\n'}
        {'            '}<span className="text-primary-container">except</span> TimeoutError <span className="text-primary-container">as</span> exc:{'\n'}
        {'                '}s.error(<span className="text-outline">&quot;Fetch timed out&quot;</span>, payload={'{'}<span className="text-outline">&quot;attempt&quot;</span>: attempt{'}'}){'\n'}
        {'                '}s.exception(exc, payload={'{'}<span className="text-outline">&quot;attempt&quot;</span>: attempt{'}'}){'\n'}
        {'                '}s.set_error(str(exc)){'\n'}
        {'                '}<span className="text-primary-container">if</span> attempt == 3:{'\n'}
        {'                    '}<span className="text-primary-container">raise</span>{'\n'}
        {'\n'}
        run_resilient_agent(<span className="text-outline">&quot;task_123&quot;</span>){'\n'}
        client.flush()
      </pre>
    ),
  },
  {
    label: 'Complex Logic',
    content: (
      <pre className="font-mono text-sm leading-relaxed text-on-surface md:text-lg">
        <span className="text-primary-container">from</span> continua{' '}
        <span className="text-primary-container">import</span> Continua, session, span, trace{'\n'}
        {'\n'}
        client = Continua.init(api_key=<span className="text-outline">&quot;default&quot;</span>, endpoint=<span className="text-outline">&quot;http://localhost:8080&quot;</span>){'\n'}
        {'\n'}
        <span className="text-primary-container">@trace</span>(name=<span className="text-outline">&quot;code_review_agent&quot;</span>){'\n'}
        <span className="text-primary-container">def</span>{' '}
        <span className="text-tertiary">run_code_review_agent</span>(code: str):{'\n'}
        {'    '}<span className="text-primary-container">with</span> span(<span className="text-outline">&quot;code_analysis_chain&quot;</span>, kind=<span className="text-outline">&quot;chain&quot;</span>):
        {'\n'}
        {'        '}<span className="text-primary-container">with</span> span(<span className="text-outline">&quot;parse_code&quot;</span>, kind=<span className="text-outline">&quot;tool&quot;</span>) <span className="text-primary-container">as</span> s:{'\n'}
        {'            '}parsed = parse_code(code){'\n'}
        {'            '}s.set_tool_call(<span className="text-outline">&quot;parse_code&quot;</span>, {'{'}<span className="text-outline">&quot;code&quot;</span>: code[:100]{'}'}, parsed){'\n'}
        {'\n'}
        {'        '}<span className="text-primary-container">with</span> span(<span className="text-outline">&quot;security_scan&quot;</span>, kind=<span className="text-outline">&quot;tool&quot;</span>) <span className="text-primary-container">as</span> s:{'\n'}
        {'            '}scan = security_scan(code){'\n'}
        {'            '}s.set_output(scan){'\n'}
        {'\n'}
        {'    '}<span className="text-primary-container">with</span> span(<span className="text-outline">&quot;generate_review&quot;</span>, kind=<span className="text-outline">&quot;llm&quot;</span>) <span className="text-primary-container">as</span> s:{'\n'}
        {'        '}review = call_llm({'{'}<span className="text-outline">&quot;parsed&quot;</span>: parsed, <span className="text-outline">&quot;scan&quot;</span>: scan{'}'}){'\n'}
        {'        '}s.set_llm_response(<span className="text-outline">&quot;claude-3-opus&quot;</span>, code, review, provider=<span className="text-outline">&quot;anthropic&quot;</span>){'\n'}
        {'        '}<span className="text-primary-container">return</span> {'{'}<span className="text-outline">&quot;review&quot;</span>: review{'}'}{'\n'}
        {'\n'}
        <span className="text-primary-container">with</span> session(<span className="text-outline">&quot;demo-sdk-review&quot;</span>, user_id=<span className="text-outline">&quot;user_123&quot;</span>):{'\n'}
        {'    '}run_code_review_agent(code){'\n'}
        {'\n'}
        client.flush()
      </pre>
    ),
  },
];

const GITHUB_REPO_URL = 'https://github.com/aryanVijaywargia/Continua';
const GITHUB_DOCS_URL = `${GITHUB_REPO_URL}/tree/main/docs`;
const GITHUB_LICENSE_URL = `${GITHUB_REPO_URL}/blob/main/LICENSE`;
const GITHUB_OPENAPI_URL = `${GITHUB_REPO_URL}/blob/main/contracts/openapi/openapi.yaml`;
const RUN_LOCALLY_DOCS_URL = `${GITHUB_REPO_URL}/blob/main/docs/setup.md`;

/* ── Page component ──────────────────────────────────────────────── */

export function LandingPage() {
  const [activeTab, setActiveTab] = useState(0);
  const isDesktopNav = useMediaQuery('(min-width: 768px)');
  const runtimeAuth = useRuntimeAuth();
  const isPublicDemo = runtimeAuth.public_demo_enabled === true;
  const isConsoleAvailable = runtimeAuth.console_available !== false;
  const consoleCtaLabel = isPublicDemo
    ? 'Open Demo'
    : isConsoleAvailable
      ? 'Open Console'
      : 'Run Locally';
  const topSecondaryLabel = isPublicDemo ? 'Run locally' : 'Docs';
  const topSecondaryHref = isPublicDemo ? RUN_LOCALLY_DOCS_URL : GITHUB_DOCS_URL;
  const heroPrimaryLabel = isPublicDemo
    ? 'Open Demo'
    : isConsoleAvailable
      ? 'Open Console'
      : 'Run Locally';
  const heroSecondaryLabel = isPublicDemo ? 'Run Locally' : 'See how it works';
  const heroSecondaryHref = isPublicDemo ? RUN_LOCALLY_DOCS_URL : '#how-it-works';
  const heroSecondaryIcon = isPublicDemo ? 'open_in_new' : 'play_circle';
  const footerConsoleLabel = isPublicDemo
    ? 'Open Demo'
    : isConsoleAvailable
      ? 'Operator Console'
      : 'Run locally';

  return (
    <div
      className="min-h-screen selection:bg-primary-container selection:text-white"
      style={{
        fontFamily: "'Inter', sans-serif",
        backgroundColor: '#f9f9f9',
        color: '#1b1b1b',
      }}
    >
      <header className="fixed inset-x-0 top-0 z-[60]">
        <div className="border-b border-black/5 bg-secondary-container px-4 py-2 text-on-secondary-container sm:px-6 sm:py-2.5">
          <div className="mx-auto flex max-w-7xl items-center justify-center gap-2 text-center sm:gap-3">
            <span className="hidden text-xs font-bold uppercase tracking-widest font-label sm:inline">
              {isPublicDemo ? 'Public demo' : 'New in Continua'}
            </span>
            <span className="text-xs font-semibold tracking-tight sm:text-sm">
              {isPublicDemo ? (
                <>
                  <span className="sm:hidden">
                    Seeded sample traces only. Run locally for your own data.
                  </span>
                  <span className="hidden sm:inline">
                    Public demo is live with seeded sample traces. Run Continua locally to inspect your own data.
                  </span>
                </>
              ) : (
                <>
                  <span className="sm:hidden">
                    Operator auth and project switching are live.
                  </span>
                  <span className="hidden sm:inline">
                    Hosted operator auth, project switching, and debugger triage are live.
                  </span>
                </>
              )}
            </span>
          </div>
        </div>

        <nav className="border-b border-zinc-900/5 bg-white/80 backdrop-blur-xl">
          <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-6">
            <div className="text-xl font-black tracking-tighter text-zinc-900">
              Continua
            </div>
            {isDesktopNav ? (
              <div className="flex items-center gap-10">
                <a
                  className="font-bold tracking-tight text-zinc-900 transition-colors hover:text-zinc-700"
                  href="#how-it-works"
                >
                  How it works
                </a>
                <a
                  className="tracking-tight text-zinc-500 transition-colors hover:text-zinc-900"
                  href="#examples"
                >
                  Examples
                </a>
                <a
                  className="tracking-tight text-zinc-500 transition-colors hover:text-zinc-900"
                  href="#open-source"
                >
                  Open source
                </a>
              </div>
            ) : null}
            <div className="flex items-center gap-4">
              <a
                href={topSecondaryHref}
                target="_blank"
                rel="noreferrer"
                className="hidden text-sm font-bold text-zinc-600 transition-colors hover:text-zinc-900 sm:inline"
              >
                {topSecondaryLabel}
              </a>
              {isConsoleAvailable ? (
                <Link
                  to="/dashboard"
                  className="scale-95 rounded-full bg-zinc-900 px-5 py-2 text-sm font-bold text-white transition-all duration-200 hover:bg-zinc-800 active:scale-90"
                >
                  {consoleCtaLabel}
                </Link>
              ) : (
                <a
                  href={RUN_LOCALLY_DOCS_URL}
                  target="_blank"
                  rel="noreferrer"
                  className="scale-95 rounded-full bg-zinc-900 px-5 py-2 text-sm font-bold text-white transition-all duration-200 hover:bg-zinc-800 active:scale-90"
                >
                  {consoleCtaLabel}
                </a>
              )}
            </div>
          </div>
          {!isDesktopNav ? (
            <div className="border-t border-zinc-900/5 px-4 py-2">
              <div className="mx-auto flex max-w-7xl gap-2 overflow-x-auto whitespace-nowrap">
                <a
                  className="rounded-full border border-zinc-900/10 bg-white px-3 py-1.5 text-xs font-semibold text-zinc-700 transition-colors hover:text-zinc-900"
                  href="#how-it-works"
                  aria-label="Jump to how it works"
                >
                  How
                </a>
                <a
                  className="rounded-full border border-zinc-900/10 bg-white px-3 py-1.5 text-xs font-semibold text-zinc-700 transition-colors hover:text-zinc-900"
                  href="#examples"
                  aria-label="Jump to examples"
                >
                  Examples
                </a>
                <a
                  className="rounded-full border border-zinc-900/10 bg-white px-3 py-1.5 text-xs font-semibold text-zinc-700 transition-colors hover:text-zinc-900"
                  href="#open-source"
                  aria-label="Jump to open source"
                >
                  Source
                </a>
                <a
                  className="rounded-full border border-zinc-900/10 bg-white px-3 py-1.5 text-xs font-semibold text-zinc-700 transition-colors hover:text-zinc-900"
                  href={topSecondaryHref}
                  target="_blank"
                  rel="noreferrer"
                  aria-label={isPublicDemo ? 'Open run locally guide' : 'Open docs'}
                >
                  {topSecondaryLabel}
                </a>
              </div>
            </div>
          ) : null}
        </nav>
      </header>

      <main className={isDesktopNav ? 'pt-24 sm:pt-28 md:pt-32' : 'pt-36'}>
        {/* ── Hero Section ── */}
        <section className="mx-auto flex max-w-7xl flex-col items-center px-6 py-20 text-center md:py-32">
          <div className="mb-8 inline-flex items-center gap-2 rounded-full bg-surface-container px-3 py-1">
            <span className="h-2 w-2 rounded-full bg-primary-container" />
            <span className="text-[10px] font-bold uppercase tracking-widest text-on-surface-variant">
              {isPublicDemo ? 'Read-only sample data' : 'Open source debugger'}
            </span>
          </div>
          <h1 className="tight-headline mb-8 max-w-5xl text-5xl font-black leading-[0.94] text-on-surface md:text-8xl">
            {isPublicDemo ? (
              <>
                <span className="block sm:inline">Explore a seeded</span>
                <span className="mt-3 inline-block bg-primary-container px-4 py-1 text-white sm:mt-0 sm:ml-3 sm:-rotate-1">
                  debugger demo
                </span>
                <span className="mt-3 block sm:mt-0 sm:ml-3 sm:inline">
                  with sample traces.
                </span>
              </>
            ) : (
              <>
                <span className="block sm:inline">Run and inspect</span>
                <span className="mt-3 inline-block bg-primary-container px-4 py-1 text-white sm:mt-0 sm:ml-3 sm:-rotate-1">
                  durable
                </span>
                <span className="mt-3 block sm:mt-0 sm:ml-3 sm:inline">
                  AI agents.
                </span>
              </>
            )}
          </h1>
          <p className="mb-12 max-w-3xl text-lg font-medium leading-relaxed text-on-surface-variant md:text-2xl">
            {isPublicDemo
              ? 'Browse a safe portfolio workspace with fake traces, nested runs, failures, sessions, and compare flows. Run locally to inspect your own data.'
              : 'Open-source durable execution and debugger tooling for agent workflows. Run it locally, inspect traces, and debug failures from the operator console.'}
          </p>
          <div className="flex flex-col items-center gap-4 sm:flex-row">
            {isConsoleAvailable ? (
              <Link
                to="/dashboard"
                className="rounded-full bg-on-surface px-10 py-4 text-lg font-bold text-surface-container-lowest transition-all hover:opacity-90 active:scale-95"
              >
                {heroPrimaryLabel}
              </Link>
            ) : (
              <a
                href={RUN_LOCALLY_DOCS_URL}
                target="_blank"
                rel="noreferrer"
                className="rounded-full bg-on-surface px-10 py-4 text-lg font-bold text-surface-container-lowest transition-all hover:opacity-90 active:scale-95"
              >
                {heroPrimaryLabel}
              </a>
            )}
            <a
              href={heroSecondaryHref}
              target={isPublicDemo ? '_blank' : undefined}
              rel={isPublicDemo ? 'noreferrer' : undefined}
              className="flex items-center gap-2 rounded-full bg-transparent px-10 py-4 text-lg font-bold text-on-surface transition-all hover:bg-surface-container"
            >
              {heroSecondaryLabel}{' '}
              <span className="material-symbols-outlined">{heroSecondaryIcon}</span>
            </a>
          </div>
          {isPublicDemo ? (
            <p className="mt-6 max-w-2xl text-sm font-medium leading-7 text-on-surface-variant">
              The hosted debugger uses seeded sample traces only. Run Continua locally to inspect your own traces and sessions.
            </p>
          ) : null}
        </section>

        {/* ── Hero Illustration — animated SVG ── */}
        <HeroIllustration />

        {/* ── How It Works — Interactive Workflow ── */}
        <ScrollReveal>
          <section
            id="how-it-works"
            className="w-full scroll-mt-32 border-y border-outline-variant/10 bg-[#f9f9f9] px-6 py-24 sm:py-28 lg:py-36"
          >
            <div className="mx-auto max-w-7xl">
              <div className="text-center">
                <h2 className="tight-headline mb-7 text-[52px] font-black leading-[0.95] text-[#1b1b1b] md:text-[72px]">
                How it works
                </h2>
                <p className="mx-auto max-w-3xl text-xl leading-8 text-[#404753]">
                  Visualize the lifecycle of a durable agent execution from trigger
                  to completion.
                </p>
              </div>
              <div className="relative mt-24 flex flex-col items-center justify-center gap-12 md:flex-row md:gap-[76px]">
                <WorkflowStep
                  icon="bolt"
                  label="Trigger"
                  detail="HTTP, Webhook, Schedule"
                  bgClass="bg-[#e8f0fb]"
                  iconColor="text-primary-container"
                  isLast={false}
                />
                <WorkflowStep
                  icon="database"
                  label="Checkpoint"
                  detail="State saved to DB"
                  bgClass="border-2 border-dashed border-[#b9d7ad] bg-[#edf8e8]"
                  iconColor="text-tertiary-container"
                  isLast={false}
                />
                <WorkflowStep
                  icon="psychology"
                  label="Execution"
                  detail="Active Step"
                  bgClass="bg-primary-container shadow-[0_18px_34px_-16px_rgba(0,117,214,0.72)]"
                  iconColor="text-white"
                  isLast={false}
                  detailClass="text-primary-container font-bold"
                />
                <WorkflowStep
                  icon="history"
                  label="Retry"
                  detail="Auto-recovery on fail"
                  bgClass="bg-[#fff8e8]"
                  iconColor="text-secondary-container"
                  isLast={false}
                />
                <WorkflowStep
                  icon="check_circle"
                  label="Complete"
                  detail="Final result delivered"
                  bgClass="bg-[#1b1b1b]"
                  iconColor="text-white"
                  isLast
                />
              </div>
            </div>
          </section>
        </ScrollReveal>

        {/* ── Feature Cards Section ── */}
        <section className="mx-auto max-w-7xl px-6 py-32">
          <ScrollReveal>
            <div className="mb-20 grid items-center gap-16 md:grid-cols-2">
              <div>
                <div className="mb-6 inline-block rounded-sm bg-tertiary-container px-4 py-1.5 text-xs font-bold uppercase tracking-widest text-white">
                  Reviewing
                </div>
                <h2 className="tight-headline mb-6 text-4xl font-black md:text-6xl">
                  Reliable by default
                </h2>
                <p className="text-xl leading-relaxed text-on-surface-variant">
                  Every step in your workflow is automatically checkpointed. If
                  your agent crashes mid-execution, it resumes exactly where it
                  left off.
                </p>
              </div>
              {/* Custom SVG illustration of the checkpoint/resume story */}
              <CheckpointIllustration />
            </div>
          </ScrollReveal>

          {/* Concept grid — eight primitives Continua ships against */}
          <div className="mb-12 text-center">
            <p className="mb-3 text-[10px] font-black uppercase tracking-[0.2em] text-on-surface-variant/60">
              The primitives
            </p>
            <h3 className="tight-headline text-3xl font-black md:text-4xl">
              Eight things durable execution gives you
            </h3>
          </div>
          <div className="grid grid-cols-1 gap-6 md:grid-cols-2 xl:grid-cols-4">
            {CONCEPTS.map((concept, i) => (
              <ScrollReveal key={concept.src} delay={(i % 4) * 80}>
                <ConceptCard {...concept} />
              </ScrollReveal>
            ))}
          </div>
        </section>

        {/* ── Tabbed Code Examples ── */}
        <ScrollReveal>
          <section
            id="examples"
            className="mx-auto max-w-5xl scroll-mt-32 px-6 py-24"
          >
            <div className="mb-12 text-center">
              <h3 className="tight-headline mb-4 text-3xl font-black">
                Written in the language you love
              </h3>
              <p className="text-on-surface-variant">
                Python SDK helpers for traces, spans, sessions, and timeline events.
              </p>
            </div>
            <div className="overflow-hidden rounded-xl border border-outline-variant/10 bg-surface-container-lowest shadow-editorial">
              <div className="flex overflow-x-auto border-b border-outline-variant/10 whitespace-nowrap">
                {CODE_TABS.map((tab, i) => (
                  <button
                    key={tab.label}
                    type="button"
                    onClick={() => setActiveTab(i)}
                    className={`shrink-0 border-b-2 px-4 py-3 text-xs font-bold transition-all sm:px-6 sm:py-4 sm:text-sm ${
                      activeTab === i
                        ? 'border-primary-container text-primary-container'
                        : 'border-transparent text-on-surface-variant hover:text-on-surface'
                    }`}
                  >
                    {tab.label}
                  </button>
                ))}
              </div>
              <div className="p-8 md:p-12">{CODE_TABS[activeTab].content}</div>
            </div>
          </section>
        </ScrollReveal>

        {/* ── Integration Grid ── */}
        <ScrollReveal>
          <section className="mx-auto max-w-7xl px-6 py-24 text-center">
            <h3 className="mb-12 text-sm font-black uppercase tracking-widest text-on-surface-variant/50">
              Works with your existing tech stack
            </h3>
            <div className="grid grid-cols-2 gap-8 opacity-40 grayscale transition-all duration-500 hover:grayscale-0 md:grid-cols-6">
              <IntegrationItem icon="terminal" name="NEXT.JS" />
              <IntegrationItem icon="cloud_queue" name="VERCEL" />
              <IntegrationItem icon="database" name="SUPABASE" />
              <IntegrationItem icon="hub" name="GITHUB" />
              <IntegrationItem icon="javascript" name="NODE.JS" />
              <IntegrationItem icon="deployed_code" name="DOCKER" />
            </div>
          </section>
        </ScrollReveal>

        {/* ── Open Source Trust Section ── */}
        <ScrollReveal>
          <section
            id="open-source"
            className="scroll-mt-32 bg-zinc-900 py-24 text-white"
          >
            <div className="mx-auto flex max-w-7xl flex-col items-center justify-between gap-12 px-6 md:flex-row">
              <div className="max-w-xl">
                <div className="mb-6 inline-flex items-center gap-2 rounded-full bg-white/10 px-3 py-1">
                  <span className="text-[10px] font-bold uppercase tracking-widest text-primary-fixed">
                    Proudly Open Source
                  </span>
                </div>
                <h2 className="tight-headline mb-6 text-4xl font-black md:text-5xl">
                  Built in public, for the community.
                </h2>
                <p className="mb-8 text-lg text-zinc-400">
                  Continua is Apache 2.0 licensed. Self-host it, inspect the
                  code, and keep ownership of your infrastructure.
                </p>
                <div className="flex gap-4">
                  <a
                    href={GITHUB_REPO_URL}
                    target="_blank"
                    rel="noreferrer"
                    className="flex items-center gap-2 rounded-full bg-white px-8 py-3 font-bold text-zinc-900 transition-colors hover:bg-zinc-200"
                  >
                    <span className="material-symbols-outlined">terminal</span>{' '}
                    View GitHub
                  </a>
                  <a
                    href={GITHUB_LICENSE_URL}
                    target="_blank"
                    rel="noreferrer"
                    className="rounded-full border border-white/10 bg-white/5 px-8 py-3 font-bold transition-colors hover:bg-white/10"
                  >
                    Read License
                  </a>
                </div>
              </div>
              <div className="max-w-md rounded-xl border border-white/10 bg-white/5 p-8 text-left">
                <div className="text-xs font-black uppercase tracking-[0.2em] text-primary-fixed">
                  Repository
                </div>
                <p className="mt-4 text-3xl font-black tracking-tight text-white">
                  Source code, license, and local setup docs.
                </p>
                <p className="mt-4 text-sm leading-6 text-zinc-400">
                  Use the repository to review the implementation, run the
                  debugger locally, and follow the Apache 2.0 license terms.
                </p>
              </div>
            </div>
          </section>
        </ScrollReveal>

        {/* ── Final CTA ── */}
        <ScrollReveal>
          <section
            id="get-started"
            className="mx-auto max-w-7xl scroll-mt-32 px-6 py-32 text-center"
          >
            <div className="flex flex-col items-center rounded-lg bg-on-surface px-6 py-24 text-surface-container-lowest">
              <h2 className="tight-headline mb-10 max-w-4xl text-4xl font-black md:text-7xl">
                Ready to build the{' '}
                <span className="text-primary-fixed">immortal agent</span>?
              </h2>
              <div className="flex flex-col gap-6 sm:flex-row">
                {isConsoleAvailable ? (
                  <Link
                    to="/dashboard"
                    className="rounded-full bg-primary-container px-12 py-5 text-xl font-black text-white transition-all hover:bg-primary"
                  >
                    {isPublicDemo ? 'Open the public demo' : 'Open the operator console'}
                  </Link>
                ) : (
                  <a
                    href={RUN_LOCALLY_DOCS_URL}
                    target="_blank"
                    rel="noreferrer"
                    className="rounded-full bg-primary-container px-12 py-5 text-xl font-black text-white transition-all hover:bg-primary"
                  >
                    Run locally
                  </a>
                )}
                <a
                  href={isPublicDemo ? RUN_LOCALLY_DOCS_URL : GITHUB_DOCS_URL}
                  target="_blank"
                  rel="noreferrer"
                  className="rounded-full border border-white/20 bg-white/10 px-12 py-5 text-xl font-black text-white transition-all hover:bg-white/20"
                >
                  {isPublicDemo ? 'Run locally' : 'Read the docs'}
                </a>
              </div>
              <p className="mt-8 text-sm font-medium text-surface-container opacity-60">
                {isPublicDemo
                  ? 'Hosted demo uses safe sample data. Local self-hosting is the path for real traces.'
                  : isConsoleAvailable
                    ? 'Open the console or read the local setup guide.'
                    : 'The hosted Pages site is static. Run locally to open the console with your own data.'}
              </p>
            </div>
          </section>
        </ScrollReveal>
      </main>

      {/* ── Footer ── */}
      <footer className="w-full border-t border-outline-variant/10 bg-zinc-50 px-6 py-12">
        <div className="mx-auto flex max-w-7xl flex-col items-center justify-between gap-8 md:flex-row">
          <div className="flex flex-col gap-4">
            <div className="text-lg font-black uppercase tracking-tighter text-zinc-900">
              Continua
            </div>
            <p className="max-w-xs text-sm leading-relaxed text-zinc-500">
              &copy; 2026 Continua durable execution tooling. Built for
              technical precision.
            </p>
          </div>
          <div className="flex flex-wrap justify-center gap-8">
            <a
              className="text-sm font-semibold uppercase tracking-wide text-zinc-500 transition-colors hover:text-zinc-900"
              href={GITHUB_DOCS_URL}
              target="_blank"
              rel="noreferrer"
            >
              Documentation
            </a>
            <a
              className="text-sm font-semibold uppercase tracking-wide text-zinc-500 transition-colors hover:text-zinc-900"
              href={GITHUB_OPENAPI_URL}
              target="_blank"
              rel="noreferrer"
            >
              API Reference
            </a>
            <a
              className="text-sm font-semibold uppercase tracking-wide text-zinc-500 transition-colors hover:text-zinc-900"
              href={GITHUB_LICENSE_URL}
              target="_blank"
              rel="noreferrer"
            >
              License
            </a>
            {isConsoleAvailable ? (
              <Link
                className="text-sm font-semibold uppercase tracking-wide text-zinc-500 transition-colors hover:text-zinc-900"
                to="/dashboard"
              >
                {footerConsoleLabel}
              </Link>
            ) : (
              <a
                className="text-sm font-semibold uppercase tracking-wide text-zinc-500 transition-colors hover:text-zinc-900"
                href={RUN_LOCALLY_DOCS_URL}
                target="_blank"
                rel="noreferrer"
              >
                {footerConsoleLabel}
              </a>
            )}
            <a
              className="text-sm font-semibold uppercase tracking-wide text-zinc-500 transition-colors hover:text-zinc-900"
              href={RUN_LOCALLY_DOCS_URL}
              target="_blank"
              rel="noreferrer"
            >
              Run locally
            </a>
          </div>
          <div className="flex gap-4">
            <a
              className="flex h-10 w-10 items-center justify-center rounded-full bg-surface-container transition-colors hover:bg-surface-container-high"
              href={GITHUB_REPO_URL}
              target="_blank"
              rel="noreferrer"
              aria-label="Open Continua on GitHub"
            >
              <span className="material-symbols-outlined text-zinc-900">
                terminal
              </span>
            </a>
            {isConsoleAvailable ? (
              <Link
                className="flex h-10 w-10 items-center justify-center rounded-full bg-surface-container transition-colors hover:bg-surface-container-high"
                to="/dashboard"
                aria-label="Open the Continua operator console"
              >
                <span className="material-symbols-outlined text-zinc-900">
                  hub
                </span>
              </Link>
            ) : (
              <a
                className="flex h-10 w-10 items-center justify-center rounded-full bg-surface-container transition-colors hover:bg-surface-container-high"
                href={RUN_LOCALLY_DOCS_URL}
                target="_blank"
                rel="noreferrer"
                aria-label="Open the local setup guide"
              >
                <span className="material-symbols-outlined text-zinc-900">
                  hub
                </span>
              </a>
            )}
          </div>
        </div>
      </footer>
    </div>
  );
}

/* ── Sub-components ──────────────────────────────────────────────── */

function WorkflowStep({
  icon,
  label,
  detail,
  bgClass,
  iconColor,
  isLast,
  detailClass,
}: {
  icon: string;
  label: string;
  detail: string;
  bgClass: string;
  iconColor: string;
  isLast: boolean;
  detailClass?: string;
}) {
  return (
    <div
      className={`relative flex min-w-[170px] flex-col items-center text-center ${!isLast ? 'marketing-workflow-step' : ''}`}
    >
      <div
        className={`mb-7 flex h-24 w-24 items-center justify-center rounded-full ${bgClass}`}
      >
        <span className={`material-symbols-outlined text-[42px] leading-none ${iconColor}`}>
          {icon}
        </span>
      </div>
      <span className="text-[22px] font-black leading-7 text-[#1b1b1b]">
        {label}
      </span>
      <span
        className={`mt-1 whitespace-nowrap text-lg leading-6 ${detailClass ?? 'text-[#404753]'}`}
      >
        {detail}
      </span>
    </div>
  );
}

interface Concept {
  src: string;
  alt: string;
  title: string;
  description: string;
}

const CONCEPTS: Concept[] = [
  {
    src: '/illustrations/durable-execution.svg',
    alt: 'Durable execution',
    title: 'Durable execution',
    description:
      'Workflows run through crashes. State is preserved and execution resumes from the last checkpoint.',
  },
  {
    src: '/illustrations/retry-recovery.svg',
    alt: 'Retry and recovery',
    title: 'Retry & recovery',
    description:
      'Failed attempts surface in the timeline. Successful retries close the loop without duplicate work.',
  },
  {
    src: '/illustrations/orchestration.svg',
    alt: 'Workflow orchestration',
    title: 'Workflow orchestration',
    description:
      'Trigger, branch, join, done. Compose durable DAGs from steps you already know how to write.',
  },
  {
    src: '/illustrations/scheduling.svg',
    alt: 'Scheduling',
    title: 'Scheduling',
    description:
      'Cron-based runs with queued ticks. Fires on time, every time, even across restarts.',
  },
  {
    src: '/illustrations/observability.svg',
    alt: 'Observability',
    title: 'Observability',
    description:
      'Every span on a waterfall, every payload inspectable. Move from signal to root cause quickly.',
  },
  {
    src: '/illustrations/checkpointing.svg',
    alt: 'State checkpointing',
    title: 'State checkpointing',
    description:
      'An append-only journal, committed layer by layer. Replay-safe and hash-verified.',
  },
  {
    src: '/illustrations/background-jobs.svg',
    alt: 'Resilient background jobs',
    title: 'Resilient background jobs',
    description:
      'Dispatcher fans out to a pool of workers. Long-running work survives the network.',
  },
  {
    src: '/illustrations/execution-timeline.svg',
    alt: 'Execution timeline',
    title: 'Execution timeline',
    description:
      'Segmented progress with completed, running, and scheduled steps. A NOW marker anchors the frame.',
  },
];

function ConceptCard({ src, alt, title, description }: Concept) {
  return (
    <div className="flex h-full flex-col overflow-hidden rounded-xl border border-outline-variant/20 bg-surface-container-lowest transition-colors hover:border-primary-container/30">
      <div className="aspect-[4/3] border-b border-outline-variant/10 bg-surface-container">
        <img src={src} alt={alt} className="block h-full w-full" />
      </div>
      <div className="flex flex-1 flex-col gap-2 p-6">
        <h4 className="text-lg font-black tracking-tight">{title}</h4>
        <p className="text-sm leading-relaxed text-on-surface-variant">
          {description}
        </p>
      </div>
    </div>
  );
}

function IntegrationItem({ icon, name }: { icon: string; name: string }) {
  return (
    <div className="flex flex-col items-center gap-2">
      <span className="material-symbols-outlined text-4xl">{icon}</span>
      <span className="font-mono text-xs font-bold">{name}</span>
    </div>
  );
}
