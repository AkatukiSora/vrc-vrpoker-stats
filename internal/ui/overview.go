package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

func overviewMetricCard(metric MetricDefinition, value MetricValue, win fyne.Window, hero bool) fyne.CanvasObject {
	footnote := metricFootnoteText(value.Opportunities, metric.MinSamples)
	showWarn := metric.MinSamples > 0 && value.Opportunities < metric.MinSamples

	title := widget.NewLabel(metric.Label)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Wrapping = fyne.TextWrapWord

	helpBtn := widget.NewButton(lang.X("settings.help_button", "?"), func() {
		if win == nil {
			return
		}
		dialog.ShowInformation(metric.Label, metric.HelpText(), win)
	})
	helpBtn.Importance = widget.LowImportance

	warn := newWarnMark(showWarn)

	header := container.NewBorder(nil, nil, nil, container.NewHBox(warn, helpBtn), title)

	valueText := widget.NewRichTextFromMarkdown("`" + value.Display + "`")
	valueText.Wrapping = fyne.TextWrapOff

	foot := newSubtleText(footnote)
	cardBody := container.NewVBox(header, valueText, container.NewHBox(layout.NewSpacer(), foot))

	if hero {
		return newHeroCard(cardBody)
	}
	return newSectionCard(cardBody)
}

func splitOverviewMetrics(metricDefs []MetricDefinition) ([]MetricDefinition, []MetricDefinition) {
	priority := []string{"hands", "profit", "vpip", "pfr", "bb_per_100"}
	hero := make([]MetricDefinition, 0, 3)
	other := make([]MetricDefinition, 0, len(metricDefs))
	used := make(map[string]struct{}, 3)

	for _, want := range priority {
		for _, metric := range metricDefs {
			if metric.ID != want {
				continue
			}
			hero = append(hero, metric)
			used[metric.ID] = struct{}{}
			break
		}
	}

	for _, metric := range metricDefs {
		if _, ok := used[metric.ID]; ok {
			continue
		}
		other = append(other, metric)
	}

	if len(hero) == 0 {
		if len(metricDefs) <= 3 {
			return metricDefs, nil
		}
		return metricDefs[:3], metricDefs[3:]
	}
	return hero, other
}

func insightAccent(priority string) color.Color {
	switch priority {
	case "P0":
		return uiDangerAccent
	case "P1":
		return uiWarningColor
	default:
		return uiInfoAccent
	}
}

func overviewInsightCard(in trendInsight) fyne.CanvasObject {
	title := widget.NewLabel(in.Title)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Wrapping = fyne.TextWrapWord

	severity := newMetricChip(in.Severity, insightAccent(in.Priority))
	chips := []fyne.CanvasObject{severity}
	if in.LowSample {
		chips = append(chips, newMetricChip(lang.X("overview.low_sample_badge", "[low sample]"), uiWarningColor))
	}

	body := widget.NewLabel(in.Text)
	body.Wrapping = fyne.TextWrapWord

	rows := []fyne.CanvasObject{container.NewHBox(chips...), title, body}
	if len(in.Evidence) > 0 {
		evidenceRows := make([]fyne.CanvasObject, 0, len(in.Evidence))
		for _, ev := range in.Evidence {
			evLabel := widget.NewLabel("- " + ev)
			evLabel.Wrapping = fyne.TextWrapWord
			evidenceRows = append(evidenceRows, evLabel)
		}
		rows = append(rows, widget.NewAccordion(
			widget.NewAccordionItem(lang.X("overview.insight.evidence_toggle", "Show evidence"), container.NewVBox(evidenceRows...)),
		))
	}

	return newSectionCard(container.NewVBox(rows...))
}

// NewOverviewTab returns the "Overview" tab canvas object.
func NewOverviewTab(s *stats.Stats, visibility *MetricVisibilityState, win fyne.Window) fyne.CanvasObject {
	if s == nil || s.TotalHands == 0 {
		return newCenteredEmptyState(lang.X("overview.no_hands", "No hands recorded yet.\nStart playing in the VR Poker world!"))
	}

	metricDefs := metricsForOverview(visibility)
	if len(metricDefs) == 0 {
		return newCenteredEmptyState(lang.X("overview.no_metrics", "No metrics selected. Enable metrics in Settings."))
	}

	heroDefs, otherDefs := splitOverviewMetrics(metricDefs)
	heroCards := make([]fyne.CanvasObject, 0, len(heroDefs))
	for _, metric := range heroDefs {
		heroCards = append(heroCards, overviewMetricCard(metric, metric.OverviewValue(s), win, true))
	}

	otherCards := make([]fyne.CanvasObject, 0, len(otherDefs))
	for _, metric := range otherDefs {
		otherCards = append(otherCards, overviewMetricCard(metric, metric.OverviewValue(s), win, false))
	}

	insights := buildTrendInsights(s)
	insightRows := make([]fyne.CanvasObject, 0, len(insights)+1)
	if len(insights) == 0 {
		none := widget.NewLabel(lang.X("overview.no_insight_signal", "No strong leak signal is detected right now."))
		none.Wrapping = fyne.TextWrapWord
		insightRows = append(insightRows, newSectionCard(none))
	} else {
		for _, in := range insights {
			insightRows = append(insightRows, overviewInsightCard(in))
		}
	}

	title := widget.NewLabelWithStyle(lang.X("overview.title", "Overall Statistics"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	subtitle := widget.NewLabel(lang.X("overview.subtitle", "Track your baseline first, then drill into actionable leaks."))
	subtitle.Wrapping = fyne.TextWrapWord

	sections := []fyne.CanvasObject{
		container.NewVBox(title, subtitle),
		newSectionDivider(),
		widget.NewLabelWithStyle(lang.X("overview.section.insights", "Leak Insights"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewVBox(insightRows...),
		newSectionDivider(),
		widget.NewLabelWithStyle(lang.X("overview.section.key_metrics", "Key Metrics"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	}

	if len(heroCards) > 0 {
		sections = append(sections, container.NewGridWithColumns(min(4, len(heroCards)), heroCards...))
	}

	if len(otherCards) > 0 {
		sections = append(sections,
			newSectionDivider(),
			widget.NewLabelWithStyle(lang.X("overview.section.all_metrics", "All Visible Metrics"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			container.NewGridWithColumns(min(3, len(otherCards)), otherCards...),
		)
	}

	content := container.NewPadded(container.NewVBox(sections...))
	return withFixedLowSampleLegend(container.NewScroll(content))
}
