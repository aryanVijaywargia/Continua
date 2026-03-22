## ADDED Requirements

### Requirement: Settings Page
A settings page at `/settings` SHALL allow users to view, update, and clear their API key. The page SHALL show the current key (masked) when set, provide an input to enter a new key, and a button to clear the key.

#### Scenario: View current API key
- **WHEN** a user navigates to `/settings` with an API key already set
- **THEN** the page shows the masked current key (e.g., "sk-...xxxx")

#### Scenario: Update API key
- **WHEN** a user enters a new API key and submits
- **THEN** `setApiKey` is called with the new value
- **AND** the page reflects the updated key

#### Scenario: Clear API key
- **WHEN** a user clicks the clear button
- **THEN** `clearApiKey` is called
- **AND** the page shows that no key is set

### Requirement: Settings Route and Navigation Link
The application SHALL include a `/settings` route accessible from the main navigation. The navigation bar SHALL include a "Settings" link.

#### Scenario: Navigate to settings via nav
- **WHEN** a user clicks "Settings" in the navigation
- **THEN** they are routed to `/settings`
- **AND** the Settings page renders

### Requirement: 401 Error Recovery
All pages that make authenticated API requests SHALL detect HTTP 401 responses and render a page-level auth-error banner with "Invalid or missing API key" and a "Go to Settings" link, instead of the generic error state. Non-401 errors SHALL continue to render using the existing generic error pattern. No toast system SHALL be introduced.

#### Scenario: 401 on a data page
- **WHEN** any authenticated API request returns a 401 response
- **THEN** the page renders an auth-error banner instead of the generic error state
- **AND** the banner includes a "Go to Settings" link

#### Scenario: Non-401 errors render normally
- **WHEN** the API returns a 500 response
- **THEN** the existing generic error rendering is used (no auth-error banner)
