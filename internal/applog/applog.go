// Package applog initialises the global slog logger for the application.
// Call Init once at startup; all other packages use log/slog directly.
package applog

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
)

var debugMode atomic.Bool
var logFile *os.File

// Init sets up the global slog logger.
// It writes structured text logs to both stdout and a temporary log file.
// If debug is true, the minimum log level is Debug; otherwise Info.
func Init(debug bool) {
	debugMode.Store(debug)

	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	writers := []io.Writer{os.Stdout}
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
	if f, err := os.OpenFile(tempLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		logFile = f
		writers = append(writers, f)
	}

	h := slog.NewTextHandler(io.MultiWriter(writers...), &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h))
}

// IsDebug reports whether debug mode is active.
func IsDebug() bool {
	return debugMode.Load()
}

func tempLogPath() string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("vrc-vrpoker-stats-%d.log", os.Getpid()))
}
