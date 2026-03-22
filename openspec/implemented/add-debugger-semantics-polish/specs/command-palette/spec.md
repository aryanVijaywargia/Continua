## ADDED Requirements

### Requirement: Command Palette Component
A `CommandPalette` component SHALL provide a searchable, keyboard-navigable command list. It SHALL support arrow-key navigation, Enter to execute, and Escape to dismiss. The initial command set SHALL include: navigate to Traces, navigate to Sessions, navigate to Settings, and toggle theme.

#### Scenario: Open and search
- **WHEN** the command palette is open and the user types "tra"
- **THEN** the "Go to Traces" command is shown as a filtered match

#### Scenario: Arrow key navigation
- **WHEN** the user presses the down arrow key
- **THEN** the selection moves to the next command in the filtered list

#### Scenario: Execute command with Enter
- **WHEN** the user presses Enter with "Go to Traces" selected
- **THEN** the application navigates to `/traces`
- **AND** the palette closes

#### Scenario: Dismiss with Escape
- **WHEN** the user presses Escape
- **THEN** the palette closes without executing any command

### Requirement: Global Keyboard Shortcut
The command palette SHALL be toggled with `Cmd+K` (macOS) or `Ctrl+K` (other platforms). The shortcut SHALL be registered globally. A discoverability hint (`⌘K` or `Ctrl+K`) SHALL be visible in the navigation bar. The shortcut SHALL NOT activate when a text input, textarea, or contenteditable element has focus.

#### Scenario: Open palette with Cmd+K
- **WHEN** the user presses `Cmd+K` on macOS
- **THEN** the command palette opens

#### Scenario: Close palette with Cmd+K when open
- **WHEN** the command palette is open and the user presses `Cmd+K`
- **THEN** the palette closes

#### Scenario: Keyboard hint visible in navigation
- **WHEN** the navigation bar is rendered
- **THEN** a `⌘K` (macOS) or `Ctrl+K` hint is visible

#### Scenario: Shortcut does not hijack focused text inputs
- **WHEN** the user presses `Cmd+K` / `Ctrl+K` while a text input, textarea, or contenteditable element has focus
- **THEN** the command palette does NOT open
- **AND** the default browser/input behavior is preserved
