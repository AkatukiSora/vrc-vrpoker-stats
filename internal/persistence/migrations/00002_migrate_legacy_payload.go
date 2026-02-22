package migrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
)

func init() {
	goose.AddMigrationContext(Up00002, Down00002)
}

func Up00002(ctx context.Context, tx *sql.Tx) error {
	state, err := detectHandsSchemaState(ctx, tx)
	if err != nil {
		return err
	}
	if state == handsSchemaNormalized {
		return createNormalizedHandsTables(ctx, tx)
	}
	if state == handsSchemaMissing {
		return createNormalizedHandsTables(ctx, tx)
	}
	if state != handsSchemaLegacy {
		return nil
	}

	if _, err := tx.ExecContext(ctx, `ALTER TABLE hands RENAME TO hands_legacy`); err != nil {
		return fmt.Errorf("rename legacy hands table: %w", err)
	}
	if err := createNormalizedHandsTables(ctx, tx); err != nil {
		return err
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT hand_uid, source_path, start_byte, end_byte, start_line, end_line,
		       start_time, end_time, is_complete, local_seat, payload_json, updated_at
		FROM hands_legacy
		ORDER BY start_time ASC`)
	if err != nil {
		return fmt.Errorf("query legacy hands: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var handUID, sourcePath string
		var startByte, endByte, startLine, endLine int64
		var startTimeStr, endTimeStr string
		var isComplete, localSeat int
		var payload []byte
		var updatedAt string

		if err := rows.Scan(
			&handUID,
			&sourcePath,
			&startByte,
			&endByte,
			&startLine,
			&endLine,
			&startTimeStr,
			&endTimeStr,
			&isComplete,
			&localSeat,
			&payload,
			&updatedAt,
		); err != nil {
			return fmt.Errorf("scan legacy hand row: %w", err)
		}

		hand, migrateErr := decodeLegacyHand(payload)
		if migrateErr != nil {
			hand = &parser.Hand{
				LocalPlayerSeat: localSeat,
				IsComplete:      isComplete == 1,
				StatsEligible:   false,
				HasAnomaly:      true,
				Anomalies: []parser.HandAnomaly{{
					Code:     "MIGRATION_PAYLOAD_DECODE_ERROR",
					Severity: "warn",
					Detail:   migrateErr.Error(),
				}},
				Players: make(map[int]*parser.PlayerHandInfo),
			}
		}

		if hand.Players == nil {
			hand.Players = make(map[int]*parser.PlayerHandInfo)
		}
		hand.HandUID = handUID
		if hand.LocalPlayerSeat < 0 {
			hand.LocalPlayerSeat = localSeat
		}
		hand.IsComplete = hand.IsComplete || (isComplete == 1)
		if hand.StartTime.IsZero() {
			hand.StartTime = parseTimeOrZero(startTimeStr)
		}
		if hand.EndTime.IsZero() {
			hand.EndTime = parseTimeOrZero(endTimeStr)
		}
		if hand.WorldDisplayName == "" {
			hand.WorldDisplayName = ""
		}
		if hand.InstanceType == "" {
			hand.InstanceType = parser.InstanceTypeUnknown
		}

		if err := upsertWorldAndInstance(ctx, tx, hand, updatedAt); err != nil {
			return err
		}
		if err := upsertHandRoot(ctx, tx, hand, handUID, updatedAt); err != nil {
			return err
		}
		if err := upsertHandChildren(ctx, tx, hand, handUID); err != nil {
			return err
		}
		if err := upsertInstanceUsers(ctx, tx, hand, updatedAt); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO hand_occurrences(
			hand_uid, source_path, start_byte, end_byte, start_line, end_line, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?)`, handUID, sourcePath, startByte, endByte, startLine, endLine, updatedAt); err != nil {
			return fmt.Errorf("insert hand occurrence: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate legacy hands: %w", err)
	}

	return nil
}

func Down00002(context.Context, *sql.Tx) error {
	return nil
}

type handsSchemaState int

const (
	handsSchemaMissing handsSchemaState = iota
	handsSchemaLegacy
	handsSchemaNormalized
)

