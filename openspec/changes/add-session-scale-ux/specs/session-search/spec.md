## ADDED Requirements

### Requirement: Session Search Ranking
Session search SHALL match against `external_id` and `name` using case-insensitive matching. Results SHALL be ranked in the following tiers: (1) exact `external_id` match, (2) `external_id` prefix match, (3) all other ILIKE matches, (4) `created_at DESC` as tiebreaker within each tier.

#### Scenario: Exact external_id match ranks first
- **WHEN** a session has `external_id = "conv-123"` and the user searches `q=conv-123`
- **THEN** that session appears before sessions with `external_id = "conv-1234"` or `name = "conv-123 discussion"`

#### Scenario: Prefix match on external_id ranks second
- **WHEN** sessions exist with `external_id` values `"conv-123"`, `"conv-1234"`, and `name = "conv-123 test"`
- **AND** the user searches `q=conv-123`
- **THEN** exact match `"conv-123"` ranks first
- **AND** prefix match `"conv-1234"` ranks second
- **AND** name-only match ranks third

#### Scenario: Name-only match ranks below external_id matches
- **WHEN** a session has `name = "test session"` but `external_id = "sess-001"`
- **AND** another session has `external_id = "test-session"` and `name = "other"`
- **AND** the user searches `q=test`
- **THEN** the `external_id` prefix match ranks above the name-only match

### Requirement: Session Search Simplicity
Session search SHALL NOT use trigram indexes, Postgres full-text search, or multi-tier scoring beyond the three tiers defined in the ranking requirement. The implementation SHALL use ILIKE matching with a CASE expression for tier ordering.

#### Scenario: Search implementation uses ILIKE
- **WHEN** a session search query is executed
- **THEN** the query uses ILIKE for matching against `external_id` and `name`
- **AND** uses a CASE expression in ORDER BY for tier-based ranking
- **AND** does not use `tsvector`, `tsquery`, or `pg_trgm` operators
