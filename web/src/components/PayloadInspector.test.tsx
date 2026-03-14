import {
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PayloadInspector } from './PayloadInspector';

const scrollIntoViewMock = vi.fn();

describe('PayloadInspector', () => {
  beforeEach(() => {
    scrollIntoViewMock.mockReset();
    Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
      configurable: true,
      value: scrollIntoViewMock,
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: undefined,
    });
  });

  it('renders a placeholder for undefined payloads', () => {
    render(<PayloadInspector data={undefined} />);

    expect(screen.getByText('No data')).toBeInTheDocument();
  });

  it('finds key matches and value matches independently', async () => {
    render(
      <PayloadInspector
        data={{
          temperature: 42,
          prompt: 'temperature high',
        }}
      />
    );

    const searchInput = screen.getByLabelText('Search payload');

    fireEvent.change(searchInput, { target: { value: 'temp' } });
    await waitFor(() => {
      expect(screen.getByText('2 matches')).toBeInTheDocument();
    });
    expect(screen.getByText('temperature')).toHaveAttribute(
      'data-match-state',
      'active'
    );
    expect(screen.getByText('"temperature high"')).toHaveAttribute(
      'data-match-state',
      'match'
    );

    fireEvent.change(searchInput, { target: { value: '42' } });
    await waitFor(() => {
      expect(screen.getByText('1 match')).toBeInTheDocument();
    });
    expect(screen.getByText('42')).toHaveAttribute('data-match-state', 'active');
  });

  it('supports match ordering and distinct active highlighting', async () => {
    const user = userEvent.setup();

    render(
      <PayloadInspector
        data={{
          first: 'match first',
          second: 'match second',
        }}
      />
    );

    fireEvent.change(screen.getByLabelText('Search payload'), {
      target: { value: 'match' },
    });

    await waitFor(() => {
      expect(screen.getByText('"match first"')).toHaveAttribute(
        'data-match-state',
        'active'
      );
    });
    expect(screen.getByText('"match second"')).toHaveAttribute(
      'data-match-state',
      'match'
    );

    await user.click(screen.getByRole('button', { name: 'Next' }));

    expect(screen.getByText('"match first"')).toHaveAttribute(
      'data-match-state',
      'match'
    );
    expect(screen.getByText('"match second"')).toHaveAttribute(
      'data-match-state',
      'active'
    );
  });

  it('preserves the active match while toggling nodes during an active search', async () => {
    const user = userEvent.setup();

    render(
      <PayloadInspector
        data={{
          nested: {
            first: 'match first',
            second: 'match second',
          },
          unrelated: {
            child: 'other value',
          },
        }}
      />
    );

    fireEvent.change(screen.getByLabelText('Search payload'), {
      target: { value: 'match' },
    });

    await waitFor(() => {
      expect(screen.getByText('"match first"')).toHaveAttribute(
        'data-match-state',
        'active'
      );
    });

    await user.click(screen.getByRole('button', { name: 'Next' }));
    expect(screen.getByText('"match second"')).toHaveAttribute(
      'data-match-state',
      'active'
    );

    await user.click(screen.getByRole('button', { name: 'Collapse unrelated' }));
    await user.click(screen.getByRole('button', { name: 'Expand unrelated' }));

    expect(screen.getByText('"match second"')).toHaveAttribute(
      'data-match-state',
      'active'
    );
  });

  it('keeps object keys and array paths with similar shapes isolated during search', async () => {
    const user = userEvent.setup();

    render(
      <PayloadInspector
        data={{
          'a[0]': {
            nested: 'match first',
          },
          a: [
            {
              nested: 'match second',
            },
          ],
        }}
      />
    );

    fireEvent.change(screen.getByLabelText('Search payload'), {
      target: { value: 'match' },
    });

    await waitFor(() => {
      expect(screen.getByText('2 matches')).toBeInTheDocument();
    });

    expect(screen.getByText('"match first"')).toHaveAttribute(
      'data-match-state',
      'active'
    );
    expect(screen.getByText('"match second"')).toHaveAttribute(
      'data-match-state',
      'match'
    );
    expect(document.querySelectorAll('[data-match-state="active"]')).toHaveLength(1);

    await user.click(screen.getByRole('button', { name: 'Next' }));

    expect(screen.getByText('"match first"')).toHaveAttribute(
      'data-match-state',
      'match'
    );
    expect(screen.getByText('"match second"')).toHaveAttribute(
      'data-match-state',
      'active'
    );
  });

  it('shows the no-match state when the query is absent from the payload', async () => {
    render(<PayloadInspector data={{ model: 'gpt-4' }} />);

    fireEvent.change(screen.getByLabelText('Search payload'), {
      target: { value: 'nonexistent' },
    });

    await waitFor(() => {
      expect(screen.getByText('0 matches')).toBeInTheDocument();
    });
    expect(document.querySelector('[data-match-state]')).toBeNull();
  });

  it('scrolls the active match into view without moving focus away from the input', async () => {
    render(
      <PayloadInspector
        data={{
          first: 'match first',
          second: 'match second',
        }}
      />
    );

    const searchInput = screen.getByLabelText('Search payload');
    searchInput.focus();

    fireEvent.change(searchInput, { target: { value: 'match' } });

    await waitFor(() => {
      expect(scrollIntoViewMock).toHaveBeenCalled();
    });
    expect(searchInput).toHaveFocus();
  });

  it('supports keyboard activation for expand and collapse toggles', async () => {
    const user = userEvent.setup();

    render(
      <PayloadInspector
        data={{
          nested: {
            child: 'value',
          },
        }}
      />
    );

    const toggle = screen.getByRole('button', { name: 'Collapse payload' });
    toggle.focus();

    await user.keyboard('{Enter}');
    expect(screen.queryByText('nested')).not.toBeInTheDocument();

    const expandToggle = screen.getByRole('button', { name: 'Expand payload' });
    expandToggle.focus();
    await user.keyboard(' ');

    expect(screen.getByText('nested')).toBeInTheDocument();
  });

  it('disables expand all for very large payloads', () => {
    render(
      <PayloadInspector
        data={Array.from({ length: 5001 }, (_, index) => index)}
      />
    );

    expect(screen.getByRole('button', { name: 'Expand all' })).toBeDisabled();
  });

  it('copies full JSON and leaf values via the shared copy button', async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);

    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });

    render(<PayloadInspector data={{ prompt: 'hello' }} />);

    await user.click(screen.getByRole('button', { name: 'Copy value for prompt' }));
    await user.click(screen.getByRole('button', { name: 'Copy full JSON' }));

    expect(writeText).toHaveBeenNthCalledWith(1, 'hello');
    expect(writeText).toHaveBeenNthCalledWith(
      2,
      JSON.stringify({ prompt: 'hello' }, null, 2)
    );
  });

  it('copies subtree JSON for collection nodes', async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);

    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });

    render(
      <PayloadInspector
        data={{
          nested: {
            child: 'hello',
          },
        }}
      />
    );

    await user.click(screen.getByRole('button', { name: 'Copy JSON for nested' }));

    expect(writeText).toHaveBeenCalledWith(
      JSON.stringify({ child: 'hello' }, null, 2)
    );
  });

  it('supports keyboard activation for copy buttons', async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);

    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });

    render(<PayloadInspector data={{ prompt: 'hello' }} />);

    const copyValueButton = screen.getByRole('button', {
      name: 'Copy value for prompt',
    });
    copyValueButton.focus();
    await user.keyboard('{Enter}');

    const copyJsonButton = screen.getByRole('button', { name: 'Copy full JSON' });
    copyJsonButton.focus();
    await user.keyboard(' ');

    expect(writeText).toHaveBeenNthCalledWith(1, 'hello');
    expect(writeText).toHaveBeenNthCalledWith(
      2,
      JSON.stringify({ prompt: 'hello' }, null, 2)
    );
  });

  it('renders primitive and null root payloads without placeholders', () => {
    const { rerender } = render(<PayloadInspector data={null} />);

    expect(screen.getByText('null')).toBeInTheDocument();
    expect(screen.queryByText('No data')).not.toBeInTheDocument();

    rerender(<PayloadInspector data="hello" />);
    expect(screen.getByText('"hello"')).toBeInTheDocument();
  });

  it('keeps search state isolated between inspector instances', async () => {
    render(
      <>
        <PayloadInspector data={{ first: 'match me' }} />
        <PayloadInspector data={{ second: 'stay idle' }} />
      </>
    );

    const [firstSearchInput, secondSearchInput] =
      screen.getAllByLabelText('Search payload');

    fireEvent.change(firstSearchInput, {
      target: { value: 'match' },
    });

    await waitFor(() => {
      expect(screen.getByText('1 match')).toBeInTheDocument();
    });

    expect(secondSearchInput).toHaveValue('');
    expect(screen.getAllByText('Search keys and values')).toHaveLength(1);
  });

  it('renders multiline strings with wrapped scrollable styling', () => {
    render(
      <PayloadInspector
        data={{
          prompt: 'line 1\nline 2',
        }}
      />
    );

    const value = screen.getByText(
      (_, element) => element?.textContent === '"line 1\nline 2"'
    );
    expect(value).toHaveClass('max-h-36');
    expect(value).toHaveClass('overflow-auto');
    expect(value).toHaveClass('whitespace-pre-wrap');
    expect(value).toHaveClass('break-words');
  });
});
