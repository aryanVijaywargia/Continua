## ADDED Requirements

### Requirement: Trace Context Section

The trace detail page SHALL display a Trace Context section with identity and environment fields.

#### Scenario: Context section rendered above spans
- **WHEN** a trace detail page is loaded
- **THEN** a full-width Trace Context section appears above the spans/detail split

#### Scenario: Internal and external IDs displayed
- **WHEN** a trace has both internal UUID and external `trace_id`
- **THEN** both are displayed, with the external ID labeled "External Trace ID"

#### Scenario: Session ID links to session page
- **WHEN** a trace has a `session_id`
- **THEN** it is rendered as a clickable link to `/sessions/{id}`

#### Scenario: Tags rendered as chips
- **WHEN** a trace has `tags: ["prod", "v2"]`
- **THEN** tags are rendered as chip/badge elements

#### Scenario: Tags show dash when absent
- **WHEN** a trace has no tags
- **THEN** a `-` placeholder is displayed

#### Scenario: Environment and release displayed
- **WHEN** a trace has `environment` and/or `release`
- **THEN** they are displayed as labeled rows in the context section

#### Scenario: User ID displayed
- **WHEN** a trace has `user_id`
- **THEN** it is displayed as a labeled row in the context section

#### Scenario: Missing context values show dash
- **WHEN** any context field (`user_id`, `environment`, `release`, `trace_id`) is absent
- **THEN** the row is still rendered with `-` as the value (rows are never hidden)

### Requirement: Trace Input/Output Display

The trace detail page SHALL render trace-level input and output payloads.

#### Scenario: Input rendered when present
- **WHEN** a trace has `input !== undefined`
- **THEN** it is rendered in a `JsonViewer` panel labeled "Input"

#### Scenario: Output rendered when present
- **WHEN** a trace has `output !== undefined`
- **THEN** it is rendered in a `JsonViewer` panel labeled "Output"

#### Scenario: Falsy values not suppressed
- **WHEN** a trace has `input = 0` or `input = false` or `input = null`
- **THEN** the input panel is still rendered (presence check, not truthiness)

### Requirement: LLM Context Block in Span Detail

The span detail panel SHALL show model and provider for LLM spans.

#### Scenario: LLM Context shown for LLM spans
- **WHEN** the selected span has `kind === 'LLM'` and at least one of `model` or `provider` is present
- **THEN** an "LLM Context" block is rendered above the Input section

#### Scenario: Model and provider displayed
- **WHEN** an LLM span has `model = "gpt-4o"` and `provider = "openai"`
- **THEN** both are displayed as labeled rows

#### Scenario: Missing model or provider shows dash
- **WHEN** an LLM span has `model` but not `provider` (or vice versa)
- **THEN** the present field is displayed and the missing field shows `-`

#### Scenario: LLM Context hidden for non-LLM spans
- **WHEN** the selected span has `kind !== 'LLM'`
- **THEN** no LLM Context block is rendered

#### Scenario: LLM Context hidden when no model or provider
- **WHEN** the selected span has `kind === 'LLM'` but both `model` and `provider` are undefined
- **THEN** no LLM Context block is rendered

### Requirement: TraceDetail Client Type

The web client SHALL define a `TraceDetail` type extending `Trace` for detail page use.

#### Scenario: TraceDetail extends Trace
- **WHEN** `TraceDetail` is defined in `client.ts`
- **THEN** it extends the `Trace` interface with `trace_id`, `user_id`, `tags`, `environment`, `release`, `input`, `output`

#### Scenario: fetchTrace returns TraceDetail
- **WHEN** `fetchTrace(id)` is called
- **THEN** it returns `TraceDetail`

#### Scenario: List functions return Trace
- **WHEN** `fetchTraces` or `fetchTracesBySession` is called
- **THEN** they return `Trace` (not `TraceDetail`)

### Requirement: JsonValue Type

The web client SHALL define a `JsonValue` type for arbitrary JSON values.

#### Scenario: JsonValue covers all JSON types
- **WHEN** `JsonValue` is defined
- **THEN** it is a union of `string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue }`

#### Scenario: JsonValue used for input/output
- **WHEN** `Span.input`, `Span.output`, `TraceDetail.input`, `TraceDetail.output` are typed
- **THEN** they use `JsonValue` (or `JsonValue | undefined` for optional)
