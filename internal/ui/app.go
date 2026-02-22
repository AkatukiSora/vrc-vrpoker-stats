package ui

import (
	"context"
	"image/color"
	"os"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/watcher"
)

type appService interface {
	BootstrapImportAllLogs(ctx context.Context) (string, error)
	ChangeLogFile(ctx context.Context, path string) error
	ImportLines(ctx context.Context, sourcePath string, lines []string, startOffset int64, endOffset int64) error
	Snapshot(ctx context.Context) (*stats.Stats, []*parser.Hand, int, error)
	NextOffset(ctx context.Context, path string) (int64, error)
	Close() error
}

type appTab int

const (
	tabOverview appTab = iota
	tabPositionStats
	tabHandRange
	tabHandHistory
	tabSettings
)

type AppMetadata struct {
	Version       string
	Commit        string
	BuildDate     string
	RepositoryURL string
}

// App is the main application controller
type App struct {
	ctx             context.Context
	cancel          context.CancelFunc
	fyneApp         fyne.App
	win             fyne.Window
	logPath         string
	service         appService
	watcher         *watcher.LogWatcher
	watcherGen      uint64
	changeReqCh     chan string
	workerStopCh    chan struct{}
	workerWG        sync.WaitGroup
	closeOnce       sync.Once
	isShuttingDown  bool
	mu              sync.Mutex
	lastStats       *stats.Stats
	lastHands       []*parser.Hand
	lastLocalSeat   int
	rangeState      *HandRangeViewState
	historyState    *HandHistoryViewState
	metricState     *MetricVisibilityState
	settingsTab     fyne.CanvasObject
	settingsPath    string
	overviewView    *overviewTabView
	positionView    *positionStatsTabView
	handRangeView   *handRangeTabView
	handHistoryView *handHistoryTabView
	currentTab      appTab
	navExpanded     bool

	mainContent *fyne.Container
	railPanel   *fyne.Container
	overlayNav  *fyne.Container

	statusText *widget.Label
	metadata   AppMetadata
}

// Run starts the application
func Run(service appService, metadata AppMetadata) {
	if service == nil {
		return
	}

	a := app.New()
	a.Settings().SetTheme(newPokerTheme())

	win := a.NewWindow(lang.X("app.window.title", "VRC VRPoker Stats"))
	win.Resize(fyne.NewSize(1280, 820))
	win.SetMaster()

	ctx, cancel := context.WithCancel(context.Background())

	appCtrl := &App{
		ctx:          ctx,
		cancel:       cancel,
		fyneApp:      a,
		win:          win,
		service:      service,
		rangeState:   &HandRangeViewState{},
		historyState: &HandHistoryViewState{SelectedHandID: -1},
		metricState:  NewMetricVisibilityState(),
		currentTab:   tabOverview,
		metadata:     metadata,
	}
	appCtrl.startLogChangeWorker()
	win.SetCloseIntercept(func() {
		appCtrl.shutdown()
		win.SetCloseIntercept(nil)
		win.Close()
	})

	win.SetContent(appCtrl.buildUI())
	win.ShowAndRun()
}

func (a *App) buildUI() fyne.CanvasObject {
	a.statusText = widget.NewLabel(lang.X("app.status.initializing", "Initializing..."))
	a.statusText.Wrapping = fyne.TextWrapOff

	statusRow := container.NewHBox(widget.NewIcon(theme.InfoIcon()), a.statusText)
	statusBar := newSectionCard(statusRow)

	a.mainContent = container.NewMax()
	a.railPanel = container.NewMax()
	a.overlayNav = container.NewMax()
	a.rebuildNavigation()
	a.doRefreshCurrentTab()

	// Auto-detect and start watching log file
	go a.initLogFile()

	content := container.NewStack(
		container.NewBorder(nil, nil, a.railPanel, nil, a.mainContent),
		a.overlayNav,
	)

	return container.NewBorder(
		nil,
		container.NewPadded(statusBar),
		nil,
		nil,
		content,
	)
}

type navOverlayLayout struct {
	panelWidth float32
}

