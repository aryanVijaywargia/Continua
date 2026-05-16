import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { RuntimeAuthStateProvider, type RuntimeAuthState } from '../auth/runtime';
import { ThemeProvider } from '../hooks/ThemeProvider';
import { LandingPage } from './LandingPage';

const GITHUB_REPO_URL = 'https://github.com/aryanVijaywargia/Continua';
const DOCS_URL = 'https://www.continua.in/docs';
const RUN_LOCALLY_DOCS_URL = `${DOCS_URL}/guides/installation`;

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
    expect(screen.getByRole('heading', { name: /Debug your agents today\. Run them durably tomorrow/i })).toBeInTheDocument();
    expect(document.body).toHaveTextContent(/AI agent observability today, durable execution tomorrow/i);
    expect(screen.getByRole('tablist', { name: 'Python SDK examples' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'agent.py' })).toHaveAttribute('aria-selected', 'true');
    expect(document.body).toHaveTextContent(/MIT licensed/i);
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
