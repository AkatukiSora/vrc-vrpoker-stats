package watcher

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// LogWatcher monitors VRChat log files for new content
type LogWatcher struct {
	LogPath  string
	offset   int64
	watcher  *fsnotify.Watcher
	done     chan struct{}
	mu       sync.Mutex
	readMu   sync.Mutex
	stopOnce sync.Once

	cleanLogPath string
	onNewData    func(lines []string, startOffset int64, endOffset int64)
	onNewLogFile func(path string)
	onError      func(err error)
}

type WatcherConfig struct {
	OnNewData    func(lines []string, startOffset int64, endOffset int64)
	OnNewLogFile func(path string)
	OnError      func(err error)
}

// NewLogWatcher creates a watcher for the given log file path
func NewLogWatcher(logPath string, cfg WatcherConfig) (*LogWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	return &LogWatcher{
		LogPath:      logPath,
		watcher:      w,
		done:         make(chan struct{}),
		cleanLogPath: filepath.Clean(logPath),
		onNewData:    cfg.OnNewData,
		onNewLogFile: cfg.OnNewLogFile,
		onError:      cfg.OnError,
	}, nil
}

// Start begins watching for file changes
func (lw *LogWatcher) Start() error {
	slog.Info("watcher starting", "path", lw.LogPath)
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
	lw.stopOnce.Do(func() {
		slog.Info("watcher stopped", "path", lw.LogPath)
		close(lw.done)
		_ = lw.watcher.Close()
	})
}

// SetOffset sets the initial read offset (for resuming)
func (lw *LogWatcher) SetOffset(offset int64) {
	lw.mu.Lock()
	defer lw.mu.Unlock()
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
			if event.Has(fsnotify.Create) && isVRChatLogFile(event.Name) {
				if filepath.Clean(event.Name) != lw.cleanLogPath && lw.onNewLogFile != nil {
					lw.onNewLogFile(event.Name)
				}
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if filepath.Clean(event.Name) == lw.cleanLogPath {
					if err := lw.readNewContent(); err != nil && lw.onError != nil {
						lw.onError(err)
					}
				}
			}
		case err, ok := <-lw.watcher.Errors:
			if !ok {
				return
			}
			if lw.onError != nil {
				lw.onError(err)
			}
		case <-ticker.C:
			// Periodic poll as fallback
			if err := lw.readNewContent(); err != nil && lw.onError != nil {
				lw.onError(err)
			}
		}
	}
}

func (lw *LogWatcher) readNewContent() error {
	lw.readMu.Lock()
	defer lw.readMu.Unlock()

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

	lw.mu.Lock()
	defer lw.mu.Unlock()
	if info.Size() < lw.offset {
		lw.offset = 0
	}
	if info.Size() <= lw.offset {
		return nil // No new content
	}
	startOffset := lw.offset

	if _, err := f.Seek(startOffset, 0 /* io.SeekStart */); err != nil {
		return err
	}

	endOffset := info.Size()
	lw.offset = endOffset

	// Stream lines without loading the entire new content into memory at once.
	lines := make([]string, 0, 512)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if len(lines) > 0 && lw.onNewData != nil {
		slog.Debug("new data detected", "path", lw.LogPath, "lines", len(lines))
		lw.onNewData(lines, startOffset, endOffset)
	}

	return nil
}

// collectLogFiles builds the list of all VRChat log files found in known
// platform-specific directories. It does not sort the results.
func collectLogFiles() []string {
	dirs := logDirectories()

	var files []string
	for _, dir := range dirs {
		expanded := expandHome(dir)
		matches, err := filepath.Glob(filepath.Join(expanded, "output_log_*.txt"))
		if err != nil {
			continue
		}
		files = append(files, matches...)
	}
	return files
}

// DetectLatestLogFile finds the most recent VRChat log file
func DetectLatestLogFile() (string, error) {
	candidates := collectLogFiles()

	if len(candidates) == 0 {
		return "", fmt.Errorf("no VRChat log files found in known locations")
	}

	sortByModTimeDesc(candidates)
	return candidates[0], nil
}

// DetectAllLogFiles finds all VRChat log files sorted newest first
func DetectAllLogFiles() ([]string, error) {
	candidates := collectLogFiles()

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no VRChat log files found in known locations")
	}

	sortByModTimeDesc(candidates)
	return candidates, nil
}

// sortByModTimeDesc sorts paths newest-first using a single os.Stat per file,
// avoiding the O(n²) stat calls that arise from calling os.Stat inside the
// sort comparator.
func sortByModTimeDesc(paths []string) {
	modTimes := make(map[string]time.Time, len(paths))
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil {
			modTimes[p] = info.ModTime()
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		return modTimes[paths[i]].After(modTimes[paths[j]])
	})
}

// logDirectories returns OS-specific VRChat log directories
func logDirectories() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	switch runtime.GOOS {
	case "windows":
		return []string{
			filepath.Join(os.Getenv("APPDATA"), "..", "LocalLow", "VRChat", "VRChat"),
			filepath.Join(os.Getenv("USERPROFILE"), "AppData", "LocalLow", "VRChat", "VRChat"),
		}
	case "linux":
		return []string{
			// Steam Proton (most common for Linux VRChat)
			filepath.Join(home, ".local", "share", "Steam", "steamapps", "compatdata", "438100", "pfx", "drive_c", "users", "steamuser", "AppData", "LocalLow", "VRChat", "VRChat"),
			// Flatpak Steam
			filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", "data", "Steam", "steamapps", "compatdata", "438100", "pfx", "drive_c", "users", "steamuser", "AppData", "LocalLow", "VRChat", "VRChat"),
		}
	case "darwin":
		return []string{
			filepath.Join(home, "Library", "Application Support", "com.vrchat.VRChat"),
		}
	default:
		return []string{}
	}
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.Getenv("HOME")
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func isVRChatLogFile(path string) bool {
	name := filepath.Base(path)
	matched, err := filepath.Match("output_log_*.txt", name)
	return err == nil && matched
}
