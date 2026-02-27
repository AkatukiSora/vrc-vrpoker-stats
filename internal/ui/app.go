package ui

import (
	"context"
	"image/color"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/application"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/watcher"
)

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
	dbPath          string
	service         application.AppService
	watcher         *watcher.LogWatcher
	watcherGen      uint64
	changeReqCh     chan string
	workerStopCh    chan struct{}
	workerWG        sync.WaitGroup
	closeOnce       sync.Once
	isShuttingDown  atomic.Bool
	mu              sync.Mutex
	updateMu        sync.Mutex
	debounceTimer   *time.Timer
	debounceMu      sync.Mutex
	lastStats       *stats.Stats
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
	// historyPageRunning is 1 while loadHandHistoryPage is executing.
	// pendingHistoryPage holds the next page to load (-1 = none).
	historyPageRunning int32
	pendingHistoryPage int32

	mainContent *fyne.Container
	railPanel   *fyne.Container
	overlayNav  *fyne.Container

	statusText *widget.Label
	metadata   AppMetadata
}

// Run starts the application
func Run(service application.AppService, metadata AppMetadata, dbPath string) {
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
		ctx:                ctx,
		cancel:             cancel,
		fyneApp:            a,
		win:                win,
		dbPath:             dbPath,
		service:            service,
		rangeState:         &HandRangeViewState{},
		historyState:       &HandHistoryViewState{},
		metricState:        NewMetricVisibilityState(),
		currentTab:         tabOverview,
		metadata:           metadata,
		pendingHistoryPage: -1,
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
	slog.Info("bootstrapping log import")

	stopRefresh := make(chan struct{})
	defer close(stopRefresh)
	a.workerWG.Add(1)
	go func() {
		defer a.workerWG.Done()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-stopRefresh:
				return
			case <-ticker.C:
				a.doUpdateStats()
			}
		}
	}()

	// Show any data already in the DB before bootstrap begins.
	// This makes the UI responsive even when re-importing takes a while.
	a.doUpdateStats()

	a.doSetStatus(lang.X("app.status.importing", "Importing VRChat logs..."))

	onProgress := func(p application.BootstrapProgress) {
		msg := lang.X("app.status.importing_progress",
			"Importing logs… ({{.Current}}/{{.Total}}) {{.File}}",
			map[string]any{
				"Current": p.Current,
				"Total":   p.Total,
				"File":    shortPath(p.Path),
			})
		a.doSetStatus(msg)
		// Update UI with data available so far so the user sees something.
		a.doUpdateStats()
	}

	logPath, err := a.service.BootstrapImportAllLogsWithProgress(a.ctx, onProgress)
	if err != nil {
		slog.Error("no log file found during bootstrap", "error", err)
		a.doSetStatus(lang.X("app.error.no_log_file", "No log file found: {{.Error}} — configure in Settings.", map[string]any{"Error": err}))
		return
	}

	slog.Info("bootstrap complete", "logPath", logPath)
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
	if a.isShuttingDown.Load() || a.changeReqCh == nil {
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
		slog.Info("stopping previous watcher")
		prevWatcher.Stop()
	}

	slog.Info("changing log file", "path", path)
	a.doSetStatus(lang.X("app.status.loading", "Loading: {{.Path}}", map[string]any{"Path": shortPath(path)}))

	// Parse entire existing file first
	if err := a.service.ChangeLogFile(a.ctx, path); err != nil {
		slog.Error("failed to read log file", "path", path, "error", err)
		a.doSetStatus(lang.X("app.error.read_log", "Error reading log: {{.Error}}", map[string]any{"Error": err}))
		return
	}

	a.mu.Lock()
	a.logPath = path
	a.mu.Unlock()

	a.doUpdateStats()
	slog.Info("log file loaded", "path", path)
	a.doSetStatus(lang.X("app.status.loaded", "Loaded: {{.Path}} — watching for changes…", map[string]any{"Path": shortPath(path)}))

	// Start tail watcher from current end-of-file
	w, err := watcher.NewLogWatcher(path, watcher.WatcherConfig{
		OnNewData: func(lines []string, startOffset int64, endOffset int64) {
			if !a.isCurrentWatcherGeneration(gen) {
				return
			}
			if err := a.service.ImportLines(a.ctx, path, lines, startOffset, endOffset); err != nil {
				slog.Error("import error", "path", path, "error", err)
				a.doSetStatus(lang.X("app.error.import", "Import error: {{.Error}}", map[string]any{"Error": err}))
				return
			}
			if !a.isCurrentWatcherGeneration(gen) {
				return
			}
			// Debounce: collapse rapid-fire OnNewData events into a single update
			a.debounceMu.Lock()
			if a.debounceTimer != nil {
				a.debounceTimer.Stop()
			}
			a.debounceTimer = time.AfterFunc(time.Second, func() {
				if a.isCurrentWatcherGeneration(gen) {
					a.doUpdateStats()
				}
			})
			a.debounceMu.Unlock()
		},
		OnNewLogFile: func(nextPath string) {
			if !a.isCurrentWatcherGeneration(gen) {
				return
			}
			slog.Info("new log file detected", "path", nextPath)
			// Mark the current (now-old) log file as fully imported so future
			// bootstrap runs skip it entirely.
			a.service.MarkLogFullyImported(a.ctx, path)
			a.requestLogFileChange(nextPath)
		},
		OnError: func(err error) {
			if !a.isCurrentWatcherGeneration(gen) {
				return
			}
			slog.Error("watcher error", "path", path, "error", err)
			a.doSetStatus(lang.X("app.error.watcher", "Watcher error: {{.Error}}", map[string]any{"Error": err}))
		},
	})
	if err != nil {
		slog.Error("watcher creation failed", "path", path, "error", err)
		a.doSetStatus(lang.X("app.error.watcher", "Watcher error: {{.Error}}", map[string]any{"Error": err}))
		return
	}

	// Start from end of file (we already parsed the full history above)
	if offset, err := a.service.NextOffset(a.ctx, path); err == nil && offset > 0 {
		w.SetOffset(offset)
	} else if info, err := os.Stat(path); err == nil {
		w.SetOffset(info.Size())
	}

	if err := w.Start(); err != nil {
		slog.Error("watcher start failed", "path", path, "error", err)
		a.doSetStatus(lang.X("app.error.watcher_start", "Failed to start watcher: {{.Error}}", map[string]any{"Error": err}))
		return
	}
	slog.Info("watcher started", "path", path)

	a.mu.Lock()
	if a.watcherGen == gen && !a.isShuttingDown.Load() {
		a.watcher = w
	} else {
		w.Stop()
	}
	a.mu.Unlock()
}

