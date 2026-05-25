-- Deterministic engine fixtures for the public demo project.
--
-- This file is intentionally direct SQL instead of API calls. The public demo
-- should be inspectable after seeding without requiring a workflow worker.

DELETE FROM traces
WHERE (
    project_id = :'demo_project_id'::uuid
    AND (
      engine_run_id IS NOT NULL
      OR trace_id IN (
        'engine:20000000-0000-4000-8000-000000000001',
        'engine:20000000-0000-4000-8000-000000000002',
        'engine:20000000-0000-4000-8000-000000000003',
        'engine:demo:checkout-approval',
        'engine:demo:checkout-completed',
        'engine:demo:darklaunch-stale'
      )
    )
  )
  OR engine_run_id IN (
    '20000000-0000-4000-8000-000000000001'::uuid,
    '20000000-0000-4000-8000-000000000002'::uuid,
    '20000000-0000-4000-8000-000000000003'::uuid
  )
  OR trace_id IN (
    'engine:20000000-0000-4000-8000-000000000001',
    'engine:20000000-0000-4000-8000-000000000002',
    'engine:20000000-0000-4000-8000-000000000003',
    'engine:demo:checkout-approval',
    'engine:demo:checkout-completed',
    'engine:demo:darklaunch-stale'
  );

DELETE FROM engine.activity_tasks
WHERE project_id = :'demo_project_id'::uuid
  OR run_id IN (
    '20000000-0000-4000-8000-000000000001'::uuid,
    '20000000-0000-4000-8000-000000000002'::uuid,
    '20000000-0000-4000-8000-000000000003'::uuid
  )
  OR instance_id IN (
    '10000000-0000-4000-8000-000000000001'::uuid,
    '10000000-0000-4000-8000-000000000002'::uuid,
    '10000000-0000-4000-8000-000000000003'::uuid
  );

DELETE FROM engine.inbox
WHERE project_id = :'demo_project_id'::uuid
  OR run_id IN (
    '20000000-0000-4000-8000-000000000001'::uuid,
    '20000000-0000-4000-8000-000000000002'::uuid,
    '20000000-0000-4000-8000-000000000003'::uuid
  )
  OR instance_id IN (
    '10000000-0000-4000-8000-000000000001'::uuid,
    '10000000-0000-4000-8000-000000000002'::uuid,
    '10000000-0000-4000-8000-000000000003'::uuid
  );

DELETE FROM engine.request_dedupe
WHERE project_id = :'demo_project_id'::uuid
  OR run_id IN (
    '20000000-0000-4000-8000-000000000001'::uuid,
    '20000000-0000-4000-8000-000000000002'::uuid,
    '20000000-0000-4000-8000-000000000003'::uuid
  )
  OR instance_id IN (
    '10000000-0000-4000-8000-000000000001'::uuid,
    '10000000-0000-4000-8000-000000000002'::uuid,
    '10000000-0000-4000-8000-000000000003'::uuid
  );

DELETE FROM engine.child_workflows
WHERE project_id = :'demo_project_id'::uuid
  OR parent_instance_id IN (
    '10000000-0000-4000-8000-000000000001'::uuid,
    '10000000-0000-4000-8000-000000000002'::uuid,
    '10000000-0000-4000-8000-000000000003'::uuid
  )
  OR child_instance_id IN (
    '10000000-0000-4000-8000-000000000001'::uuid,
    '10000000-0000-4000-8000-000000000002'::uuid,
    '10000000-0000-4000-8000-000000000003'::uuid
  )
  OR parent_run_id IN (
    '20000000-0000-4000-8000-000000000001'::uuid,
    '20000000-0000-4000-8000-000000000002'::uuid,
    '20000000-0000-4000-8000-000000000003'::uuid
  )
  OR current_child_run_id IN (
    '20000000-0000-4000-8000-000000000001'::uuid,
    '20000000-0000-4000-8000-000000000002'::uuid,
    '20000000-0000-4000-8000-000000000003'::uuid
  )
  OR terminal_child_run_id IN (
    '20000000-0000-4000-8000-000000000001'::uuid,
    '20000000-0000-4000-8000-000000000002'::uuid,
    '20000000-0000-4000-8000-000000000003'::uuid
  );

