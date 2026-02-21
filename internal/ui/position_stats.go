package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

// positionOrder defines the display order (most aggressive position first).
var positionDisplayOrder = []parser.Position{
	parser.PosBTN,
	parser.PosCO,
	parser.PosMP1,
	parser.PosMP,
	parser.PosUTG1,
	parser.PosUTG,
	parser.PosBB,
	parser.PosSB,
}

type positionCellData struct {
	Main   string
	Note   string
	Color  color.Color
	IsHead bool
}

type positionTableCell struct {
	widget.BaseWidget
	main *canvas.Text
	note *canvas.Text
	wrap *fyne.Container
}

func newPositionTableCell() *positionTableCell {
	c := &positionTableCell{}
	c.ExtendBaseWidget(c)
	return c
}

func (c *positionTableCell) CreateRenderer() fyne.WidgetRenderer {
	c.main = canvas.NewText("", theme.ForegroundColor())
	c.main.Alignment = fyne.TextAlignCenter

	c.note = canvas.NewText("", theme.DisabledColor())
	c.note.Alignment = fyne.TextAlignTrailing
	c.note.TextSize = theme.TextSize() * 0.78
	c.note.Color = color.NRGBA{R: 0xA8, G: 0xAF, B: 0xB8, A: 0xFF}

	c.wrap = container.NewBorder(
		nil,
		container.NewHBox(layout.NewSpacer(), c.note),
		nil,
		nil,
		container.NewCenter(c.main),
	)

	return widget.NewSimpleRenderer(c.wrap)
}

func (c *positionTableCell) Set(data positionCellData) {
	if c.main == nil || c.note == nil {
		return
	}
	c.main.Text = data.Main
	if data.Color != nil {
		c.main.Color = data.Color
	} else {
		c.main.Color = theme.ForegroundColor()
	}
	c.main.TextStyle = fyne.TextStyle{Bold: data.IsHead}

	c.note.Text = data.Note
	if data.IsHead {
		c.note.Text = ""
	}

	c.main.Refresh()
	c.note.Refresh()
}

func positionColumnWidth(metric MetricDefinition) float32 {
	switch metric.ID {
	case "position":
		return 74
	case "profit":
		return 110
	default:
		return 92
	}
}

// NewPositionStatsTab returns the "Position Stats" tab canvas object.
func NewPositionStatsTab(s *stats.Stats, visibility *MetricVisibilityState) fyne.CanvasObject {
	if s == nil || len(s.ByPosition) == 0 {
		msg := widget.NewLabel("No position data yet.")
		msg.Alignment = fyne.TextAlignCenter
		return container.NewCenter(msg)
	}

	metricDefs := metricsForPosition(visibility)
	if len(metricDefs) == 0 {
		msg := widget.NewLabel("No metrics selected. Enable metrics in Settings.")
		msg.Alignment = fyne.TextAlignCenter
		return container.NewCenter(msg)
	}

	headers := []positionCellData{{Main: "Position", IsHead: true}}
	for _, metric := range metricDefs {
		headers = append(headers, positionCellData{Main: metric.Label, IsHead: true})
	}

	rows := [][]positionCellData{headers}

	for _, pos := range positionDisplayOrder {
		ps, ok := s.ByPosition[pos]
		if !ok || ps.Hands == 0 {
			continue
		}

		row := []positionCellData{{Main: pos.String(), IsHead: false}}
		for _, metric := range metricDefs {
			value := metric.PositionValue(ps)
			row = append(row, positionCellData{
				Main:  value.Display,
				Note:  metricFootnoteText(value.Opportunities, metric.MinSamples),
				Color: value.Color,
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

	return container.NewScroll(t)
}
