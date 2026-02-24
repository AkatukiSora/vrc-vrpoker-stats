-- +goose Up
ALTER TABLE import_cursors ADD COLUMN is_fully_imported INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite does not support DROP COLUMN in older versions; leave as-is on downgrade.
