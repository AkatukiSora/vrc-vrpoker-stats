package ui

import (
	"net/url"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"
)

type SettingsTab struct {
	logPath         string
	dbPath          string
	onPathChange    func(string)
	onMetricsChange func()
	onReset         func()
	metricState     *MetricVisibilityState
	metadata        AppMetadata
	win             fyne.Window
}

type SettingsTabConfig struct {
	CurrentPath     string
	Window          fyne.Window
	OnPathChange    func(string)
	MetricState     *MetricVisibilityState
	Metadata        AppMetadata
	OnMetricsChange func()
	DBPath          string
	OnReset         func()
}

func NewSettingsTab(cfg SettingsTabConfig) fyne.CanvasObject {
	metricState := cfg.MetricState
	if metricState == nil {
		metricState = NewMetricVisibilityState()
	}
	st := &SettingsTab{
		logPath:         cfg.CurrentPath,
		dbPath:          cfg.DBPath,
		onPathChange:    cfg.OnPathChange,
		onMetricsChange: cfg.OnMetricsChange,
		onReset:         cfg.OnReset,
		metricState:     metricState,
		metadata:        cfg.Metadata,
		win:             cfg.Window,
	}
	return st.build()
}

func (st *SettingsTab) buildLogSourceSection() fyne.CanvasObject {
	pathLabel := widget.NewLabel(lang.X("settings.log_path_label", "VRChat Log File Path:"))
	pathEntry := widget.NewEntry()
	pathEntry.SetPlaceHolder(lang.X("settings.log_path_placeholder", "Path to VRChat output_log_*.txt"))
	pathEntry.SetText(st.logPath)

	browseBtn := widget.NewButton(lang.X("settings.browse", "Browse..."), func() {
		dialog.ShowFileOpen(func(f fyne.URIReadCloser, err error) {
			if err != nil || f == nil {
				return
			}
			f.Close()
			path := f.URI().Path()
			pathEntry.SetText(path)
			st.logPath = path
			if st.onPathChange != nil {
				st.onPathChange(path)
			}
		}, st.win)
	})

	applyBtn := widget.NewButton(lang.X("settings.apply", "Apply"), func() {
		path := pathEntry.Text
		st.logPath = path
		if st.onPathChange != nil {
			st.onPathChange(path)
		}
	})
	applyBtn.Importance = widget.HighImportance

	pathRow := container.NewBorder(nil, nil, nil, container.NewHBox(browseBtn, applyBtn), pathEntry)
	pathInfoLabel := widget.NewLabel(lang.X("settings.log_path_info", "The application monitors your VRChat log file in real-time.\nLog files are typically found at:\n\n  Linux (Steam Proton):\n  ~/.local/share/Steam/steamapps/compatdata/438100/pfx/\n  drive_c/users/steamuser/AppData/LocalLow/VRChat/VRChat/\n\n  Windows:\n  %APPDATA%\\..\\LocalLow\\VRChat\\VRChat\\\n\nStatistics are calculated for VR Poker world sessions only.\nHistorical logs (from before the app was started) are also analyzed."))
	pathInfoLabel.Wrapping = fyne.TextWrapWord

	return newSectionCard(container.NewVBox(pathLabel, pathRow, pathInfoLabel))
}

func categoryLabel(category metricCategoryID) string {
	switch category {
	case metricCategoryResult:
		return lang.X("settings.metrics.group.result", "Results")
	case metricCategoryPreflop:
		return lang.X("settings.metrics.group.preflop", "Preflop")
	case metricCategoryPostflop:
		return lang.X("settings.metrics.group.postflop", "Postflop")
	default:
		return lang.X("settings.metrics.group.showdown", "Showdown")
	}
}

func (st *SettingsTab) buildMetricsSection() fyne.CanvasObject {
	metricsHint := widget.NewLabel(lang.X("settings.metrics_hint", "Choose which metrics are shown in Overview and Position Stats."))
	metricsHint.Wrapping = fyne.TextWrapWord

	presets := metricPresets()
	checks := make(map[string]*widget.Check, len(metricRegistry))
	suppressMetricsNotify := false

	refreshMetrics := func() {
		suppressMetricsNotify = true
		for metricID, chk := range checks {
			chk.SetChecked(st.metricState.IsVisible(metricID))
		}
		suppressMetricsNotify = false
		if st.onMetricsChange != nil {
			st.onMetricsChange()
		}
	}

	presetButtons := make([]fyne.CanvasObject, 0, len(presets))
	for _, preset := range presets {
		preset := preset
		presetButtons = append(presetButtons, widget.NewButton(preset.ButtonText, func() {
			st.metricState.ApplyPreset(preset)
			refreshMetrics()
		}))
	}
	presetRow := container.NewGridWithColumns(4, presetButtons...)

	byCategory := map[metricCategoryID][]MetricDefinition{}
	for _, metric := range metricRegistry {
		cat := metricCategoryForMetricID(metric.ID)
		byCategory[cat] = append(byCategory[cat], metric)
	}
	for cat := range byCategory {
		sort.Slice(byCategory[cat], func(i, j int) bool {
			return byCategory[cat][i].Label < byCategory[cat][j].Label
		})
	}

	orderedCategories := []metricCategoryID{metricCategoryResult, metricCategoryPreflop, metricCategoryPostflop, metricCategoryShowdown}
	groupItems := make([]*widget.AccordionItem, 0, len(orderedCategories))
	for _, cat := range orderedCategories {
		metrics := byCategory[cat]
		if len(metrics) == 0 {
			continue
		}
		rows := make([]fyne.CanvasObject, 0, len(metrics))
		for _, metric := range metrics {
			metric := metric
			check := widget.NewCheck(metric.Label, nil)
			check.SetChecked(st.metricState.IsVisible(metric.ID))
			check.OnChanged = func(checked bool) {
				if suppressMetricsNotify {
					return
				}
				st.metricState.SetVisible(metric.ID, checked)
				if st.onMetricsChange != nil {
					st.onMetricsChange()
				}
			}
			checks[metric.ID] = check

			helpBtn := widget.NewButton(lang.X("settings.help_button", "?"), func() {
				dialog.ShowInformation(metric.Label, metric.HelpText(), st.win)
			})
			helpBtn.Importance = widget.LowImportance

			rows = append(rows, container.NewBorder(nil, nil, nil, helpBtn, check))
		}
		groupItems = append(groupItems, widget.NewAccordionItem(categoryLabel(cat), container.NewVBox(rows...)))
	}

	groups := widget.NewAccordion(groupItems...)
	for _, item := range groups.Items {
		item.Open = false
	}

	return newSectionCard(container.NewVBox(metricsHint, presetRow, groups))
}

