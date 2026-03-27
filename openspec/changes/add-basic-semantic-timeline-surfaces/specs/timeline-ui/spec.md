## ADDED Requirements

### Requirement: Effect Payload Extraction

The system SHALL provide a `getEffectDetails()` function in `eventSemantics.ts` that extracts structured effect information from timeline event payloads.

The function SHALL return a typed object containing `effectKind` (string), `hasExternalSideEffect` (boolean), and optional `effectId` (string), `idempotent` (boolean), and `idempotencyKey` (string) when the event has `event_type` of `effect` and the payload contains the required fields `effect_kind` (string) and `has_external_side_effect` (boolean).

The function SHALL return `null` when the event type is not `effect`, the payload is missing, or required fields are absent or have incorrect types.

#### Scenario: Well-formed effect payload with all fields
- **WHEN** a timeline event has `event_type: "effect"` and payload `{ effect_kind: "api_call", has_external_side_effect: true, effect_id: "effect_abc123", idempotent: false, idempotency_key: "key-1" }`
- **THEN** `getEffectDetails()` returns `{ effectKind: "api_call", hasExternalSideEffect: true, effectId: "effect_abc123", idempotent: false, idempotencyKey: "key-1" }`

#### Scenario: Minimal effect payload with required fields only
- **WHEN** a timeline event has `event_type: "effect"` and payload `{ effect_kind: "model_call", has_external_side_effect: false }`
- **THEN** `getEffectDetails()` returns `{ effectKind: "model_call", hasExternalSideEffect: false }` with optional fields undefined

#### Scenario: Missing required field returns null
- **WHEN** a timeline event has `event_type: "effect"` and payload `{ effect_kind: "model_call" }` (missing `has_external_side_effect`)
- **THEN** `getEffectDetails()` returns `null`

#### Scenario: Wrong event type returns null
- **WHEN** a timeline event has `event_type: "log"` with an effect-shaped payload
- **THEN** `getEffectDetails()` returns `null`

#### Scenario: Missing payload returns null
- **WHEN** a timeline event has `event_type: "effect"` and no payload
- **THEN** `getEffectDetails()` returns `null`

### Requirement: Wait Payload Extraction

The system SHALL provide a `getWaitDetails()` function in `eventSemantics.ts` that extracts structured wait information from timeline event payloads.

The function SHALL return a typed object containing `waitKind` (string), `phase` (string), and optional `resolution` (string) and `waitId` (string) when the event has `event_type` of `wait` and the payload contains the required fields `wait_kind` (string) and `phase` (string).

The function SHALL return `null` when the event type is not `wait`, the payload is missing, or required fields are absent or have incorrect types.

#### Scenario: Well-formed wait payload with all fields
- **WHEN** a timeline event has `event_type: "wait"` and payload `{ wait_kind: "human_approval", phase: "resolved", resolution: "approved", wait_id: "wait_xyz789" }`
- **THEN** `getWaitDetails()` returns `{ waitKind: "human_approval", phase: "resolved", resolution: "approved", waitId: "wait_xyz789" }`

#### Scenario: Minimal wait payload with required fields only
- **WHEN** a timeline event has `event_type: "wait"` and payload `{ wait_kind: "human_approval", phase: "entered" }`
- **THEN** `getWaitDetails()` returns `{ waitKind: "human_approval", phase: "entered" }` with optional fields undefined

#### Scenario: Missing required phase field returns null
- **WHEN** a timeline event has `event_type: "wait"` and payload `{ wait_kind: "external" }` (missing `phase`)
- **THEN** `getWaitDetails()` returns `null`

#### Scenario: Wrong event type returns null
- **WHEN** a timeline event has `event_type: "decision"` with a wait-shaped payload
- **THEN** `getWaitDetails()` returns `null`

### Requirement: Effect Event Summary Text

The `summarizeTimelineEvent()` function SHALL produce structured summary text for `effect` events when `getEffectDetails()` returns a non-null result.

The summary format SHALL be `"{effectKind} (mutating)"` when `hasExternalSideEffect` is `true`, and `"{effectKind} (read-only)"` when `hasExternalSideEffect` is `false`.

When extraction fails, the function SHALL fall back to `event.message` if present, or `"effect"` otherwise.

#### Scenario: Effect with external side effect
- **WHEN** `summarizeTimelineEvent()` is called with an effect event where `getEffectDetails()` returns `{ effectKind: "api_call", hasExternalSideEffect: true }`
- **THEN** the summary returns `"api_call (mutating)"`

#### Scenario: Read-only effect
- **WHEN** `summarizeTimelineEvent()` is called with an effect event where `getEffectDetails()` returns `{ effectKind: "model_call", hasExternalSideEffect: false }`
- **THEN** the summary returns `"model_call (read-only)"`