func (l navOverlayLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) < 2 {
		return
	}
	dim := objects[0]
	panel := objects[1]

	dim.Move(fyne.NewPos(0, 0))
	dim.Resize(size)

	w := l.panelWidth
	if w > size.Width {
		w = size.Width
	}
	panel.Move(fyne.NewPos(0, 0))
	panel.Resize(fyne.NewSize(w, size.Height))
}

func (l navOverlayLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(l.panelWidth, 0)
}

func (a *App) setNavExpanded(expanded bool) {
	if a.navExpanded == expanded {
		return
	}
	a.navExpanded = expanded
	a.rebuildNavigation()
}

func (a *App) rebuildNavigation() {
	if a.railPanel == nil || a.overlayNav == nil {
		return
	}

	navItems := []struct {
		tab      appTab
		key      string
		fallback string
		icon     fyne.Resource
	}{
		{tab: tabOverview, key: "app.tab.overview", fallback: "Overview", icon: theme.HomeIcon()},
		{tab: tabPositionStats, key: "app.tab.position_stats", fallback: "Position Stats", icon: theme.GridIcon()},
		{tab: tabHandRange, key: "app.tab.hand_range", fallback: "Hand Range", icon: theme.ColorPaletteIcon()},
		{tab: tabHandHistory, key: "app.tab.hand_history", fallback: "Hand History", icon: theme.HistoryIcon()},
		{tab: tabSettings, key: "app.tab.settings", fallback: "Settings", icon: theme.SettingsIcon()},
	}

	railButtons := make([]fyne.CanvasObject, 0, len(navItems)+2)
	expandBtn := widget.NewButtonWithIcon("", theme.NavigateNextIcon(), func() {
		a.setNavExpanded(true)
	})
	expandBtn.Importance = widget.LowImportance
	railButtons = append(railButtons, expandBtn, newSectionDivider())

	flyoutButtons := make([]fyne.CanvasObject, 0, len(navItems)+2)
	closeBtn := widget.NewButtonWithIcon(lang.X("app.nav.collapse", "Collapse"), theme.NavigateBackIcon(), func() {
		a.setNavExpanded(false)
	})
	closeBtn.Importance = widget.LowImportance
	flyoutButtons = append(flyoutButtons, closeBtn, newSectionDivider())

	for _, item := range navItems {
		item := item

		railBtn := widget.NewButtonWithIcon("", item.icon, func() {
			a.currentTab = item.tab
			a.doRefreshCurrentTab()
			a.rebuildNavigation()
		})
		if a.currentTab == item.tab {
			railBtn.Importance = widget.HighImportance
		}
		railButtons = append(railButtons, railBtn)

		flyoutBtn := widget.NewButtonWithIcon(lang.X(item.key, item.fallback), item.icon, func() {
			a.currentTab = item.tab
			a.doRefreshCurrentTab()
			a.setNavExpanded(false)
		})
		if a.currentTab == item.tab {
			flyoutBtn.Importance = widget.HighImportance
		}
		flyoutButtons = append(flyoutButtons, flyoutBtn)
	}

	railHeader := container.NewCenter(widget.NewIcon(theme.FyneLogo()))
	railBody := container.NewBorder(railHeader, nil, nil, nil, container.NewVBox(railButtons...))
	railCard := newSectionCard(railBody)
	railWidth := canvas.NewRectangle(color.Transparent)
	railWidth.SetMinSize(fyne.NewSize(64, 0))
	a.railPanel.Objects = []fyne.CanvasObject{container.NewStack(railWidth, railCard)}
	a.railPanel.Refresh()

	if !a.navExpanded {
		a.overlayNav.Objects = nil
		a.overlayNav.Refresh()
		return
	}

	flyoutTitle := widget.NewLabelWithStyle(lang.X("app.window.title", "VRC VRPoker Stats"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	flyoutBody := container.NewBorder(container.NewVBox(flyoutTitle, newSectionDivider()), nil, nil, nil, container.NewVBox(flyoutButtons...))
	flyoutCard := newSectionCard(flyoutBody)
	panelWidth := canvas.NewRectangle(color.Transparent)
	panelWidth.SetMinSize(fyne.NewSize(220, 0))
	panel := container.NewStack(panelWidth, flyoutCard)

	dimBg := canvas.NewRectangle(color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x80})
	dimTap := newTapArea(func() {
		a.setNavExpanded(false)
	})
	dim := container.NewStack(dimBg, dimTap)

	overlay := container.New(navOverlayLayout{panelWidth: 220}, dim, panel)
	a.overlayNav.Objects = []fyne.CanvasObject{overlay}
	a.overlayNav.Refresh()
}

func (a *App) initLogFile() {
	a.doSetStatus(lang.X("app.status.importing", "Importing VRChat logs..."))

	logPath, err := a.service.BootstrapImportAllLogs(a.ctx)
	if err != nil {
		a.doSetStatus(lang.X("app.error.no_log_file", "No log file found: {{.Error}} — configure in Settings.", map[string]any{"Error": err}))
		return
	}

	a.doUpdateStats()
	a.requestLogFileChange(logPath)
}

func (a *App) startLogChangeWorker() {
	a.mu.Lock()
	if a.changeReqCh != nil {
		a.mu.Unlock()
		return
	}
	a.changeReqCh = make(chan string, 1)
	a.workerStopCh = make(chan struct{})
	changeReqCh := a.changeReqCh
	stopCh := a.workerStopCh
	a.mu.Unlock()

	a.workerWG.Add(1)
	go func() {
		defer a.workerWG.Done()
		for {
			select {
			case <-stopCh:
				return
			case path := <-changeReqCh:
				a.changeLogFile(path)
			}
		}
	}()
}

func (a *App) requestLogFileChange(path string) {
	if path == "" {
		return
	}
	a.mu.Lock()
	if a.isShuttingDown || a.changeReqCh == nil {
		a.mu.Unlock()
		return
	}
	changeReqCh := a.changeReqCh
	a.mu.Unlock()

	select {
	case changeReqCh <- path:
	default:
		select {
		case <-changeReqCh:
		default:
		}
		select {
		case changeReqCh <- path:
		default:
		}
	}
}

func (a *App) changeLogFile(path string) {
	// Stop existing watcher and invalidate stale callbacks.
	a.mu.Lock()
	a.watcherGen++
	gen := a.watcherGen
	prevWatcher := a.watcher
	a.watcher = nil
	a.mu.Unlock()
	if prevWatcher != nil {
		prevWatcher.Stop()
	}

	a.doSetStatus(lang.X("app.status.loading", "Loading: {{.Path}}", map[string]any{"Path": shortPath(path)}))

	// Parse entire existing file first
	if err := a.service.ChangeLogFile(a.ctx, path); err != nil {
		a.doSetStatus(lang.X("app.error.read_log", "Error reading log: {{.Error}}", map[string]any{"Error": err}))
		return
	}

	a.mu.Lock()
	a.logPath = path
	a.mu.Unlock()

	a.doUpdateStats()
	a.doSetStatus(lang.X("app.status.loaded", "Loaded: {{.Path}} — watching for changes…", map[string]any{"Path": shortPath(path)}))

	// Start tail watcher from current end-of-file
	w, err := watcher.NewLogWatcher(path)
	if err != nil {
		a.doSetStatus(lang.X("app.error.watcher", "Watcher error: {{.Error}}", map[string]any{"Error": err}))
		return
	}

	w.OnNewData = func(lines []string, startOffset int64, endOffset int64) {
		if !a.isCurrentWatcherGeneration(gen) {
			return
		}
		if err := a.service.ImportLines(a.ctx, path, lines, startOffset, endOffset); err != nil {
			a.doSetStatus(lang.X("app.error.import", "Import error: {{.Error}}", map[string]any{"Error": err}))
			return
		}
		if !a.isCurrentWatcherGeneration(gen) {
			return
		}
		a.doUpdateStats()
	}
	w.OnError = func(err error) {
		if !a.isCurrentWatcherGeneration(gen) {
			return
		}
		a.doSetStatus(lang.X("app.error.watcher", "Watcher error: {{.Error}}", map[string]any{"Error": err}))
	}

	// Start from end of file (we already parsed the full history above)
	if offset, err := a.service.NextOffset(a.ctx, path); err == nil && offset > 0 {
		w.SetOffset(offset)
	} else if info, err := os.Stat(path); err == nil {
		w.SetOffset(info.Size())
	}

	if err := w.Start(); err != nil {
		a.doSetStatus(lang.X("app.error.watcher_start", "Failed to start watcher: {{.Error}}", map[string]any{"Error": err}))
		return
	}

	a.mu.Lock()
	if a.watcherGen == gen && !a.isShuttingDown {
		a.watcher = w
	} else {
		w.Stop()
	}
	a.mu.Unlock()
}

func (a *App) isCurrentWatcherGeneration(gen uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.isShuttingDown && a.watcherGen == gen
}

func (a *App) shutdown() {
	a.closeOnce.Do(func() {
		a.mu.Lock()
		a.isShuttingDown = true
		a.watcherGen++
		if a.cancel != nil {
			a.cancel()
		}
		stopCh := a.workerStopCh
		prevWatcher := a.watcher
		a.watcher = nil
		a.mu.Unlock()

		if prevWatcher != nil {
			prevWatcher.Stop()
		}
		if stopCh != nil {
			close(stopCh)
		}
		a.workerWG.Wait()
		if a.service != nil {
			_ = a.service.Close()
		}
	})
}

// doUpdateStats recalculates stats and schedules a UI refresh on the main thread.
func (a *App) doUpdateStats() {
	s, hands, localSeat, err := a.service.Snapshot(a.ctx)
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
		if a.currentTab != tabSettings {
			a.doRefreshCurrentTab()
		}
	})
}

