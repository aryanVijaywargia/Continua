import { useState } from 'react';
import { Link } from 'react-router-dom';
import { CheckpointIllustration } from '../components/landing/CheckpointIllustration';
import { AnimatedCounter } from '../components/landing/AnimatedCounter';
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
        <span className="text-primary-container">export const</span>{' '}
        <span className="text-tertiary">agentProcess</span> ={' '}
        <span className="text-primary-container">workflow</span>(
        <span className="text-secondary">async</span> {'(input) => {'}
        {'\n'}
        {'  '}
        <span className="text-on-surface-variant">
          {'// Checkpointed automatically'}
        </span>
        {'\n'}
        {'  '}
        <span className="text-primary-container">const</span> research ={' '}
        <span className="text-secondary">await</span> step(
        <span className="text-outline">&apos;research&apos;</span>,{' '}
        {'() => fetchAPI(input.query)'});{'\n'}
        {'\n'}
        {'  '}
        <span className="text-primary-container">const</span> summary ={' '}
        <span className="text-secondary">await</span> step(
        <span className="text-outline">&apos;summarize&apos;</span>, {'{'}
        {'\n'}
        {'    '}
        <span className="text-primary-container">retries</span>:{' '}
        <span className="text-primary-container">5</span>
        {'\n'}
        {'  }'}, {'() => llm.generate(research)'});{'\n'}
        {'\n'}
        {'  '}
        <span className="text-secondary">return</span> summary;{'\n'}
        {'}'});
      </pre>
    ),
  },
  {
    label: 'With Retries',
    content: (
      <pre className="font-mono text-sm leading-relaxed text-on-surface md:text-lg">
        <span className="text-primary-container">export const</span>{' '}
        <span className="text-tertiary">resilientAgent</span> ={' '}
        <span className="text-primary-container">workflow</span>(
        <span className="text-secondary">async</span> {'(input) => {'}
        {'\n'}
        {'  '}
        <span className="text-primary-container">const</span> data ={' '}
        <span className="text-secondary">await</span> step(
        <span className="text-outline">&apos;fetch-data&apos;</span>, {'{'}
        {'\n'}
        {'    '}
        <span className="text-primary-container">retries</span>:{' '}
        <span className="text-primary-container">3</span>,{'\n'}
        {'    '}
        <span className="text-primary-container">backoff</span>:{' '}
        <span className="text-outline">&apos;exponential&apos;</span>,{'\n'}
        {'    '}
        <span className="text-primary-container">timeout</span>:{' '}
        <span className="text-outline">&apos;30s&apos;</span>,{'\n'}
        {'  }'}, {'() => externalApi.fetch(input.id)'});{'\n'}
        {'\n'}
        {'  '}
        <span className="text-primary-container">const</span> result ={' '}
        <span className="text-secondary">await</span> step(
        <span className="text-outline">&apos;process&apos;</span>,{' '}
        {'() => transform(data)'});{'\n'}
        {'  '}
        <span className="text-secondary">await</span> step(
        <span className="text-outline">&apos;notify&apos;</span>,{' '}
        {'() => slack.send(result)'});{'\n'}
        {'\n'}
        {'  '}
        <span className="text-secondary">return</span> result;{'\n'}
        {'}'});
      </pre>
    ),
  },
  {
    label: 'Complex Logic',
    content: (
      <pre className="font-mono text-sm leading-relaxed text-on-surface md:text-lg">
        <span className="text-primary-container">export const</span>{' '}
        <span className="text-tertiary">orchestrator</span> ={' '}
        <span className="text-primary-container">workflow</span>(
        <span className="text-secondary">async</span> {'(input) => {'}
        {'\n'}
        {'  '}
        <span className="text-primary-container">const</span> tasks ={' '}
        <span className="text-secondary">await</span> step(
        <span className="text-outline">&apos;plan&apos;</span>,{' '}
        {'() =>'} {'\n'}
        {'    '}llm.plan(input.objective){'\n'}
        {'  )'});{'\n'}
        {'\n'}
        {'  '}
        <span className="text-primary-container">const</span> results ={' '}
        <span className="text-secondary">await</span>{' '}
        <span className="text-primary-container">parallel</span>({'\n'}
        {'    '}tasks.map((task, i) ={'>'}{'\n'}
        {'      '}step(
        <span className="text-outline">{`\`execute-\${i}\``}</span>,{' '}
        {'{ '}
        <span className="text-primary-container">retries</span>:{' '}
        <span className="text-primary-container">2</span>
        {' }'}, {'() =>'} {'\n'}
        {'        '}agent.run(task){'\n'}
        {'      )'}){'\n'}
        {'    )'}){'\n'}
        {'  )'});{'\n'}
        {'\n'}
        {'  '}
        <span className="text-secondary">return</span>{' '}
        <span className="text-secondary">await</span> step(
        <span className="text-outline">&apos;synthesize&apos;</span>,{' '}
        {'() =>'}{'\n'}
        {'    '}llm.merge(results){'\n'}
        {'  )'});{'\n'}
        {'}'});
      </pre>
    ),
  },
];

