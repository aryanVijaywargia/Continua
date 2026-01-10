<!--
  Source: Synthesized from taches-cc-resources and Claude-Code-Workflow patterns
  Type: example
  Description: Complete example of using subagent patterns for database migration
-->

# Example: Database Migration

This example demonstrates how to use the subagent patterns for adding a new database migration to Continua.

## Scenario
Add a new table `trace_spans` to store span data within traces, with proper foreign key relationships.

## Phase 1: Plan Review

### Review Migration Plan
```javascript
Task(
  subagent_type: "plan-reviewer",
  prompt: `
    Review the following database migration plan:

    ## Migration: Add trace_spans table

    ### Up Migration
    CREATE TABLE trace_spans (
      id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      trace_id UUID NOT NULL REFERENCES traces(id) ON DELETE CASCADE,
      parent_span_id UUID REFERENCES trace_spans(id),
      name VARCHAR(255) NOT NULL,
      start_time TIMESTAMPTZ NOT NULL,
      end_time TIMESTAMPTZ,
      status VARCHAR(50),
      attributes JSONB DEFAULT '{}',
      created_at TIMESTAMPTZ DEFAULT NOW()
    );

    CREATE INDEX idx_trace_spans_trace_id ON trace_spans(trace_id);
    CREATE INDEX idx_trace_spans_parent ON trace_spans(parent_span_id);

    ### Down Migration
    DROP TABLE IF EXISTS trace_spans;

    ### Validation
    - Foreign key to traces table exists
    - Self-referential FK for parent spans
    - Indexes for common query patterns

    Review for:
    - Schema correctness
    - Index strategy
    - Rollback safety
    - Data integrity
  `,
  description: "Review migration plan"
)
```

### Expected Plan Review Output
```json
{
  "executive_summary": "Migration is well-designed with proper FK constraints and indexes.",
  "viability": "APPROVE",
  "critical_issues": [],
  "missing_considerations": [
    "Consider adding index on start_time for time-range queries",
    "Add CHECK constraint for status values"
  ],
  "checklist_results": {
    "database": true,
    "migration_has_down": true,
    "no_data_loss": true,
    "indexes_for_queries": true
  }
}
```

## Phase 2: Segmented Execution with Checkpoints

### Execute Migration with Verification Points
```javascript
Task(
  subagent_type: "general-purpose",
  prompt: `
    Execute the database migration plan:

    <tasks>
      <task type="auto">
        <name>Create migration file</name>
        <action>
          Run: make migrate-create name=add_trace_spans
          Write UP migration to new file
          Write DOWN migration to new file
        </action>
        <verify>Migration file exists in db/platform/migrations/</verify>
      </task>

      <task type="checkpoint:human-verify" gate="blocking">
        <what-built>Migration files created</what-built>
        <how-to-verify>
          1. Check db/platform/migrations/ for new migration
          2. Review UP migration SQL
          3. Review DOWN migration SQL
          4. Verify file naming follows pattern
        </how-to-verify>
        <resume-signal>Type "approved" to apply migration</resume-signal>
      </task>

      <task type="auto">
        <name>Apply migration to dev database</name>
        <action>Run: make migrate-up</action>
        <verify>No errors in migration output</verify>
      </task>

      <task type="checkpoint:human-verify" gate="blocking">
        <what-built>Migration applied to development database</what-built>
        <how-to-verify>
          1. Connect: psql $DATABASE_URL
          2. Check: \d trace_spans
          3. Verify: Table matches expected schema
          4. Check: \di to verify indexes exist
        </how-to-verify>
        <resume-signal>Type "approved" if schema correct</resume-signal>
      </task>

      <task type="auto">
        <name>Test rollback</name>
        <action>
          Run: make migrate-down
          Verify table dropped
          Run: make migrate-up
          Verify table recreated
        </action>
        <verify>Both operations complete without error</verify>
      </task>

      <task type="auto">
        <name>Generate sqlc queries</name>
        <files>internal/store/trace_spans.sql</files>
        <action>
          Create queries: InsertSpan, GetSpansByTraceID, GetSpan
          Run: make generate
        </action>
        <verify>internal/store/trace_spans.sql.go generated</verify>
      </task>

      <task type="checkpoint:human-verify" gate="blocking">
        <what-built>Complete migration with generated queries</what-built>
        <how-to-verify>
          1. Run: make test-integration
          2. Verify: No failures
          3. Check: Generated code compiles
        </how-to-verify>
        <resume-signal>Type "approved" to complete</resume-signal>
      </task>
    </tasks>
  `,
  description: "Execute migration"
)
```

