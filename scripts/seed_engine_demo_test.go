package scripts_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/internal/testutil"
)

func TestSeedDemoIncludesEngineFixtures(t *testing.T) {
	repoRoot := repoRoot(t)

	seedScript := readFile(t, filepath.Join(repoRoot, "scripts", "seed-demo.sh"))
	assertContains(t, seedScript, "scripts/seed-engine-demo.sql")

	fixtureSQL := readFile(t, filepath.Join(repoRoot, "scripts", "seed-engine-demo.sql"))
	for _, expected := range []string{
		"engine:20000000-0000-4000-8000-000000000001",
		"engine:20000000-0000-4000-8000-000000000002",
		"engine:20000000-0000-4000-8000-000000000003",
		"'summary_only'",
		"demo-checkout-approval:approval-timeout",
		"approval_received",
	} {
		assertContains(t, fixtureSQL, expected)
	}
}

func TestSeedDemoScriptSyntax(t *testing.T) {
	repoRoot := repoRoot(t)

	cmd := exec.Command("bash", "-n", filepath.Join(repoRoot, "scripts", "seed-demo.sh"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("seed-demo.sh has invalid shell syntax: %v\n%s", err, output)
	}
}

func TestSeedEngineDemoSQLCreatesUsableEngineFixtures(t *testing.T) {
	if _, err := exec.LookPath("psql"); err != nil {
		t.Skipf("psql is required for seed fixture smoke test: %v", err)
	}

	repoRoot := repoRoot(t)
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = testutil.DefaultTestDBURL
	}
	pool := testutil.TestDB(t)
	ctx := context.Background()
	projectID := uuid.New()

	_, err := pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO projects (id, name, api_key_hash)
		VALUES ($1, 'engine demo fixture smoke', 'engine-demo-fixture-smoke-key')
	`, projectID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO traces (project_id, trace_id, name, status)
		VALUES ($1, 'sdk-demo-trace', 'sdk demo trace', 'ok')
	`, projectID)
	require.NoError(t, err)

	cmd := exec.Command(
		"psql",
		dbURL,
		"-v",
		"ON_ERROR_STOP=1",
		"-v",
		"demo_project_id="+projectID.String(),
		"-f",
		filepath.Join(repoRoot, "scripts", "seed-engine-demo.sql"),
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))

	var engineTraceCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM traces
		WHERE project_id = $1
		  AND engine_run_id IS NOT NULL
	`, projectID).Scan(&engineTraceCount))
	assert.Equal(t, 3, engineTraceCount)

	var canonicalTraceIDCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM traces
		WHERE project_id = $1
		  AND engine_run_id IS NOT NULL
		  AND trace_id = 'engine:' || engine_run_id::text
	`, projectID).Scan(&canonicalTraceIDCount))
	assert.Equal(t, 3, canonicalTraceIDCount)

	var repairCandidateCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM traces AS t
		INNER JOIN engine.runs AS r
		    ON r.project_id = t.project_id
		   AND r.id = t.engine_run_id
		INNER JOIN LATERAL (
		    SELECT COALESCE(MAX(h.id), 0)::bigint AS latest_retained_history_id
		    FROM engine.history AS h
		    WHERE h.run_id = r.id
		) AS hist ON true
		WHERE t.project_id = $1
		  AND t.engine_projection_state = 'summary_only'
		  AND hist.latest_retained_history_id > COALESCE(t.engine_last_projected_history_id, 0)
	`, projectID).Scan(&repairCandidateCount))
	assert.Equal(t, 1, repairCandidateCount)

	var nonEngineTraceCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM traces
		WHERE project_id = $1
		  AND trace_id = 'sdk-demo-trace'
		  AND engine_run_id IS NULL
	`, projectID).Scan(&nonEngineTraceCount))
	assert.Equal(t, 1, nonEngineTraceCount)
}

func repoRoot(t *testing.T) string {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	return filepath.Dir(cwd)
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()

	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected file contents to include %q", needle)
	}
}
