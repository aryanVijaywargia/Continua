import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { CommandPalette, type CommandPaletteCommand } from './CommandPalette';

function renderCommandPalette(commands: CommandPaletteCommand[]) {
  return render(<CommandPalette commands={commands} />);
}

const commands: CommandPaletteCommand[] = [
  {
    id: 'traces',
    title: 'Go to Traces',
    keywords: ['navigate', 'trace'],
    action: vi.fn(),
  },
  {
    id: 'sessions',
    title: 'Go to Sessions',
    keywords: ['navigate', 'session'],
    action: vi.fn(),
  },
  {
    id: 'theme',
    title: 'Toggle Theme',
    keywords: ['appearance', 'dark', 'light'],
    action: vi.fn(),
  },
];

describe('CommandPalette', () => {
  it('filters commands as the search query changes', async () => {
    const user = userEvent.setup();
    renderCommandPalette(commands);

    await user.click(screen.getByRole('button', { name: /Command Palette/i }));
    await user.type(screen.getByRole('combobox', { name: 'Search commands' }), 'theme');

    expect(screen.getByRole('option', { name: /Toggle Theme/ })).toBeInTheDocument();
    expect(screen.queryByRole('option', { name: /Go to Traces/ })).not.toBeInTheDocument();
  });

  it('supports arrow-key navigation and Enter to execute the active command', async () => {
    const user = userEvent.setup();
    const themeAction = commands[2].action as ReturnType<typeof vi.fn>;
    renderCommandPalette(commands);

    await user.click(screen.getByRole('button', { name: /Command Palette/i }));
    const input = screen.getByRole('combobox', { name: 'Search commands' });

    await user.type(input, 'toggle');
    fireEvent.keyDown(input, { key: 'ArrowDown' });
    fireEvent.keyDown(input, { key: 'Enter' });

    expect(themeAction).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('combobox', { name: 'Search commands' })).not.toBeInTheDocument();
  });

  it('opens from the global shortcut and suppresses activation inside text inputs, textareas, and contenteditable elements', () => {
    render(
      <div>
        <input aria-label="Plain input" />
        <textarea aria-label="Plain textarea" />
        <div aria-label="Editable div" contentEditable />
        <CommandPalette commands={commands} />
      </div>
    );

    const textInput = screen.getByLabelText('Plain input');
    textInput.focus();
    fireEvent.keyDown(window, { key: 'k', ctrlKey: true });
    expect(screen.queryByRole('combobox', { name: 'Search commands' })).not.toBeInTheDocument();

    const textArea = screen.getByLabelText('Plain textarea');
    textArea.focus();
    fireEvent.keyDown(window, { key: 'k', ctrlKey: true });
    expect(screen.queryByRole('combobox', { name: 'Search commands' })).not.toBeInTheDocument();

    const editable = screen.getByLabelText('Editable div');
    editable.focus();
    fireEvent.keyDown(window, { key: 'k', ctrlKey: true });
    expect(screen.queryByRole('combobox', { name: 'Search commands' })).not.toBeInTheDocument();

    editable.blur();
    fireEvent.keyDown(window, { key: 'k', ctrlKey: true });
    expect(screen.getByRole('combobox', { name: 'Search commands' })).toBeInTheDocument();
  });

  it('closes from the global shortcut while open and dismisses on backdrop click', async () => {
    const user = userEvent.setup();
    renderCommandPalette(commands);

    fireEvent.keyDown(window, { key: 'k', ctrlKey: true });
    expect(screen.getByRole('combobox', { name: 'Search commands' })).toBeInTheDocument();

    fireEvent.keyDown(window, { key: 'k', ctrlKey: true });
    expect(screen.queryByRole('combobox', { name: 'Search commands' })).not.toBeInTheDocument();

    fireEvent.keyDown(window, { key: 'k', ctrlKey: true });
    expect(screen.getByRole('combobox', { name: 'Search commands' })).toBeInTheDocument();

    await user.click(screen.getByTestId('command-palette-backdrop'));
    expect(screen.queryByRole('combobox', { name: 'Search commands' })).not.toBeInTheDocument();
  });
});