func (st *SettingsTab) buildDataManagementSection() fyne.CanvasObject {
	dbPathHint := widget.NewLabel(lang.X("settings.data.db_path_label", "Database File:"))
	dbPathValue := widget.NewLabel(st.dbPath)
	dbPathValue.Wrapping = fyne.TextWrapBreak

	resetBtn := widget.NewButton(lang.X("settings.data.reset_button", "Reset Database"), func() {
		dialog.ShowConfirm(
			lang.X("settings.data.reset_confirm_title", "Reset Database?"),
			lang.X("settings.data.reset_confirm_body", "All recorded hands and statistics will be permanently deleted.\n\nAfter the reset, the application will restart and re-import hands from any VRChat log files that are still present on disk. Hands from log files that have already been deleted or rotated away will be lost.\n\nThis action cannot be undone."),
			func(ok bool) {
				if !ok {
					return
				}
				if st.onReset != nil {
					st.onReset()
				}
			},
			st.win,
		)
	})
	resetBtn.Importance = widget.DangerImportance

	return newSectionCard(container.NewVBox(dbPathHint, dbPathValue, resetBtn))
}

func (st *SettingsTab) buildAboutSection() fyne.CanvasObject {
	version := st.metadata.Version
	if version == "" {
		version = "dev"
	}

	aboutText := widget.NewLabel(lang.X("settings.about.text", "Tracks your poker statistics in the VRChat VR Poker world.\n\nIncludes configurable metric visibility presets and per-metric help.\nUse Settings to tailor the dashboard for your study goal.\n\nOther features:\n  • Hand Range Analysis (13x13 grid)\n  • Position-based statistics"))
	aboutText.Wrapping = fyne.TextWrapWord

	versionLabel := widget.NewLabelWithStyle(
		lang.X("settings.about.version", "Version: {{.Version}}", map[string]any{"Version": version}),
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)

	repoURL := st.metadata.RepositoryURL
	if repoURL == "" {
		repoURL = "https://github.com/AkatukiSora/vrc-vrpoker-ststs"
	}
	repoLabel := widget.NewLabel(lang.X("settings.about.repository", "Repository: {{.URL}}", map[string]any{"URL": repoURL}))
	repoLabel.Wrapping = fyne.TextWrapWord
	openRepoBtn := widget.NewButton(lang.X("settings.about.open_repository", "Open Repository"), func() {
		u, err := url.Parse(repoURL)
		if err != nil {
			dialog.ShowError(err, st.win)
			return
		}
		if err := fyne.CurrentApp().OpenURL(u); err != nil {
			dialog.ShowError(err, st.win)
		}
	})
	openRepoBtn.Importance = widget.LowImportance

	return newSectionCard(container.NewVBox(versionLabel, aboutText, repoLabel, openRepoBtn))
}

func (st *SettingsTab) build() fyne.CanvasObject {
	title := widget.NewLabelWithStyle(lang.X("settings.title", "Settings"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	intro := widget.NewLabel(lang.X("settings.intro", "Tune data sources and metric visibility for your study workflow."))
	intro.Wrapping = fyne.TextWrapWord

	sections := widget.NewAccordion(
		widget.NewAccordionItem(lang.X("settings.section.log_source", "Log Source"), st.buildLogSourceSection()),
		widget.NewAccordionItem(lang.X("settings.section.metrics", "Metrics"), st.buildMetricsSection()),
		widget.NewAccordionItem(lang.X("settings.section.data_management", "Data Management"), st.buildDataManagementSection()),
	)
	for _, item := range sections.Items {
		item.Open = false
	}

	aboutTitle := widget.NewLabelWithStyle(lang.X("settings.about.title", "About"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	aboutSection := st.buildAboutSection()

	content := container.NewVBox(title, intro, newSectionDivider(), sections, newSectionDivider(), aboutTitle, aboutSection)
	return container.NewScroll(container.NewPadded(content))
}
