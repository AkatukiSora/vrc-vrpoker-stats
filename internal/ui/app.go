package ui

import (
	"bufio"
	"fmt"
	"os"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/watcher"
)

// App is the main application controller
type App struct {
	fyneApp  fyne.App
	win      fyne.Window
	logPath  string
	parser   *parser.Parser
	calc     *stats.Calculator
	watcher  *watcher.LogWatcher
	mu       sync.Mutex
	lastStats *stats.Stats

	// UI tabs content containers (for refresh)
	overviewContent  *fyne.Container
	posStatsContent  *fyne.Container
	handHistContent  *fyne.Container
	handRangeContent *fyne.Container
	statusBar        *widget.Label
	tabs             *container.AppTabs
}

// Run starts the application
func Run() {
	a := app.New()
	a.Settings().SetTheme(newPokerTheme())

	win := a.NewWindow("VRC VRPoker Stats")
	win.Resize(fyne.NewSize(1100, 700))
	win.SetMaster()

	appCtrl := &App{
		fyneApp: a,
		win:     win,
		parser:  parser.NewParser(),
		calc:    stats.NewCalculator(),
	}

	win.SetContent(appCtrl.buildUI())
	win.ShowAndRun()
}

func (a *App) buildUI() fyne.CanvasObject {
	// Status bar
	a.statusBar = widget.NewLabel("Initializing...")

	// Tab content containers
	a.overviewContent = container.NewMax()
	a.posStatsContent = container.NewMax()
	a.handHistContent = container.NewMax()
	a.handRangeContent = container.NewMax()

	a.tabs = container.NewAppTabs(
		container.NewTabItem("Overview", a.overviewContent),
		container.NewTabItem("Position Stats", a.posStatsContent),
		container.NewTabItem("Hand Range", a.handRangeContent),
		container.NewTabItem("Hand History", a.handHistContent),
		container.NewTabItem("Settings", NewSettingsTab("", a.win, func(path string) {
			go a.changeLogFile(path)
		})),
	)
	a.tabs.SetTabLocation(container.TabLocationTop)

	// Refresh content when tab is selected
	a.tabs.OnChanged = func(_ *container.TabItem) {
		a.doRefreshCurrentTab()
	}

	// Auto-detect and start watching log file
	go a.initLogFile()

	return container.NewBorder(
		nil,
		container.NewPadded(a.statusBar),
		nil, nil,
		a.tabs,
	)
}

func (a *App) initLogFile() {
	a.doSetStatus("Searching for VRChat log files...")

	logPath, err := watcher.DetectLatestLogFile()
	if err != nil {
		a.doSetStatus(fmt.Sprintf("No log file found: %v — configure in Settings.", err))
		return
	}

	a.changeLogFile(logPath)
}

func (a *App) changeLogFile(path string) {
	// Stop existing watcher
	a.mu.Lock()
	if a.watcher != nil {
		a.watcher.Stop()
		a.watcher = nil
	}
	// Reset parser
	a.parser = parser.NewParser()
	a.logPath = path
	a.mu.Unlock()

	a.doSetStatus(fmt.Sprintf("Loading: %s", shortPath(path)))

	// Parse entire existing file first
	if err := a.parseFullFile(path); err != nil {
		a.doSetStatus(fmt.Sprintf("Error reading log: %v", err))
		return
	}
	a.doUpdateStats()
	a.doSetStatus(fmt.Sprintf("Loaded: %s — watching for changes…", shortPath(path)))

	// Start tail watcher from current end-of-file
	w, err := watcher.NewLogWatcher(path)
	if err != nil {
		a.doSetStatus(fmt.Sprintf("Watcher error: %v", err))
		return
	}

	w.OnNewData = func(lines []string) {
		a.mu.Lock()
		p := a.parser
		for _, line := range lines {
			_ = p.ParseLine(line)
		}
		a.mu.Unlock()
		a.doUpdateStats()
	}
	w.OnError = func(err error) {
		a.doSetStatus(fmt.Sprintf("Watcher error: %v", err))
	}

	// Start from end of file (we already parsed the full history above)
	if info, err := os.Stat(path); err == nil {
		w.SetOffset(info.Size())
	}

	if err := w.Start(); err != nil {
		a.doSetStatus(fmt.Sprintf("Failed to start watcher: %v", err))
		return
	}

	a.mu.Lock()
	a.watcher = w
	a.mu.Unlock()

	go a.statusLoop()
}

func (a *App) parseFullFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	a.mu.Lock()
	p := a.parser
	a.mu.Unlock()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		_ = p.ParseLine(scanner.Text())
	}
	return scanner.Err()
}

// doUpdateStats recalculates stats and schedules a UI refresh on the main thread.
func (a *App) doUpdateStats() {
	a.mu.Lock()
	p := a.parser
	localSeat := p.GetLocalSeat()
	hands := p.GetHands()
	a.mu.Unlock()

	s := a.calc.Calculate(hands, localSeat)

	a.mu.Lock()
	a.lastStats = s
	a.mu.Unlock()

	// All UI updates must happen on the Fyne main thread
	fyne.Do(func() {
		a.doRefreshCurrentTab()
	})
}

// doRefreshCurrentTab rebuilds the content for the currently selected tab.
// MUST be called from the Fyne main thread (or wrapped in fyne.Do).
func (a *App) doRefreshCurrentTab() {
	if a.tabs == nil {
		return
	}

	a.mu.Lock()
	s := a.lastStats
	p := a.parser
	localSeat := p.GetLocalSeat()
	hands := p.GetHands()
	a.mu.Unlock()

	selected := a.tabs.SelectedIndex()
	switch selected {
	case 0: // Overview
		obj := NewOverviewTab(s)
		a.overviewContent.Objects = []fyne.CanvasObject{obj}
		a.overviewContent.Refresh()
	case 1: // Position Stats
		obj := NewPositionStatsTab(s)
		a.posStatsContent.Objects = []fyne.CanvasObject{obj}
		a.posStatsContent.Refresh()
	case 2: // Hand Range
		obj := NewHandRangeTab(s, a.win)
		a.handRangeContent.Objects = []fyne.CanvasObject{obj}
		a.handRangeContent.Refresh()
	case 3: // Hand History
		obj := NewHandHistoryTab(hands, localSeat)
		a.handHistContent.Objects = []fyne.CanvasObject{obj}
		a.handHistContent.Refresh()
	}
}

// doSetStatus safely updates the status bar label from any goroutine.
func (a *App) doSetStatus(msg string) {
	fyne.Do(func() {
		if a.statusBar != nil {
			a.statusBar.SetText(msg)
		}
	})
}

func (a *App) statusLoop() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		a.mu.Lock()
		s := a.lastStats
		path := a.logPath
		a.mu.Unlock()

		if s != nil {
			a.doSetStatus(fmt.Sprintf(
				"Watching: %s | Hands: %d | VPIP: %.1f%% | PFR: %.1f%%",
				shortPath(path), s.TotalHands, s.VPIPRate(), s.PFRRate(),
			))
		}
	}
}

func shortPath(path string) string {
	if len(path) > 60 {
		return "..." + path[len(path)-57:]
	}
	return path
}

// newPokerTheme returns a dark theme for the poker stats app
func newPokerTheme() fyne.Theme {
	return defaultPokerTheme{}
}
