package watcher

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// LogWatcher monitors VRChat log files for new content
type LogWatcher struct {
	LogPath   string
	offset    int64
	watcher   *fsnotify.Watcher
	OnNewData func(lines []string, startOffset int64, endOffset int64)
	OnError   func(err error)
	done      chan struct{}
}

// NewLogWatcher creates a watcher for the given log file path
func NewLogWatcher(logPath string) (*LogWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	return &LogWatcher{
		LogPath: logPath,
		watcher: w,
		done:    make(chan struct{}),
	}, nil
}

// Start begins watching for file changes
func (lw *LogWatcher) Start() error {
	// Watch the directory (more reliable than watching file directly)
	dir := filepath.Dir(lw.LogPath)
	if err := lw.watcher.Add(dir); err != nil {
		return fmt.Errorf("watch directory %s: %w", dir, err)
	}

	// Read existing content first only when no explicit offset is set.
	// This keeps caller-specified offsets (e.g. EOF after initial import).
	if lw.offset == 0 {
		if err := lw.readNewContent(); err != nil {
			_ = err // non-fatal
		}
	}

	go lw.watchLoop()
	return nil
}

// Stop stops the watcher
func (lw *LogWatcher) Stop() {
	close(lw.done)
	lw.watcher.Close()
}

// SetOffset sets the initial read offset (for resuming)
func (lw *LogWatcher) SetOffset(offset int64) {
	lw.offset = offset
}

func (lw *LogWatcher) watchLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-lw.done:
			return
		case event, ok := <-lw.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if filepath.Clean(event.Name) == filepath.Clean(lw.LogPath) {
					if err := lw.readNewContent(); err != nil && lw.OnError != nil {
						lw.OnError(err)
					}
				}
			}
		case err, ok := <-lw.watcher.Errors:
			if !ok {
				return
			}
			if lw.OnError != nil {
				lw.OnError(err)
			}
		case <-ticker.C:
			// Periodic poll as fallback
			if err := lw.readNewContent(); err != nil && lw.OnError != nil {
				lw.OnError(err)
			}
		}
	}
}

func (lw *LogWatcher) readNewContent() error {
	f, err := os.Open(lw.LogPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Check file size
	info, err := f.Stat()
	if err != nil {
		return err
	}

	if info.Size() <= lw.offset {
		return nil // No new content
	}
	startOffset := lw.offset

	if _, err := f.Seek(lw.offset, io.SeekStart); err != nil {
		return err
	}

	buf, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	lw.offset = info.Size()

	if len(buf) == 0 {
		return nil
	}

	// Split into lines
	content := string(buf)
	lines := strings.Split(content, "\n")

	// Remove empty trailing line
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	if len(lines) > 0 && lw.OnNewData != nil {
		lw.OnNewData(lines, startOffset, lw.offset)
	}

	return nil
}

// DetectLatestLogFile finds the most recent VRChat log file
func DetectLatestLogFile() (string, error) {
	dirs := logDirectories()

	var candidates []string
	for _, dir := range dirs {
		expanded := expandHome(dir)
		matches, err := filepath.Glob(filepath.Join(expanded, "output_log_*.txt"))
		if err != nil {
			continue
		}
		candidates = append(candidates, matches...)
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no VRChat log files found in known locations")
	}

	// Sort by modification time, newest first
	sort.Slice(candidates, func(i, j int) bool {
		si, ei := os.Stat(candidates[i])
		sj, ej := os.Stat(candidates[j])
		if ei != nil || ej != nil {
			return false
		}
		return si.ModTime().After(sj.ModTime())
	})

	return candidates[0], nil
}

// DetectAllLogFiles finds all VRChat log files sorted newest first
func DetectAllLogFiles() ([]string, error) {
	dirs := logDirectories()

	var candidates []string
	for _, dir := range dirs {
		expanded := expandHome(dir)
		matches, err := filepath.Glob(filepath.Join(expanded, "output_log_*.txt"))
		if err != nil {
			continue
		}
		candidates = append(candidates, matches...)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no VRChat log files found in known locations")
	}

	sort.Slice(candidates, func(i, j int) bool {
		si, ei := os.Stat(candidates[i])
		sj, ej := os.Stat(candidates[j])
		if ei != nil || ej != nil {
			return false
		}
		return si.ModTime().After(sj.ModTime())
	})

	return candidates, nil
}

// logDirectories returns OS-specific VRChat log directories
func logDirectories() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			filepath.Join(os.Getenv("APPDATA"), "..", "LocalLow", "VRChat", "VRChat"),
			filepath.Join(os.Getenv("USERPROFILE"), "AppData", "LocalLow", "VRChat", "VRChat"),
		}
	case "linux":
		home := os.Getenv("HOME")
		// Steam Proton (most common for Linux VRChat)
		protonBase := filepath.Join(home, ".local", "share", "Steam", "steamapps", "compatdata", "438100", "pfx", "drive_c", "users", "steamuser", "AppData", "LocalLow", "VRChat", "VRChat")
		return []string{
			protonBase,
			// Flatpak Steam
			filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", "data", "Steam", "steamapps", "compatdata", "438100", "pfx", "drive_c", "users", "steamuser", "AppData", "LocalLow", "VRChat", "VRChat"),
			// Alternative paths
			filepath.Join(home, ".local", "share", "Steam", "steamapps", "compatdata", "438100", "pfx", "drive_c", "users", "steamuser", "AppData", "LocalLow", "VRChat", "VRChat"),
		}
	case "darwin":
		home := os.Getenv("HOME")
		return []string{
			filepath.Join(home, "Library", "Application Support", "com.vrchat.VRChat"),
		}
	default:
		return []string{}
	}
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home := os.Getenv("HOME")
		return filepath.Join(home, path[2:])
	}
	return path
}