func (a *App) isCurrentWatcherGeneration(gen uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.isShuttingDown.Load() && a.watcherGen == gen
}

func (a *App) shutdown() {
	a.closeOnce.Do(func() {
		slog.Info("shutting down")
		a.mu.Lock()
		a.isShuttingDown.Store(true)
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

// doResetDB shuts down the app, deletes the database file, and restarts the process.
func (a *App) doResetDB() {
	a.shutdown()
	if a.dbPath != "" {
		_ = os.Remove(a.dbPath)
	}
	restartSelf()
	a.fyneApp.Quit()
}

func (a *App) doUpdateStats() {
	if !a.updateMu.TryLock() {
		return
	}
	defer a.updateMu.Unlock()

	s, localSeat, err := a.service.Stats(a.ctx, persistence.HandFilter{})
	if err != nil {
		slog.Error("stats failed", "error", err)
		a.doSetStatus(lang.X("app.error.stats", "Stats error: {{.Error}}", map[string]any{"Error": err}))
		return
	}

	handCount := 0
	if s != nil {
		handCount = s.TotalHands
	}
	slog.Info("stats updated", "hands", handCount, "localSeat", localSeat)

	a.mu.Lock()
	a.lastStats = s
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
	localSeat := a.lastLocalSeat
	lastStats := a.lastStats
	path := a.logPath
	a.mu.Unlock()

	var obj fyne.CanvasObject
	switch a.currentTab {
	case tabOverview:
		if a.overviewView == nil {
			a.overviewView = newOverviewTabView(a.win, a.metricState)
		}
		a.overviewView.Update(lastStats, localSeat)
		obj = a.overviewView.CanvasObject()
	case tabPositionStats:
		if a.positionView == nil {
			a.positionView = newPositionStatsTabView(a.metricState)
		}
		a.positionView.Update(lastStats, localSeat)
		obj = a.positionView.CanvasObject()
	case tabHandRange:
		if a.handRangeView == nil {
			a.handRangeView = newHandRangeTabView(a.win, a.rangeState)
		}
		a.handRangeView.Update(lastStats, localSeat)
		obj = a.handRangeView.CanvasObject()
	case tabHandHistory:
		isNew := a.handHistoryView == nil
		if isNew {
			a.handHistoryView = newHandHistoryTabView(a.historyState, func(page int) {
				go a.loadHandHistoryPage(page)
			}, func(uid string) {
				go a.loadHandDetail(uid)
			})
		}
		// Show current (possibly stale) state immediately.
		obj = a.handHistoryView.CanvasObject()
		// On first creation load page 0.  On subsequent refreshes (e.g. triggered
		// by doUpdateStats during an import) reload the current page so the total
		// count stays accurate and newly imported hands are visible.
		if isNew {
			go a.loadHandHistoryPage(0)
		} else {
			go a.loadHandHistoryPage(a.handHistoryView.page)
		}
	case tabSettings:
		if a.settingsTab == nil || a.settingsPath != path {
			dbPath := a.dbPath
			a.settingsTab = NewSettingsTab(SettingsTabConfig{
				CurrentPath:  path,
				Window:       a.win,
				OnPathChange: func(nextPath string) { a.requestLogFileChange(nextPath) },
				MetricState:  a.metricState,
				Metadata:     a.metadata,
				OnMetricsChange: func() {
					a.mu.Lock()
					activeTab := a.currentTab
					a.mu.Unlock()
					if activeTab != tabSettings {
						a.doRefreshCurrentTab()
					}
				},
				DBPath:  dbPath,
				OnReset: func() { a.doResetDB() },
			})
			a.settingsPath = path
		}
		obj = a.settingsTab
	default:
		if a.overviewView == nil {
			a.overviewView = newOverviewTabView(a.win, a.metricState)
		}
		a.overviewView.Update(lastStats, localSeat)
		obj = a.overviewView.CanvasObject()
	}

	a.mainContent.Objects = []fyne.CanvasObject{obj}
	a.mainContent.Refresh()
}

func (a *App) buildHandHistoryFilter() persistence.HandFilter {
	var filter persistence.HandFilter
	if a.handHistoryView == nil {
		return filter
	}

	selectedPocket := a.handHistoryView.handFilter.PocketCategories
	if len(selectedPocket) > 0 {
		ids := make([]int, 0, len(selectedPocket))
		for _, cat := range selectedPocket {
			ids = append(ids, int(cat)+1)
		}
		filter.PocketCategoryIDs = ids
	}

	selectedFinal := a.handHistoryView.handFilter.FinalClasses
	if len(selectedFinal) > 0 {
		ids := make([]int, 0, len(selectedFinal))
		for _, cls := range selectedFinal {
			id := stats.MadeHandClassID(cls)
			if id > 0 {
				ids = append(ids, id)
			}
		}
		filter.FinalClassIDs = ids
	}

	return filter
}

// loadHandHistoryPage fetches one page of hand summaries in a background goroutine
// and then updates the handHistoryView on the Fyne main thread.
//
// If a load is already in progress, the requested page is stored as a pending
// request and executed immediately after the current load completes. This ensures
// that user navigation is never silently dropped during an import.
func (a *App) loadHandHistoryPage(page int) {
	// Store the page the caller wants.  If another goroutine is running, it will
	// pick this up when it finishes (see the retry loop below).
	atomic.StoreInt32(&a.pendingHistoryPage, int32(page))

	// Only one goroutine should do the actual DB work at a time.
	if !atomic.CompareAndSwapInt32(&a.historyPageRunning, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&a.historyPageRunning, 0)

	// Drain pending requests: after each load, check whether a new page was
	// requested while we were working, and service it before giving up the slot.
	for {
		// Take ownership of whatever page is currently pending.
		wantPage := int(atomic.SwapInt32(&a.pendingHistoryPage, -1))
		if wantPage < 0 {
			// No pending request; we are done.
			return
		}

		offset := wantPage * handHistoryPageSize
		f := a.buildHandHistoryFilter()
		f.Limit = handHistoryPageSize
		f.Offset = offset
		summaries, totalCount, err := a.service.ListHandSummaries(a.ctx, f)
		if err != nil {
			slog.Error("list hand summaries failed", "error", err)
			a.doSetStatus(lang.X("app.error.snapshot", "Snapshot error: {{.Error}}", map[string]any{"Error": err}))
			// Do not update UI; loop to try any further pending request.
			continue
		}

		capturedPage := wantPage
		capturedTotal := totalCount
		capturedSummaries := summaries
		fyne.Do(func() {
			if a.handHistoryView == nil {
				return
			}
			a.handHistoryView.UpdatePage(capturedSummaries, capturedPage, capturedTotal)
			if a.mainContent != nil {
				a.mainContent.Refresh()
			}
		})
	}
}

func (a *App) doSetStatus(msg string) {
	fyne.Do(func() {
		if a.statusText != nil {
			a.statusText.SetText(msg)
		}
	})
}

// loadHandDetail fetches the full hand data for a single UID in a background goroutine
// and then updates the handHistoryView detail panel on the Fyne main thread.
func (a *App) loadHandDetail(uid string) {
	a.mu.Lock()
	localSeat := a.lastLocalSeat
	a.mu.Unlock()

	h, err := a.service.GetHandByUID(a.ctx, uid)
	if err != nil {
		slog.Error("get hand by uid failed", "uid", uid, "error", err)
		fyne.Do(func() {
			if a.handHistoryView == nil {
				return
			}
			a.handHistoryView.UpdateDetail(
				newCenteredEmptyState(lang.X("hand_history.detail.error", "Failed to load hand details.")),
			)
		})
		return
	}

	detail := buildDetailPanel(h, localSeat)
	fyne.Do(func() {
		if a.handHistoryView == nil {
			return
		}
		a.handHistoryView.UpdateDetail(detail)
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
