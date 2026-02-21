package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

// statCard builds a single stat card with a name label and a large colored value label.
func statCard(metric MetricDefinition, value string, valueColor color.Color, footnote string, showWarn bool, win fyne.Window) fyne.CanvasObject {
	nameLabel := widget.NewLabel(metric.Label)
	nameLabel.TextStyle = fyne.TextStyle{Bold: true}
	nameLabel.Alignment = fyne.TextAlignCenter

	helpBtn := widget.NewButton(lang.X("settings.help_button", "?"), func() {
		if win == nil {
			return
		}
		dialog.ShowInformation(metric.Label, metric.Help, win)
	})
	helpBtn.Importance = widget.LowImportance
	helpBtn.Resize(fyne.NewSize(24, helpBtn.MinSize().Height))
	helpSlot := canvas.NewRectangle(color.Transparent)
	helpSlot.SetMinSize(helpBtn.MinSize())

	warn := canvas.NewText("", color.NRGBA{R: 0xFF, G: 0xC1, B: 0x07, A: 0xE8})
	warn.TextStyle = fyne.TextStyle{Bold: true}
	warn.Alignment = fyne.TextAlignTrailing
	warn.TextSize = theme.TextSize() * 0.86
	if showWarn {
		warn.Text = lang.X("warn_icon.mark", "!")
	}

	titleRow := container.NewHBox(helpSlot, nameLabel, helpBtn)
	head := container.NewBorder(nil, nil, nil, warn, container.NewCenter(titleRow))

	valueText := canvas.NewText(value, valueColor)
	valueText.TextStyle = fyne.TextStyle{Bold: true}
	valueText.TextSize = theme.TextSize() * 1.6
	valueText.Alignment = fyne.TextAlignCenter

	separator := canvas.NewRectangle(theme.ShadowColor())
	separator.SetMinSize(fyne.NewSize(0, 1))

	foot := canvas.NewText(footnote, color.NRGBA{R: 0xA8, G: 0xAF, B: 0xB8, A: 0xFF})
	foot.TextSize = theme.TextSize() * 0.82
	foot.Alignment = fyne.TextAlignTrailing

	card := container.NewVBox(
		head,
		separator,
		container.NewCenter(valueText),
		container.NewHBox(layout.NewSpacer(), foot),
	)

	bg := canvas.NewRectangle(theme.OverlayBackgroundColor())
	bg.CornerRadius = 6

	return container.NewStack(bg, container.NewPadded(card))
}

// NewOverviewTab returns the "Overview" tab canvas object.
func NewOverviewTab(s *stats.Stats, visibility *MetricVisibilityState, win fyne.Window) fyne.CanvasObject {
	// Empty state
	if s == nil || s.TotalHands == 0 {
		msg := widget.NewLabel(lang.X("overview.no_hands", "No hands recorded yet.\nStart playing in the VR Poker world!"))
		msg.Alignment = fyne.TextAlignCenter
		msg.Wrapping = fyne.TextWrapWord
		return container.NewCenter(msg)
	}

	// Title
	title := widget.NewLabel(lang.X("overview.title", "Overall Statistics"))
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	titleSep := canvas.NewRectangle(theme.PrimaryColor())
	titleSep.SetMinSize(fyne.NewSize(0, 2))

	// Build stat cards from metric registry
	metricDefs := metricsForOverview(visibility)
	cards := make([]fyne.CanvasObject, 0, len(metricDefs))
	for _, metric := range metricDefs {
		value := metric.OverviewValue(s)
		footnote := metricFootnoteText(value.Opportunities, metric.MinSamples)
		showWarn := metric.MinSamples > 0 && value.Opportunities < metric.MinSamples
		cards = append(cards, statCard(metric, value.Display, value.Color, footnote, showWarn, win))
	}

	if len(cards) == 0 {
		msg := widget.NewLabel(lang.X("overview.no_metrics", "No metrics selected. Enable metrics in Settings."))
		msg.Alignment = fyne.TextAlignCenter
		msg.Wrapping = fyne.TextWrapWord
		return container.NewCenter(msg)
	}

	gridColumns := 3
	if len(cards) < gridColumns {
		gridColumns = len(cards)
	}
	grid := container.NewGridWithColumns(gridColumns, cards...)

	insights := buildTrendInsights(s)
	insightHeader := widget.NewLabel(lang.X("overview.leak_insights", "Leak Insights"))
	insightHeader.TextStyle = fyne.TextStyle{Bold: true}
	insightRows := make([]fyne.CanvasObject, 0, len(insights))
	if len(insights) == 0 {
		insightRows = append(insightRows, widget.NewLabel(lang.X("overview.no_samples", "Not enough reliable samples yet for trend diagnostics.")))
	} else {
		for _, in := range insights {
			prefix := "[info]"
			if in.Level == "warn" {
				prefix = "[warn]"
			}
			if in.Level == "action" {
				prefix = "[action]"
			}
			lbl := widget.NewLabel(prefix + " " + in.Text)
			lbl.Wrapping = fyne.TextWrapWord
			insightRows = append(insightRows, lbl)
		}
	}
	insightBox := container.NewVBox(append([]fyne.CanvasObject{insightHeader}, insightRows...)...)

	content := container.NewVBox(
		container.NewPadded(title),
		titleSep,
		container.NewPadded(grid),
		widget.NewSeparator(),
		container.NewPadded(insightBox),
	)

	return withFixedLowSampleLegend(container.NewScroll(content))
}
