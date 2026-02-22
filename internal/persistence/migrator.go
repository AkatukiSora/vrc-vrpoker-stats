package persistence

import (
	"database/sql"
	"embed"
	"fmt"
	"sync"

	"github.com/pressly/goose/v3"

	_ "github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence/migrations"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

var migrationSetupOnce sync.Once

func runMigrations(db *sql.DB) error {
	var setupErr error
	migrationSetupOnce.Do(func() {
		goose.SetBaseFS(migrationFS)
		setupErr = goose.SetDialect("sqlite3")
	})
	if setupErr != nil {
		return fmt.Errorf("setup goose: %w", setupErr)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