const GITHUB_REPO_URL = 'https://github.com/aryanVijaywargia/Continua';
const GITHUB_DOCS_URL = `${GITHUB_REPO_URL}/tree/main/docs`;
const GITHUB_LICENSE_URL = `${GITHUB_REPO_URL}/blob/main/LICENSE`;
const GITHUB_OPENAPI_URL = `${GITHUB_REPO_URL}/blob/main/contracts/openapi/openapi.yaml`;
const RUN_LOCALLY_DOCS_URL = `${GITHUB_REPO_URL}/blob/main/docs/guides/run-locally.md`;

/* ── Page component ──────────────────────────────────────────────── */

export function LandingPage() {
  const [activeTab, setActiveTab] = useState(0);
  const isDesktopNav = useMediaQuery('(min-width: 768px)');
  const runtimeAuth = useRuntimeAuth();
  const isPublicDemo = runtimeAuth.public_demo_enabled === true;
  const consoleCtaLabel = isPublicDemo ? 'Open Demo' : 'Open Console';
  const topSecondaryLabel = isPublicDemo ? 'Run locally' : 'Docs';
  const topSecondaryHref = isPublicDemo ? RUN_LOCALLY_DOCS_URL : GITHUB_DOCS_URL;
  const heroPrimaryLabel = isPublicDemo ? 'Open Demo' : 'Start Building Free';
  const heroSecondaryLabel = isPublicDemo ? 'Run Locally' : 'See how it works';
  const heroSecondaryHref = isPublicDemo ? RUN_LOCALLY_DOCS_URL : '#how-it-works';
  const heroSecondaryIcon = isPublicDemo ? 'open_in_new' : 'play_circle';
  const footerConsoleLabel = isPublicDemo ? 'Open Demo' : 'Operator Console';

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
              <Link
                to="/dashboard"
                className="scale-95 rounded-full bg-zinc-900 px-5 py-2 text-sm font-bold text-white transition-all duration-200 hover:bg-zinc-800 active:scale-90"
              >
                {consoleCtaLabel}
              </Link>
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
              {isPublicDemo ? 'Read-only sample data' : 'Production Ready'}
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
                <span className="block sm:inline">Build and deploy</span>
                <span className="mt-3 inline-block bg-primary-container px-4 py-1 text-white sm:mt-0 sm:ml-3 sm:-rotate-1">
                  fully-managed
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
              : 'The durable execution engine designed for the next generation of autonomous workflows. Immortal code that survives restarts and network failures.'}
          </p>
          <div className="flex flex-col items-center gap-4 sm:flex-row">
            <Link
              to="/dashboard"
              className="rounded-full bg-on-surface px-10 py-4 text-lg font-bold text-surface-container-lowest transition-all hover:opacity-90 active:scale-95"
            >
              {heroPrimaryLabel}
            </Link>
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

        {/* ── Proof Points (Logo Cloud) ── */}
        <ScrollReveal>
          <section className="mb-20 mt-8 overflow-hidden bg-surface-container-low py-12">
            <div className="mx-auto max-w-7xl px-6">
              <p className="mb-10 text-center text-[10px] font-black uppercase tracking-[0.2em] text-on-surface-variant/40">
                Trusted by Infrastructure Pioneers
              </p>
              <div className="flex flex-wrap justify-center gap-12 opacity-60 saturate-0 transition-all hover:saturate-100 md:gap-24">
                <span className="text-2xl font-black tracking-tighter">
                  NEXUS
                </span>
                <span className="text-2xl font-black tracking-tighter">
                  QUANTUM
                </span>
                <span className="text-2xl font-black tracking-tighter">
                  VORTEX
                </span>
                <span className="text-2xl font-black tracking-tighter">
                  BYTEBOUND
                </span>
                <span className="text-2xl font-black tracking-tighter">
                  SYNTH
                </span>
              </div>
            </div>
          </section>
        </ScrollReveal>

        {/* ── How It Works — Interactive Workflow ── */}
        <ScrollReveal>
          <section
            id="how-it-works"
            className="mx-auto max-w-7xl scroll-mt-32 border-y border-outline-variant/10 px-6 py-24"
          >
            <div className="mb-16 text-center">
              <h2 className="tight-headline mb-4 text-3xl font-black md:text-5xl">
                How it works
              </h2>
              <p className="mx-auto max-w-2xl text-on-surface-variant">
                Visualize the lifecycle of a durable agent execution from trigger
                to completion.
              </p>
            </div>
            <div className="relative flex flex-col items-center justify-center gap-8 md:flex-row md:gap-16">
              <WorkflowStep
                icon="bolt"
                label="Trigger"
                detail="HTTP, Webhook, Schedule"
                bgClass="bg-primary-container/10"
                iconColor="text-primary-container"
                isLast={false}
              />
              <WorkflowStep
                icon="database"
                label="Checkpoint"
                detail="State saved to DB"
                bgClass="bg-tertiary-container/10 border-2 border-dashed border-tertiary-container/30"
                iconColor="text-tertiary-container"
                isLast={false}
              />
              <WorkflowStep
                icon="psychology"
                label="Execution"
                detail="Active Step"
                bgClass="bg-primary-container shadow-lg shadow-primary-container/30"
                iconColor="text-white"
                isLast={false}
                detailClass="text-primary-container font-bold"
              />
              <WorkflowStep
                icon="history"
                label="Retry"
                detail="Auto-recovery on fail"
                bgClass="bg-secondary-container/10"
                iconColor="text-secondary-container"
                isLast={false}
              />
              <div className="relative flex flex-col items-center">
                <div className="mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-on-surface">
                  <span className="material-symbols-outlined text-3xl text-white">
                    check_circle
                  </span>
                </div>
                <span className="text-sm font-bold">Complete</span>
                <span className="text-xs text-on-surface-variant">
                  Final result delivered
                </span>
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
                Simple TypeScript SDK for complex agent orchestration.
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
                  Continua is Apache 2.0 licensed. Self-host it or use our
                  managed cloud. You always own your infrastructure and your
                  code.
                </p>
                <div className="flex gap-4">
                  <a
                    href={GITHUB_REPO_URL}
                    target="_blank"
                    rel="noreferrer"
                    className="flex items-center gap-2 rounded-full bg-white px-8 py-3 font-bold text-zinc-900 transition-colors hover:bg-zinc-200"
                  >
                    <span className="material-symbols-outlined">star</span>{' '}
                    Star on GitHub
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
              <div className="grid grid-cols-2 gap-8 text-center md:text-left">
                <StatBlock
                  value={
                    <AnimatedCounter end={12.4} suffix="k+" decimals={1} />
                  }
                  label="Stars on GitHub"
                />
                <StatBlock
                  value={<AnimatedCounter end={250} suffix="+" />}
                  label="Contributors"
                />
                <StatBlock value="Apache" label="2.0 License" />
                <StatBlock
                  value={
                    <AnimatedCounter end={4.8} suffix="k+" decimals={1} />
                  }
                  label="Discord Members"
                />
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
                <Link
                  to="/dashboard"
                  className="rounded-full bg-primary-container px-12 py-5 text-xl font-black text-white transition-all hover:bg-primary"
                >
                  {isPublicDemo ? 'Open the public demo' : 'Open the operator console'}
                </Link>
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
                  : 'Join 2,400+ developers building with durable execution.'}
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
            <Link
              className="text-sm font-semibold uppercase tracking-wide text-zinc-500 transition-colors hover:text-zinc-900"
              to="/dashboard"
            >
              {footerConsoleLabel}
            </Link>
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
            <Link
              className="flex h-10 w-10 items-center justify-center rounded-full bg-surface-container transition-colors hover:bg-surface-container-high"
              to="/dashboard"
              aria-label="Open the Continua operator console"
            >
              <span className="material-symbols-outlined text-zinc-900">
                hub
              </span>
            </Link>
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
      className={`relative flex flex-col items-center ${!isLast ? 'workflow-step' : ''}`}
    >
      <div
        className={`mb-4 flex h-16 w-16 items-center justify-center rounded-full ${bgClass}`}
      >
        <span className={`material-symbols-outlined text-3xl ${iconColor}`}>
          {icon}
        </span>
      </div>
      <span className="text-sm font-bold">{label}</span>
      <span className={`text-xs ${detailClass ?? 'text-on-surface-variant'}`}>
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

function StatBlock({
  value,
  label,
}: {
  value: string | JSX.Element;
  label: string;
}) {
  return (
    <div>
      <div className="mb-2 text-5xl font-black text-primary-fixed">{value}</div>
      <p className="text-xs font-bold uppercase tracking-widest text-zinc-500">
        {label}
      </p>
    </div>
  );
}
