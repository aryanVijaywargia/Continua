-- Fix default project API key hash
-- The original migration used unhashed "default" as api_key_hash,
-- but the auth middleware hashes API keys before lookup.
-- This updates the hash to SHA-256("default") so API key "default" works.

UPDATE projects
SET api_key_hash = '37a8eec1ce19687d132fe29051dca629d164e2c4958ba141d5f4133a33f0688f',
    updated_at = NOW()
WHERE id = '00000000-0000-0000-0000-000000000001'
  AND api_key_hash = 'default';
