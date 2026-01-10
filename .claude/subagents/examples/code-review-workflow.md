<!--
  Source: Synthesized from agentic-ai-systems and Claude-Code-Workflow patterns
  Type: example
  Description: Complete example of using parallel review workflow
-->

# Example: Code Review Workflow

This example demonstrates how to use the fan-out parallel pattern for comprehensive code review.

## Scenario
Review a new authentication module before merging a PR with changes to:
- `internal/api/auth.go` (new handler)
- `internal/store/users.sql.go` (new queries)
- `internal/middleware/jwt.go` (new middleware)

## Step 1: Launch Parallel Reviews

### Single Message with Multiple Task Calls
```javascript
// CRITICAL: All tasks in single message for parallel execution

Task(
  subagent_type: "security-reviewer",
  prompt: `
    Review the authentication module for security vulnerabilities:

    Files:
    - internal/api/auth.go
    - internal/store/users.sql.go
    - internal/middleware/jwt.go

    Focus on:
    - SQL injection in user queries
    - JWT validation correctness
    - Password handling (bcrypt)
    - Session management security
    - Rate limiting on auth endpoints
    - OWASP Top 10 compliance
  `,
  description: "Security review"
)

Task(
  subagent_type: "code-reviewer",
  prompt: `
    Review the authentication module for Go best practices:

    Files:
    - internal/api/auth.go
    - internal/store/users.sql.go
    - internal/middleware/jwt.go

    Focus on:
    - Error handling patterns
    - Context usage
    - Interface design
    - Code organization
    - Test coverage
    - Documentation
  `,
  description: "Code quality review"
)

Task(
  subagent_type: "plan-reviewer",
  prompt: `
    Review the authentication architecture:

    Files:
    - internal/api/auth.go
    - internal/store/users.sql.go
    - internal/middleware/jwt.go

    Focus on:
    - Follows Continua's layered architecture
    - Proper separation of concerns
    - Scalability considerations
    - Integration with existing patterns
  `,
  description: "Architecture review"
)
```

## Step 2: Collect Results

### Security Reviewer Output
```json
{
  "risk_level": "MEDIUM",
  "findings": [
    {
      "severity": "HIGH",
      "location": "internal/api/auth.go:78",
      "issue": "Missing rate limiting on login endpoint",
      "cwe": "CWE-307",
      "recommendation": "Add rate limiting middleware to prevent brute force attacks"
    },
    {
      "severity": "MEDIUM",
      "location": "internal/middleware/jwt.go:34",
      "issue": "JWT expiry time of 24h may be too long",
      "cwe": "CWE-613",
      "recommendation": "Consider reducing to 1-4 hours with refresh tokens"
    },
    {
      "severity": "LOW",
      "location": "internal/api/auth.go:92",
      "issue": "Generic error message may aid enumeration",
      "cwe": "CWE-200",
      "recommendation": "Use same message for both invalid user and wrong password"
    }
  ],
  "attack_vectors": [
    "Brute force password attacks (no rate limiting)",
    "Token theft window extended by long expiry"
  ],
  "mitigations": [
    "Add rate limiting middleware",
    "Implement refresh token pattern",
    "Add account lockout after failed attempts"
  ]
}
```

### Code Reviewer Output
```json
{
  "review_summary": "Good implementation with some areas for improvement",
  "critical_issues": [],
  "warnings": [
    {
      "file": "internal/api/auth.go",
      "line": 45,
      "issue": "Error not wrapped with context",
      "fix": "Use fmt.Errorf(\"failed to authenticate: %w\", err)"
    },
    {
      "file": "internal/store/users.sql.go",
      "line": 23,
      "issue": "Missing context timeout",
      "fix": "Add context.WithTimeout for database operations"
    }
  ],
  "suggestions": [
    {
      "file": "internal/api/auth.go",
      "issue": "Consider using table-driven tests",
      "improvement": "Refactor TestLogin to use test cases slice"
    }
  ],
  "strengths": [
    "Good separation between handler and store layers",
    "Consistent error handling patterns",
    "Well-documented public functions"
  ]
}
```

### Plan Reviewer Output
```json
{
  "executive_summary": "Implementation follows Continua architecture patterns well.",
  "viability": "APPROVE",
  "missing_considerations": [
    "Consider adding audit logging for auth events",
    "May need caching layer for user lookups"
  ],
  "recommendations": [
    "Add integration test for full auth flow",
    "Document authentication flow in docs/"
  ]
}
```

