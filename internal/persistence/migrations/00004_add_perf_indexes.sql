-- +goose Up
-- Performance indexes: FK lookup columns used in batch-load WHERE hand_uid IN (...)
CREATE INDEX IF NOT EXISTS idx_hand_players_hand_uid     ON hand_players(hand_uid);
CREATE INDEX IF NOT EXISTS idx_hand_actions_hand_uid     ON hand_actions(hand_uid);
CREATE INDEX IF NOT EXISTS idx_hand_hole_cards_hand_uid  ON hand_hole_cards(hand_uid);
CREATE INDEX IF NOT EXISTS idx_hand_board_cards_hand_uid ON hand_board_cards(hand_uid);
CREATE INDEX IF NOT EXISTS idx_hand_anomalies_hand_uid   ON hand_anomalies(hand_uid);

-- Covering index for the source-span deduplication lookup in upsertHandsTx
CREATE INDEX IF NOT EXISTS idx_hand_occurrences_lookup
    ON hand_occurrences(source_path, start_byte, end_byte);

-- Index to speed up ORDER BY start_time and date-range filters in ListHands
CREATE INDEX IF NOT EXISTS idx_hands_start_time ON hands(start_time);

-- +goose Down
DROP INDEX IF EXISTS idx_hand_players_hand_uid;
DROP INDEX IF EXISTS idx_hand_actions_hand_uid;
DROP INDEX IF EXISTS idx_hand_hole_cards_hand_uid;
DROP INDEX IF EXISTS idx_hand_board_cards_hand_uid;
DROP INDEX IF EXISTS idx_hand_anomalies_hand_uid;
DROP INDEX IF EXISTS idx_hand_occurrences_lookup;
DROP INDEX IF EXISTS idx_hands_start_time;