func detectHandsSchemaState(ctx context.Context, tx *sql.Tx) (handsSchemaState, error) {
	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(hands)`) // #nosec G201
	if err != nil {
		return handsSchemaMissing, fmt.Errorf("pragma table_info(hands): %w", err)
	}
	defer rows.Close()

	hasAny := false
	hasPayload := false
	hasStatsEligible := false

	for rows.Next() {
		hasAny = true
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return handsSchemaMissing, err
		}
		if name == "payload_json" {
			hasPayload = true
		}
		if name == "stats_eligible" {
			hasStatsEligible = true
		}
	}
	if err := rows.Err(); err != nil {
		return handsSchemaMissing, err
	}
	if !hasAny {
		return handsSchemaMissing, nil
	}
	if hasPayload {
		return handsSchemaLegacy, nil
	}
	if hasStatsEligible {
		return handsSchemaNormalized, nil
	}
	return handsSchemaMissing, nil
}

func createNormalizedHandsTables(ctx context.Context, tx *sql.Tx) error {
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS hands (
			hand_uid TEXT PRIMARY KEY,
			start_time TEXT NOT NULL,
			end_time TEXT NOT NULL,
			is_complete INTEGER NOT NULL,
			stats_eligible INTEGER NOT NULL,
			has_anomaly INTEGER NOT NULL,
			local_seat INTEGER NOT NULL,
			world_id TEXT,
			world_display_name TEXT NOT NULL,
			instance_uid TEXT,
			instance_type TEXT NOT NULL,
			instance_owner_user_uid TEXT,
			instance_region TEXT,
			sb_seat INTEGER NOT NULL,
			bb_seat INTEGER NOT NULL,
			num_players INTEGER NOT NULL,
			total_pot INTEGER NOT NULL,
			winner_seat INTEGER NOT NULL,
			win_type TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(world_id) REFERENCES worlds(world_id),
			FOREIGN KEY(instance_uid) REFERENCES instances(instance_uid)
		);`,
		`CREATE TABLE IF NOT EXISTS hand_occurrences (
			hand_uid TEXT NOT NULL,
			source_path TEXT NOT NULL,
			start_byte INTEGER NOT NULL,
			end_byte INTEGER NOT NULL,
			start_line INTEGER NOT NULL,
			end_line INTEGER NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(source_path, start_byte, end_byte),
			FOREIGN KEY(hand_uid) REFERENCES hands(hand_uid)
		);`,
		`CREATE TABLE IF NOT EXISTS hand_players (
			hand_uid TEXT NOT NULL,
			seat_id INTEGER NOT NULL,
			position INTEGER NOT NULL,
			showed_down INTEGER NOT NULL,
			won INTEGER NOT NULL,
			pot_won INTEGER NOT NULL,
			vpip INTEGER NOT NULL,
			pfr INTEGER NOT NULL,
			three_bet INTEGER NOT NULL,
			fold_to_3bet INTEGER NOT NULL,
			folded_pf INTEGER NOT NULL,
			PRIMARY KEY(hand_uid, seat_id),
			FOREIGN KEY(hand_uid) REFERENCES hands(hand_uid)
		);`,
		`CREATE TABLE IF NOT EXISTS hand_hole_cards (
			hand_uid TEXT NOT NULL,
			seat_id INTEGER NOT NULL,
			card_index INTEGER NOT NULL,
			rank TEXT NOT NULL,
			suit TEXT NOT NULL,
			PRIMARY KEY(hand_uid, seat_id, card_index),
			FOREIGN KEY(hand_uid, seat_id) REFERENCES hand_players(hand_uid, seat_id)
		);`,
		`CREATE TABLE IF NOT EXISTS hand_board_cards (
			hand_uid TEXT NOT NULL,
			card_index INTEGER NOT NULL,
			rank TEXT NOT NULL,
			suit TEXT NOT NULL,
			PRIMARY KEY(hand_uid, card_index),
			FOREIGN KEY(hand_uid) REFERENCES hands(hand_uid)
		);`,
		`CREATE TABLE IF NOT EXISTS hand_actions (
			hand_uid TEXT NOT NULL,
			seat_id INTEGER NOT NULL,
			action_index INTEGER NOT NULL,
			timestamp TEXT NOT NULL,
			street INTEGER NOT NULL,
			action INTEGER NOT NULL,
			amount INTEGER NOT NULL,
			PRIMARY KEY(hand_uid, seat_id, action_index),
			FOREIGN KEY(hand_uid, seat_id) REFERENCES hand_players(hand_uid, seat_id)
		);`,
		`CREATE TABLE IF NOT EXISTS hand_anomalies (
			hand_uid TEXT NOT NULL,
			anomaly_index INTEGER NOT NULL,
			code TEXT NOT NULL,
			severity TEXT NOT NULL,
			detail TEXT NOT NULL,
			PRIMARY KEY(hand_uid, anomaly_index),
			FOREIGN KEY(hand_uid) REFERENCES hands(hand_uid)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_hands_start_time ON hands(start_time);`,
		`CREATE INDEX IF NOT EXISTS idx_hands_stats_eligible ON hands(stats_eligible, start_time);`,
		`CREATE INDEX IF NOT EXISTS idx_hands_instance_uid ON hands(instance_uid, start_time);`,
		`CREATE INDEX IF NOT EXISTS idx_occ_hand_uid ON hand_occurrences(hand_uid);`,
	}
	for _, q := range ddl {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("create normalized table/index: %w", err)
		}
	}
	return nil
}

