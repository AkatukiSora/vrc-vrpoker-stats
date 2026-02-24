-- +goose Up
-- Add world context columns to import_cursors so that on restart the active
-- log file can be resumed from the last fully-committed hand boundary without
-- re-scanning from byte 0.
ALTER TABLE import_cursors ADD COLUMN world_id TEXT;
ALTER TABLE import_cursors ADD COLUMN world_display_name TEXT;
ALTER TABLE import_cursors ADD COLUMN instance_uid TEXT;
ALTER TABLE import_cursors ADD COLUMN instance_type TEXT;
ALTER TABLE import_cursors ADD COLUMN instance_owner TEXT;
ALTER TABLE import_cursors ADD COLUMN instance_region TEXT;
ALTER TABLE import_cursors ADD COLUMN in_poker_world INTEGER NOT NULL DEFAULT 0;
ALTER TABLE import_cursors ADD COLUMN hand_id_counter INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite does not support DROP COLUMN; leave as-is on downgrade.
