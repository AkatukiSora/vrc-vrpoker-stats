-- +goose Up
CREATE TABLE IF NOT EXISTS pocket_categories (
    id INTEGER PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    label TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS final_classes (
    id INTEGER PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    label TEXT NOT NULL
);

ALTER TABLE hand_players ADD COLUMN pocket_category_id INTEGER;
ALTER TABLE hand_players ADD COLUMN final_class_id INTEGER;

CREATE INDEX IF NOT EXISTS idx_hand_players_pocket_category_id ON hand_players(pocket_category_id);
CREATE INDEX IF NOT EXISTS idx_hand_players_final_class_id ON hand_players(final_class_id);

-- +goose Down
DROP INDEX IF EXISTS idx_hand_players_pocket_category_id;
DROP INDEX IF EXISTS idx_hand_players_final_class_id;

-- SQLite does not support DROP COLUMN; keep columns.
DROP TABLE IF EXISTS final_classes;
DROP TABLE IF EXISTS pocket_categories;