func decodeLegacyHand(payload []byte) (*parser.Hand, error) {
	var h parser.Hand
	if err := json.Unmarshal(payload, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

func parseTimeOrZero(v string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}
	}
	return t
}

func upsertWorldAndInstance(ctx context.Context, tx *sql.Tx, h *parser.Hand, updatedAt string) error {
	if h.WorldID != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO worlds(world_id, display_name, updated_at)
			VALUES(?, ?, ?)
			ON CONFLICT(world_id) DO UPDATE SET display_name=excluded.display_name, updated_at=excluded.updated_at`,
			h.WorldID,
			h.WorldDisplayName,
			updatedAt,
		); err != nil {
			return fmt.Errorf("upsert world: %w", err)
		}
	}
	if h.InstanceUID != "" && h.WorldID != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO instances(instance_uid, world_id, instance_type, owner_user_uid, region, world_display_name, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(instance_uid) DO UPDATE SET
				world_id=excluded.world_id,
				instance_type=excluded.instance_type,
				owner_user_uid=excluded.owner_user_uid,
				region=excluded.region,
				world_display_name=excluded.world_display_name,
				updated_at=excluded.updated_at`,
			h.InstanceUID,
			h.WorldID,
			defaultInstanceType(h.InstanceType),
			nullIfEmpty(h.InstanceOwner),
			nullIfEmpty(h.InstanceRegion),
			h.WorldDisplayName,
			updatedAt,
		); err != nil {
			return fmt.Errorf("upsert instance: %w", err)
		}
	}
	return nil
}

