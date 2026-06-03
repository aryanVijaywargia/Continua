import { act, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { RuntimeAuthStateProvider, type RuntimeAuthState } from '../auth/runtime';
import { ThemeProvider } from '../hooks/ThemeProvider';
import { LandingPage } from './LandingPage';

const GITHUB_REPO_URL = 'https://github.com/aryanVijaywargia/Continua';
const DOCS_URL = 'https://www.continua.in/docs';
const RUN_LOCALLY_DOCS_URL = `${DOCS_URL}/guides/installation`;
const originalIntersectionObserver = globalThis.IntersectionObserver;
const originalFetch = globalThis.fetch;

let intersectionCallback: IntersectionObserverCallback | null = null;

class MockIntersectionObserver implements IntersectionObserver {
  readonly root = null;
  readonly rootMargin = '';
  readonly thresholds = [];

  constructor(callback: IntersectionObserverCallback) {
    intersectionCallback = callback;
  }

  disconnect = vi.fn();
  observe = vi.fn();
  takeRecords = vi.fn(() => []);
  unobserve = vi.fn();
}

function renderLandingPage(auth?: Partial<RuntimeAuthState>) {
  const runtimeAuth: RuntimeAuthState = {
    status: 'ready',
    enabled: false,
    ...auth,
  };

  return render(
    <ThemeProvider>
      <RuntimeAuthStateProvider auth={runtimeAuth}>
        <MemoryRouter>
          <LandingPage />
        </MemoryRouter>
      </RuntimeAuthStateProvider>
    </ThemeProvider>
  );
}

describe('LandingPage', () => {
  afterEach(() => {
    intersectionCallback = null;
    Object.defineProperty(window, 'IntersectionObserver', {
      configurable: true,
      value: originalIntersectionObserver,
    });
    Object.defineProperty(globalThis, 'IntersectionObserver', {
      configurable: true,
      value: originalIntersectionObserver,
    });
    Object.defineProperty(globalThis, 'fetch', {
      configurable: true,
      value: originalFetch,
    });
    window.history.replaceState(null, '', '/');
    vi.restoreAllMocks();
  });

  it('uses Continua branding and wires the primary landing actions', () => {
    renderLandingPage();

    expect(screen.queryByText('Agentic Engine')).not.toBeInTheDocument();
    expect(screen.getAllByText('Continua').length).toBeGreaterThan(0);

    expect(screen.getByRole('link', { name: 'How it works' })).toHaveAttribute(
      'href',
      '#how'
    );
    expect(
      screen.getAllByRole('link', { name: 'SDK' }).some((link) => link.getAttribute('href') === '#sdk')
    ).toBe(true);
    expect(screen.getAllByRole('link', { name: 'Product' }).length).toBeGreaterThan(0);
    expect(
      screen
        .getAllByRole('link', { name: 'Open source' })
        .some((link) => link.getAttribute('href') === '#open-source')
    ).toBe(true);
    for (const link of screen.getAllByRole('link', { name: 'Open Console' })) {
      expect(link).toHaveAttribute('href', '/dashboard');
    }
    expect(screen.getByRole('link', { name: /View on GitHub/i })).toHaveAttribute(
      'href',
      GITHUB_REPO_URL
    );
    expect(screen.getByRole('link', { name: 'License' })).not.toHaveAttribute('href', '#');
    expect(
      screen.getAllByRole('link', { name: 'Docs' }).some((link) => link.getAttribute('href') !== '#')
    ).toBe(true);
    expect(screen.getByText(/The Python SDK batches spans/i)).toBeInTheDocument();
    expect(screen.queryByText(/TypeScript SDK/i)).not.toBeInTheDocument();
    expect(document.body).toHaveTextContent(/from continua import Continua, span, trace/i);
    expect(screen.getByRole('heading', { name: /Your agent's black box\. Opened\./i })).toBeInTheDocument();
    expect(document.body).toHaveTextContent(/Durable execution engine for AI agents, with built-in observability/i);
    expect(screen.getByRole('tablist', { name: 'Python SDK examples' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'agent.py' })).toHaveAttribute('aria-selected', 'true');
    expect(document.body).toHaveTextContent(/MIT licensed/i);
    expect(screen.getByText('Release')).toBeInTheDocument();
    expect(screen.getByText('Alpha')).toBeInTheDocument();
    expect(document.body).not.toHaveTextContent(/Apache 2\.0/i);
    expect(document.body).not.toHaveTextContent(/1,247/i);
    expect(document.body).not.toHaveTextContent(/pip install continua/i);
    expect(document.body).not.toHaveTextContent(/continua serve --port 8080/i);
    expect(document.body).toHaveTextContent(/make demo/i);
    expect(document.body).toHaveTextContent(/Sample traces/i);
    expect(document.body).not.toHaveTextContent(/Live ingest/i);
    expect(document.body).not.toHaveTextContent(/99\.97%/i);
  });

  it('keeps mockup section navigation available', () => {
    renderLandingPage();

    expect(
      screen
        .getAllByRole('link', { name: 'Product' })
        .some((link) => link.getAttribute('href') === '#product')
    ).toBe(true);
    expect(screen.getByRole('link', { name: 'How it works' })).toHaveAttribute('href', '#how');
    expect(
      screen.getAllByRole('link', { name: 'SDK' }).some((link) => link.getAttribute('href') === '#sdk')
    ).toBe(true);
    expect(
      screen
        .getAllByRole('link', { name: 'Docs' })
        .some((link) => link.getAttribute('href') === DOCS_URL)
    ).toBe(true);
  });

  it('updates the hash as landing sections become active while scrolling', () => {
    Object.defineProperty(window, 'IntersectionObserver', {
      configurable: true,
      value: MockIntersectionObserver,
    });
    Object.defineProperty(globalThis, 'IntersectionObserver', {
      configurable: true,
      value: MockIntersectionObserver,
    });

    renderLandingPage();
    const sdkSection = document.getElementById('sdk');
    expect(sdkSection).not.toBeNull();

    act(() => {
      intersectionCallback?.(
        [
          {
            isIntersecting: true,
            intersectionRatio: 0.75,
            target: sdkSection!,
          } as unknown as IntersectionObserverEntry,
        ],
        {} as IntersectionObserver
      );
    });

    expect(window.location.hash).toBe('#sdk');
  });

  it('renders commit activity from the bundled repo stats', () => {
    renderLandingPage();

    expect(document.body).toHaveTextContent(/\d+ commits · since [A-Z][a-z]{2} \d{4}/);
    expect(document.body).not.toHaveTextContent(/last 26 weeks/);
  });

  it('switches landing CTAs to the public demo flow when demo mode is enabled', () => {
    renderLandingPage({
      public_demo_enabled: true,
      public_demo_label: 'Sample data',
    });

    for (const link of screen.getAllByRole('link', { name: 'Open Demo' })) {
      expect(link).toHaveAttribute('href', '/dashboard');
    }
    expect(
      screen
        .getAllByRole('link', { name: /run locally/i })
        .some((link) => link.getAttribute('href') === RUN_LOCALLY_DOCS_URL)
    ).toBe(true);
    expect(
      screen.getByText(/hosted debugger uses seeded sample traces only/i)
    ).toBeInTheDocument();
  });

  it('points console CTAs to the local setup guide when static hosting has no console backend', () => {
    renderLandingPage({
      console_available: false,
    });

    for (const link of screen.getAllByRole('link', { name: 'Run Locally' })) {
      expect(link).toHaveAttribute('href', RUN_LOCALLY_DOCS_URL);
    }
    expect(screen.queryByRole('link', { name: 'Open Console' })).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'open console ↗' })).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'run locally ↗' })).toHaveAttribute(
      'href',
      RUN_LOCALLY_DOCS_URL
    );
    expect(screen.getByRole('link', { name: 'Console' })).toHaveAttribute(
      'href',
      RUN_LOCALLY_DOCS_URL
    );
    expect(screen.getByRole('link', { name: 'Traces' })).toHaveAttribute(
      'href',
      RUN_LOCALLY_DOCS_URL
    );
    for (const link of screen.getAllByRole('link', { name: 'Run Locally' })) {
      expect(link).toHaveAttribute('href', RUN_LOCALLY_DOCS_URL);
    }
    expect(
      screen.getByText(/This hosted Pages deployment is static/i)
    ).toBeInTheDocument();
  });
});
