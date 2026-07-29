-- Blueprint-backed schedules: a schedule references a blueprint (+ params)
-- instead of carrying an inline session_request. Exactly one of the two is
-- set; blueprint_params is a JSON object of parameter values ('' = none).
ALTER TABLE schedules ADD COLUMN blueprint_id TEXT NOT NULL DEFAULT '';
ALTER TABLE schedules ADD COLUMN blueprint_params TEXT NOT NULL DEFAULT '';
