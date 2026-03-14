import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { CopyButton } from './CopyButton';
import { copyToClipboard } from '../utils/clipboard';

vi.mock('../utils/clipboard', () => ({
  copyToClipboard: vi.fn(),
}));

describe('CopyButton', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it('shows error feedback when clipboard writes fail and recovers after the timeout', async () => {
    vi.mocked(copyToClipboard)
      .mockRejectedValueOnce(new Error('write failed'))
      .mockResolvedValueOnce(undefined);

    render(<CopyButton value="hello" />);

    const button = screen.getByRole('button', { name: 'Copy' });

    await act(async () => {
      fireEvent.click(button);
    });
    expect(button).toHaveTextContent('Failed');
    expect(button).toHaveAttribute('data-copy-status', 'error');

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(button).toHaveAttribute('data-copy-status', 'idle');
    expect(button).toHaveTextContent('Copy');

    await act(async () => {
      fireEvent.click(button);
    });
    expect(button).toHaveTextContent('Copied');
    expect(button).toHaveAttribute('data-copy-status', 'success');
  });
});
