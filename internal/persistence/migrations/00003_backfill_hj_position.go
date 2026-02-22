package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
)

func init() {
	goose.AddMigrationContext(Up00003, Down00003)
}

func Up00003(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `UPDATE hand_players
		SET position = ?
		WHERE position = ?
			AND EXISTS (
				SELECT 1 FROM hands
				WHERE hands.hand_uid = hand_players.hand_uid
					AND hands.num_players IN (6, 7)
			)`, int(parser.PosHJ), int(parser.PosMP)); err != nil {
		return fmt.Errorf("backfill hj position for 6/7-max hands: %w", err)
	}
	return nil
}

func Down00003(context.Context, *sql.Tx) error {
	return nil
}
