package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
)

func TestBootstrapImportAllLogsImportsEachFileOnce(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	oldPath := filepath.Join(tmp, "old.log")
	newPath := filepath.Join(tmp, "new.log")
	if err := os.WriteFile(oldPath, []byte(testHandLog("00:00")), 0o600); err != nil {
		t.Fatalf("write old log: %v", err)
	}
	if err := os.WriteFile(newPath, []byte(testHandLog("00:10")), 0o600); err != nil {
		t.Fatalf("write new log: %v", err)
	}

	svc := NewService(persistence.NewMemoryRepository(), func() ([]string, error) {
		return []string{newPath, oldPath}, nil
	})

	latest, err := svc.BootstrapImportAllLogs(context.Background())
	if err != nil {
		t.Fatalf("bootstrap import: %v", err)
	}
	if latest != newPath {
		t.Fatalf("latest path = %q, want %q", latest, newPath)
	}

	_, hands, _, err := svc.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(hands) != 2 {
		t.Fatalf("hand count = %d, want 2", len(hands))
	}
}

func TestImportLinesSkipsStaleSource(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	pathA := filepath.Join(tmp, "a.log")
	pathB := filepath.Join(tmp, "b.log")
	if err := os.WriteFile(pathA, []byte(testHandLog("01:00")), 0o600); err != nil {
		t.Fatalf("write log A: %v", err)
	}
	if err := os.WriteFile(pathB, []byte(testHandLog("01:10")), 0o600); err != nil {
		t.Fatalf("write log B: %v", err)
	}

	svc := NewService(persistence.NewMemoryRepository(), nil)
	if err := svc.ChangeLogFile(context.Background(), pathA); err != nil {
		t.Fatalf("activate A: %v", err)
	}
	if err := svc.ChangeLogFile(context.Background(), pathB); err != nil {
		t.Fatalf("activate B: %v", err)
	}

	_, handsBefore, _, err := svc.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot before stale import: %v", err)
	}

	staleLines := strings.Split(strings.TrimSpace(testHandLog("01:20")), "\n")
	if err := svc.ImportLines(context.Background(), pathA, staleLines, 0, 128); err != nil {
		t.Fatalf("stale import call: %v", err)
	}

	_, handsAfter, _, err := svc.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot after stale import: %v", err)
	}
	if len(handsAfter) != len(handsBefore) {
		t.Fatalf("hand count changed by stale import: before=%d after=%d", len(handsBefore), len(handsAfter))
	}
}

// TestBootstrapSkipsFullyImportedFiles verifies that files marked as fully
// imported in the first bootstrap run are skipped on a second run.
func TestBootstrapSkipsFullyImportedFiles(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	oldPath := filepath.Join(tmp, "old.log")
	newPath := filepath.Join(tmp, "new.log")
	if err := os.WriteFile(oldPath, []byte(testHandLog("02:00")), 0o600); err != nil {
		t.Fatalf("write old log: %v", err)
	}
	if err := os.WriteFile(newPath, []byte(testHandLog("02:10")), 0o600); err != nil {
		t.Fatalf("write new log: %v", err)
	}

	repo := persistence.NewMemoryRepository()
	locator := func() ([]string, error) { return []string{newPath, oldPath}, nil }

	// First bootstrap: both files imported, old file marked fully-imported.
	svc1 := NewService(repo, locator)
	if _, err := svc1.BootstrapImportAllLogs(context.Background()); err != nil {
		t.Fatalf("first bootstrap: %v", err)
	}

	// Verify the old file was marked.
	cursor, err := repo.GetCursor(context.Background(), oldPath)
	if err != nil {
		t.Fatalf("get cursor: %v", err)
	}
	if cursor == nil || !cursor.IsFullyImported {
		t.Fatal("old file should be marked fully imported after first bootstrap")
	}

	// Second bootstrap: old file should be skipped (progress count check).
	svc2 := NewService(repo, locator)
	var progressEvents []BootstrapProgress
	if _, err := svc2.BootstrapImportAllLogsWithProgress(context.Background(), func(p BootstrapProgress) {
		progressEvents = append(progressEvents, p)
	}); err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}

	// Only 1 file (the active/new one) should have been imported.
	for _, p := range progressEvents {
		if p.Path == oldPath {
			t.Errorf("old (fully-imported) file was processed again: %+v", p)
		}
	}
	if len(progressEvents) != 1 {
		t.Errorf("expected 1 progress event (active file), got %d: %+v", len(progressEvents), progressEvents)
	}
}

// TestBootstrapParallelImport verifies that parallel import yields the same
// hand count as sequential import.
func TestBootstrapParallelImport(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	// Create 5 historical files + 1 active file.
	paths := make([]string, 6)
	for i := range paths {
		minute := fmt.Sprintf("%02d:%02d", i/60, i%60)
		p := filepath.Join(tmp, fmt.Sprintf("log_%02d.log", i))
		if err := os.WriteFile(p, []byte(testHandLog(minute)), 0o600); err != nil {
			t.Fatalf("write log %d: %v", i, err)
		}
		paths[i] = p
	}

	// locator returns newest first (paths[5], paths[4], ..., paths[0]).
	reversed := make([]string, len(paths))
	for i := range paths {
		reversed[i] = paths[len(paths)-1-i]
	}

	repo := persistence.NewMemoryRepository()
	svc := NewService(repo, func() ([]string, error) { return reversed, nil })

	if _, err := svc.BootstrapImportAllLogs(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	_, hands, _, err := svc.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	// Each file has one complete hand.
	if len(hands) != len(paths) {
		t.Fatalf("hand count = %d, want %d", len(hands), len(paths))
	}
}

func testHandLog(minute string) string {
	return strings.Join([]string{
		"2026.02.21 " + minute + ":00 Debug      -  [Table]: Preparing for New Game: ",
		"2026.02.21 " + minute + ":01 Debug      -  [Seat]: Player 0 SB BET IN = 10",
		"2026.02.21 " + minute + ":02 Debug      -  [Seat]: Player 1 BB BET IN = 20",
		"2026.02.21 " + minute + ":03 Debug      -  [PotManager]: All players folded, player 0 won 30",
		"2026.02.21 " + minute + ":04 Debug      -  [Table]: Preparing for New Game: ",
	}, "\n") + "\n"
}
