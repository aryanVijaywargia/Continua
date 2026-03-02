-- Drop tables in reverse order of creation (respecting FK dependencies)
DROP TABLE IF EXISTS payloads;
DROP TABLE IF EXISTS ingest_batches;
DROP TABLE IF EXISTS span_events;
DROP TABLE IF EXISTS spans;
DROP TABLE IF EXISTS traces;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS projects;
