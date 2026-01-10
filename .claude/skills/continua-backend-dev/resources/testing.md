# Testing Guide

## Test Structure

```
pkg/
└── redaction/
    ├── redact.go
    └── redact_test.go      # Unit tests alongside code

internal/
└── api/
    ├── handlers.go
    └── handlers_test.go

testdata/                    # Golden test fixtures
└── traces/
    └── sample_trace.json
```

## Running Tests

```bash
make test              # All tests
make test-go           # Go tests only
make test-integration  # Integration tests (needs DB)
```

## Unit Tests

```go
// pkg/redaction/redact_test.go
func TestRedactAPIKey(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
    }{
        {
            name:     "redacts OpenAI key",
            input:    `{"api_key": "sk-abc123xyz"}`,
            expected: `{"api_key": "[REDACTED]"}`,
        },
        {
            name:     "no key present",
            input:    `{"message": "hello"}`,
            expected: `{"message": "hello"}`,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := Redact(tt.input)
            if got != tt.expected {
                t.Errorf("got %q, want %q", got, tt.expected)
            }
        })
    }
}
```

## Integration Tests

Use build tag to separate from unit tests:

```go
//go:build integration

package api_test

import (
    "testing"
    "github.com/continua-ai/continua/internal/api"
)

func TestListTraces_Integration(t *testing.T) {
    // Requires running database
    db := setupTestDB(t)
    defer db.Close()

    server := api.NewServer(db.Queries())

    // Insert test data
    // Make request
    // Assert response
}
```

Run with: `make test-integration`

## Golden Tests

For complex outputs, use golden files:

```go
func TestMapTraceToJSON(t *testing.T) {
    trace := createTestTrace()
    got, _ := json.MarshalIndent(mapTraceToAPI(trace), "", "  ")

    golden := filepath.Join("testdata", "trace_response.golden.json")

    if *update {
        os.WriteFile(golden, got, 0644)
        return
    }

    expected, _ := os.ReadFile(golden)
    if !bytes.Equal(got, expected) {
        t.Errorf("output mismatch, run with -update to update golden file")
    }
}
```

## HTTP Handler Tests

```go
func TestGetTrace(t *testing.T) {
    // Setup
    queries := &mockQueries{}
    server := NewServer(queries)

    // Create request
    req := httptest.NewRequest("GET", "/api/traces/123", nil)
    w := httptest.NewRecorder()

    // Execute
    server.GetTrace(w, req, "123")

    // Assert
    resp := w.Result()
    if resp.StatusCode != http.StatusOK {
        t.Errorf("got status %d, want 200", resp.StatusCode)
    }

    var trace Trace
    json.NewDecoder(resp.Body).Decode(&trace)
    if trace.Id != "123" {
        t.Errorf("got id %s, want 123", trace.Id)
    }
}
```

## Mocking SQLC

Create interface for testing:

```go
// internal/api/queries.go
type Querier interface {
    GetTrace(ctx context.Context, id uuid.UUID) (platform.Trace, error)
    ListTraces(ctx context.Context, params platform.ListTracesParams) ([]platform.Trace, error)
}

// Real implementation
type Server struct {
    queries Querier  // Interface, not concrete type
}

// In tests
type mockQueries struct{}

func (m *mockQueries) GetTrace(ctx context.Context, id uuid.UUID) (platform.Trace, error) {
    return platform.Trace{ID: id, Name: "test"}, nil
}
```

## Test Helpers

```go
// testutil/testutil.go
func MustParseUUID(s string) uuid.UUID {
    id, err := uuid.Parse(s)
    if err != nil {
        panic(err)
    }
    return id
}

func SetupTestDB(t *testing.T) *pgxpool.Pool {
    t.Helper()
    url := os.Getenv("TEST_DATABASE_URL")
    if url == "" {
        t.Skip("TEST_DATABASE_URL not set")
    }
    pool, err := pgxpool.New(context.Background(), url)
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { pool.Close() })
    return pool
}
```

## Coverage

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

No explicit threshold, but aim for coverage on critical paths (handlers, business logic).