DELETE FROM engine.history
WHERE project_id = :'demo_project_id'::uuid
  OR run_id IN (
    '20000000-0000-4000-8000-000000000001'::uuid,
    '20000000-0000-4000-8000-000000000002'::uuid,
    '20000000-0000-4000-8000-000000000003'::uuid
  )
  OR instance_id IN (
    '10000000-0000-4000-8000-000000000001'::uuid,
    '10000000-0000-4000-8000-000000000002'::uuid,
    '10000000-0000-4000-8000-000000000003'::uuid
  );

DELETE FROM engine.runs
WHERE project_id = :'demo_project_id'::uuid
  OR id IN (
    '20000000-0000-4000-8000-000000000001'::uuid,
    '20000000-0000-4000-8000-000000000002'::uuid,
    '20000000-0000-4000-8000-000000000003'::uuid
  )
  OR instance_id IN (
    '10000000-0000-4000-8000-000000000001'::uuid,
    '10000000-0000-4000-8000-000000000002'::uuid,
    '10000000-0000-4000-8000-000000000003'::uuid
  );

DELETE FROM engine.instances
WHERE project_id = :'demo_project_id'::uuid
  OR id IN (
    '10000000-0000-4000-8000-000000000001'::uuid,
    '10000000-0000-4000-8000-000000000002'::uuid,
    '10000000-0000-4000-8000-000000000003'::uuid
  );

INSERT INTO engine.definition_catalog (definition_name, definition_version, published_at, updated_at)
VALUES
  ('checkout', 'v1', NOW() - INTERVAL '2 days', NOW() - INTERVAL '2 days'),
  ('darklaunch.demo', 'v1', NOW() - INTERVAL '2 days', NOW() - INTERVAL '2 days')
ON CONFLICT (definition_name, definition_version) DO UPDATE
SET updated_at = EXCLUDED.updated_at;

INSERT INTO engine.instances (
    id,
    project_id,
    instance_key,
    definition_name,
    status,
    metadata,
    created_at,
    updated_at
)
VALUES
  (
    '10000000-0000-4000-8000-000000000001',
    :'demo_project_id'::uuid,
    'demo-checkout-approval',
    'checkout',
    'active',
    '{"demo": true, "scenario": "waiting_for_approval"}'::jsonb,
    NOW() - INTERVAL '55 minutes',
    NOW() - INTERVAL '2 minutes'
  ),
  (
    '10000000-0000-4000-8000-000000000002',
    :'demo_project_id'::uuid,
    'demo-checkout-completed',
    'checkout',
    'completed',
    '{"demo": true, "scenario": "completed_checkout"}'::jsonb,
    NOW() - INTERVAL '2 hours',
    NOW() - INTERVAL '85 minutes'
  ),
  (
    '10000000-0000-4000-8000-000000000003',
    :'demo_project_id'::uuid,
    'demo-darklaunch-stale',
    'darklaunch.demo',
    'completed',
    '{"demo": true, "scenario": "projection_repair"}'::jsonb,
    NOW() - INTERVAL '3 hours',
    NOW() - INTERVAL '150 minutes'
  );

