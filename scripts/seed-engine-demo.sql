-- Clean stale engine state for the public demo project.
--
-- The real-agent demo currently records provider-backed SDK traces only. Engine
-- run shells are intentionally not seeded here because catalog-only definitions
-- would stay queued until matching compiled workflow definitions exist.

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

DELETE FROM engine.definition_catalog
WHERE definition_version = 'v1'
  AND definition_name IN (
    'agent.research',
    'agent.code_review',
    'agent.incident_response'
  );