func upsertHandRoot(ctx context.Context, tx *sql.Tx, h *parser.Hand, handUID, updatedAt string) error {
	instanceType := defaultInstanceType(h.InstanceType)
	if instanceType == parser.InstanceTypeUnknown && h.InstanceUID == "" {
		instanceType = parser.InstanceTypePublic
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO hands(
		hand_uid, start_time, end_time, is_complete, stats_eligible, has_anomaly, local_seat,
		world_id, world_display_name, instance_uid, instance_type, instance_owner_user_uid, instance_region,
		sb_seat, bb_seat, num_players, total_pot, winner_seat, win_type, updated_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		handUID,
		h.StartTime.UTC().Format(time.RFC3339Nano),
		h.EndTime.UTC().Format(time.RFC3339Nano),
		boolToInt(h.IsComplete),
		boolToInt(h.IsStatsEligible()),
		boolToInt(h.HasDataAnomaly()),
		h.LocalPlayerSeat,
		nullIfEmpty(h.WorldID),
		h.WorldDisplayName,
		nullIfEmpty(h.InstanceUID),
		string(instanceType),
		nullIfEmpty(h.InstanceOwner),
		nullIfEmpty(h.InstanceRegion),
		h.SBSeat,
		h.BBSeat,
		h.NumPlayers,
		h.TotalPot,
		h.WinnerSeat,
		h.WinType,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert hand root: %w", err)
	}
	return nil
}

func upsertHandChildren(ctx context.Context, tx *sql.Tx, h *parser.Hand, handUID string) error {
	for i, c := range h.CommunityCards {
		if _, err := tx.ExecContext(ctx, `INSERT INTO hand_board_cards(hand_uid, card_index, rank, suit) VALUES(?, ?, ?, ?)`, handUID, i, c.Rank, c.Suit); err != nil {
			return fmt.Errorf("insert board cards: %w", err)
		}
	}

	seats := make([]int, 0, len(h.Players))
	for seat := range h.Players {
		seats = append(seats, seat)
	}
	sort.Ints(seats)

	for _, seat := range seats {
		pi := h.Players[seat]
		if pi == nil {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hand_players(
			hand_uid, seat_id, position, showed_down, won, pot_won, vpip, pfr, three_bet, fold_to_3bet, folded_pf
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			handUID,
			seat,
			int(pi.Position),
			boolToInt(pi.ShowedDown),
			boolToInt(pi.Won),
			pi.PotWon,
			boolToInt(pi.VPIP),
			boolToInt(pi.PFR),
			boolToInt(pi.ThreeBet),
			boolToInt(pi.FoldTo3Bet),
			boolToInt(pi.FoldedPF),
		); err != nil {
			return fmt.Errorf("insert hand player: %w", err)
		}

		for ci, hc := range pi.HoleCards {
			if _, err := tx.ExecContext(ctx, `INSERT INTO hand_hole_cards(hand_uid, seat_id, card_index, rank, suit) VALUES(?, ?, ?, ?, ?)`, handUID, seat, ci, hc.Rank, hc.Suit); err != nil {
				return fmt.Errorf("insert hand hole cards: %w", err)
			}
		}

		for ai, act := range pi.Actions {
			if _, err := tx.ExecContext(ctx, `INSERT INTO hand_actions(hand_uid, seat_id, action_index, timestamp, street, action, amount)
				VALUES(?, ?, ?, ?, ?, ?, ?)`,
				handUID,
				seat,
				ai,
				act.Timestamp.UTC().Format(time.RFC3339Nano),
				int(act.Street),
				int(act.Action),
				act.Amount,
			); err != nil {
				return fmt.Errorf("insert hand action: %w", err)
			}
		}
	}

	for i, a := range h.Anomalies {
		if _, err := tx.ExecContext(ctx, `INSERT INTO hand_anomalies(hand_uid, anomaly_index, code, severity, detail) VALUES(?, ?, ?, ?, ?)`, handUID, i, a.Code, a.Severity, a.Detail); err != nil {
			return fmt.Errorf("insert hand anomaly: %w", err)
		}
	}

	return nil
}

func upsertInstanceUsers(ctx context.Context, tx *sql.Tx, h *parser.Hand, updatedAt string) error {
	if h.InstanceUID == "" {
		return nil
	}
	for _, u := range h.InstanceUsers {
		if u.UserUID == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO users(user_uid, display_name, updated_at)
			VALUES(?, ?, ?)
			ON CONFLICT(user_uid) DO UPDATE SET display_name=excluded.display_name, updated_at=excluded.updated_at`,
			u.UserUID,
			u.DisplayName,
			updatedAt,
		); err != nil {
			return fmt.Errorf("upsert user: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO instance_participants(instance_uid, user_uid, first_seen_at, last_seen_at)
			VALUES(?, ?, ?, ?)
			ON CONFLICT(instance_uid, user_uid) DO UPDATE SET last_seen_at=excluded.last_seen_at`,
			h.InstanceUID,
			u.UserUID,
			updatedAt,
			updatedAt,
		); err != nil {
			return fmt.Errorf("upsert instance participant: %w", err)
		}
	}
	return nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func defaultInstanceType(t parser.InstanceType) parser.InstanceType {
	if t == "" {
		return parser.InstanceTypeUnknown
	}
	return t
}
