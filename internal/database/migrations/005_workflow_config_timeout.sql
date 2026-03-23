-- Add timeout_seconds column to workflow_configs for per-workflow timeout override.
ALTER TABLE workflow_configs ADD COLUMN timeout_seconds INTEGER NOT NULL DEFAULT 0;