// doRefreshCurrentTab rebuilds the content for the currently selected tab.
// MUST be called from the Fyne main thread (or wrapped in fyne.Do).
func (a *App) doRefreshCurrentTab() {
	if a.mainContent == nil {
		return
	}

	a.mu.Lock()
	s := a.lastStats
	localSeat := a.lastLocalSeat
	hands := a.lastHands
	path := a.logPath
	a.mu.Unlock()

	var obj fyne.CanvasObject
	switch a.currentTab {
	case tabOverview:
		if a.overviewView == nil {
			a.overviewView = newOverviewTabView(a.win, a.metricState)
		}
		a.overviewView.Update(s)
		obj = a.overviewView.CanvasObject()
	case tabPositionStats:
		if a.positionView == nil {
			a.positionView = newPositionStatsTabView(a.metricState)
		}
		a.positionView.Update(s)
		obj = a.positionView.CanvasObject()
	case tabHandRange:
		if a.handRangeView == nil {
			a.handRangeView = newHandRangeTabView(a.win, a.rangeState)
		}
		a.handRangeView.Update(s)
		obj = a.handRangeView.CanvasObject()
	case tabHandHistory:
		if a.handHistoryView == nil {
			a.handHistoryView = newHandHistoryTabView(a.historyState)
		}
		a.handHistoryView.Update(hands, localSeat)
		obj = a.handHistoryView.CanvasObject()
	case tabSettings:
		if a.settingsTab == nil || a.settingsPath != path {
			a.settingsTab = NewSettingsTab(
				path,
				a.win,
				func(nextPath string) { a.requestLogFileChange(nextPath) },
				a.metricState,
				a.metadata,
				func() {
					a.mu.Lock()
					activeTab := a.currentTab
					a.mu.Unlock()
					if activeTab != tabSettings {
						a.doRefreshCurrentTab()
					}
				},
			)
			a.settingsPath = path
		}
		obj = a.settingsTab
	default:
		obj = NewOverviewTab(s, a.metricState, a.win)
	}

	a.mainContent.Objects = []fyne.CanvasObject{obj}
	a.mainContent.Refresh()
}

// doSetStatus safely updates the status bar label from any goroutine.
func (a *App) doSetStatus(msg string) {
	fyne.Do(func() {
		if a.statusText != nil {
			a.statusText.SetText(msg)
		}
	})
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
