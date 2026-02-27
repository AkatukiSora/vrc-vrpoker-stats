package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(Up00010, Down00010)
}

func Up00010(ctx context.Context, tx *sql.Tx) error {
	res, err := tx.ExecContext(ctx, `UPDATE hands
		SET stats_eligible = 1
		WHERE stats_eligible = 0 AND has_anomaly = 0`)
	if err != nil {
		return fmt.Errorf("backfill stats_eligible: %w", err)
	}
	if _, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("backfill stats_eligible rows: %w", err)
	}
	return nil
}

func Down00010(context.Context, *sql.Tx) error {
	return nil
}