INSERT INTO engine.runs (
    id,
    project_id,
    instance_id,
    run_number,
    definition_version,
    status,
    ready_at,
    result,
    custom_status,
    waiting_for,
    completed_at,
    root_run_id,
    child_depth,
    created_at,
    updated_at
)
VALUES
  (
    '20000000-0000-4000-8000-000000000001',
    :'demo_project_id'::uuid,
    '10000000-0000-4000-8000-000000000001',
    1,
    'v1',
    'waiting',
    NOW() - INTERVAL '55 minutes',
    NULL,
    '{"phase": "awaiting approval", "cart_total": 129.99}'::jsonb,
    '{"kind": "signal", "signal_name": "approval_received"}'::jsonb,
    NULL,
    '20000000-0000-4000-8000-000000000001',
    0,
    NOW() - INTERVAL '55 minutes',
    NOW() - INTERVAL '2 minutes'
  ),
  (
    '20000000-0000-4000-8000-000000000002',
    :'demo_project_id'::uuid,
    '10000000-0000-4000-8000-000000000002',
    1,
    'v1',
    'completed',
    NOW() - INTERVAL '2 hours',
    '{"order_id": "demo-order-1001", "approved": true, "total": 84.25}'::jsonb,
    '{"phase": "fulfilled"}'::jsonb,
    NULL,
    NOW() - INTERVAL '85 minutes',
    '20000000-0000-4000-8000-000000000002',
    0,
    NOW() - INTERVAL '2 hours',
    NOW() - INTERVAL '85 minutes'
  ),
  (
    '20000000-0000-4000-8000-000000000003',
    :'demo_project_id'::uuid,
    '10000000-0000-4000-8000-000000000003',
    1,
    'v1',
    'completed',
    NOW() - INTERVAL '3 hours',
    '{"variant": "treatment", "converted": true}'::jsonb,
    '{"phase": "summary retained"}'::jsonb,
    NULL,
    NOW() - INTERVAL '150 minutes',
    '20000000-0000-4000-8000-000000000003',
    0,
    NOW() - INTERVAL '3 hours',
    NOW() - INTERVAL '150 minutes'
  );

INSERT INTO engine.history (
    project_id,
    instance_id,
    run_id,
    sequence_no,
    event_type,
    payload,
    created_at
)
VALUES
  (
    :'demo_project_id'::uuid,
    '10000000-0000-4000-8000-000000000001',
    '20000000-0000-4000-8000-000000000001',
    1,
    'workflow.started',
    '{"definition_name": "checkout", "definition_version": "v1", "instance_key": "demo-checkout-approval", "input": {"cart_id": "demo-cart-42", "total": 129.99}}'::jsonb,
    NOW() - INTERVAL '55 minutes'
  ),
  (
    :'demo_project_id'::uuid,
    '10000000-0000-4000-8000-000000000001',
    '20000000-0000-4000-8000-000000000001',
    2,
    'activity.scheduled',
    '{"activity_key": "reserve-inventory", "activity_type": "inventory.reserve", "input": {"sku": "demo-sku-7", "quantity": 1}}'::jsonb,
    NOW() - INTERVAL '54 minutes'
  ),
  (
    :'demo_project_id'::uuid,
    '10000000-0000-4000-8000-000000000001',
    '20000000-0000-4000-8000-000000000001',
    3,
    'timer.scheduled',
    jsonb_build_object('timer_key', 'approval-timeout', 'due_at', to_jsonb(NOW() + INTERVAL '5 minutes')),
    NOW() - INTERVAL '53 minutes'
  ),
  (
    :'demo_project_id'::uuid,
    '10000000-0000-4000-8000-000000000002',
    '20000000-0000-4000-8000-000000000002',
    1,
    'workflow.started',
    '{"definition_name": "checkout", "definition_version": "v1", "instance_key": "demo-checkout-completed", "input": {"cart_id": "demo-cart-17", "total": 84.25}}'::jsonb,
    NOW() - INTERVAL '2 hours'
  ),
  (
    :'demo_project_id'::uuid,
    '10000000-0000-4000-8000-000000000002',
    '20000000-0000-4000-8000-000000000002',
    2,
    'activity.completed',
    '{"activity_key": "charge-card", "activity_type": "payments.charge", "output": {"charge_id": "demo-charge-1001", "approved": true}}'::jsonb,
    NOW() - INTERVAL '90 minutes'
  ),
  (
    :'demo_project_id'::uuid,
    '10000000-0000-4000-8000-000000000002',
    '20000000-0000-4000-8000-000000000002',
    3,
    'workflow.completed',
    '{"result": {"order_id": "demo-order-1001", "approved": true, "total": 84.25}}'::jsonb,
    NOW() - INTERVAL '85 minutes'
  ),
  (
    :'demo_project_id'::uuid,
    '10000000-0000-4000-8000-000000000003',
    '20000000-0000-4000-8000-000000000003',
    1,
    'workflow.started',
    '{"definition_name": "darklaunch.demo", "definition_version": "v1", "instance_key": "demo-darklaunch-stale", "input": {"account_id": "demo-account-9"}}'::jsonb,
    NOW() - INTERVAL '3 hours'
  ),
  (
    :'demo_project_id'::uuid,
    '10000000-0000-4000-8000-000000000003',
    '20000000-0000-4000-8000-000000000003',
    2,
    'custom_status.updated',
    '{"status": {"phase": "evaluating rollout", "percent": 25}}'::jsonb,
    NOW() - INTERVAL '170 minutes'
  ),
  (
    :'demo_project_id'::uuid,
    '10000000-0000-4000-8000-000000000003',
    '20000000-0000-4000-8000-000000000003',
    3,
    'workflow.completed',
    '{"result": {"variant": "treatment", "converted": true}}'::jsonb,
    NOW() - INTERVAL '150 minutes'
  );

