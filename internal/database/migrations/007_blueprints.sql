-- Blueprints: unified session request templates (replace presets + workflow
-- definitions + workflow configs). A preset is a blueprint without parameters;
-- a workflow is a blueprint with parameters.
CREATE TABLE IF NOT EXISTS blueprints (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL UNIQUE,
    description  TEXT NOT NULL DEFAULT '',
    builtin      INTEGER NOT NULL DEFAULT 0,
    request_json TEXT NOT NULL,
    params_json  TEXT NOT NULL DEFAULT '[]',
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);
