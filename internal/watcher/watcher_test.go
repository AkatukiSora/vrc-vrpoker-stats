package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLogWatcherStopIsIdempotent(t *testing.T) {
	t.Parallel()

	logPath := filepath.Join(t.TempDir(), "output_log_2026-02-21_00-00-00.txt")
	if err := os.WriteFile(logPath, []byte(""), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}

	lw, err := NewLogWatcher(logPath, WatcherConfig{})
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}

	lw.Stop()
	lw.Stop()
}

func TestLogWatcherDetectsNewSessionLogOnCreate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	currentLogPath := filepath.Join(dir, "output_log_2026-02-21_00-00-00.txt")
	if err := os.WriteFile(currentLogPath, []byte(""), 0o600); err != nil {
		t.Fatalf("write current log: %v", err)
	}

	newLogCh := make(chan string, 1)
	lw, err := NewLogWatcher(currentLogPath, WatcherConfig{OnNewLogFile: func(path string) {
		select {
		case newLogCh <- path:
		default:
		}
	}})
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer lw.Stop()

	if err := lw.Start(); err != nil {
		t.Fatalf("start watcher: %v", err)
	}

	newLogPath := filepath.Join(dir, "output_log_2026-02-21_00-10-00.txt")
	if err := os.WriteFile(newLogPath, []byte("new session"), 0o600); err != nil {
		t.Fatalf("write new log: %v", err)
	}

	select {
	case got := <-newLogCh:
		if filepath.Clean(got) != filepath.Clean(newLogPath) {
			t.Fatalf("detected path = %q, want %q", got, newLogPath)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for new log file detection")
	}
}

func TestLogWatcherIgnoresNonVRChatCreatedFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	currentLogPath := filepath.Join(dir, "output_log_2026-02-21_00-00-00.txt")
	if err := os.WriteFile(currentLogPath, []byte(""), 0o600); err != nil {
		t.Fatalf("write current log: %v", err)
	}

	newLogCh := make(chan string, 1)
	lw, err := NewLogWatcher(currentLogPath, WatcherConfig{OnNewLogFile: func(path string) {
		select {
		case newLogCh <- path:
		default:
		}
	}})
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer lw.Stop()

	if err := lw.Start(); err != nil {
		t.Fatalf("start watcher: %v", err)
	}

	nonLogPath := filepath.Join(dir, "not_vrchat.log")
	if err := os.WriteFile(nonLogPath, []byte("ignore me"), 0o600); err != nil {
		t.Fatalf("write non-log file: %v", err)
	}

	select {
	case got := <-newLogCh:
		t.Fatalf("unexpected new log file detection: %q", got)
	case <-time.After(500 * time.Millisecond):
	}
}
