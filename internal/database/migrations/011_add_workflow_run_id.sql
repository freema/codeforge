ALTER TABLE tasks ADD COLUMN workflow_run_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_tasks_workflow_run_id ON tasks(workflow_run_id);
