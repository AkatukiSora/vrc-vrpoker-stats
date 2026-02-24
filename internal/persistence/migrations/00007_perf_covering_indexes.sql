-- +goose Up
-- Covering index for is_complete + stats_eligible + start_time filter used in ListHandsAfter.
-- This avoids a full table scan when fetching only eligible hands since a given timestamp.
CREATE INDEX IF NOT EXISTS idx_hands_complete_eligible_time
    ON hands(is_complete, stats_eligible, start_time);

-- Covering index for hand summaries (complete + time ordering).
CREATE INDEX IF NOT EXISTS idx_hands_complete_start_time
    ON hands(is_complete, start_time);

-- The source-span deduplication lookup in upsertHandsTx uses (source_path, start_byte, end_byte).
-- hand_occurrences has a PK on (hand_uid, source_path) so source_path is also covered by the PK.
-- Drop the now-redundant secondary index to reduce write overhead.
DROP INDEX IF EXISTS idx_hand_occurrences_lookup;

-- +goose Down
DROP INDEX IF EXISTS idx_hands_complete_eligible_time;
DROP INDEX IF EXISTS idx_hands_complete_start_time;
CREATE INDEX IF NOT EXISTS idx_hand_occurrences_lookup
    ON hand_occurrences(source_path, start_byte, end_byte);
