import { useEffect, useMemo, useState, type ReactNode } from 'react';
import {
  LEGACY_THEME_STORAGE_KEY,
  ThemeContext,
  THEME_STORAGE_KEY,
  type ResolvedTheme,
  type ThemeMode,
} from './themeContext';
import { useMediaQuery } from './useMediaQuery';

const DARK_MODE_MEDIA_QUERY = '(prefers-color-scheme: dark)';

function readInitialMode(): ThemeMode {
  if (typeof window === 'undefined') {
    return 'system';
  }

  const storedMode =
    window.localStorage.getItem(THEME_STORAGE_KEY) ??
    window.localStorage.getItem(LEGACY_THEME_STORAGE_KEY);
  const normalizedMode = normalizeStoredMode(storedMode);
  if (normalizedMode) {
    return normalizedMode;
  }

  return 'system';
}

function normalizeStoredMode(mode: string | null): ThemeMode | null {
  if (mode === 'true') {
    return 'dark';
  }

  if (mode === 'false') {
    return 'light';
  }

  return mode === 'light' || mode === 'dark' || mode === 'system' ? mode : null;
}

function resolveTheme(mode: ThemeMode, prefersDark: boolean): ResolvedTheme {
  if (mode === 'system') {
    return prefersDark ? 'dark' : 'light';
  }

  return mode;
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const prefersDark = useMediaQuery(DARK_MODE_MEDIA_QUERY);
  const [mode, setMode] = useState<ThemeMode>(readInitialMode);
  const resolvedTheme = resolveTheme(mode, prefersDark);

  useEffect(() => {
    if (typeof window === 'undefined') {
      return;
    }

    window.localStorage.setItem(THEME_STORAGE_KEY, mode);
    window.localStorage.setItem(LEGACY_THEME_STORAGE_KEY, mode);
  }, [mode]);

  useEffect(() => {
    const root = document.documentElement;
    root.dataset.theme = resolvedTheme;
    root.classList.toggle('dark', resolvedTheme === 'dark');

    return () => {
      delete root.dataset.theme;
      root.classList.remove('dark');
    };
  }, [resolvedTheme]);

  const value = useMemo(
    () => ({
      mode,
      resolvedTheme,
      setMode,
      toggleTheme: () => {
        setMode((currentMode) =>
          resolveTheme(currentMode, prefersDark) === 'dark' ? 'light' : 'dark'
        );
      },
    }),
    [mode, prefersDark, resolvedTheme]
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}
