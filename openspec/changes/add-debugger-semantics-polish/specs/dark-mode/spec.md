## ADDED Requirements

### Requirement: Tailwind Dark Mode Configuration
Tailwind SHALL be configured with `darkMode: 'class'` in `tailwind.config.js`. CSS custom properties SHALL be defined in `globals.css` only for non-Tailwind contexts: waterfall bar inline styles and resizable panel separator styling. A synchronous pre-hydration script in `index.html` SHALL read the theme preference from localStorage and apply the `dark` class to `<html>` before React renders, preventing FOUC.

#### Scenario: Dark mode class activation
- **WHEN** the `dark` class is present on the `<html>` element
- **THEN** all `dark:` Tailwind variants are active

#### Scenario: No FOUC on page load
- **WHEN** a user with dark mode preference loads the page
- **THEN** the dark theme is applied before any visible content renders

#### Scenario: CSS custom properties for inline styles
- **WHEN** the waterfall component renders bars with inline styles
- **THEN** it reads colors from CSS custom properties that change with the dark class

### Requirement: useTheme Hook
A `useTheme` hook SHALL manage theme state with three modes: `system`, `light`, `dark`. The selected mode SHALL be persisted to localStorage. When `system` is selected, the hook SHALL follow the OS preference via `prefers-color-scheme` media query. The hook SHALL apply or remove the `dark` class on `<html>`.

#### Scenario: System theme follows OS
- **WHEN** the user selects "system" theme and the OS is in dark mode
- **THEN** the `dark` class is applied to `<html>`

#### Scenario: Explicit dark mode
- **WHEN** the user selects "dark" theme
- **THEN** the `dark` class is applied regardless of OS preference
- **AND** the preference is persisted to localStorage

#### Scenario: Switch to light mode
- **WHEN** the user selects "light" theme
- **THEN** the `dark` class is removed from `<html>`
- **AND** the preference is persisted to localStorage

### Requirement: Theme Toggle UI
A theme toggle SHALL be present in the `Navigation` component and mirrored on the `SettingsPage`. The toggle SHALL cycle through or select between system, light, and dark modes.

#### Scenario: Toggle theme from navigation
- **WHEN** the user clicks the theme toggle in the navigation bar
- **THEN** the theme changes and the UI updates immediately

#### Scenario: Theme setting on settings page
- **WHEN** the user visits the Settings page
- **THEN** they see the current theme selection and can change it

### Requirement: Dark Variants on Shell Components
All shell components SHALL have `dark:` Tailwind variants applied: `Navigation`, page layouts, cards, tables, inputs, `StatusBadge`, and any shared UI primitives. Colors SHALL use appropriate dark palette values (darker backgrounds, lighter text, adjusted borders).

#### Scenario: Navigation in dark mode
- **WHEN** dark mode is active
- **THEN** the Navigation component uses dark background and light text colors

#### Scenario: Tables in dark mode
- **WHEN** dark mode is active
- **THEN** table rows, headers, and borders use dark-appropriate colors

### Requirement: Dark Variants on Workspace Components
All workspace components SHALL have `dark:` Tailwind variants: `SpanTree`, `SpanDetail`, `Timeline`, `Waterfall`, `PayloadInspector`, `InspectorTabs`, `TreeRail`, and `StateDiffViewer`. Interactive states (hover, focus, selected) SHALL have appropriate dark variants.

#### Scenario: SpanTree in dark mode
- **WHEN** dark mode is active
- **THEN** tree nodes, selection highlights, and connecting lines use dark-appropriate colors

#### Scenario: PayloadInspector in dark mode
- **WHEN** dark mode is active
- **THEN** the payload tree viewer, search input, match highlighting, expand/collapse controls, and copy buttons adapt to dark-safe colors
