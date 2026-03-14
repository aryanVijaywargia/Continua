import { afterEach, describe, expect, it, vi } from 'vitest';
import { copyToClipboard } from './clipboard';

describe('copyToClipboard', () => {
  afterEach(() => {
    vi.restoreAllMocks();
    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: undefined,
    });
  });

  it('writes text when the clipboard API is available', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);

    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });

    await copyToClipboard('trace-123');

    expect(writeText).toHaveBeenCalledWith('trace-123');
  });

  it('rejects when the clipboard API is unavailable', async () => {
    Object.defineProperty(window.navigator, 'clipboard', {
      configurable: true,
      value: undefined,
    });

    await expect(copyToClipboard('trace-123')).rejects.toThrow(
      'Clipboard API is unavailable'
    );
  });
});
