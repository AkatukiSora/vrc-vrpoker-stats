package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

// statCard builds a single stat card with a name label and a large colored value label.
func statCard(name, value string, valueColor color.Color, footnote string, showReferenceWarn bool) fyne.CanvasObject {
	nameLabel := widget.NewLabel(name)
	nameLabel.TextStyle = fyne.TextStyle{Bold: true}
	nameLabel.Alignment = fyne.TextAlignCenter

	warnIcon := fyne.CanvasObject(layout.NewSpacer())
	overlay := container.NewWithoutLayout()
	if showReferenceWarn {
		warnIcon, overlay = newHoverHint("参考値: サンプル不足のため、値が不安定・不正確な可能性があります。", HintSideLeft)
	}

	head := container.NewBorder(nil, nil, nil, warnIcon, container.NewCenter(nameLabel))

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

	return container.NewStack(bg, container.NewPadded(card), overlay)
}

// NewOverviewTab returns the "Overview" tab canvas object.
func NewOverviewTab(s *stats.Stats, visibility *MetricVisibilityState) fyne.CanvasObject {
	// Empty state
	if s == nil || s.TotalHands == 0 {
		msg := widget.NewLabel("No hands recorded yet.\nStart playing in the VR Poker world!")
		msg.Alignment = fyne.TextAlignCenter
		msg.Wrapping = fyne.TextWrapWord
		return container.NewCenter(msg)
	}

	// Title
	title := widget.NewLabel("Overall Statistics")
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
		cards = append(cards, statCard(metric.Label, value.Display, value.Color, footnote, showWarn))
	}

	if len(cards) == 0 {
		msg := widget.NewLabel("No metrics selected. Enable metrics in Settings.")
		msg.Alignment = fyne.TextAlignCenter
		msg.Wrapping = fyne.TextWrapWord
		return container.NewCenter(msg)
	}

	grid := container.NewGridWithColumns(2, cards...)

	insights := buildTrendInsights(s)
	insightHeader := widget.NewLabel("Leak Insights")
	insightHeader.TextStyle = fyne.TextStyle{Bold: true}
	insightRows := make([]fyne.CanvasObject, 0, len(insights))
	if len(insights) == 0 {
		insightRows = append(insightRows, widget.NewLabel("Not enough reliable samples yet for trend diagnostics."))
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

	return container.NewScroll(content)
}
