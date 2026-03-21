import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { clearApiKey, getApiKey, setApiKey } from '../api/client';
import { renderTraceRoutes } from './testUtils';

beforeEach(() => {
  localStorage.clear();
});

afterEach(() => {
  clearApiKey();
  localStorage.clear();
});

describe('SettingsPage', () => {
  it('shows the masked current key, saves a new key, and clears it', async () => {
    const user = userEvent.setup();
    setApiKey('ck_live_existing_key_1234');

    renderTraceRoutes(['/settings']);

    expect(await screen.findByText('ck_l••••••1234')).toBeInTheDocument();

    const input = screen.getByLabelText('New API key');
    await user.type(input, 'ck_live_updated_key_9876');
    await user.click(screen.getByRole('button', { name: 'Save key' }));

    expect(getApiKey()).toBe('ck_live_updated_key_9876');
    expect(await screen.findByText('API key saved.')).toBeInTheDocument();
    expect(screen.getByText('ck_l••••••9876')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Clear key' }));
    expect(getApiKey()).toBeNull();
    expect(await screen.findByText('API key cleared.')).toBeInTheDocument();
    expect(screen.getByText('Not configured')).toBeInTheDocument();
  });
});