## Phase 3: Security Review

### Post-Implementation Security Check
```javascript
Task(
  subagent_type: "security-reviewer",
  prompt: `
    Review the new trace_spans table and queries for security:

    Files to review:
    - db/platform/migrations/XXXXXX_add_trace_spans.up.sql
    - internal/store/trace_spans.sql
    - internal/store/trace_spans.sql.go

    Focus on:
    - SQL injection in generated queries
    - Proper foreign key constraints
    - Data exposure through JSONB attributes field
    - Cascade delete implications
  `,
  description: "Security review migration"
)
```

## Handling Deviations

### Example: Table Already Exists
```xml
<deviation type="major">
  <expected>CREATE TABLE trace_spans</expected>
  <actual>Table trace_spans already exists (from previous attempt)</actual>
  <action>
    Verified schema matches spec.
    Changed to CREATE TABLE IF NOT EXISTS.
    Documented existing table discovery.
  </action>
  <risk>Low - schema matches expected</risk>
</deviation>
```

### Example: Index Strategy Change
```xml
<deviation type="minor">
  <expected>CREATE INDEX idx_trace_spans_trace_id</expected>
  <actual>Created composite index (trace_id, start_time) for query patterns</actual>
  <action>
    Adapted index strategy based on common query patterns.
    Improved performance for time-range queries within traces.
  </action>
  <risk>None - enhancement to original plan</risk>
</deviation>
```

### Example: Data Migration Required (Blocking)
```xml
<deviation type="blocking">
  <expected>Simple table creation</expected>
  <actual>Existing span data in legacy format needs migration</actual>
  <action>STOPPED - User decision required</action>
  <options>
    <option id="migrate">Create data migration script first</option>
    <option id="dual">Create new table, migrate data separately</option>
    <option id="abort">Cancel and redesign approach</option>
  </options>
</deviation>
```

## TodoWrite Tracking

```javascript
TodoWrite({
  todos: [
    {
      content: "Phase 1: Plan Review",
      status: "completed",
      activeForm: "Reviewing migration plan"
    },
    {
      content: "Phase 2: Create Migration",
      status: "in_progress",
      activeForm: "Creating migration"
    },
    {
      content: "  → Create migration file",
      status: "completed",
      activeForm: "Creating file"
    },
    {
      content: "  → CHECKPOINT: Verify migration file",
      status: "completed",
      activeForm: "Verified"
    },
    {
      content: "  → Apply to dev database",
      status: "completed",
      activeForm: "Applying migration"
    },
    {
      content: "  → CHECKPOINT: Verify schema",
      status: "in_progress",
      activeForm: "Awaiting verification"
    },
    {
      content: "  → Test rollback",
      status: "pending",
      activeForm: "Testing rollback"
    },
    {
      content: "  → Generate sqlc queries",
      status: "pending",
      activeForm: "Generating queries"
    },
    {
      content: "  → CHECKPOINT: Final verification",
      status: "pending",
      activeForm: "Final check"
    },
    {
      content: "Phase 3: Security Review",
      status: "pending",
      activeForm: "Security review"
    }
  ]
})
```

## Key Patterns Used

1. **Plan Reviewer**: Validates migration design before execution
2. **Segmented Pipeline**: Multiple checkpoints for verification
3. **Deviation Rules**: Handles unexpected situations
4. **Security Reviewer**: Post-implementation security check

## Files Created/Modified

```
db/platform/migrations/
  └── XXXXXX_add_trace_spans.up.sql   # UP migration
  └── XXXXXX_add_trace_spans.down.sql # DOWN migration

internal/store/
  └── trace_spans.sql      # Query definitions
  └── trace_spans.sql.go   # Generated code
```

## Verification Commands

```bash
# Verify migration applied
psql $DATABASE_URL -c "\d trace_spans"

# Verify indexes
psql $DATABASE_URL -c "\di *trace_spans*"

# Verify foreign keys
psql $DATABASE_URL -c "SELECT conname, conrelid::regclass, confrelid::regclass
FROM pg_constraint WHERE conrelid = 'trace_spans'::regclass"

# Run integration tests
make test-integration
```
