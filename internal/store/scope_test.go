package store

import (
	"testing"

	"github.com/google/uuid"
)

func TestScopeNullableProjectFilter(t *testing.T) {
	projectID := uuid.New()

	bound := BoundScope(projectID).nullableProjectFilter()
	if !bound.Valid {
		t.Fatal("expected bound scope to yield a valid project filter")
	}
	if bound.Bytes != projectID {
		t.Fatalf("expected project filter %s, got %s", projectID, bound.Bytes)
	}

	unbounded := UnboundedScope().nullableProjectFilter()
	if unbounded.Valid {
		t.Fatal("expected unbounded scope to yield an invalid project filter")
	}
}