INSERT INTO engine.activity_tasks (
    project_id,
    instance_id,
    run_id,
    history_id,
    activity_key,
    activity_type,
    input,
    available_at,
    execution_target,
    max_attempts
)
VALUES (
    :'demo_project_id'::uuid,
    '10000000-0000-4000-8000-000000000001',
    '20000000-0000-4000-8000-000000000001',
    (
      SELECT id
      FROM engine.history
      WHERE run_id = '20000000-0000-4000-8000-000000000001'
        AND sequence_no = 2
    ),
    'reserve-inventory',
    'inventory.reserve',
    '{"sku": "demo-sku-7", "quantity": 1}'::jsonb,
    NOW() - INTERVAL '54 minutes',
    'local',
    3
);

INSERT INTO engine.inbox (
    project_id,
    instance_id,
    run_id,
    history_id,
    kind,
    payload,
    available_at,
    dedupe_key
)
VALUES
  (
    :'demo_project_id'::uuid,
    '10000000-0000-4000-8000-000000000001',
    '20000000-0000-4000-8000-000000000001',
    (
      SELECT id
      FROM engine.history
      WHERE run_id = '20000000-0000-4000-8000-000000000001'
        AND sequence_no = 3
    ),
    'timer',
    jsonb_build_object('timer_key', 'approval-timeout', 'due_at', to_jsonb(NOW() + INTERVAL '5 minutes')),
    NOW() + INTERVAL '5 minutes',
    'demo-checkout-approval:approval-timeout'
  ),
  (
    :'demo_project_id'::uuid,
    '10000000-0000-4000-8000-000000000001',
    '20000000-0000-4000-8000-000000000001',
    NULL,
    'signal',
    '{"signal_name": "approval_received", "payload": {"approved": true}}'::jsonb,
    NOW() - INTERVAL '2 minutes',
    'demo-checkout-approval:approval-received'
  );

