# Clipboard Utilities

Reusable copy infrastructure: a `CopyButton` component and a `clipboard.ts` helper.

## ADDED Requirements

### Requirement: Clipboard Helper

The system MUST provide a shared `clipboard.ts` utility that handles clipboard interaction and error fallback.

#### Scenario: Successful copy

Given the browser supports `navigator.clipboard.writeText`
When `copyToClipboard(text)` is called
Then the text is written to the clipboard and the function resolves successfully

#### Scenario: Clipboard API unavailable

Given the browser does not support `navigator.clipboard.writeText`
When `copyToClipboard(text)` is called
Then the function rejects with an error (no silent failure)

### Requirement: CopyButton Component

The system MUST provide a reusable `CopyButton` component that shows transient feedback after copying.

#### Scenario: Copy success feedback

Given a CopyButton with value "some-id"
When the user clicks the button
Then the clipboard receives "some-id" and the button shows a transient success indicator (e.g., checkmark) for ~2 seconds before reverting

#### Scenario: Copy failure feedback

Given a CopyButton and the clipboard write fails
When the user clicks the button
Then the button shows a transient error indicator before reverting

#### Scenario: Keyboard activation

Given focus on a CopyButton
When the user presses Enter or Space
Then the copy action triggers as if clicked

### Requirement: Copy Targets

The system MUST add copy controls for the following values on the trace detail page.

#### Scenario: Trace identifiers

Given a trace detail page
When the user views trace metadata
Then copy controls are available for: trace internal UUID, external trace ID, and session ID

#### Scenario: Span identifiers

Given a selected span
When the user views span detail
Then copy controls are available for: span external `span_id` and parent span ID (when present)

#### Scenario: Payload values

Given a PayloadInspector rendering a payload
When the user interacts with the inspector
Then copy controls are available for: full payload JSON, subtree JSON, and individual leaf values

#### Scenario: Trace URL

Given a trace detail page with or without a selected span
When the user clicks "Copy Trace URL"
Then an absolute URL is copied, constructed from the current trace route plus the effective selected span (if any), regardless of whether `?span=` is currently in the browser URL

#### Scenario: Copied URL preserves non-span params

Given the current URL is `/traces/123?debug=true` with effective selected span `abc`
When the user clicks "Copy Trace URL"
Then the copied URL includes both `debug=true` and `span=abc`
