package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// SettingsTab holds settings UI state
type SettingsTab struct {
	LogPath         string
	OnPathChange    func(string)
	OnMetricsChange func()
	MetricState     *MetricVisibilityState
	win             fyne.Window
}

// NewSettingsTab creates the settings tab
func NewSettingsTab(
	currentPath string,
	win fyne.Window,
	onPathChange func(string),
	metricState *MetricVisibilityState,
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
		win:             win,
	}
	return st.build()
}

func (st *SettingsTab) build() fyne.CanvasObject {
	title := widget.NewLabelWithStyle("Settings", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// Log file path
	pathLabel := widget.NewLabel("VRChat Log File Path:")
	pathEntry := widget.NewEntry()
	pathEntry.SetPlaceHolder("Path to VRChat output_log_*.txt")
	pathEntry.SetText(st.LogPath)

	browseBtn := widget.NewButton("Browse...", func() {
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

	applyBtn := widget.NewButton("Apply", func() {
		path := pathEntry.Text
		st.LogPath = path
		if st.OnPathChange != nil {
			st.OnPathChange(path)
		}
	})
	applyBtn.Importance = widget.HighImportance

	pathRow := container.NewBorder(nil, nil, nil, container.NewHBox(browseBtn, applyBtn), pathEntry)

	metricsTitle := widget.NewLabelWithStyle("Metrics Visibility", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	metricsHint := widget.NewLabel("Choose which metrics are shown in Overview and Position Stats.")
	metricsHint.Wrapping = fyne.TextWrapWord

	presets := metricPresets()
	refreshMetrics := func(checks map[string]*widget.Check) {
		for metricID, chk := range checks {
			chk.SetChecked(st.MetricState.IsVisible(metricID))
		}
		if st.OnMetricsChange != nil {
			st.OnMetricsChange()
		}
	}

	presetButtons := make([]fyne.CanvasObject, 0, len(presets))
	checks := make(map[string]*widget.Check, len(metricRegistry))
	for _, preset := range presets {
		preset := preset
		presetButtons = append(presetButtons, widget.NewButton(preset.ButtonText, func() {
			st.MetricState.ApplyPreset(preset)
			refreshMetrics(checks)
		}))
	}
	presetRow := container.NewGridWithColumns(4, presetButtons...)

	metricRows := make([]fyne.CanvasObject, 0, len(metricRegistry))
	for _, metric := range metricRegistry {
		metric := metric
		check := widget.NewCheck(metric.Label, func(checked bool) {
			st.MetricState.SetVisible(metric.ID, checked)
			if st.OnMetricsChange != nil {
				st.OnMetricsChange()
			}
		})
		check.SetChecked(st.MetricState.IsVisible(metric.ID))
		checks[metric.ID] = check

		helpBtn := widget.NewButton("?", func() {
			dialog.ShowInformation(metric.Label, metric.Help, st.win)
		})

		row := container.NewBorder(nil, nil, nil, helpBtn, check)
		metricRows = append(metricRows, row)
	}
	metricsSection := container.NewVBox(metricRows...)

	// Info box
	infoLabel := widget.NewLabel(
		"The application monitors your VRChat log file in real-time.\n" +
			"Log files are typically found at:\n\n" +
			"  Linux (Steam Proton):\n" +
			"  ~/.local/share/Steam/steamapps/compatdata/438100/pfx/\n" +
			"  drive_c/users/steamuser/AppData/LocalLow/VRChat/VRChat/\n\n" +
			"  Windows:\n" +
			"  %APPDATA%\\..\\LocalLow\\VRChat\\VRChat\\\n\n" +
			"Statistics are calculated for VR Poker world sessions only.\n" +
			"Historical logs (from before the app was started) are also analyzed.",
	)
	infoLabel.Wrapping = fyne.TextWrapBreak

	// About section
	aboutTitle := widget.NewLabelWithStyle("About", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	aboutText := widget.NewLabel(
		"VRC VRPoker Stats v1.0\n" +
			"Tracks your poker statistics in the VRChat VR Poker world.\n\n" +
			"Includes configurable metric visibility presets and per-metric help.\n" +
			"Use Settings to tailor the dashboard for your study goal.\n\n" +
			"Other features:\n" +
			"  • Hand Range Analysis (13x13 grid)\n" +
			"  • Position-based statistics",
	)
	aboutText.Wrapping = fyne.TextWrapBreak

	form := container.NewVBox(
		title,
		widget.NewSeparator(),
		pathLabel,
		pathRow,
		widget.NewSeparator(),
		metricsTitle,
		metricsHint,
		presetRow,
		metricsSection,
		widget.NewSeparator(),
		infoLabel,
		widget.NewSeparator(),
		aboutTitle,
		aboutText,
	)

	return container.NewScroll(container.NewPadded(form))
}
