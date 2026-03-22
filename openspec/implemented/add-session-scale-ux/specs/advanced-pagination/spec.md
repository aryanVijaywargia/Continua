## ADDED Requirements

### Requirement: Shared Advanced Paginator
A shared advanced pagination component SHALL replace the current minimal `PaginationControls` and be used by `/traces`, `/sessions`, and session-detail trace tables. It SHALL provide First, Previous, Next, and Last navigation buttons, current page number, total pages, a page-size selector, and a "showing X-Y of Z" label.

#### Scenario: Full pagination controls render
- **WHEN** a list view renders the advanced paginator with `total=150`, `limit=20`, `offset=0`
- **THEN** the paginator displays "Showing 1-20 of 150", "Page 1 of 8"
- **AND** First and Previous buttons are disabled
- **AND** Next and Last buttons are enabled
- **AND** a page-size selector shows options 20, 50, 100

#### Scenario: Navigate to last page
- **WHEN** the user clicks the Last button with `total=150`, `limit=20`, `offset=0`
- **THEN** the offset updates to `140` (last page)
- **AND** the paginator displays "Showing 141-150 of 150", "Page 8 of 8"
- **AND** Next and Last buttons are disabled

### Requirement: Page Size Selector
The paginator SHALL offer page sizes of 20, 50, and 100 in the UI selector. The UI default page size SHALL be 20. Changing the page size SHALL reset the offset to 0. These are UI-only choices; the backend API continues to accept any `limit` within the existing server-wide range (up to `maxPageLimit=200`).

#### Scenario: Change page size resets offset
- **WHEN** the user is on page 3 (offset=40, limit=20) and changes page size to 50
- **THEN** the offset resets to 0
- **AND** the URL updates with `limit=50&offset=0`

### Requirement: Previous Row Preservation
List views using the advanced paginator SHALL preserve previous rows during refetch so that the table does not flash empty while new data loads.

#### Scenario: Data preserved during refetch
- **WHEN** the user changes the sort order on a list view
- **THEN** the previous rows remain visible until the new data arrives
- **AND** the table then updates with the new data

### Requirement: Stale Offset Repair
When the total shrinks such that the current offset exceeds the total, the paginator SHALL automatically repair the offset to the last valid page.

#### Scenario: Offset exceeds new total
- **WHEN** the user is at `offset=80` with `limit=20` and the total shrinks from 100 to 60
- **THEN** the offset automatically repairs to `40` (last valid page)
- **AND** the URL updates to reflect the repaired offset