## Step 3: Aggregate Findings

### Aggregation Logic
```javascript
// Normalize all findings to common format
const allFindings = [
  ...securityFindings.map(f => ({ ...f, source: "security", category: "security" })),
  ...qualityFindings.map(f => ({ ...f, source: "quality", category: "quality" })),
  ...archFindings.map(f => ({ ...f, source: "architecture", category: "architecture" }))
];

// Deduplicate by location
const deduplicated = deduplicateByLocation(allFindings);

// Prioritize
const prioritized = prioritize(deduplicated, {
  order: ["CRITICAL", "HIGH", "MEDIUM", "LOW"],
  categoryPriority: ["security", "quality", "architecture"]
});
```

### Unified Report
```json
{
  "summary": {
    "total_findings": 6,
    "critical": 0,
    "high": 1,
    "medium": 2,
    "low": 1,
    "suggestions": 2,
    "sources": ["security-reviewer", "code-reviewer", "plan-reviewer"]
  },

  "findings_by_priority": [
    {
      "severity": "HIGH",
      "category": "security",
      "location": "internal/api/auth.go:78",
      "issue": "Missing rate limiting on login endpoint",
      "recommendation": "Add rate limiting middleware to prevent brute force attacks",
      "source": "security-reviewer"
    },
    {
      "severity": "MEDIUM",
      "category": "security",
      "location": "internal/middleware/jwt.go:34",
      "issue": "JWT expiry time of 24h may be too long",
      "recommendation": "Consider reducing to 1-4 hours with refresh tokens",
      "source": "security-reviewer"
    },
    {
      "severity": "MEDIUM",
      "category": "quality",
      "location": "internal/api/auth.go:45",
      "issue": "Error not wrapped with context",
      "recommendation": "Use fmt.Errorf(\"failed to authenticate: %w\", err)",
      "source": "code-reviewer"
    }
  ],

  "strengths": [
    "Good separation between handler and store layers",
    "Consistent error handling patterns",
    "Well-documented public functions",
    "Follows Continua architecture patterns"
  ],

  "action_items": [
    {
      "priority": 1,
      "action": "Add rate limiting middleware to auth endpoints",
      "assignee": "Security team",
      "blocking": true
    },
    {
      "priority": 2,
      "action": "Implement refresh token pattern",
      "assignee": "Backend team",
      "blocking": false
    },
    {
      "priority": 3,
      "action": "Add context to error wrapping",
      "assignee": "PR author",
      "blocking": false
    }
  ],

  "verdict": "NEEDS_CHANGES",
  "blocking_issues": 1,
  "merge_when": "After addressing rate limiting (HIGH severity)"
}
```

## TodoWrite Tracking

```javascript
TodoWrite({
  todos: [
    {
      content: "Launch parallel reviewers",
      status: "completed",
      activeForm: "Launching reviewers"
    },
    {
      content: "  → Security Review [parallel]",
      status: "completed",
      activeForm: "Security review complete"
    },
    {
      content: "  → Code Quality Review [parallel]",
      status: "completed",
      activeForm: "Quality review complete"
    },
    {
      content: "  → Architecture Review [parallel]",
      status: "completed",
      activeForm: "Architecture review complete"
    },
    {
      content: "Aggregate findings",
      status: "completed",
      activeForm: "Aggregating findings"
    },
    {
      content: "Generate unified report",
      status: "completed",
      activeForm: "Report generated"
    },
    {
      content: "Present verdict",
      status: "completed",
      activeForm: "NEEDS_CHANGES - 1 blocking issue"
    }
  ]
})
```

## Key Patterns Used

1. **Fan-Out Parallel**: Three reviewers running simultaneously
2. **Aggregation**: Combining findings with deduplication and prioritization
3. **Expert Agents**: Security, quality, and architecture specialists

## Performance Benefits

| Execution Model | Time |
|----------------|------|
| Sequential (3 reviews) | ~9 minutes |
| Parallel (3 reviews) | ~3 minutes |
| **Speedup** | **3x** |

## When to Use This Pattern

- **PR reviews**: Multiple aspects need checking
- **Security audits**: Cross-reference multiple perspectives
- **Pre-release checks**: Quality, security, and architecture validation
- **Code archaeology**: Multiple exploration angles
