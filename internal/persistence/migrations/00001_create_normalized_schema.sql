-- +goose Up
CREATE TABLE IF NOT EXISTS worlds (
    world_id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS instances (
    instance_uid TEXT PRIMARY KEY,
    world_id TEXT NOT NULL,
    instance_type TEXT NOT NULL,
    owner_user_uid TEXT,
    region TEXT,
    world_display_name TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(world_id) REFERENCES worlds(world_id)
);

CREATE TABLE IF NOT EXISTS users (
    user_uid TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS instance_participants (
    instance_uid TEXT NOT NULL,
    user_uid TEXT NOT NULL,
    first_seen_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    PRIMARY KEY(instance_uid, user_uid),
    FOREIGN KEY(instance_uid) REFERENCES instances(instance_uid),
    FOREIGN KEY(user_uid) REFERENCES users(user_uid)
);

CREATE TABLE IF NOT EXISTS import_cursors (
    source_path TEXT PRIMARY KEY,
    next_byte_offset INTEGER NOT NULL,
    next_line_number INTEGER NOT NULL,
    last_event_time TEXT,
    last_hand_uid TEXT,
    parser_state_json BLOB,
    updated_at TEXT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS hand_anomalies;
DROP TABLE IF EXISTS hand_actions;
DROP TABLE IF EXISTS hand_board_cards;
DROP TABLE IF EXISTS hand_hole_cards;
DROP TABLE IF EXISTS hand_players;
DROP TABLE IF EXISTS hand_occurrences;
DROP TABLE IF EXISTS hands;
DROP TABLE IF EXISTS hands_legacy;
DROP TABLE IF EXISTS instance_participants;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS instances;
DROP TABLE IF EXISTS worlds;
DROP TABLE IF EXISTS import_cursors;
