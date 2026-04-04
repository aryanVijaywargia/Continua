package store

import (
	"testing"

	"github.com/google/uuid"
)

func uuidOrFatal(t *testing.T) uuid.UUID {
	t.Helper()

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("uuid.NewV7() error = %v", err)
	}
	return id
}
