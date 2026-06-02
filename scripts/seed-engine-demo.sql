-- Prepare engine state for the public demo project.
--
-- Demo engine runs and projected traces are created through /v1/engine/runs by
-- sdks/python/examples/e2e_demo.py. This SQL only removes previous demo rows
-- that are not covered by project deletion and registers the demo definitions
-- required by the engine start API.

DELETE FROM traces
WHERE project_id = :'demo_project_id'::uuid
  AND (
    engine_run_id IS NOT NULL
    OR trace_id LIKE 'engine:%'
  );

DELETE FROM engine.activity_tasks
WHERE project_id = :'demo_project_id'::uuid
   OR run_id IN (
     SELECT id FROM engine.runs WHERE project_id = :'demo_project_id'::uuid
   )
   OR instance_id IN (
     SELECT id FROM engine.instances WHERE project_id = :'demo_project_id'::uuid
   );

DELETE FROM engine.inbox
WHERE project_id = :'demo_project_id'::uuid
   OR run_id IN (
     SELECT id FROM engine.runs WHERE project_id = :'demo_project_id'::uuid
   )
   OR instance_id IN (
     SELECT id FROM engine.instances WHERE project_id = :'demo_project_id'::uuid
   );

DELETE FROM engine.request_dedupe
WHERE project_id = :'demo_project_id'::uuid
   OR run_id IN (
     SELECT id FROM engine.runs WHERE project_id = :'demo_project_id'::uuid
   )
   OR instance_id IN (
     SELECT id FROM engine.instances WHERE project_id = :'demo_project_id'::uuid
   );

DELETE FROM engine.child_workflows
WHERE project_id = :'demo_project_id'::uuid
   OR parent_run_id IN (
     SELECT id FROM engine.runs WHERE project_id = :'demo_project_id'::uuid
   )
   OR current_child_run_id IN (
     SELECT id FROM engine.runs WHERE project_id = :'demo_project_id'::uuid
   )
   OR terminal_child_run_id IN (
     SELECT id FROM engine.runs WHERE project_id = :'demo_project_id'::uuid
   )
   OR parent_instance_id IN (
     SELECT id FROM engine.instances WHERE project_id = :'demo_project_id'::uuid
   )
   OR child_instance_id IN (
     SELECT id FROM engine.instances WHERE project_id = :'demo_project_id'::uuid
   );

DELETE FROM engine.history
WHERE project_id = :'demo_project_id'::uuid
   OR run_id IN (
     SELECT id FROM engine.runs WHERE project_id = :'demo_project_id'::uuid
   )
   OR instance_id IN (
     SELECT id FROM engine.instances WHERE project_id = :'demo_project_id'::uuid
   );

DELETE FROM engine.runs
WHERE project_id = :'demo_project_id'::uuid;

DELETE FROM engine.instances
WHERE project_id = :'demo_project_id'::uuid;

INSERT INTO engine.definition_catalog (definition_name, definition_version, published_at, updated_at)
VALUES
  ('agent.research', 'v1', NOW(), NOW()),
  ('agent.code_review', 'v1', NOW(), NOW()),
  ('agent.incident_response', 'v1', NOW(), NOW())
ON CONFLICT (definition_name, definition_version) DO UPDATE
SET updated_at = EXCLUDED.updated_at;
