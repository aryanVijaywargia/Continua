## ADDED Requirements

### Requirement: Error-Only Timeline Filter

The timeline header SHALL include an `Errors only` toggle button rendered as a keyboard-accessible toggle with `aria-pressed` attribute reflecting its state.

When active, the timeline SHALL display only events matching the error predicate: `event_type` of `error`, `exception`, or `span_failed`, or `level === 'error'`.

When active and no events match the predicate, the timeline SHALL display a filtered empty state message such as "No error events for this trace."

#### Scenario: Toggle activated with error events present

- **WHEN** the user activates the `Errors only` toggle and the trace has error events
- **THEN** only error events are displayed in the timeline

#### Scenario: Toggle activated with no error events

- **WHEN** the user activates the `Errors only` toggle and the trace has no error events
- **THEN** the timeline shows the filtered empty state message

#### Scenario: Toggle deactivated

- **WHEN** the user deactivates the `Errors only` toggle
- **THEN** all timeline events are displayed again

### Requirement: Error Filter Accessibility

The `Errors only` toggle SHALL be keyboard focusable and activatable with Enter or Space. It SHALL use `aria-pressed` to communicate its state to assistive technologies.

#### Scenario: Keyboard activation of filter toggle

- **WHEN** a user focuses the toggle and presses Enter or Space
- **THEN** the filter state toggles and `aria-pressed` updates accordingly
