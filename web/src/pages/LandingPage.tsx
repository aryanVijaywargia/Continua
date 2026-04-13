import { Link } from 'react-router-dom';

export function LandingPage() {
  return (
    <div
      className="min-h-screen selection:bg-primary-container selection:text-white"
      style={{ fontFamily: "'Inter', sans-serif", backgroundColor: '#f9f9f9', color: '#1b1b1b' }}
    >
      {/* Announcement Banner */}
      <div className="relative z-[60] flex items-center justify-center gap-3 overflow-hidden bg-secondary-container px-6 py-2.5 text-on-secondary-container">
        <span className="text-xs font-bold uppercase tracking-widest font-label">V2.0 Launched</span>
        <span className="text-sm font-semibold tracking-tight">Agentic Engine secures $12.2M Series A to redefine durable computing.</span>
        <span className="material-symbols-outlined text-base">arrow_right_alt</span>
      </div>

      {/* Top Navigation */}
      <nav className="fixed top-0 z-50 w-full bg-white/80 backdrop-blur-xl">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-6">
          <div className="text-xl font-black tracking-tighter text-zinc-900">
            Agentic Engine
          </div>
          <div className="hidden items-center gap-10 md:flex">
            <a className="border-b-2 border-zinc-900 pb-1 font-bold tracking-tight text-zinc-900" href="#">Product</a>
            <a className="tracking-tight text-zinc-500 transition-colors hover:text-zinc-900" href="#">Solutions</a>
            <a className="tracking-tight text-zinc-500 transition-colors hover:text-zinc-900" href="#">Pricing</a>
          </div>
          <div className="flex items-center gap-4">
            <Link
              to="/dashboard"
              className="scale-95 rounded-full bg-zinc-900 px-5 py-2 text-sm font-bold text-white transition-all duration-200 hover:bg-zinc-800 active:scale-90"
            >
              Get Started
            </Link>
          </div>
        </div>
      </nav>

      <main className="pt-24">
        {/* Hero Section */}
        <section className="mx-auto flex max-w-7xl flex-col items-center px-6 py-20 text-center md:py-32">
          <div className="mb-8 inline-flex items-center gap-2 rounded-full bg-surface-container px-3 py-1">
            <span className="h-2 w-2 rounded-full bg-primary-container"></span>
            <span className="text-[10px] font-bold uppercase tracking-widest text-on-surface-variant">Production Ready</span>
          </div>
          <h1 className="tight-headline mb-8 max-w-5xl text-5xl font-black text-on-surface md:text-8xl">
            Build and deploy <span className="inline-block -rotate-1 bg-primary-container px-4 py-1 text-white">fully-managed</span> AI agents.
          </h1>
          <p className="mb-12 max-w-3xl text-lg font-medium leading-relaxed text-on-surface-variant md:text-2xl">
            The durable execution engine designed for the next generation of autonomous workflows. Immortal code that survives restarts and network failures.
          </p>
          <div className="flex flex-col items-center gap-4 sm:flex-row">
            <Link
              to="/dashboard"
              className="rounded-full bg-on-surface px-10 py-4 text-lg font-bold text-surface-container-lowest transition-all hover:opacity-90 active:scale-95"
            >
              Start Building Free
            </Link>
            <button className="flex items-center gap-2 rounded-full bg-transparent px-10 py-4 text-lg font-bold text-on-surface transition-all hover:bg-surface-container">
              View Demo <span className="material-symbols-outlined">play_circle</span>
            </button>
          </div>
        </section>

        {/* Proof Points (Logo Cloud) */}
        <section className="mb-20 overflow-hidden bg-surface-container-low py-12">
          <div className="mx-auto max-w-7xl px-6">
            <p className="mb-10 text-center text-[10px] font-black uppercase tracking-[0.2em] text-on-surface-variant/40">Trusted by Infrastructure Pioneers</p>
            <div className="flex flex-wrap justify-center gap-12 opacity-60 saturate-0 transition-all hover:saturate-100 md:gap-24">
              <span className="text-2xl font-black tracking-tighter">NEXUS</span>
              <span className="text-2xl font-black tracking-tighter">QUANTUM</span>
              <span className="text-2xl font-black tracking-tighter">VORTEX</span>
              <span className="text-2xl font-black tracking-tighter">BYTEBOUND</span>
              <span className="text-2xl font-black tracking-tighter">SYNTH</span>
            </div>
          </div>
        </section>

        {/* Interactive Workflow Visualization */}
        <section className="mx-auto max-w-7xl border-y border-outline-variant/10 px-6 py-24">
          <div className="mb-16 text-center">
            <h2 className="tight-headline mb-4 text-3xl font-black md:text-5xl">How it works</h2>
            <p className="mx-auto max-w-2xl text-on-surface-variant">Visualize the lifecycle of a durable agent execution from trigger to completion.</p>
          </div>
          <div className="relative flex flex-col items-center justify-center gap-8 md:flex-row md:gap-16">
            <WorkflowStep icon="bolt" label="Trigger" detail="HTTP, Webhook, Schedule" bgClass="bg-primary-container/10" iconColor="text-primary-container" isLast={false} />
            <WorkflowStep icon="database" label="Checkpoint" detail="State saved to DB" bgClass="bg-tertiary-container/10 border-2 border-dashed border-tertiary-container/30" iconColor="text-tertiary-container" isLast={false} />
            <WorkflowStep icon="psychology" label="Execution" detail="Active Step" bgClass="bg-primary-container shadow-lg shadow-primary-container/30" iconColor="text-white" isLast={false} detailClass="text-primary-container font-bold" />
            <WorkflowStep icon="history" label="Retry" detail="Auto-recovery on fail" bgClass="bg-secondary-container/10" iconColor="text-secondary-container" isLast={false} />
            <div className="relative flex flex-col items-center">
              <div className="mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-on-surface">
                <span className="material-symbols-outlined text-3xl text-white">check_circle</span>
              </div>
              <span className="text-sm font-bold">Complete</span>
              <span className="text-xs text-on-surface-variant">Final result delivered</span>
            </div>
          </div>
        </section>

        {/* Detailed Feature Cards */}
        <section className="mx-auto max-w-7xl px-6 py-32">
          <div className="mb-20 grid items-center gap-16 md:grid-cols-2">
            <div>
              <div className="mb-6 inline-block rounded-sm bg-tertiary-container px-4 py-1.5 text-xs font-bold uppercase tracking-widest text-white">Reviewing</div>
              <h2 className="tight-headline mb-6 text-4xl font-black md:text-6xl">Reliable by default</h2>
              <p className="text-xl leading-relaxed text-on-surface-variant">
                Every step in your workflow is automatically checkpointed. If your agent crashes mid-execution, it resumes exactly where it left off.
              </p>
            </div>
            <div className="relative aspect-[4/3] overflow-hidden rounded-xl bg-surface-container-high p-8">
              <img
                className="h-full w-full rounded-lg object-cover shadow-editorial"
                src="https://lh3.googleusercontent.com/aida-public/AB6AXuCGDlMl4ImeMzxVTOHQM8za8mQx40raXWeTsIXftv4SryUEbvJ3gKtENVOayK8lYBFk5G9YS0efkutRIVAL41T0KevM3RJiTbEt8v-9Q76t4t5b_s719TWgtcfxd7v6iBts0Re1DPiiKkKske-zpo1NXQh1X1RM2gRl3O2U43SwnzxPppM0ar7ubH3yHWOrX8KXGay8VZDMZXfv-YsAa-7fpJhojDVAgf5JqffxrnQPe7EFw-i1BArOuCYv18C7201loFbG92cJShg"
                alt="Abstract UI dashboard showing complex workflow steps"
              />
            </div>
          </div>

          {/* Multi-column technical grid */}
          <div className="grid grid-cols-1 gap-8 md:grid-cols-3">
            <FeatureCard
              icon="settings_backup_restore"
              title="Automated Checkpointing"
              description="Zero-config persistence for local variables and execution state across server restarts."
            />
            <FeatureCard
              icon="sync_problem"
              title="Deadlock Detection"
              description="Built-in mechanisms to identify and resolve hanging tasks before they impact throughput."
            />
            <FeatureCard
              icon="account_tree"
              title="Idempotency Keys"
              description="Built-in support for ensuring third-party API calls never execute more than once."
            />
          </div>
        </section>

        {/* Tabbed Code Examples */}
        <section className="mx-auto max-w-5xl px-6 py-24">
          <div className="mb-12 text-center">
            <h3 className="tight-headline mb-4 text-3xl font-black">Written in the language you love</h3>
            <p className="text-on-surface-variant">Simple TypeScript SDK for complex agent orchestration.</p>
          </div>
          <div className="overflow-hidden rounded-xl border border-outline-variant/10 bg-surface-container-lowest shadow-editorial">
            <div className="flex border-b border-outline-variant/10">
              <button className="border-b-2 border-primary-container px-6 py-4 text-sm font-bold text-primary-container">Basic Agent</button>
              <button className="border-b-2 border-transparent px-6 py-4 text-sm font-bold text-on-surface-variant transition-all hover:text-on-surface">With Retries</button>
              <button className="border-b-2 border-transparent px-6 py-4 text-sm font-bold text-on-surface-variant transition-all hover:text-on-surface">Complex Logic</button>
            </div>
            <div className="p-8 md:p-12">
              <pre className="font-mono text-sm leading-relaxed text-on-surface md:text-lg">
{`\u001b[0m`}<span className="text-primary-container">export const</span> <span className="text-tertiary">agentProcess</span> = <span className="text-primary-container">workflow</span>(<span className="text-secondary">async</span> {'(input) => {'}{'\n'}
{'  '}<span className="text-on-surface-variant">{'// Checkpointed automatically'}</span>{'\n'}
{'  '}<span className="text-primary-container">const</span> research = <span className="text-secondary">await</span> step(<span className="text-outline">'research'</span>, {'() => fetchAPI(input.query)'});{'\n'}
{'\n'}
{'  '}<span className="text-primary-container">const</span> summary = <span className="text-secondary">await</span> step(<span className="text-outline">'summarize'</span>, {'{'}{'\n'}
{'    '}<span className="text-primary-container">retries</span>: <span className="text-primary-container">5</span>{'\n'}
{'  }'}, {'() => llm.generate(research)'});{'\n'}
{'\n'}
{'  '}<span className="text-secondary">return</span> summary;{'\n'}
{'}'});
              </pre>
            </div>
          </div>
        </section>

        {/* Integration Grid */}
        <section className="mx-auto max-w-7xl px-6 py-24 text-center">
          <h3 className="mb-12 text-sm font-black uppercase tracking-widest text-on-surface-variant/50">Works with your existing tech stack</h3>
          <div className="grid grid-cols-2 gap-8 opacity-40 grayscale transition-all duration-500 hover:grayscale-0 md:grid-cols-6">
            <IntegrationItem icon="terminal" name="NEXT.JS" />
            <IntegrationItem icon="cloud_queue" name="VERCEL" />
            <IntegrationItem icon="database" name="SUPABASE" />
            <IntegrationItem icon="hub" name="GITHUB" />
            <IntegrationItem icon="javascript" name="NODE.JS" />
            <IntegrationItem icon="deployed_code" name="DOCKER" />
          </div>
        </section>

        {/* Open Source Trust Section */}
        <section className="bg-zinc-900 py-24 text-white">
          <div className="mx-auto flex max-w-7xl flex-col items-center justify-between gap-12 px-6 md:flex-row">
            <div className="max-w-xl">
              <div className="mb-6 inline-flex items-center gap-2 rounded-full bg-white/10 px-3 py-1">
                <span className="text-[10px] font-bold uppercase tracking-widest text-primary-fixed">Proudly Open Source</span>
              </div>
              <h2 className="tight-headline mb-6 text-4xl font-black md:text-5xl">Built in public, for the community.</h2>
              <p className="mb-8 text-lg text-zinc-400">Agentic Engine is Apache 2.0 licensed. Self-host it or use our managed cloud. You always own your infrastructure and your code.</p>
              <div className="flex gap-4">
                <button className="flex items-center gap-2 rounded-full bg-white px-8 py-3 font-bold text-zinc-900 transition-colors hover:bg-zinc-200">
                  <span className="material-symbols-outlined">star</span> Star on GitHub
                </button>
                <button className="rounded-full border border-white/10 bg-white/5 px-8 py-3 font-bold transition-colors hover:bg-white/10">
                  Read License
                </button>
              </div>
            </div>
            <div className="grid grid-cols-2 gap-8 text-center md:text-left">
              <StatBlock value="12.4k+" label="Stars on GitHub" />
              <StatBlock value="250+" label="Contributors" />
              <StatBlock value="Apache" label="2.0 License" />
              <StatBlock value="4.8k+" label="Discord Members" />
            </div>
          </div>
        </section>

        {/* Final CTA */}
        <section className="mx-auto max-w-7xl px-6 py-32 text-center">
          <div className="flex flex-col items-center rounded-lg bg-on-surface px-6 py-24 text-surface-container-lowest">
            <h2 className="tight-headline mb-10 max-w-4xl text-4xl font-black md:text-7xl">
              Ready to build the <span className="text-primary-fixed">immortal agent</span>?
            </h2>
            <div className="flex flex-col gap-6 sm:flex-row">
              <Link
                to="/dashboard"
                className="rounded-full bg-primary-container px-12 py-5 text-xl font-black text-white transition-all hover:bg-primary"
              >
                Deploy Your First Agent
              </Link>
              <button className="rounded-full border border-white/20 bg-white/10 px-12 py-5 text-xl font-black text-white transition-all hover:bg-white/20">
                Contact Sales
              </button>
            </div>
            <p className="mt-8 text-sm font-medium text-surface-container opacity-60">Join 2,400+ developers building with durable execution.</p>
          </div>
        </section>
      </main>

      {/* Footer */}
      <footer className="w-full border-t border-outline-variant/10 bg-zinc-50 px-6 py-12">
        <div className="mx-auto flex max-w-7xl flex-col items-center justify-between gap-8 md:flex-row">
          <div className="flex flex-col gap-4">
            <div className="text-lg font-black uppercase tracking-tighter text-zinc-900">
              Agentic Engine
            </div>
            <p className="max-w-xs text-sm leading-relaxed text-zinc-500">
              &copy; 2024 Agentic Durable Execution Engine. Built for technical precision.
            </p>
          </div>
          <div className="flex flex-wrap justify-center gap-8">
            <a className="text-sm font-semibold uppercase tracking-wide text-zinc-500 transition-colors hover:text-zinc-900" href="#">Documentation</a>
            <a className="text-sm font-semibold uppercase tracking-wide text-zinc-500 transition-colors hover:text-zinc-900" href="#">API Reference</a>
            <a className="text-sm font-semibold uppercase tracking-wide text-zinc-500 transition-colors hover:text-zinc-900" href="#">Status</a>
            <a className="text-sm font-semibold uppercase tracking-wide text-zinc-500 transition-colors hover:text-zinc-900" href="#">Privacy</a>
            <a className="text-sm font-semibold uppercase tracking-wide text-zinc-500 transition-colors hover:text-zinc-900" href="#">Terms</a>
          </div>
          <div className="flex gap-4">
            <a className="flex h-10 w-10 items-center justify-center rounded-full bg-surface-container transition-colors hover:bg-surface-container-high" href="#">
              <span className="material-symbols-outlined text-zinc-900">terminal</span>
            </a>
            <a className="flex h-10 w-10 items-center justify-center rounded-full bg-surface-container transition-colors hover:bg-surface-container-high" href="#">
              <span className="material-symbols-outlined text-zinc-900">hub</span>
            </a>
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
    <div className={`relative flex flex-col items-center ${!isLast ? 'workflow-step' : ''}`}>
      <div className={`mb-4 flex h-16 w-16 items-center justify-center rounded-full ${bgClass}`}>
        <span className={`material-symbols-outlined text-3xl ${iconColor}`}>{icon}</span>
      </div>
      <span className="text-sm font-bold">{label}</span>
      <span className={`text-xs ${detailClass ?? 'text-on-surface-variant'}`}>{detail}</span>
    </div>
  );
}

function FeatureCard({
  icon,
  title,
  description,
}: {
  icon: string;
  title: string;
  description: string;
}) {
  return (
    <div className="rounded-xl border border-outline-variant/20 p-8 transition-colors hover:border-primary-container/30">
      <span className="material-symbols-outlined mb-4 text-primary-container">{icon}</span>
      <h4 className="mb-2 font-bold">{title}</h4>
      <p className="text-sm text-on-surface-variant">{description}</p>
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

function StatBlock({ value, label }: { value: string; label: string }) {
  return (
    <div>
      <div className="mb-2 text-5xl font-black text-primary-fixed">{value}</div>
      <p className="text-xs font-bold uppercase tracking-widest text-zinc-500">{label}</p>
    </div>
  );
}
