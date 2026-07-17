package store

import (
	"os"
	"strings"
	"testing"
)

func TestRunsQueriesHaveNoUnguardedStatusWrite(t *testing.T) {
	assertQueryMarkerAbsent(
		t,
		"../../db/queries/runs.sql",
		"-- name: UpdateRunStatus",
		"every run-state mutation must go through the CAS-guarded Transition*/Wake*/Claim* queries; an unguarded status write bypasses the claimed_by/status CAS discipline",
	)
}

func TestInboxQueriesHaveNoDeadClaimPath(t *testing.T) {
	assertQueryMarkerAbsent(
		t,
		"../../db/queries/inbox.sql",
		"-- name: ClaimNextInboxItem",
		"inbox consumption happens via ListPendingInboxByRun inside workflow activations; the standalone claim/lease path is dead surface",
	)
}

func assertQueryMarkerAbsent(t *testing.T, path, marker, invariant string) {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if strings.Contains(string(contents), marker) {
		t.Fatalf("%s contains %q: %s", path, marker, invariant)
	}
}
