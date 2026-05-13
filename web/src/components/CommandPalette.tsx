import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
} from 'react';

export interface CommandPaletteCommand {
  id: string;
  title: string;
  keywords?: string[];
  action: () => void;
}

interface CommandPaletteProps {
  commands: CommandPaletteCommand[];
}

export function CommandPalette({ commands }: CommandPaletteProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [query, setQuery] = useState('');
  const [activeIndex, setActiveIndex] = useState(0);
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const listboxId = useId();
  const shortcutHint = getCommandPaletteShortcutHint();
  const filteredCommands = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    if (!normalizedQuery) {
      return commands;
    }

    return commands.filter((command) => {
      const haystack = [
        command.title,
        ...(command.keywords ?? []),
      ]
        .join(' ')
        .toLowerCase();

      return haystack.includes(normalizedQuery);
    });
  }, [commands, query]);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        if (!isOpen && shouldSuppressShortcut(document.activeElement)) {
          return;
        }

        event.preventDefault();
        setIsOpen((open) => !open);
        return;
      }

      if (event.key === 'Escape') {
        setIsOpen(false);
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => {
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen) {
      return;
    }

    setQuery('');
    setActiveIndex(0);
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen) {
      return;
    }

    searchInputRef.current?.focus();
  }, [isOpen]);

  useEffect(() => {
    if (filteredCommands.length === 0) {
      setActiveIndex(0);
      return;
    }

    if (activeIndex >= filteredCommands.length) {
      setActiveIndex(0);
    }
  }, [activeIndex, filteredCommands.length]);

  const executeCommand = (command: CommandPaletteCommand) => {
    command.action();
    setIsOpen(false);
  };

  const handleSearchKeyDown = (event: ReactKeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      setActiveIndex((current) =>
        filteredCommands.length === 0 ? 0 : (current + 1) % filteredCommands.length
      );
      return;
    }

    if (event.key === 'ArrowUp') {
      event.preventDefault();
      setActiveIndex((current) =>
        filteredCommands.length === 0
          ? 0
          : (current - 1 + filteredCommands.length) % filteredCommands.length
      );
      return;
    }

    if (event.key === 'Enter') {
      event.preventDefault();
      const command = filteredCommands[activeIndex];
      if (command) {
        executeCommand(command);
      }
      return;
    }

    if (event.key === 'Escape') {
      event.preventDefault();
      setIsOpen(false);
    }
  };

  return (
    <>
      <button
        type="button"
        aria-label="Command Palette"
        onClick={() => setIsOpen(true)}
        className="inline-flex h-7 items-center gap-2 rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-2.5 text-[12.5px] font-medium text-[var(--c-text-muted)] transition hover:border-[var(--c-border-strong)] hover:text-[var(--c-text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)]"
      >
        <span>Search</span>
        <kbd className="rounded-[3px] border border-[var(--c-border)] bg-[var(--c-kbd-bg)] px-1.5 py-px font-mono text-[10.5px] font-semibold text-[var(--c-text-muted)]">
          {shortcutHint}
        </kbd>
      </button>

      {isOpen ? (
        <div
          data-testid="command-palette-backdrop"
          className="fixed inset-0 z-50 flex items-start justify-center bg-[#111318]/50 px-4 py-20 backdrop-blur-sm"
          onClick={() => setIsOpen(false)}
        >
          <div
            className="w-full max-w-2xl overflow-hidden rounded-lg border border-[var(--c-border-strong)] bg-[var(--c-surface-elevated)] shadow-2xl"
            onClick={(event) => event.stopPropagation()}
          >
            <div className="border-b border-[var(--c-border)] px-4 py-4">
              <label className="sr-only" htmlFor="command-palette-search">
                Search commands
              </label>
              <input
                id="command-palette-search"
                ref={searchInputRef}
                type="search"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                onKeyDown={handleSearchKeyDown}
                placeholder="Search commands"
                role="combobox"
                aria-expanded="true"
                aria-controls={listboxId}
                aria-activedescendant={
                  filteredCommands[activeIndex]
                    ? `command-option-${filteredCommands[activeIndex].id}`
                    : undefined
                }
                className="app-input"
              />
            </div>

            <div className="max-h-[24rem] overflow-y-auto p-2">
              {filteredCommands.length === 0 ? (
                <div className="px-4 py-12 text-center text-sm text-[var(--continua-text-muted)]">
                  No commands match your search.
                </div>
              ) : (
                <ul id={listboxId} role="listbox" className="space-y-1">
                  {filteredCommands.map((command, index) => {
                    const isActive = index === activeIndex;

                    return (
                      <li key={command.id}>
                        <button
                          id={`command-option-${command.id}`}
                          type="button"
                          role="option"
                          aria-selected={isActive}
                          className={`flex w-full items-center justify-between rounded-[0.75rem] px-4 py-3 text-left text-sm transition ${
                            isActive
                              ? 'bg-[var(--c-accent-faint)] text-[var(--c-accent-text)]'
                              : 'text-[var(--c-text-secondary)] hover:bg-[var(--c-surface-muted)]'
                          }`}
                          onMouseEnter={() => setActiveIndex(index)}
                          onClick={() => executeCommand(command)}
                        >
                          <span className="font-medium">{command.title}</span>
                          <span
                            className={`text-xs ${
                              isActive
                                ? 'opacity-70'
                                : 'text-[var(--c-text-muted)]'
                            }`}
                          >
                            Enter
                          </span>
                        </button>
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>
          </div>
        </div>
      ) : null}
    </>
  );
}

function getCommandPaletteShortcutHint() {
  return isMacLikePlatform() ? '⌘K' : 'Ctrl+K';
}

function isMacLikePlatform() {
  if (typeof navigator === 'undefined') {
    return false;
  }

  return /(mac|iphone|ipad)/i.test(navigator.platform);
}

function shouldSuppressShortcut(activeElement: Element | null) {
  if (!activeElement) {
    return false;
  }

  if (activeElement instanceof HTMLTextAreaElement) {
    return true;
  }

  if (activeElement instanceof HTMLInputElement) {
    return true;
  }

  if (activeElement instanceof HTMLElement && activeElement.isContentEditable) {
    return true;
  }

  return Boolean(activeElement.closest('[contenteditable="true"]'));
}
