-- 001_initial_schema.up.sql
-- Initial database schema for Gradlog ML experiment tracking platform.

-- Enable UUID extension for generating unique identifiers.
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Users table stores registered users authenticated via Google OAuth.
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    picture_url TEXT,
    google_id VARCHAR(255) UNIQUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Index for faster email lookups during authentication.
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_google_id ON users(google_id);

-- API keys for machine-to-machine authentication.
-- Used for pushing ML training runs from clusters/scripts.
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    -- Store only the hash of the key for security.
    -- The actual key is shown only once at creation.
    key_hash VARCHAR(64) NOT NULL UNIQUE,
    -- Key prefix for identification (first 8 chars of the key).
    key_prefix VARCHAR(8) NOT NULL,
    last_used_at TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);

-- Projects are top-level containers for experiments.
-- Multiple users can collaborate on a single project.
CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_projects_owner_id ON projects(owner_id);

-- Project members table for multi-user collaboration.
CREATE TABLE project_members (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL DEFAULT 'member', -- 'owner', 'admin', 'member', 'viewer'
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (project_id, user_id)
);

CREATE INDEX idx_project_members_user_id ON project_members(user_id);

-- Experiments group related runs within a project.
CREATE TABLE experiments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    -- Ensure unique experiment names within a project.
    UNIQUE(project_id, name)
);

CREATE INDEX idx_experiments_project_id ON experiments(project_id);

-- Runs represent individual ML training/evaluation runs.
CREATE TABLE runs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    experiment_id UUID NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    name VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'running', -- 'running', 'completed', 'failed', 'killed'
    -- Store flexible run data as JSONB for schema flexibility.
    -- This includes parameters, tags, and other metadata.
    params JSONB DEFAULT '{}',
    tags JSONB DEFAULT '{}',
    start_time TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    end_time TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_runs_experiment_id ON runs(experiment_id);
CREATE INDEX idx_runs_status ON runs(status);
-- GIN index for efficient JSONB queries on params and tags.
CREATE INDEX idx_runs_params ON runs USING GIN (params);
CREATE INDEX idx_runs_tags ON runs USING GIN (tags);

-- Metrics store time-series data for training runs.
-- Each metric can have multiple values logged at different steps/timestamps.
CREATE TABLE metrics (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    run_id UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    key VARCHAR(255) NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    step BIGINT DEFAULT 0,
    timestamp TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_metrics_run_id ON metrics(run_id);
CREATE INDEX idx_metrics_run_id_key ON metrics(run_id, key);
CREATE INDEX idx_metrics_run_id_step ON metrics(run_id, step);

-- Artifacts track files associated with runs.
-- Actual file content is stored on the filesystem, not in the database.
CREATE TABLE artifacts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    run_id UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    path VARCHAR(1024) NOT NULL, -- Relative path within the run's artifact directory.
    file_name VARCHAR(255) NOT NULL,
    file_size BIGINT NOT NULL,
    content_type VARCHAR(255),
    checksum VARCHAR(64), -- SHA-256 hash for integrity verification.
    -- Storage path on the filesystem (relative to artifact root).
    storage_path VARCHAR(1024) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    -- Ensure unique paths within a run.
    UNIQUE(run_id, path)
);

CREATE INDEX idx_artifacts_run_id ON artifacts(run_id);

-- Artifact chunks track upload progress for chunked uploads.
-- This supports resumable uploads and bypasses Cloudflare's 100MB limit.
CREATE TABLE artifact_chunks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    artifact_id UUID NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    chunk_number INT NOT NULL,
    chunk_size BIGINT NOT NULL,
    checksum VARCHAR(64) NOT NULL, -- SHA-256 of this chunk.
    storage_path VARCHAR(1024) NOT NULL,
    uploaded_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    -- Ensure unique chunk numbers per artifact.
    UNIQUE(artifact_id, chunk_number)
);

CREATE INDEX idx_artifact_chunks_artifact_id ON artifact_chunks(artifact_id);

-- Function to automatically update updated_at timestamp.
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply the trigger to tables with updated_at column.
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_projects_updated_at BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_experiments_updated_at BEFORE UPDATE ON experiments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_runs_updated_at BEFORE UPDATE ON runs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
