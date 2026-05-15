import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';
import { RuntimeAuthStateProvider, type RuntimeAuthState } from '../auth/runtime';
import { setMatchMediaMatches } from '../test/matchMedia';
import { LandingPage } from './LandingPage';

const GITHUB_REPO_URL = 'https://github.com/aryanVijaywargia/Continua';
const RUN_LOCALLY_DOCS_URL = `${GITHUB_REPO_URL}/blob/main/docs/setup.md`;

function renderLandingPage(auth?: Partial<RuntimeAuthState>) {
  const runtimeAuth: RuntimeAuthState = {
    status: 'ready',
    enabled: false,
    ...auth,
  };

  return render(
    <RuntimeAuthStateProvider auth={runtimeAuth}>
      <MemoryRouter>
        <LandingPage />
      </MemoryRouter>
    </RuntimeAuthStateProvider>
  );
}

describe('LandingPage', () => {
  it('uses Continua branding and wires the primary landing actions', () => {
    setMatchMediaMatches(true);

    renderLandingPage();

    expect(screen.queryByText('Agentic Engine')).not.toBeInTheDocument();
    expect(screen.getAllByText('Continua').length).toBeGreaterThan(0);

    expect(screen.getByRole('link', { name: 'How it works' })).toHaveAttribute(
      'href',
      '#how-it-works'
    );
    expect(screen.getByRole('link', { name: 'Examples' })).toHaveAttribute(
      'href',
      '#examples'
    );
    expect(screen.getByRole('link', { name: 'Open source' })).toHaveAttribute(
      'href',
      '#open-source'
    );
    for (const link of screen.getAllByRole('link', { name: 'Open Console' })) {
      expect(link).toHaveAttribute('href', '/dashboard');
    }
    expect(
      screen.getByRole('link', { name: /See how it works/i })
    ).toHaveAttribute('href', '#how-it-works');
    expect(screen.getByRole('link', { name: /View GitHub/i })).toHaveAttribute(
      'href',
      GITHUB_REPO_URL
    );
    expect(screen.getByRole('link', { name: 'Read License' })).not.toHaveAttribute(
      'href',
      '#'
    );
    expect(screen.getByRole('link', { name: 'Read the docs' })).not.toHaveAttribute(
      'href',
      '#'
    );
    expect(screen.getByText(/Python SDK helpers/i)).toBeInTheDocument();
    expect(screen.queryByText(/TypeScript SDK/i)).not.toBeInTheDocument();
    expect(document.body).toHaveTextContent(/from continua import Continua, span, trace/i);
  });

  it('keeps section navigation available on mobile', () => {
    setMatchMediaMatches(false);

    renderLandingPage();

    expect(screen.getByRole('link', { name: 'Jump to how it works' })).toHaveAttribute(
      'href',
      '#how-it-works'
    );
    expect(screen.getByRole('link', { name: 'Jump to examples' })).toHaveAttribute(
      'href',
      '#examples'
    );
    expect(screen.getByRole('link', { name: 'Jump to open source' })).toHaveAttribute(
      'href',
      '#open-source'
    );
    expect(screen.getByRole('link', { name: 'Open docs' })).toHaveAttribute(
      'href',
      expect.stringContaining('/tree/main/docs')
    );
  });

  it('switches landing CTAs to the public demo flow when demo mode is enabled', () => {
    setMatchMediaMatches(true);

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
    setMatchMediaMatches(true);

    renderLandingPage({
      console_available: false,
    });

    for (const link of screen.getAllByRole('link', { name: 'Run Locally' })) {
      expect(link).toHaveAttribute('href', RUN_LOCALLY_DOCS_URL);
    }
    expect(screen.queryByRole('link', { name: 'Open Console' })).not.toBeInTheDocument();
    expect(
      screen.getByText(/hosted Pages site is static/i)
    ).toBeInTheDocument();
  });
});
