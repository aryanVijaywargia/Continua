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

func TestSeedDemoRequiresRealAgentRuns(t *testing.T) {
	repoRoot := repoRoot(t)

	seedScript := readFile(t, filepath.Join(repoRoot, "scripts", "seed-demo.sh"))
	assertContains(t, seedScript, "scripts/seed-engine-demo.sql")
	assertContains(t, seedScript, "OPENAI_API_KEY must be set")
	assertContains(t, seedScript, "CONTINUA_DEMO_AGENT_MODE=openai")
	assertContains(t, seedScript, "CONTINUA_DEMO_SEED_ENGINE_RUNS=1")

	fixtureSQL := readFile(t, filepath.Join(repoRoot, "scripts", "seed-engine-demo.sql"))
	for _, expected := range []string{
		"Demo engine runs and projected traces are created through /v1/engine/runs",
		"'agent.research'",
		"'agent.code_review'",
		"'agent.incident_response'",
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

func TestSeedEngineDemoSQLCleansStaleEngineRowsAndRegistersDefinitions(t *testing.T) {
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
	instanceID := uuid.New()
	runID := uuid.New()
	_, err = pool.Exec(ctx, `
		INSERT INTO engine.instances (id, project_id, instance_key, definition_name, status)
		VALUES ($1, $2, 'stale-demo-instance', 'agent.research', 'active')
	`, instanceID, projectID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO engine.runs (
			id,
			project_id,
			instance_id,
			run_number,
			definition_version,
			status,
			ready_at,
			root_run_id,
			child_depth
		)
		VALUES ($1, $2, $3, 1, 'v1', 'queued', NOW(), $1, 0)
	`, runID, projectID, instanceID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO engine.history (project_id, instance_id, run_id, sequence_no, event_type)
		VALUES ($1, $2, $3, 1, 'workflow.started')
	`, projectID, instanceID, runID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO traces (project_id, trace_id, name, status, engine_run_id)
		VALUES ($1, 'engine:' || $2::text, 'stale engine trace', 'running', $2::uuid)
	`, projectID, runID)
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

	var engineRowCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM traces WHERE project_id = $1 AND engine_run_id IS NOT NULL)
			+ (SELECT COUNT(*) FROM engine.history WHERE project_id = $1)
			+ (SELECT COUNT(*) FROM engine.runs WHERE project_id = $1)
			+ (SELECT COUNT(*) FROM engine.instances WHERE project_id = $1)
	`, projectID).Scan(&engineRowCount))
	assert.Equal(t, 0, engineRowCount)

	var nonEngineTraceCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM traces
		WHERE project_id = $1
		  AND trace_id = 'sdk-demo-trace'
		  AND engine_run_id IS NULL
	`, projectID).Scan(&nonEngineTraceCount))
	assert.Equal(t, 1, nonEngineTraceCount)

	var definitionCount int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM engine.definition_catalog
		WHERE definition_version = 'v1'
		  AND definition_name = ANY($1::text[])
	`, []string{"agent.research", "agent.code_review", "agent.incident_response"}).Scan(&definitionCount))
	assert.Equal(t, 3, definitionCount)
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
