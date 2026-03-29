## ADDED Requirements

### Requirement: Cumulative Cost Strip
The execution waterfall SHALL display a fixed cumulative-cost strip between the time-axis header row and the virtualized row scroller.

The strip SHALL use the same two-column grid as the waterfall: the left cell contains a static "Cumulative cost" label; the right cell contains a non-interactive inline SVG step chart aligned to the existing time axis.

#### Scenario: Completed trace with cost-bearing spans
- **WHEN** a completed trace has spans with non-zero `cost_usd`
- **THEN** the cost strip renders a step chart where each step increment is anchored at the span's `ended_at` timestamp
- **AND** a cumulative total label is displayed at the final step

#### Scenario: Span missing ended_at uses started_at fallback
- **WHEN** a terminal span has `cost_usd` but no `ended_at`
- **THEN** the cost increment is anchored at the span's `started_at` timestamp

#### Scenario: Running spans excluded from finalized cost
- **WHEN** a trace is in RUNNING status and some spans are still running
- **THEN** running spans are excluded from the cumulative cost computation
- **AND** only currently terminal cost-bearing spans contribute to the strip

#### Scenario: Running trace partial cost updates
- **WHEN** a running trace receives updated span data from the existing polling refresh
- **THEN** the cost strip re-derives from the updated spans without additional polling

#### Scenario: Running trace partial indicator
- **WHEN** a trace is in RUNNING status and the cost strip is visible
- **THEN** the cumulative total label includes a "Partial" indicator to communicate that the shown total is not final

#### Scenario: Zero-cost trace
- **WHEN** no spans in the trace have `cost_usd` values
- **THEN** `buildTraceCostSeries()` returns `null` (not an empty array)
- **AND** the cost strip is hidden entirely and does not reserve vertical space

#### Scenario: Tied anchor timestamps
- **WHEN** two cost-bearing spans share the same terminal timestamp
- **THEN** `buildTraceCostSeries()` collapses them into a single combined step at that timestamp
- **AND** the SVG component receives pre-aggregated points and does not perform its own collapsing

### Requirement: Waterfall Row Cost Annotations
The execution waterfall label column SHALL display compact inline token count and cost annotations on the existing status/duration line for rows with non-zero token or cost data.

Annotations MUST fit within the existing uniform `WATERFALL_ROW_HEIGHT` (68px) budget. The waterfall row height SHALL remain uniform across all rows; no variable-height rows are introduced. These annotations are independent of the span tree's "Show metrics" toggle.

#### Scenario: Span with tokens and cost
- **WHEN** a waterfall row represents a span with non-zero `tokens_in`, `tokens_out`, or `cost_usd`
- **THEN** compact token and cost values are appended inline to the existing status/duration line in the label column
- **AND** the row height remains `WATERFALL_ROW_HEIGHT` (68px)

#### Scenario: Span with no cost or token data
- **WHEN** a waterfall row represents a span with no token or cost data
- **THEN** no annotation is rendered for that row and the status/duration line is unchanged

#### Scenario: Annotations visible when tree metrics toggle is off
- **WHEN** the span tree's "Show metrics" toggle is off
- **THEN** waterfall row annotations are still displayed independently

### Requirement: Cost Derivation Source
The cost strip and row annotations SHALL derive values exclusively from existing span `cost_usd`, `tokens_in`, and `tokens_out` fields. No inferred or synthetic cost events are introduced. No runtime mismatch detection against `trace.total_cost_usd` is performed.

#### Scenario: Cost derived from span fields only
- **WHEN** the waterfall renders cost surfaces
- **THEN** all values come from the in-memory span objects returned by existing queries
- **AND** no additional API calls or synthetic cost events are created
