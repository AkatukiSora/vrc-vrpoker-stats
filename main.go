package main

import (
	"fmt"
	"path/filepath"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/application"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/ui"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/watcher"
)

var (
	version   = "dev"
	commit    = "local"
	buildDate = "unknown"
)

func main() {
	dbPath := filepath.Join(".", "vrpoker-stats.db")
	repo, err := persistence.NewSQLiteRepository(dbPath)
	if err != nil {
		fmt.Printf("warning: failed to initialize sqlite repository: %v\n", err)
		repo = nil
	}

	meta := ui.AppMetadata{
		Version:       version,
		Commit:        commit,
		BuildDate:     buildDate,
		RepositoryURL: "https://github.com/AkatukiSora/vrc-vrpoker-stats",
	}
	if repo != nil {
		ui.Run(application.NewService(repo, watcher.DetectAllLogFiles), meta)
		return
	}

	ui.Run(application.NewService(persistence.NewMemoryRepository(), watcher.DetectAllLogFiles), meta)
}
