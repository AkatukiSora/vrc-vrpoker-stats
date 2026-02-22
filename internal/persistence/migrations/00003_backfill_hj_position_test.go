package migrations

import (
	"context"
	"database/sql"
	"testing"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	_ "modernc.org/sqlite"
)

func TestUp00003BackfillsMPToHJForSixAndSevenMax(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}

	ddl := []string{
		`CREATE TABLE hands (hand_uid TEXT PRIMARY KEY, num_players INTEGER NOT NULL);`,
		`CREATE TABLE hand_players (hand_uid TEXT NOT NULL, seat_id INTEGER NOT NULL, position INTEGER NOT NULL, PRIMARY KEY(hand_uid, seat_id));`,
	}
	for _, q := range ddl {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			_ = tx.Rollback()
			t.Fatalf("create test schema: %v", err)
		}
	}

	fixtures := []struct {
		handUID    string
		numPlayers int
		position   parser.Position
		wantPos    parser.Position
	}{
		{handUID: "h6", numPlayers: 6, position: parser.PosMP, wantPos: parser.PosHJ},
		{handUID: "h7", numPlayers: 7, position: parser.PosMP, wantPos: parser.PosHJ},
		{handUID: "h8", numPlayers: 8, position: parser.PosMP, wantPos: parser.PosMP},
		{handUID: "h6-utg", numPlayers: 6, position: parser.PosUTG, wantPos: parser.PosUTG},
	}

	for i, f := range fixtures {
		if _, err := tx.ExecContext(ctx, `INSERT INTO hands(hand_uid, num_players) VALUES(?, ?)`, f.handUID, f.numPlayers); err != nil {
			_ = tx.Rollback()
			t.Fatalf("insert hand %d: %v", i, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO hand_players(hand_uid, seat_id, position) VALUES(?, ?, ?)`, f.handUID, i, int(f.position)); err != nil {
			_ = tx.Rollback()
			t.Fatalf("insert hand_player %d: %v", i, err)
		}
	}

	if err := Up00003(ctx, tx); err != nil {
		_ = tx.Rollback()
		t.Fatalf("run migration: %v", err)
	}

	for i, f := range fixtures {
		var got int
		if err := tx.QueryRowContext(ctx, `SELECT position FROM hand_players WHERE hand_uid = ? AND seat_id = ?`, f.handUID, i).Scan(&got); err != nil {
			_ = tx.Rollback()
			t.Fatalf("query hand_player %d: %v", i, err)
		}
		if parser.Position(got) != f.wantPos {
			_ = tx.Rollback()
			t.Fatalf("hand %s got position=%v want=%v", f.handUID, parser.Position(got), f.wantPos)
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit tx: %v", err)
	}
}
