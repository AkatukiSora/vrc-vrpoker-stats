-- +goose Up
-- Performance indexes: FK lookup columns used in batch-load WHERE hand_uid IN (...)
CREATE INDEX IF NOT EXISTS idx_hand_players_hand_uid     ON hand_players(hand_uid);
CREATE INDEX IF NOT EXISTS idx_hand_actions_hand_uid     ON hand_actions(hand_uid);
CREATE INDEX IF NOT EXISTS idx_hand_hole_cards_hand_uid  ON hand_hole_cards(hand_uid);
CREATE INDEX IF NOT EXISTS idx_hand_board_cards_hand_uid ON hand_board_cards(hand_uid);
CREATE INDEX IF NOT EXISTS idx_hand_anomalies_hand_uid   ON hand_anomalies(hand_uid);

-- Index to speed up local-seat joins for hand summaries.
CREATE INDEX IF NOT EXISTS idx_hand_players_hand_uid_seat_id
    ON hand_players(hand_uid, seat_id);
CREATE INDEX IF NOT EXISTS idx_hand_actions_hand_uid_seat_id
    ON hand_actions(hand_uid, seat_id);
CREATE INDEX IF NOT EXISTS idx_hand_hole_cards_hand_uid_seat_id_card_index
    ON hand_hole_cards(hand_uid, seat_id, card_index);
CREATE INDEX IF NOT EXISTS idx_hand_board_cards_hand_uid_card_index
    ON hand_board_cards(hand_uid, card_index);

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
DROP INDEX IF EXISTS idx_hand_players_hand_uid_seat_id;
DROP INDEX IF EXISTS idx_hand_actions_hand_uid_seat_id;
DROP INDEX IF EXISTS idx_hand_hole_cards_hand_uid_seat_id_card_index;
DROP INDEX IF EXISTS idx_hand_board_cards_hand_uid_card_index;
DROP INDEX IF EXISTS idx_hand_occurrences_lookup;
DROP INDEX IF EXISTS idx_hands_start_time;
