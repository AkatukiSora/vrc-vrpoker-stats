package ui

import (
	"fmt"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

// rankDisplayName returns display label for a rank (10 -> T).
func rankDisplayName(r string) string {
	if r == "10" {
		return "T"
	}
	return r
}

// comboDisplayName returns the short display name for a hand combo cell.
// e.g. AKs, 77, T9o
func comboDisplayName(cell *stats.HandRangeCell) string {
	r1 := rankDisplayName(cell.Rank1)
	r2 := rankDisplayName(cell.Rank2)
	if cell.IsPair {
		return r1 + r2
	}
	if cell.Suited {
		return r1 + r2 + "s"
	}
	return r1 + r2 + "o"
}

// lerpColor linearly interpolates between two colors by factor t in [0,1].
func lerpColor(a, b color.NRGBA, t float64) color.NRGBA {
	t = math.Max(0, math.Min(1, t))
	lerp := func(x, y uint8) uint8 {
		return uint8(float64(x) + (float64(y)-float64(x))*t)
	}
	return color.NRGBA{
		R: lerp(a.R, b.R),
		G: lerp(a.G, b.G),
		B: lerp(a.B, b.B),
		A: 255,
	}
}

// colorForRate returns a heat-map color for a given action rate (0-100).
func colorForRate(rate float64) color.Color {
	// Clamp
	if rate < 0 {
		rate = 0
	}
	if rate > 100 {
		rate = 100
	}

	// Colour stops:
	//   0%   -> very dark gray   {50, 50, 50}
	//  10%   -> navy blue        {20, 30, 160}
	//  25%   -> green            {30, 160, 40}
	//  50%   -> yellow-green     {160, 200, 20}
	//  75%   -> orange           {220, 120, 20}
	// 100%   -> bright red       {220, 40, 40}
	darkGray  := color.NRGBA{50, 50, 50, 255}
	navy      := color.NRGBA{20, 30, 160, 255}
	green     := color.NRGBA{30, 160, 40, 255}
	yellowGrn := color.NRGBA{160, 200, 20, 255}
	orange    := color.NRGBA{220, 120, 20, 255}
	red       := color.NRGBA{220, 40, 40, 255}

	switch {
	case rate <= 10:
		return lerpColor(darkGray, navy, rate/10)
	case rate <= 25:
		return lerpColor(navy, green, (rate-10)/15)
	case rate <= 50:
		return lerpColor(green, yellowGrn, (rate-25)/25)
	case rate <= 75:
		return lerpColor(yellowGrn, orange, (rate-50)/25)
	default:
		return lerpColor(orange, red, (rate-75)/25)
	}
}

// actionFilter enumerates the available action filters.
type actionFilter int

const (
	filterVPIP actionFilter = iota
	filterPFR
	filter3Bet
	filterFold
)

// positionFilter maps select index to parser.Position (0 = All).
var positionFilters = []struct {
	Label string
	Pos   parser.Position
	IsAll bool
}{
	{Label: "All", IsAll: true},
	{Label: "SB", Pos: parser.PosSB},
	{Label: "BB", Pos: parser.PosBB},
	{Label: "UTG", Pos: parser.PosUTG},
	{Label: "UTG+1", Pos: parser.PosUTG1},
	{Label: "MP", Pos: parser.PosMP},
	{Label: "MP+1", Pos: parser.PosMP1},
	{Label: "CO", Pos: parser.PosCO},
	{Label: "BTN", Pos: parser.PosBTN},
}

// cellRate returns the action rate for a cell/filter/position combo.
// When posIdx == 0 (All) overall cell stats are used; otherwise per-position.
func cellRate(cell *stats.HandRangeCell, af actionFilter, posIdx int) float64 {
	if cell == nil || cell.Dealt == 0 {
		return -1 // sentinel: no data
	}

	if posIdx == 0 {
		// Overall
		switch af {
		case filterVPIP:
			return cell.VPIPRate()
		case filterPFR:
			return cell.PFRRate()
		case filter3Bet:
			return cell.ThreeBetRate()
		case filterFold:
			return cell.FoldRate()
		}
		return 0
	}

	// Per-position
	pf := positionFilters[posIdx]
	pc := cell.ByPosition[pf.Pos]
	if pc == nil || pc.Dealt == 0 {
		return -1
	}
	switch af {
	case filterVPIP:
		return float64(pc.VPIP) / float64(pc.Dealt) * 100
	case filterPFR:
		return float64(pc.PFR) / float64(pc.Dealt) * 100
	case filter3Bet:
		return float64(pc.ThreeBet) / float64(pc.Dealt) * 100
	case filterFold:
		return float64(pc.Fold) / float64(pc.Dealt) * 100
	}
	return 0
}

// ── Legend ────────────────────────────────────────────────────────────────────

// buildLegend creates a horizontal gradient legend strip with 0% – 100% labels.
func buildLegend() fyne.CanvasObject {
	const steps = 20
	swatches := make([]fyne.CanvasObject, 0, steps+2)

	label0 := widget.NewLabel("0%")
	swatches = append(swatches, label0)

	for i := 0; i < steps; i++ {
		rate := float64(i) / float64(steps-1) * 100
		rect := canvas.NewRectangle(colorForRate(rate))
		rect.SetMinSize(fyne.NewSize(18, 18))
		swatches = append(swatches, rect)
	}

	label100 := widget.NewLabel("100%")
	swatches = append(swatches, label100)

	legend := container.NewHBox(swatches...)
	legendTitle := widget.NewLabelWithStyle("Color: action rate  ", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	return container.NewHBox(legendTitle, legend)
}

// ── Cell detail popup ─────────────────────────────────────────────────────────

// showCellDetail opens a modal dialog with full per-position breakdown.
func showCellDetail(cell *stats.HandRangeCell, win fyne.Window) {
	if cell == nil {
		return
	}

	name := comboDisplayName(cell)
	title := widget.NewLabelWithStyle(name, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	positions := []struct {
		Label string
		Pos   parser.Position
	}{
		{"SB", parser.PosSB},
		{"BB", parser.PosBB},
		{"UTG", parser.PosUTG},
		{"UTG+1", parser.PosUTG1},
		{"MP", parser.PosMP},
		{"MP+1", parser.PosMP1},
		{"CO", parser.PosCO},
		{"BTN", parser.PosBTN},
	}

	// Header row
	headerCells := []fyne.CanvasObject{
		widget.NewLabelWithStyle("Metric", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Overall", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
	}
	for _, p := range positions {
		headerCells = append(headerCells,
			widget.NewLabelWithStyle(p.Label, fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
	}

	cols := 2 + len(positions) // Metric + Overall + positions

	pctStr := func(num, denom int) string {
		if denom == 0 {
			return "—"
		}
		return fmt.Sprintf("%.1f%%", float64(num)/float64(denom)*100)
	}

	intStr := func(n int) string {
		if n == 0 {
			return "0"
		}
		return fmt.Sprintf("%d", n)
	}

	addRow := func(metric string, overallNum, overallDenom int, posGetter func(*stats.HandRangePositionCell) (int, int)) []fyne.CanvasObject {
		row := []fyne.CanvasObject{
			widget.NewLabel(metric),
			widget.NewLabel(pctStr(overallNum, overallDenom)),
		}
		for _, p := range positions {
			pc := cell.ByPosition[p.Pos]
			if pc == nil {
				row = append(row, widget.NewLabel("—"))
			} else {
				n, d := posGetter(pc)
				row = append(row, widget.NewLabel(pctStr(n, d)))
			}
		}
		return row
	}

	addCountRow := func(metric string, overallVal int, posGetter func(*stats.HandRangePositionCell) int) []fyne.CanvasObject {
		row := []fyne.CanvasObject{
			widget.NewLabel(metric),
			widget.NewLabel(intStr(overallVal)),
		}
		for _, p := range positions {
			pc := cell.ByPosition[p.Pos]
			if pc == nil {
				row = append(row, widget.NewLabel("—"))
			} else {
				row = append(row, widget.NewLabel(intStr(posGetter(pc))))
			}
		}
		return row
	}

	tableItems := make([]fyne.CanvasObject, 0)
	tableItems = append(tableItems, headerCells...)

	tableItems = append(tableItems, addCountRow("Dealt", cell.Dealt, func(pc *stats.HandRangePositionCell) int { return pc.Dealt })...)
	tableItems = append(tableItems, addRow("VPIP%", cell.VPIP, cell.Dealt, func(pc *stats.HandRangePositionCell) (int, int) { return pc.VPIP, pc.Dealt })...)
	tableItems = append(tableItems, addRow("PFR%", cell.PFR, cell.Dealt, func(pc *stats.HandRangePositionCell) (int, int) { return pc.PFR, pc.Dealt })...)
	tableItems = append(tableItems, addRow("3Bet%", cell.ThreeBet, cell.Dealt, func(pc *stats.HandRangePositionCell) (int, int) { return pc.ThreeBet, pc.Dealt })...)
	tableItems = append(tableItems, addRow("Fold%", cell.Fold, cell.Dealt, func(pc *stats.HandRangePositionCell) (int, int) { return pc.Fold, pc.Dealt })...)
	tableItems = append(tableItems, addCountRow("Won", cell.Won, func(pc *stats.HandRangePositionCell) int { return pc.Won })...)

	tableGrid := container.NewGridWithColumns(cols, tableItems...)

	body := container.NewVBox(
		title,
		canvas.NewLine(theme.ShadowColor()),
		tableGrid,
	)

	dialog.ShowCustom(name+" details", "Close", body, win)
}

// ── Grid building ─────────────────────────────────────────────────────────────

const (
	cellW float32 = 52
	cellH float32 = 40
	headerSize float32 = 28
)

// buildGridContent constructs the 14×14 container (header row+col + 13×13 cells).
// It returns a plain container that can be placed inside a scroll.
func buildGridContent(
	s *stats.Stats,
	af actionFilter,
	posIdx int,
	win fyne.Window,
) fyne.CanvasObject {

	totalCols := 14 // 1 header col + 13 data cols
	totalRows := 14 // 1 header row + 13 data rows

	items := make([]fyne.CanvasObject, 0, totalCols*totalRows)

	rankLabels := stats.RankOrder // "A","K",...,"2"

	// ── Row 0: header row (corner + rank labels) ──────────────────────────────
	// Corner cell (blank)
	corner := canvas.NewRectangle(color.NRGBA{30, 30, 30, 255})
	corner.SetMinSize(fyne.NewSize(headerSize, headerSize))
	items = append(items, container.NewStack(corner))

	for col := 0; col < 13; col++ {
		lbl := canvas.NewText(rankDisplayName(rankLabels[col]), color.White)
		lbl.TextStyle = fyne.TextStyle{Bold: true}
		lbl.TextSize = 11
		lbl.Alignment = fyne.TextAlignCenter
		bg := canvas.NewRectangle(color.NRGBA{30, 30, 30, 255})
		bg.SetMinSize(fyne.NewSize(cellW, headerSize))
		items = append(items, container.NewStack(bg, container.NewCenter(lbl)))
	}

	// ── Rows 1–13: header label + 13 data cells ───────────────────────────────
	for row := 0; row < 13; row++ {
		// Left header cell
		lbl := canvas.NewText(rankDisplayName(rankLabels[row]), color.White)
		lbl.TextStyle = fyne.TextStyle{Bold: true}
		lbl.TextSize = 11
		lbl.Alignment = fyne.TextAlignCenter
		hdrBg := canvas.NewRectangle(color.NRGBA{30, 30, 30, 255})
		hdrBg.SetMinSize(fyne.NewSize(headerSize, cellH))
		items = append(items, container.NewStack(hdrBg, container.NewCenter(lbl)))

		// 13 data cells
		for col := 0; col < 13; col++ {
			var cell *stats.HandRangeCell
			if s != nil && s.HandRange != nil {
				cell = s.HandRange.Cells[row][col]
			}

			items = append(items, makeHandCell(row, col, cell, af, posIdx, win))
		}
	}

	return container.NewGridWithColumns(totalCols, items...)
}

// makeHandCell builds a single hand range cell widget.
func makeHandCell(
	row, col int,
	cell *stats.HandRangeCell,
	af actionFilter,
	posIdx int,
	win fyne.Window,
) fyne.CanvasObject {

	// Determine combo name
	var comboName string
	var bgColor color.Color
	var pctStr string
	var hasData bool

	if cell != nil && cell.Dealt > 0 {
		comboName = comboDisplayName(cell)
		rate := cellRate(cell, af, posIdx)
		if rate < 0 {
			// No data for selected position
			bgColor = color.NRGBA{35, 35, 35, 255}
			pctStr = "—"
		} else {
			bgColor = colorForRate(rate)
			pctStr = fmt.Sprintf("%.1f%%", rate)
			hasData = true
		}
	} else if cell != nil {
		// Cell exists but no hands dealt with it
		comboName = comboDisplayName(cell)
		bgColor = color.NRGBA{35, 35, 35, 255}
		pctStr = "—"
	} else {
		// Nil cell (shouldn't happen after init, but guard anyway)
		comboName = "---"
		bgColor = color.NRGBA{35, 35, 35, 255}
		pctStr = ""
	}

	// Determine text brightness based on background
	textColor := color.NRGBA{220, 220, 220, 255}
	if hasData {
		textColor = color.NRGBA{255, 255, 255, 255}
	}

	rect := canvas.NewRectangle(bgColor)
	rect.SetMinSize(fyne.NewSize(cellW, cellH))

	nameText := canvas.NewText(comboName, textColor)
	nameText.TextSize = 10
	nameText.TextStyle = fyne.TextStyle{Bold: true}
	nameText.Alignment = fyne.TextAlignCenter

	pctText := canvas.NewText(pctStr, color.NRGBA{220, 220, 220, 255})
	pctText.TextSize = 8
	pctText.Alignment = fyne.TextAlignCenter

	textStack := container.NewVBox(
		container.NewCenter(nameText),
		container.NewCenter(pctText),
	)

	// Invisible button for tap handling (captures the cell in closure)
	capturedCell := cell
	var btn fyne.CanvasObject
	if capturedCell != nil && capturedCell.Dealt > 0 && win != nil {
		b := widget.NewButton("", func() {
			showCellDetail(capturedCell, win)
		})
		btn = b
	} else {
		btn = canvas.NewRectangle(color.Transparent)
	}

	return container.NewStack(rect, textStack, btn)
}

// ── NewHandRangeTab ───────────────────────────────────────────────────────────

// NewHandRangeTab returns the "Hand Range" tab canvas object.
// win is needed to anchor modal dialogs; pass nil to disable popups.
func NewHandRangeTab(s *stats.Stats, win fyne.Window) fyne.CanvasObject {
	// Empty state
	if s == nil || s.HandRange == nil {
		msg := widget.NewLabel("No hand data yet.")
		msg.Alignment = fyne.TextAlignCenter
		return container.NewCenter(msg)
	}

	// ── State ────────────────────────────────────────────────────────────────
	currentAction := filterVPIP
	currentPosIdx := 0 // 0 = All

	// ── Grid container (mutable) ──────────────────────────────────────────────
	gridWrap := container.NewStack()

	rebuildGrid := func() {
		content := buildGridContent(s, currentAction, currentPosIdx, win)
		gridWrap.Objects = []fyne.CanvasObject{content}
		gridWrap.Refresh()
	}

	// Initial build
	rebuildGrid()

	// ── Filter: action ────────────────────────────────────────────────────────
	actionOptions := []string{"VPIP", "PFR", "3Bet%", "Fold%"}
	actionSelect := widget.NewSelect(actionOptions, func(val string) {
		switch val {
		case "VPIP":
			currentAction = filterVPIP
		case "PFR":
			currentAction = filterPFR
		case "3Bet%":
			currentAction = filter3Bet
		case "Fold%":
			currentAction = filterFold
		}
		rebuildGrid()
	})
	actionSelect.SetSelected("VPIP")

	// ── Filter: position ──────────────────────────────────────────────────────
	posOptions := make([]string, len(positionFilters))
	for i, p := range positionFilters {
		posOptions[i] = p.Label
	}
	posSelect := widget.NewSelect(posOptions, func(val string) {
		for i, p := range positionFilters {
			if p.Label == val {
				currentPosIdx = i
				break
			}
		}
		rebuildGrid()
	})
	posSelect.SetSelected("All")

	colorModeLabel := widget.NewLabelWithStyle("Color = action rate (darker=lower, red=higher)", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	filterPanel := container.NewHBox(
		widget.NewLabel("Action:"),
		actionSelect,
		widget.NewSeparator(),
		widget.NewLabel("Position:"),
		posSelect,
		widget.NewSeparator(),
		colorModeLabel,
	)

	// ── Legend ────────────────────────────────────────────────────────────────
	legend := buildLegend()

	// ── Final layout ──────────────────────────────────────────────────────────
	scrollable := container.NewScroll(gridWrap)

	return container.NewBorder(
		container.NewVBox(filterPanel, canvas.NewLine(theme.ShadowColor())),
		container.NewVBox(canvas.NewLine(theme.ShadowColor()), legend),
		nil,
		nil,
		scrollable,
	)
}
