package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	_ "modernc.org/sqlite"
)

func TestNewSQLiteRepositoryMigratesLegacyPayloadSchema(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}

	legacyDDL := []string{
		`CREATE TABLE hands (
			hand_uid TEXT PRIMARY KEY,
			source_path TEXT NOT NULL,
			start_byte INTEGER NOT NULL,
			end_byte INTEGER NOT NULL,
			start_line INTEGER NOT NULL,
			end_line INTEGER NOT NULL,
			start_time TEXT NOT NULL,
			end_time TEXT NOT NULL,
			is_complete INTEGER NOT NULL,
			local_seat INTEGER NOT NULL,
			payload_json BLOB NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE UNIQUE INDEX idx_hands_source_span ON hands(source_path, start_byte, end_byte);`,
		`CREATE TABLE import_cursors (
			source_path TEXT PRIMARY KEY,
			next_byte_offset INTEGER NOT NULL,
			next_line_number INTEGER NOT NULL,
			last_event_time TEXT,
			last_hand_uid TEXT,
			parser_state_json BLOB,
			updated_at TEXT NOT NULL
		);`,
	}
	for _, q := range legacyDDL {
		if _, err := legacyDB.Exec(q); err != nil {
			_ = legacyDB.Close()
			t.Fatalf("create legacy schema: %v", err)
		}
	}

	hand := &parser.Hand{
		ID:              1,
		StartTime:       time.Date(2026, 2, 22, 10, 0, 0, 0, time.UTC),
		EndTime:         time.Date(2026, 2, 22, 10, 0, 10, 0, time.UTC),
		LocalPlayerSeat: 0,
		Players: map[int]*parser.PlayerHandInfo{
			0: {
				SeatID:    0,
				HoleCards: []parser.Card{{Rank: "A", Suit: "h"}, {Rank: "K", Suit: "d"}},
				Actions: []parser.PlayerAction{
					{Timestamp: time.Date(2026, 2, 22, 10, 0, 1, 0, time.UTC), PlayerID: 0, Street: parser.StreetPreFlop, Action: parser.ActionCall, Amount: 20},
				},
				Position: parser.PosSB,
			},
		},
		CommunityCards: []parser.Card{{Rank: "Q", Suit: "s"}, {Rank: "7", Suit: "d"}, {Rank: "2", Suit: "c"}},
		SBSeat:         0,
		BBSeat:         1,
		NumPlayers:     2,
		TotalPot:       40,
		WinnerSeat:     0,
		WinType:        "showdown",
		IsComplete:     true,
	}
	payload, err := json.Marshal(hand)
	if err != nil {
		_ = legacyDB.Close()
		t.Fatalf("marshal payload: %v", err)
	}

	if _, err := legacyDB.Exec(`INSERT INTO hands(
		hand_uid, source_path, start_byte, end_byte, start_line, end_line, start_time, end_time, is_complete, local_seat, payload_json, updated_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"legacy-hand-1",
		"legacy.log",
		0,
		100,
		1,
		8,
		hand.StartTime.UTC().Format(time.RFC3339Nano),
		hand.EndTime.UTC().Format(time.RFC3339Nano),
		1,
		0,
		payload,
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		_ = legacyDB.Close()
		t.Fatalf("insert legacy hand: %v", err)
	}
	_ = legacyDB.Close()

	repo, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("open migrated repository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	hands, err := repo.ListHands(context.Background(), HandFilter{OnlyComplete: true})
	if err != nil {
		t.Fatalf("list migrated hands: %v", err)
	}
	if len(hands) != 1 {
		t.Fatalf("migrated hands count = %d, want 1", len(hands))
	}
	m := hands[0]
	if m.HandUID != "legacy-hand-1" {
		t.Fatalf("migrated hand_uid = %q, want legacy-hand-1", m.HandUID)
	}
	if len(m.Players) == 0 {
		t.Fatalf("expected migrated players")
	}
}
