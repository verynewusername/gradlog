-- 001_initial_schema.down.sql
-- Rollback script for initial schema.

-- Drop triggers first.
DROP TRIGGER IF EXISTS update_runs_updated_at ON runs;
DROP TRIGGER IF EXISTS update_experiments_updated_at ON experiments;
DROP TRIGGER IF EXISTS update_projects_updated_at ON projects;
DROP TRIGGER IF EXISTS update_users_updated_at ON users;

-- Drop the trigger function.
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop tables in reverse order of creation (respecting foreign keys).
DROP TABLE IF EXISTS artifact_chunks;
DROP TABLE IF EXISTS artifacts;
DROP TABLE IF EXISTS metrics;
DROP TABLE IF EXISTS runs;
DROP TABLE IF EXISTS experiments;
DROP TABLE IF EXISTS project_members;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS api_keys;
DROP TABLE IF EXISTS users;

-- Drop extensions.
DROP EXTENSION IF EXISTS "uuid-ossp";
