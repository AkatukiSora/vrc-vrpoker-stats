package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

var positionDisplayOrder = []parser.Position{
	parser.PosBTN,
	parser.PosCO,
	parser.PosMP,
	parser.PosHJ,
	parser.PosUTG1,
	parser.PosUTG,
	parser.PosBB,
	parser.PosSB,
}

type positionCellData struct {
	Main     string
	Note     string
	Color    color.Color
	BG       color.Color
	ShowWarn bool
	IsHead   bool
}

type positionTableCell struct {
	widget.BaseWidget
	bg     *canvas.Rectangle
	border *canvas.Rectangle
	main   *canvas.Text
	note   *canvas.Text
	warn   *canvas.Text
	wrap   *fyne.Container
}

func newPositionTableCell() *positionTableCell {
	c := &positionTableCell{}
	c.ExtendBaseWidget(c)
	return c
}

func (c *positionTableCell) CreateRenderer() fyne.WidgetRenderer {
	c.bg = canvas.NewRectangle(color.Transparent)
	c.border = canvas.NewRectangle(color.Transparent)
	c.border.StrokeWidth = 1
	c.border.StrokeColor = color.NRGBA{R: 0x8A, G: 0x92, B: 0x9C, A: 0x1F}

	c.main = canvas.NewText("", theme.ForegroundColor())
	c.main.Alignment = fyne.TextAlignCenter

	c.note = canvas.NewText("", uiMutedTextColor)
	c.note.Alignment = fyne.TextAlignTrailing
	c.note.TextSize = theme.TextSize() * 0.78

	c.warn = newWarnMark(false)
	c.warn.Alignment = fyne.TextAlignTrailing

	body := container.NewBorder(
		container.NewHBox(layout.NewSpacer(), c.warn),
		container.NewHBox(layout.NewSpacer(), c.note),
		nil,
		nil,
		container.NewCenter(c.main),
	)

	c.wrap = container.NewStack(c.bg, c.border, container.NewPadded(body))
	return widget.NewSimpleRenderer(c.wrap)
}

func (c *positionTableCell) Set(data positionCellData) {
	if c.main == nil || c.note == nil || c.bg == nil {
		return
	}
	if data.BG == nil {
		c.bg.FillColor = color.Transparent
	} else {
		c.bg.FillColor = toNRGBA(data.BG)
	}

	c.main.Text = data.Main
	if data.Color != nil {
		c.main.Color = data.Color
	} else {
		c.main.Color = theme.ForegroundColor()
	}
	c.main.TextStyle = fyne.TextStyle{Bold: data.IsHead}

	c.note.Text = data.Note
	c.note.Color = uiMutedTextColor

	c.warn.Text = ""
	if data.ShowWarn && !data.IsHead {
		c.warn.Text = lang.X("warn_icon.mark", "!")
	}
	if data.IsHead {
		c.note.Text = ""
	}

	c.bg.Refresh()
	c.main.Refresh()
	c.note.Refresh()
	c.warn.Refresh()
}

func positionColumnWidth(metric MetricDefinition) float32 {
	switch metric.ID {
	case "position":
		return 80
	case "profit":
		return 120
	default:
		return 96
	}
}

func positionRowTint(pos parser.Position) color.Color {
	switch pos {
	case parser.PosBTN, parser.PosCO:
		return color.NRGBA{R: 0x4F, G: 0x9A, B: 0xD3, A: 0x18}
	case parser.PosSB, parser.PosBB:
		return color.NRGBA{R: 0xC7, G: 0x98, B: 0x42, A: 0x16}
	default:
		return color.Transparent
	}
}

func metricCellTint(metricID string, v MetricValue) color.Color {
	switch metricID {
	case "vpip", "profit":
		n := toNRGBA(v.Color)
		if n.A == 0 {
			return color.Transparent
		}
		n.A = 0x22
		return n
	default:
		return color.Transparent
	}
}

func tintOrFallback(primary, fallback color.Color) color.Color {
	p := toNRGBA(primary)
	if p.A != 0 {
		return p
	}
	return fallback
}

// NewPositionStatsTab returns the "Position Stats" tab canvas object.
func NewPositionStatsTab(s *stats.Stats, visibility *MetricVisibilityState) fyne.CanvasObject {
	if s == nil || len(s.ByPosition) == 0 {
		return newCenteredEmptyState(lang.X("position_stats.no_data", "No position data yet."))
	}

	metricDefs := metricsForPosition(visibility)
	if len(metricDefs) == 0 {
		return newCenteredEmptyState(lang.X("position_stats.no_metrics", "No metrics selected. Enable metrics in Settings."))
	}

	headerBG := color.NRGBA{R: 0x7C, G: 0x8E, B: 0xA1, A: 0x24}
	headers := []positionCellData{{
		Main:   lang.X("position_stats.position_header", "Position"),
		IsHead: true,
		BG:     headerBG,
	}}
	for _, metric := range metricDefs {
		headers = append(headers, positionCellData{
			Main:   metric.Label,
			IsHead: true,
			BG:     headerBG,
		})
	}

	rows := [][]positionCellData{headers}

	for _, pos := range positionDisplayOrder {
		ps, ok := s.ByPosition[pos]
		if !ok || ps.Hands == 0 {
			continue
		}

		rowTint := positionRowTint(pos)
		row := []positionCellData{{Main: pos.String(), IsHead: false, BG: rowTint}}
		for _, metric := range metricDefs {
			value := metric.PositionValue(ps)
			showWarn := metric.MinSamples > 0 && value.Opportunities < metric.MinSamples
			row = append(row, positionCellData{
				Main:     value.Display,
				Note:     metricFootnoteText(value.Opportunities, metric.MinSamples),
				Color:    value.Color,
				BG:       tintOrFallback(metricCellTint(metric.ID, value), rowTint),
				ShowWarn: showWarn,
			})
		}
		rows = append(rows, row)
	}

	numCols := len(headers)
	numRows := len(rows)

	t := widget.NewTable(
		func() (int, int) { return numRows, numCols },
		func() fyne.CanvasObject {
			return newPositionTableCell()
		},
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			cell := obj.(*positionTableCell)
			if id.Row >= numRows || id.Col >= numCols {
				cell.Set(positionCellData{})
				return
			}
			cell.Set(rows[id.Row][id.Col])
		},
	)

	t.SetColumnWidth(0, positionColumnWidth(MetricDefinition{ID: "position"}))
	for i, m := range metricDefs {
		t.SetColumnWidth(i+1, positionColumnWidth(m))
	}
	for row := 0; row < numRows; row++ {
		t.SetRowHeight(row, 46)
	}

	title := widget.NewLabelWithStyle(lang.X("position_stats.title", "Position Distribution"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	subtitle := widget.NewLabel(lang.X("position_stats.subtitle", "Compare outcomes and tendencies by seat position."))
	subtitle.Wrapping = fyne.TextWrapWord

	tableScroll := container.NewScroll(t)
	minTableHeight := float32(numRows*46 + 16)
	if minTableHeight < 320 {
		minTableHeight = 320
	}
	minSlot := canvas.NewRectangle(color.Transparent)
	minSlot.SetMinSize(fyne.NewSize(0, minTableHeight))
	tableArea := container.NewStack(minSlot, tableScroll)

	header := container.NewVBox(title, subtitle, newSectionDivider())
	content := container.NewBorder(header, nil, nil, nil, newSectionCard(tableArea))

	return withFixedLowSampleLegend(container.NewPadded(content))
}