INSERT INTO traces (
    project_id,
    trace_id,
    name,
    user_id,
    tags,
    environment,
    release,
    metadata,
    input,
    output,
    status,
    start_time,
    end_time,
    duration_ms,
    total_spans,
    total_cost,
    error_count,
    total_tokens_in,
    total_tokens_out,
    engine_run_id,
    engine_instance_key,
    engine_run_status,
    engine_custom_status,
    engine_wait_state,
    engine_pending_activity_tasks,
    engine_pending_inbox_items,
    engine_definition_name,
    engine_definition_version,
    engine_parent_run_id,
    engine_root_run_id,
    engine_child_key,
    engine_child_depth,
    engine_projection_state,
    engine_latest_history_id,
    engine_last_projected_history_id,
    engine_projection_updated_at
)
VALUES
  (
    :'demo_project_id'::uuid,
    'engine:20000000-0000-4000-8000-000000000001',
    'checkout',
    'demo-user',
    ARRAY['engine', 'demo', 'checkout'],
    'demo',
    'public-demo',
    '{"demo": true, "scenario": "waiting_for_approval"}'::jsonb,
    '{"cart_id": "demo-cart-42", "total": 129.99}'::jsonb,
    NULL,
    'running',
    NOW() - INTERVAL '55 minutes',
    NULL,
    NULL,
    1,
    0,
    0,
    0,
    0,
    '20000000-0000-4000-8000-000000000001',
    'demo-checkout-approval',
    'waiting',
    '{"phase": "awaiting approval", "cart_total": 129.99}'::jsonb,
    '{"kind": "signal", "signal_name": "approval_received"}'::jsonb,
    1,
    2,
    'checkout',
    'v1',
    NULL,
    '20000000-0000-4000-8000-000000000001',
    NULL,
    0,
    'up_to_date',
    (
      SELECT MAX(id)
      FROM engine.history
      WHERE run_id = '20000000-0000-4000-8000-000000000001'
    ),
    (
      SELECT MAX(id)
      FROM engine.history
      WHERE run_id = '20000000-0000-4000-8000-000000000001'
    ),
    NOW() - INTERVAL '2 minutes'
  ),
  (
    :'demo_project_id'::uuid,
    'engine:20000000-0000-4000-8000-000000000002',
    'checkout',
    'demo-user',
    ARRAY['engine', 'demo', 'checkout'],
    'demo',
    'public-demo',
    '{"demo": true, "scenario": "completed_checkout"}'::jsonb,
    '{"cart_id": "demo-cart-17", "total": 84.25}'::jsonb,
    '{"order_id": "demo-order-1001", "approved": true, "total": 84.25}'::jsonb,
    'ok',
    NOW() - INTERVAL '2 hours',
    NOW() - INTERVAL '85 minutes',
    2100000,
    1,
    0,
    0,
    0,
    0,
    '20000000-0000-4000-8000-000000000002',
    'demo-checkout-completed',
    'completed',
    '{"phase": "fulfilled"}'::jsonb,
    NULL,
    0,
    0,
    'checkout',
    'v1',
    NULL,
    '20000000-0000-4000-8000-000000000002',
    NULL,
    0,
    'up_to_date',
    (
      SELECT MAX(id)
      FROM engine.history
      WHERE run_id = '20000000-0000-4000-8000-000000000002'
    ),
    (
      SELECT MAX(id)
      FROM engine.history
      WHERE run_id = '20000000-0000-4000-8000-000000000002'
    ),
    NOW() - INTERVAL '85 minutes'
  ),
  (
    :'demo_project_id'::uuid,
    'engine:20000000-0000-4000-8000-000000000003',
    'darklaunch.demo',
    'demo-user',
    ARRAY['engine', 'demo', 'projection-repair'],
    'demo',
    'public-demo',
    '{"demo": true, "scenario": "projection_repair"}'::jsonb,
    '{"account_id": "demo-account-9"}'::jsonb,
    '{"variant": "treatment", "converted": true}'::jsonb,
    'ok',
    NOW() - INTERVAL '3 hours',
    NOW() - INTERVAL '150 minutes',
    1800000,
    1,
    0,
    0,
    0,
    0,
    '20000000-0000-4000-8000-000000000003',
    'demo-darklaunch-stale',
    'completed',
    '{"phase": "summary retained"}'::jsonb,
    NULL,
    0,
    0,
    'darklaunch.demo',
    'v1',
    NULL,
    '20000000-0000-4000-8000-000000000003',
    NULL,
    0,
    'summary_only',
    (
      SELECT MAX(id)
      FROM engine.history
      WHERE run_id = '20000000-0000-4000-8000-000000000003'
    ),
    (
      SELECT MAX(id) - 1
      FROM engine.history
      WHERE run_id = '20000000-0000-4000-8000-000000000003'
    ),
    NOW() - INTERVAL '140 minutes'
  );
