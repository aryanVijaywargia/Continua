<!--
  Source: myclaude (development-essentials)
  Original Path: development-essentials/commands/optimize.md
  Adaptations: Go profiling tools (pprof), PostgreSQL optimization focus
-->
---
description: Perform performance optimization analysis for Go/PostgreSQL
argument-hint: [performance target or bottleneck description]
---

# Performance Optimization

## Context
- Performance target/bottleneck: $ARGUMENTS
- Relevant code and profiling data will be referenced using @ file syntax
- Current performance metrics and constraints will be analyzed

## Your Role
You are the Performance Optimization Coordinator leading systematic optimization:

1. **Profiling** - Measure execution time, memory usage, and resource consumption
2. **Algorithm Analysis** - Optimize time/space complexity and data structures
3. **Resource Management** - Optimize caching, batching, and resource allocation
4. **Database Optimization** - Query analysis, indexing, connection management

## Process

### 1. Performance Baseline
Establish current metrics using Go profiling tools:

```bash
# CPU profiling
go test -cpuprofile=cpu.prof -bench=. ./...
go tool pprof cpu.prof

# Memory profiling
go test -memprofile=mem.prof -bench=. ./...
go tool pprof mem.prof

# Trace analysis
go test -trace=trace.out ./...
go tool trace trace.out
```

### 2. Optimization Analysis

**Go Performance:**
- Identify hot paths in CPU profile
- Check for excessive allocations
- Review goroutine usage and synchronization
- Analyze garbage collection pressure

**Database Performance:**
```sql
-- Identify slow queries
SELECT query, calls, mean_time, total_time
FROM pg_stat_statements
ORDER BY mean_time DESC
LIMIT 20;

-- Check for missing indexes
SELECT schemaname, tablename, attname, n_distinct, correlation
FROM pg_stats
WHERE tablename = 'your_table';

-- Analyze query plans
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) SELECT ...;
```

**Resource Management:**
- Connection pool sizing
- Caching strategy and hit rates
- Batch processing opportunities
- Memory usage patterns

### 3. Solution Design
Create optimization strategy with measurable targets.

### 4. Impact Validation
Verify improvements don't compromise functionality or maintainability.

## Output Format

### 1. Performance Analysis
**Current Bottlenecks (quantified impact):**
- [Bottleneck 1]: [X ms / Y% of total time]
- [Bottleneck 2]: [X ms / Y% of total time]

### 2. Optimization Strategy
| Target | Current | Goal | Approach |
|--------|---------|------|----------|
| API latency p99 | 500ms | 100ms | Add caching, optimize query |
| Memory usage | 2GB | 500MB | Reduce allocations, use pools |
| Query time | 100ms | 10ms | Add index, rewrite query |

### 3. Implementation Plan
```markdown
## Priority 1: [Highest impact, lowest effort]
- Change: [Specific code/config change]
- Files: [file:line references]
- Expected impact: [Measurable improvement]

## Priority 2: [Next priority]
...
```

### 4. Measurement Framework
```bash
# Benchmark before/after
go test -bench=BenchmarkXxx -benchmem ./...

# Load testing
hey -n 1000 -c 100 http://localhost:8080/api/v1/traces
```

### 5. Verification Commands
```bash
make test-go
make lint-go
# Run specific benchmarks
go test -bench=. -benchmem ./internal/...
```

## Common Go/PostgreSQL Optimizations

### Go
- Use `sync.Pool` for frequently allocated objects
- Prefer `strings.Builder` over concatenation
- Use buffered channels appropriately
- Consider `sync.Map` for concurrent access patterns
- Profile before optimizing (avoid premature optimization)

### PostgreSQL
- Add indexes for WHERE, JOIN, and ORDER BY columns
- Use EXPLAIN ANALYZE to understand query plans
- Consider partial indexes for filtered queries
- Use connection pooling (pgbouncer or built-in)
- Batch INSERTs with COPY or multi-value INSERT
- Use prepared statements for repeated queries
