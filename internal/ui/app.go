package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/application"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/watcher"
)

// App is the main application controller
type App struct {
	fyneApp       fyne.App
	win           fyne.Window
	logPath       string
	service       *application.Service
	watcher       *watcher.LogWatcher
	mu            sync.Mutex
	lastStats     *stats.Stats
	lastHands     []*parser.Hand
	lastLocalSeat int
	rangeState    *HandRangeViewState
	historyState  *HandHistoryViewState
	metricState   *MetricVisibilityState

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

	win := a.NewWindow(lang.X("app.window.title", "VRC VRPoker Stats"))
	win.Resize(fyne.NewSize(1100, 700))
	win.SetMaster()

	appCtrl := &App{
		fyneApp:      a,
		win:          win,
		rangeState:   &HandRangeViewState{},
		historyState: &HandHistoryViewState{SelectedHandID: -1},
		metricState:  NewMetricVisibilityState(),
	}

	dbPath := filepath.Join(".", "vrpoker-stats.db")
	if sqliteRepo, err := persistence.NewSQLiteRepository(dbPath); err == nil {
		appCtrl.service = application.NewService(sqliteRepo)
	} else {
		appCtrl.service = application.NewService(persistence.NewMemoryRepository())
	}

	win.SetContent(appCtrl.buildUI())
	win.ShowAndRun()
}

func (a *App) buildUI() fyne.CanvasObject {
	// Status bar
	a.statusBar = widget.NewLabel(lang.X("app.status.initializing", "Initializing..."))

	// Tab content containers
	a.overviewContent = container.NewMax()
	a.posStatsContent = container.NewMax()
	a.handHistContent = container.NewMax()
	a.handRangeContent = container.NewMax()

	a.tabs = container.NewAppTabs(
		container.NewTabItem(lang.X("app.tab.overview", "Overview"), a.overviewContent),
		container.NewTabItem(lang.X("app.tab.position_stats", "Position Stats"), a.posStatsContent),
		container.NewTabItem(lang.X("app.tab.hand_range", "Hand Range"), a.handRangeContent),
		container.NewTabItem(lang.X("app.tab.hand_history", "Hand History"), a.handHistContent),
		container.NewTabItem(lang.X("app.tab.settings", "Settings"), NewSettingsTab(
			"",
			a.win,
			func(path string) { go a.changeLogFile(path) },
			a.metricState,
			func() { a.doRefreshCurrentTab() },
		)),
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
	a.doSetStatus(lang.X("app.status.importing", "Importing VRChat logs..."))

	logPath, err := a.service.BootstrapImportAllLogs()
	if err != nil {
		a.doSetStatus(lang.X("app.error.no_log_file", "No log file found: {{.Error}} — configure in Settings.", map[string]any{"Error": err}))
		return
	}

	a.doUpdateStats()
	a.changeLogFile(logPath)
}

func (a *App) changeLogFile(path string) {
	// Stop existing watcher
	a.mu.Lock()
	if a.watcher != nil {
		a.watcher.Stop()
		a.watcher = nil
	}
	a.logPath = path
	a.mu.Unlock()

	a.doSetStatus(lang.X("app.status.loading", "Loading: {{.Path}}", map[string]any{"Path": shortPath(path)}))

	// Parse entire existing file first
	if err := a.service.ChangeLogFile(path); err != nil {
		a.doSetStatus(lang.X("app.error.read_log", "Error reading log: {{.Error}}", map[string]any{"Error": err}))
		return
	}
	a.doUpdateStats()
	a.doSetStatus(lang.X("app.status.loaded", "Loaded: {{.Path}} — watching for changes…", map[string]any{"Path": shortPath(path)}))

	// Start tail watcher from current end-of-file
	w, err := watcher.NewLogWatcher(path)
	if err != nil {
		a.doSetStatus(lang.X("app.error.watcher", "Watcher error: {{.Error}}", map[string]any{"Error": err}))
		return
	}

	w.OnNewData = func(lines []string, startOffset int64, endOffset int64) {
		if err := a.service.ImportLines(lines, startOffset, endOffset); err != nil {
			a.doSetStatus(lang.X("app.error.import", "Import error: {{.Error}}", map[string]any{"Error": err}))
			return
		}
		a.doUpdateStats()
	}
	w.OnError = func(err error) {
		a.doSetStatus(lang.X("app.error.watcher", "Watcher error: {{.Error}}", map[string]any{"Error": err}))
	}

	// Start from end of file (we already parsed the full history above)
	if info, err := os.Stat(path); err == nil {
		w.SetOffset(info.Size())
	}

	if err := w.Start(); err != nil {
		a.doSetStatus(lang.X("app.error.watcher_start", "Failed to start watcher: {{.Error}}", map[string]any{"Error": err}))
		return
	}

	a.mu.Lock()
	a.watcher = w
	a.mu.Unlock()

	go a.statusLoop()
}

// doUpdateStats recalculates stats and schedules a UI refresh on the main thread.
func (a *App) doUpdateStats() {
	s, hands, localSeat, err := a.service.Snapshot()
	if err != nil {
		a.doSetStatus(lang.X("app.error.snapshot", "Snapshot error: {{.Error}}", map[string]any{"Error": err}))
		return
	}

	a.mu.Lock()
	a.lastStats = s
	a.lastHands = hands
	a.lastLocalSeat = localSeat
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
	localSeat := a.lastLocalSeat
	hands := a.lastHands
	a.mu.Unlock()

	selected := a.tabs.SelectedIndex()
	switch selected {
	case 0: // Overview
		obj := NewOverviewTab(s, a.metricState, a.win)
		a.overviewContent.Objects = []fyne.CanvasObject{obj}
		a.overviewContent.Refresh()
	case 1: // Position Stats
		obj := NewPositionStatsTab(s, a.metricState)
		a.posStatsContent.Objects = []fyne.CanvasObject{obj}
		a.posStatsContent.Refresh()
	case 2: // Hand Range
		obj := NewHandRangeTab(s, a.win, a.rangeState)
		a.handRangeContent.Objects = []fyne.CanvasObject{obj}
		a.handRangeContent.Refresh()
	case 3: // Hand History
		obj := NewHandHistoryTab(hands, localSeat, a.historyState)
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
			a.doSetStatus(lang.X("app.status.watching", "Watching: {{.Path}} | Hands: {{.Hands}} | VPIP: {{.VPIP}}% | PFR: {{.PFR}}%", map[string]any{"Path": shortPath(path), "Hands": s.TotalHands, "VPIP": fmt.Sprintf("%.1f", s.VPIPRate()), "PFR": fmt.Sprintf("%.1f", s.PFRRate())}))
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
