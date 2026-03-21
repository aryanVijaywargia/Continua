import { useEffect, useMemo, useState, type ReactNode } from 'react';
import {
  THEME_STORAGE_KEY,
  ThemeContext,
  type ResolvedTheme,
  type ThemeMode,
} from './themeContext';

const SYSTEM_THEME_QUERY = '(prefers-color-scheme: dark)';

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [mode, setMode] = useState<ThemeMode>(() => readStoredThemeMode());
  const [systemPrefersDark, setSystemPrefersDark] = useState(() =>
    getSystemPrefersDark()
  );

  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
      return;
    }

    const mediaQuery = window.matchMedia(SYSTEM_THEME_QUERY);
    const handleChange = (event: MediaQueryListEvent) => {
      setSystemPrefersDark(event.matches);
    };

    setSystemPrefersDark(mediaQuery.matches);
    mediaQuery.addEventListener('change', handleChange);

    return () => {
      mediaQuery.removeEventListener('change', handleChange);
    };
  }, []);

  const resolvedTheme = resolveTheme(mode, systemPrefersDark);

  useEffect(() => {
    localStorage.setItem(THEME_STORAGE_KEY, mode);
  }, [mode]);

  useEffect(() => {
    const root = document.documentElement;
    root.classList.toggle('dark', resolvedTheme === 'dark');
    root.dataset.theme = resolvedTheme;
  }, [resolvedTheme]);

  const value = useMemo(
    () => ({
      mode,
      resolvedTheme,
      setMode,
      toggleTheme: () => {
        setMode((currentMode) => {
          const currentResolvedTheme = resolveTheme(currentMode, systemPrefersDark);
          return currentResolvedTheme === 'dark' ? 'light' : 'dark';
        });
      },
    }),
    [mode, resolvedTheme, systemPrefersDark]
  );

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>;
}

function readStoredThemeMode(): ThemeMode {
  if (typeof window === 'undefined') {
    return 'system';
  }

  const storedMode = localStorage.getItem(THEME_STORAGE_KEY);
  return isThemeMode(storedMode) ? storedMode : 'system';
}

function getSystemPrefersDark(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false;
  }

  return window.matchMedia(SYSTEM_THEME_QUERY).matches;
}

function resolveTheme(mode: ThemeMode, systemPrefersDark: boolean): ResolvedTheme {
  if (mode === 'system') {
    return systemPrefersDark ? 'dark' : 'light';
  }

  return mode;
}

function isThemeMode(value: string | null): value is ThemeMode {
  return value === 'system' || value === 'light' || value === 'dark';
}