#### Scenario: Effect extraction fails with message fallback
- **WHEN** `summarizeTimelineEvent()` is called with an effect event where `getEffectDetails()` returns `null` and `event.message` is "Custom effect note"
- **THEN** the summary returns `"Custom effect note"`

#### Scenario: Effect extraction fails without message
- **WHEN** `summarizeTimelineEvent()` is called with an effect event where `getEffectDetails()` returns `null` and `event.message` is undefined
- **THEN** the summary returns `"effect"`

### Requirement: Wait Event Summary Text

The `summarizeTimelineEvent()` function SHALL produce structured summary text for `wait` events when `getWaitDetails()` returns a non-null result.

The summary format SHALL be `"{waitKind} ({phase})"` when no resolution is present, and `"{waitKind} ({phase}) → {resolution}"` when a resolution is present.

When extraction fails, the function SHALL fall back to `event.message` if present, or `"wait"` otherwise.

#### Scenario: Wait with resolution
- **WHEN** `summarizeTimelineEvent()` is called with a wait event where `getWaitDetails()` returns `{ waitKind: "human_approval", phase: "resolved", resolution: "approved" }`
- **THEN** the summary returns `"human_approval (resolved) → approved"`

#### Scenario: Wait without resolution
- **WHEN** `summarizeTimelineEvent()` is called with a wait event where `getWaitDetails()` returns `{ waitKind: "human_approval", phase: "entered" }`
- **THEN** the summary returns `"human_approval (entered)"`

#### Scenario: Wait extraction fails
- **WHEN** `summarizeTimelineEvent()` is called with a wait event where `getWaitDetails()` returns `null`
- **THEN** the summary falls back to `event.message` or `"wait"`

### Requirement: Effect Preview Component

The `Timeline` component SHALL render an `EffectPreview` component for timeline events where `getEffectDetails()` returns a non-null result.

The preview SHALL display the effect kind as primary text and a badge indicating whether the effect is externally mutating or read-only based on `hasExternalSideEffect`. Opaque identifiers (`effectId`, `idempotencyKey`) and the `idempotent` flag SHALL remain accessible via the expanded payload panel and SHALL NOT be rendered as collapsed-row badges.

The preview SHALL be rendered in the `TimelineRow` chain after the `DecisionPreview` check and before the generic text fallback.

#### Scenario: Externally mutating effect
- **WHEN** a timeline row renders an effect event with `{ effectKind: "tool_call", hasExternalSideEffect: true }`
- **THEN** the row displays "tool_call" as the kind and a mutating badge

#### Scenario: Read-only effect
- **WHEN** a timeline row renders an effect event with `{ effectKind: "model_call", hasExternalSideEffect: false }`
- **THEN** the row displays "model_call" as the kind and a read-only badge

#### Scenario: Malformed effect payload degrades to generic rendering
- **WHEN** a timeline row renders an effect event where `getEffectDetails()` returns `null`
- **THEN** the row renders the generic text summary via `summarizeTimelineEvent()`

### Requirement: Wait Preview Component

The `Timeline` component SHALL render a `WaitPreview` component for timeline events where `getWaitDetails()` returns a non-null result.

The preview SHALL display the wait kind as primary text, a phase badge (e.g. `entered`, `resolved`), and the resolution as an accent pill when present. The `waitId` SHALL remain accessible via the expanded payload panel and SHALL NOT be rendered as a collapsed-row badge.

The preview SHALL be rendered in the `TimelineRow` chain after the `EffectPreview` check and before the generic text fallback.

#### Scenario: Wait resolved with resolution
- **WHEN** a timeline row renders a wait event with `{ waitKind: "human_approval", phase: "resolved", resolution: "approved" }`
- **THEN** the row displays "human_approval" kind, "resolved" phase badge, and "approved" resolution pill

#### Scenario: Wait entered without optional fields
- **WHEN** a timeline row renders a wait event with `{ waitKind: "human_approval", phase: "entered" }`
- **THEN** the row displays "human_approval" kind and "entered" phase badge, with no resolution pill

#### Scenario: Malformed wait payload degrades to generic rendering
- **WHEN** a timeline row renders a wait event where `getWaitDetails()` returns `null`
- **THEN** the row renders the generic text summary via `summarizeTimelineEvent()`

### Requirement: Existing Row Behavior Preservation

The addition of effect and wait preview rendering SHALL NOT alter the behavior of existing timeline row features.

Explicit vs synthetic row styling, expand/collapse details, raw payload inspection via JsonViewer, and span-jump navigation via `onSelectSpan` SHALL continue to function identically for all event types including effect and wait.

#### Scenario: Effect event row expands to show payload
- **WHEN** a user clicks "Show details" on an effect event row
- **THEN** the raw payload is displayed in the JsonViewer, identical to any other event type

#### Scenario: Wait event row navigates to span
- **WHEN** a wait event has a `span_id` that exists in the `spanIndex` and the user clicks the span button
- **THEN** the `onSelectSpan` callback is invoked with the span ID
