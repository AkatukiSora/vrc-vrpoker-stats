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
	LogPath         string
	OnPathChange    func(string)
	OnMetricsChange func()
	MetricState     *MetricVisibilityState
	Metadata        AppMetadata
	win             fyne.Window
}

func NewSettingsTab(
	currentPath string,
	win fyne.Window,
	onPathChange func(string),
	metricState *MetricVisibilityState,
	metadata AppMetadata,
	onMetricsChange func(),
) fyne.CanvasObject {
	if metricState == nil {
		metricState = NewMetricVisibilityState()
	}
	st := &SettingsTab{
		LogPath:         currentPath,
		OnPathChange:    onPathChange,
		OnMetricsChange: onMetricsChange,
		MetricState:     metricState,
		Metadata:        metadata,
		win:             win,
	}
	return st.build()
}

func (st *SettingsTab) buildLogSourceSection() fyne.CanvasObject {
	pathLabel := widget.NewLabel(lang.X("settings.log_path_label", "VRChat Log File Path:"))
	pathEntry := widget.NewEntry()
	pathEntry.SetPlaceHolder(lang.X("settings.log_path_placeholder", "Path to VRChat output_log_*.txt"))
	pathEntry.SetText(st.LogPath)

	browseBtn := widget.NewButton(lang.X("settings.browse", "Browse..."), func() {
		dialog.ShowFileOpen(func(f fyne.URIReadCloser, err error) {
			if err != nil || f == nil {
				return
			}
			f.Close()
			path := f.URI().Path()
			pathEntry.SetText(path)
			st.LogPath = path
			if st.OnPathChange != nil {
				st.OnPathChange(path)
			}
		}, st.win)
	})

	applyBtn := widget.NewButton(lang.X("settings.apply", "Apply"), func() {
		path := pathEntry.Text
		st.LogPath = path
		if st.OnPathChange != nil {
			st.OnPathChange(path)
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
			chk.SetChecked(st.MetricState.IsVisible(metricID))
		}
		suppressMetricsNotify = false
		if st.OnMetricsChange != nil {
			st.OnMetricsChange()
		}
	}

	presetButtons := make([]fyne.CanvasObject, 0, len(presets))
	for _, preset := range presets {
		preset := preset
		presetButtons = append(presetButtons, widget.NewButton(preset.ButtonText, func() {
			st.MetricState.ApplyPreset(preset)
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
			check.SetChecked(st.MetricState.IsVisible(metric.ID))
			check.OnChanged = func(checked bool) {
				if suppressMetricsNotify {
					return
				}
				st.MetricState.SetVisible(metric.ID, checked)
				if st.OnMetricsChange != nil {
					st.OnMetricsChange()
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
		item.Open = true
	}

	return newSectionCard(container.NewVBox(metricsHint, presetRow, groups))
}

func (st *SettingsTab) buildAboutSection() fyne.CanvasObject {
	version := st.Metadata.Version
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

	repoURL := st.Metadata.RepositoryURL
	if repoURL == "" {
		repoURL = "https://github.com/AkatukiSora/vrc-vrpoker-stats"
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
		widget.NewAccordionItem(lang.X("settings.about.title", "About"), st.buildAboutSection()),
	)
	for _, item := range sections.Items {
		item.Open = true
	}

	content := container.NewVBox(title, intro, newSectionDivider(), sections)
	return container.NewScroll(container.NewPadded(content))
}
