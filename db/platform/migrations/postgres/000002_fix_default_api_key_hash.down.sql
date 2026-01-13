-- Revert to original (broken) api_key_hash
UPDATE projects
SET api_key_hash = 'default',
    updated_at = NOW()
WHERE id = '00000000-0000-0000-0000-000000000001';
