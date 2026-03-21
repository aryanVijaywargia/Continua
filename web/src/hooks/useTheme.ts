import { useContext } from 'react';
import { ThemeContext, type ThemeContextValue } from './themeContext';

export { THEME_STORAGE_KEY, type ResolvedTheme, type ThemeMode } from './themeContext';

export function useTheme(): ThemeContextValue {
  const context = useContext(ThemeContext);
  if (!context) {
    throw new Error('useTheme must be used within a ThemeProvider');
  }

  return context;
}
