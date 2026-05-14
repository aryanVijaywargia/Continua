# Capability: engine-runtime-config

Env-only configuration for engine runtime workers, leases, and request deduplication. Extends [engine-cli-foundation](../../../../changes/add-engine-foundation/specs/engine-cli-foundation/spec.md).

Related capabilities: [engine-runtime-execution](../engine-runtime-execution/spec.md), [engine-cli-runtime](../engine-cli-runtime/spec.md)

## ADDED Requirements

### Requirement: Worker poll intervals

The engine config MUST provide configurable poll intervals for each worker loop.

#### Scenario: Workflow poll interval
- **WHEN** `ENGINE_WORKFLOW_POLL_INTERVAL` is set
- **THEN** the workflow worker polls at the specified interval

#### Scenario: Activity poll interval
- **WHEN** `ENGINE_ACTIVITY_POLL_INTERVAL` is set
- **THEN** the activity worker polls at the specified interval

#### Scenario: Maintenance poll interval
- **WHEN** `ENGINE_MAINTENANCE_POLL_INTERVAL` is set
- **THEN** the maintenance worker polls at the specified interval

#### Scenario: Default poll intervals
- **WHEN** poll interval env vars are not set
- **THEN** reasonable defaults are used (e.g., 1s for workflow/activity, 10s for maintenance)

---

### Requirement: Lease TTL configuration

The engine config MUST provide configurable lease TTLs for run claims and activity task claims.

#### Scenario: Run lease TTL
- **WHEN** `ENGINE_RUN_LEASE_TTL` is set
- **THEN** `ClaimNextRun` uses the specified duration as the lease expiry

#### Scenario: Activity lease TTL
- **WHEN** `ENGINE_ACTIVITY_LEASE_TTL` is set
- **THEN** `ClaimNextActivityTask` uses the specified duration as the lease expiry

#### Scenario: Default lease TTLs
- **WHEN** lease TTL env vars are not set
- **THEN** reasonable defaults are used (e.g., 30s for runs, 5m for activities)

---

### Requirement: Request dedupe TTL

The engine config MUST provide `ENGINE_REQUEST_DEDUPE_TTL` to control the expiry window for request deduplication entries.

#### Scenario: Custom dedupe TTL
- **WHEN** `ENGINE_REQUEST_DEDUPE_TTL` is set to `10m`
- **THEN** new request dedupe entries are created with `expires_at = NOW() + 10m`

#### Scenario: Default dedupe TTL
- **WHEN** `ENGINE_REQUEST_DEDUPE_TTL` is not set
- **THEN** a reasonable default is used (e.g., 1h)
