<!--
  Source: Synthesized from Claude-Code-Workflow and taches-cc-resources patterns
  Type: example
  Description: Complete example of using subagent patterns for API endpoint generation
-->

# Example: API Endpoint Generation

This example demonstrates how to use the subagent patterns for adding a new API endpoint to Continua.

## Scenario
Add a new endpoint: `GET /api/v1/traces/{id}` to retrieve a single trace by ID.

## Phase 1: Planning with Plan Reviewer

### Invoke Plan Reviewer
```javascript
Task(
  subagent_type: "plan-reviewer",
  prompt: `
    Review the following implementation plan for adding GET /api/v1/traces/{id}:

    ## Planned Changes

    1. Update OpenAPI spec: contracts/api.yaml
       - Add GET /traces/{id} path
       - Define response schema

    2. Create database query: internal/store/traces.sql
       - Add GetTraceByID query
       - Generate with sqlc

    3. Implement handler: internal/api/traces.go
       - Add GetTrace function
       - Wire to router

    4. Add tests: internal/api/traces_test.go
       - Test happy path
       - Test 404 case
       - Test auth

    Verify against Continua's architecture patterns.
  `,
  description: "Review API plan"
)
```

### Expected Output
```json
{
  "executive_summary": "Plan is well-structured and follows Continua patterns.",
  "viability": "APPROVE",
  "critical_issues": [],
  "missing_considerations": [
    "Consider adding rate limiting for new endpoint"
  ],
  "recommendations": [
    "Add integration test for database layer"
  ],
  "checklist_results": {
    "database": true,
    "api": true,
    "go_code": true
  }
}
```

## Phase 2: Parallel Review with Fan-Out

### Launch Parallel Reviewers
After implementation, launch reviewers in parallel:

```javascript
// All in single message for parallel execution:

Task(
  subagent_type: "security-reviewer",
  prompt: `
    Review the new trace endpoint for security:
    - internal/api/traces.go
    - internal/store/traces.sql.go

    Focus on:
    - SQL injection prevention
    - Authorization checks
    - Data exposure risks
  `,
  description: "Security review"
)

Task(
  subagent_type: "code-reviewer",
  prompt: `
    Review the new trace endpoint for Go best practices:
    - internal/api/traces.go
    - internal/api/traces_test.go

    Focus on:
    - Error handling patterns
    - Test coverage
    - Code organization
  `,
  description: "Code quality review"
)
```

### Aggregate Results
```javascript
// After both agents complete:
aggregatedFindings = {
  critical: [],
  warnings: [
    {
      source: "security-reviewer",
      location: "internal/api/traces.go:45",
      issue: "Missing rate limit check",
      recommendation: "Add rate limiting middleware"
    }
  ],
  suggestions: [
    {
      source: "code-reviewer",
      issue: "Consider using table-driven tests",
      recommendation: "Refactor tests to use test cases slice"
    }
  ],
  strengths: [
    "Good parameterized query usage",
    "Consistent error handling"
  ]
}
```

## Phase 3: Segmented Execution with Checkpoints

### Execute Plan with Checkpoints
```javascript
Task(
  subagent_type: "general-purpose",
  prompt: `
    Execute the following plan with checkpoints:

    <tasks>
      <task type="auto">
        <name>Update OpenAPI spec</name>
        <files>contracts/api.yaml</files>
        <action>Add GET /traces/{id} endpoint definition</action>
        <verify>make generate succeeds</verify>
      </task>

      <task type="auto">
        <name>Create sqlc query</name>
        <files>internal/store/traces.sql</files>
        <action>Add GetTraceByID query</action>
        <verify>make generate creates traces.sql.go</verify>
      </task>

      <task type="checkpoint:human-verify" gate="blocking">
        <what-built>Generated query and types from contracts</what-built>
        <how-to-verify>
          1. Check contracts/api.yaml has new endpoint
          2. Check internal/store/traces.sql.go has GetTraceByID
          3. Verify types look correct
        </how-to-verify>
        <resume-signal>Type "approved" to continue</resume-signal>
      </task>

      <task type="auto">
        <name>Implement handler</name>
        <files>internal/api/traces.go</files>
        <action>Add GetTrace handler function</action>
        <verify>go build ./... succeeds</verify>
      </task>

      <task type="auto">
        <name>Add tests</name>
        <files>internal/api/traces_test.go</files>
        <action>Add unit tests for GetTrace</action>
        <verify>make test passes</verify>
      </task>

      <task type="checkpoint:human-verify" gate="blocking">
        <what-built>Complete trace endpoint with tests</what-built>
        <how-to-verify>
          1. Run: make test
          2. Run: curl localhost:8080/api/v1/traces/123
          3. Verify: Response format matches spec
        </how-to-verify>
        <resume-signal>Type "approved" if tests pass</resume-signal>
      </task>
    </tasks>
  `,
  description: "Implement trace endpoint"
)
```

## Complete TodoWrite Tracking

```javascript
TodoWrite({
  todos: [
    {
      content: "Phase 1: Plan Review",
      status: "completed",
      activeForm: "Reviewing implementation plan"
    },
    {
      content: "Phase 2: Implementation",
      status: "in_progress",
      activeForm: "Implementing trace endpoint"
    },
    {
      content: "  → Update OpenAPI spec",
      status: "completed",
      activeForm: "Updating contract"
    },
    {
      content: "  → Create sqlc query",
      status: "completed",
      activeForm: "Creating query"
    },
    {
      content: "  → CHECKPOINT: Verify generated code",
      status: "completed",
      activeForm: "Waiting for verification"
    },
    {
      content: "  → Implement handler",
      status: "in_progress",
      activeForm: "Implementing handler"
    },
    {
      content: "  → Add tests",
      status: "pending",
      activeForm: "Adding tests"
    },
    {
      content: "  → CHECKPOINT: Verify complete endpoint",
      status: "pending",
      activeForm: "Final verification"
    },
    {
      content: "Phase 3: Parallel Review",
      status: "pending",
      activeForm: "Running parallel reviews"
    },
    {
      content: "Phase 4: Aggregate & Report",
      status: "pending",
      activeForm: "Aggregating findings"
    }
  ]
})
```

## Key Patterns Used

1. **Plan Reviewer**: Validates plan before implementation
2. **Fan-Out Parallel**: Runs security and quality reviews simultaneously
3. **Aggregation**: Combines findings into unified report
4. **Segmented Pipeline**: Executes with verification checkpoints
5. **Deviation Rules**: Handles unexpected situations during execution

## Files Created/Modified

```
contracts/api.yaml           # OpenAPI spec (updated)
internal/store/traces.sql    # SQL query (created)
internal/store/traces.sql.go # Generated code (generated)
internal/api/traces.go       # Handler (created)
internal/api/traces_test.go  # Tests (created)
```
