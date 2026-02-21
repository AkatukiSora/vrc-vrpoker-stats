package ui

import (
	"fmt"
	"image/color"
	"sort"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

const (
	rangeCellW   float32 = 46
	rangeCellH   float32 = 33
	rangeHeaderH float32 = 24
	rangeHeaderW float32 = 24
)

type actionVisual struct {
	Bucket stats.RangeActionBucket
	Label  string
	Color  color.NRGBA
}

type HandRangeViewState struct {
	PositionLabel string
	SelectedCombo string
}

var actionVisuals = []actionVisual{
	{Bucket: stats.RangeActionCheck, Label: "Check", Color: color.NRGBA{R: 0x54, G: 0x6E, B: 0x7A, A: 0xFF}},
	{Bucket: stats.RangeActionCall, Label: "Call", Color: color.NRGBA{R: 0x2E, G: 0x7D, B: 0x32, A: 0xFF}},
	{Bucket: stats.RangeActionBetSmall, Label: "Bet <=1/3", Color: color.NRGBA{R: 0x03, G: 0xA9, B: 0xF4, A: 0xFF}},
	{Bucket: stats.RangeActionBetHalf, Label: "Bet ~1/2", Color: color.NRGBA{R: 0x00, G: 0x96, B: 0x88, A: 0xFF}},
	{Bucket: stats.RangeActionBetTwoThird, Label: "Bet ~2/3", Color: color.NRGBA{R: 0xFF, G: 0xB3, B: 0x00, A: 0xFF}},
	{Bucket: stats.RangeActionBetPot, Label: "Bet ~Pot", Color: color.NRGBA{R: 0xF5, G: 0x7C, B: 0x00, A: 0xFF}},
	{Bucket: stats.RangeActionBetOver, Label: "Overbet", Color: color.NRGBA{R: 0xD8, G: 0x1B, B: 0x60, A: 0xFF}},
	{Bucket: stats.RangeActionFold, Label: "Fold", Color: color.NRGBA{R: 0x6D, G: 0x4C, B: 0x41, A: 0xFF}},
}

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

type tapArea struct {
	widget.BaseWidget
	onTap func()
}

func newTapArea(onTap func()) *tapArea {
	t := &tapArea{onTap: onTap}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tapArea) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

func (t *tapArea) TappedSecondary(*fyne.PointEvent) {}

func (t *tapArea) CreateRenderer() fyne.WidgetRenderer {
	r := canvas.NewRectangle(color.Transparent)
	return widget.NewSimpleRenderer(r)
}

func rankDisplayName(r string) string {
	if r == "10" {
		return "T"
	}
	return r
}

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

func findCellByCombo(table *stats.HandRangeTable, combo string) *stats.HandRangeCell {
	if table == nil || combo == "" {
		return nil
	}
	for r := 0; r < 13; r++ {
		for c := 0; c < 13; c++ {
			cell := table.Cells[r][c]
			if cell == nil {
				continue
			}
			if comboDisplayName(cell) == combo {
				return cell
			}
		}
	}
	return nil
}

func actionCountsForCell(cell *stats.HandRangeCell, posIdx int) ([stats.RangeActionBucketCount]int, int) {
	var counts [stats.RangeActionBucketCount]int
	if cell == nil {
		return counts, 0
	}
	if posIdx == 0 {
		return cell.Actions, cell.Dealt
	}
	pc := cell.ByPosition[positionFilters[posIdx].Pos]
	if pc == nil {
		return counts, 0
	}
	return pc.Actions, pc.Dealt
}

func aggregateActions(table *stats.HandRangeTable, posIdx int) ([stats.RangeActionBucketCount]int, int) {
	var total [stats.RangeActionBucketCount]int
	hands := 0
	if table == nil {
		return total, hands
	}
	if posIdx == 0 {
		for i := 0; i < int(stats.RangeActionBucketCount); i++ {
			total[i] = table.TotalActions[i]
			hands += table.TotalActions[i]
		}
		return total, hands
	}

	for r := 0; r < 13; r++ {
		for c := 0; c < 13; c++ {
			cell := table.Cells[r][c]
			if cell == nil {
				continue
			}
			pc := cell.ByPosition[positionFilters[posIdx].Pos]
			if pc == nil {
				continue
			}
			for i := 0; i < int(stats.RangeActionBucketCount); i++ {
				total[i] += pc.Actions[i]
				hands += pc.Actions[i]
			}
		}
	}

	return total, hands
}

func makeRangeCell(cell *stats.HandRangeCell, posIdx int, selectedCombo string, onSelect func(*stats.HandRangeCell)) fyne.CanvasObject {
	bg := canvas.NewRectangle(color.NRGBA{R: 0x2B, G: 0x2B, B: 0x2B, A: 0xFF})
	bg.SetMinSize(fyne.NewSize(rangeCellW, rangeCellH))

	barsLayer := container.NewWithoutLayout()
	barsLayer.Resize(fyne.NewSize(rangeCellW, rangeCellH))

	counts, dealt := actionCountsForCell(cell, posIdx)
	if dealt > 0 {
		x := float32(0)
		for _, av := range actionVisuals {
			cnt := counts[av.Bucket]
			if cnt <= 0 {
				continue
			}
			w := rangeCellW * float32(cnt) / float32(dealt)
			rect := canvas.NewRectangle(av.Color)
			rect.Move(fyne.NewPos(x, 0))
			rect.Resize(fyne.NewSize(w, rangeCellH))
			barsLayer.Add(rect)
			x += w
		}
	}

	border := canvas.NewRectangle(color.Transparent)
	border.StrokeColor = color.NRGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xFF}
	border.StrokeWidth = 1
	border.SetMinSize(fyne.NewSize(rangeCellW, rangeCellH))

	name := "---"
	comboKey := ""
	if cell != nil {
		name = comboDisplayName(cell)
		comboKey = name
	}

	nameText := canvas.NewText(name, color.White)
	nameText.TextStyle = fyne.TextStyle{Bold: true}
	nameText.TextSize = 9
	nameText.Alignment = fyne.TextAlignCenter

	textLayer := container.NewCenter(nameText)

	grayMask := canvas.NewRectangle(color.Transparent)
	if selectedCombo != "" && comboKey != "" && comboKey != selectedCombo {
		grayMask.FillColor = color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xB5}
	}
	if selectedCombo != "" && comboKey == selectedCombo {
		border.StrokeColor = color.NRGBA{R: 0xF5, G: 0xF5, B: 0xF5, A: 0xFF}
		border.StrokeWidth = 2
	}

	var tap fyne.CanvasObject = canvas.NewRectangle(color.Transparent)
	if cell != nil {
		tap = newTapArea(func() { onSelect(cell) })
	}

	return container.NewStack(bg, barsLayer, grayMask, border, textLayer, tap)
}

func buildRangeGrid(s *stats.Stats, posIdx int, selectedCombo string, onSelect func(*stats.HandRangeCell)) fyne.CanvasObject {
	rankLabels := stats.RankOrder
	items := make([]fyne.CanvasObject, 0, 14*14)

	corner := canvas.NewRectangle(color.NRGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xFF})
	corner.SetMinSize(fyne.NewSize(rangeHeaderW, rangeHeaderH))
	items = append(items, corner)

	for col := 0; col < 13; col++ {
		bg := canvas.NewRectangle(color.NRGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xFF})
		bg.SetMinSize(fyne.NewSize(rangeCellW, rangeHeaderH))
		tx := canvas.NewText(rankDisplayName(rankLabels[col]), color.White)
		tx.TextStyle = fyne.TextStyle{Bold: true}
		tx.TextSize = 11
		tx.Alignment = fyne.TextAlignCenter
		items = append(items, container.NewStack(bg, container.NewCenter(tx)))
	}

	for row := 0; row < 13; row++ {
		bg := canvas.NewRectangle(color.NRGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xFF})
		bg.SetMinSize(fyne.NewSize(rangeHeaderW, rangeCellH))
		tx := canvas.NewText(rankDisplayName(rankLabels[row]), color.White)
		tx.TextStyle = fyne.TextStyle{Bold: true}
		tx.TextSize = 11
		tx.Alignment = fyne.TextAlignCenter
		items = append(items, container.NewStack(bg, container.NewCenter(tx)))

		for col := 0; col < 13; col++ {
			var cell *stats.HandRangeCell
			if s != nil && s.HandRange != nil {
				cell = s.HandRange.Cells[row][col]
			}
			items = append(items, makeRangeCell(cell, posIdx, selectedCombo, onSelect))
		}
	}

	return container.NewGridWithColumns(14, items...)
}

func buildActionSummaryRows(counts [stats.RangeActionBucketCount]int, total int) fyne.CanvasObject {
	rows := make([]fyne.CanvasObject, 0, len(actionVisuals)+1)
	for _, av := range actionVisuals {
		rate := 0.0
		if total > 0 {
			rate = float64(counts[av.Bucket]) / float64(total) * 100
		}
		swatch := canvas.NewRectangle(av.Color)
		swatch.SetMinSize(fyne.NewSize(12, 12))
		rows = append(rows, container.NewHBox(
			swatch,
			widget.NewLabel(fmt.Sprintf("%-10s", av.Label)),
			widget.NewLabel(fmt.Sprintf("%5.1f%%", rate)),
			widget.NewLabel(fmt.Sprintf("(%d)", counts[av.Bucket])),
		))
	}
	rows = append(rows, widget.NewLabel(fmt.Sprintf("Samples: %d", total)))
	return container.NewVBox(rows...)
}

func buildClassSummaryByMap(byClass map[string]*stats.HandClassStats) fyne.CanvasObject {
	if len(byClass) == 0 {
		return widget.NewLabel("No class-level data yet.")
	}

	keys := make([]string, 0, len(byClass))
	for k := range byClass {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	rows := make([]fyne.CanvasObject, 0, len(keys))
	for _, k := range keys {
		v := byClass[k]
		if v == nil || v.Hands == 0 {
			continue
		}
		top := stats.RangeActionCheck
		for i := stats.RangeActionCheck; i < stats.RangeActionBucketCount; i++ {
			if v.Actions[i] > v.Actions[top] {
				top = i
			}
		}
		rows = append(rows, widget.NewLabel(fmt.Sprintf("%s  |  %s %.1f%%  | n=%d",
			k,
			stats.RangeActionLabels[top],
			v.ActionRate(top),
			v.Hands,
		)))
	}

	if len(rows) == 0 {
		return widget.NewLabel("No class-level data yet.")
	}

	return container.NewVBox(rows...)
}

func buildRightPanel(table *stats.HandRangeTable, posIdx int, selected *stats.HandRangeCell) fyne.CanvasObject {
	if selected != nil {
		counts, total := actionCountsForCell(selected, posIdx)
		title := widget.NewLabelWithStyle("Selected Combo Action Frequency", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		comboName := widget.NewLabelWithStyle(comboDisplayName(selected), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

		if total == 0 {
			noData := widget.NewLabel("No records for this combo in the selected position filter.")
			noData.Wrapping = fyne.TextWrapWord
			return container.NewVBox(
				title,
				comboName,
				widget.NewSeparator(),
				noData,
			)
		}

		comboBox := buildActionSummaryRows(counts, total)
		comboSection := container.NewVBox(title, comboName, comboBox)
		classAcc := widget.NewAccordion(
			widget.NewAccordionItem("Hand Class Action Frequency (Selected Combo, All Streets)", buildClassSummaryByMap(selected.ByHandClass)),
		)
		return container.NewVBox(
			comboSection,
			widget.NewSeparator(),
			classAcc,
		)
	}

	allCounts, allTotal := aggregateActions(table, posIdx)
	globalTitle := widget.NewLabelWithStyle("Preflop Range Action Frequency", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	globalBox := buildActionSummaryRows(allCounts, allTotal)
	comboTitle := widget.NewLabelWithStyle("Selected Combo", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	comboName := widget.NewLabel("None")
	comboStats := widget.NewLabel("Click a cell in the 13x13 grid.")

	classAcc := widget.NewAccordion(
		widget.NewAccordionItem("Hand Class Action Frequency (All Streets)", buildClassSummaryByMap(table.ByHandClass)),
	)

	return container.NewVBox(
		globalTitle,
		globalBox,
		widget.NewSeparator(),
		comboTitle,
		comboName,
		comboStats,
		widget.NewSeparator(),
		classAcc,
	)
}

func buildActionLegend() fyne.CanvasObject {
	items := make([]fyne.CanvasObject, 0, len(actionVisuals))
	for _, av := range actionVisuals {
		swatch := canvas.NewRectangle(av.Color)
		swatch.SetMinSize(fyne.NewSize(12, 12))
		items = append(items, container.NewHBox(swatch, widget.NewLabel(av.Label)))
	}
	return container.NewGridWithColumns(4, items...)
}

// NewHandRangeTab renders a GTO-style mixed-strategy range view.
func NewHandRangeTab(s *stats.Stats, _ fyne.Window, state *HandRangeViewState) fyne.CanvasObject {
	if s == nil || s.HandRange == nil {
		msg := widget.NewLabel("No hand data yet.")
		msg.Alignment = fyne.TextAlignCenter
		return container.NewCenter(msg)
	}
	if state == nil {
		state = &HandRangeViewState{}
	}

	currentPosIdx := 0
	if state.PositionLabel != "" {
		for i, p := range positionFilters {
			if p.Label == state.PositionLabel {
				currentPosIdx = i
				break
			}
		}
	}
	selected := findCellByCombo(s.HandRange, state.SelectedCombo)

	leftWrap := container.NewMax()
	rightWrap := container.NewMax()
	clearSelectionButton := widget.NewButton("Show All Range", nil)

	var rebuild func()
	rebuild = func() {
		selectedCombo := ""
		if selected != nil {
			selectedCombo = comboDisplayName(selected)
		}
		grid := buildRangeGrid(s, currentPosIdx, selectedCombo, func(cell *stats.HandRangeCell) {
			selected = cell
			state.SelectedCombo = comboDisplayName(cell)
			rebuild()
		})
		leftWrap.Objects = []fyne.CanvasObject{container.NewScroll(grid)}
		leftWrap.Refresh()

		rightWrap.Objects = []fyne.CanvasObject{container.NewPadded(buildRightPanel(s.HandRange, currentPosIdx, selected))}
		rightWrap.Refresh()

		if selected == nil {
			clearSelectionButton.Disable()
		} else {
			clearSelectionButton.Enable()
		}
	}

	clearSelectionButton.OnTapped = func() {
		selected = nil
		state.SelectedCombo = ""
		rebuild()
	}

	posOptions := make([]string, len(positionFilters))
	for i, p := range positionFilters {
		posOptions[i] = p.Label
	}
	posSelect := widget.NewSelect(posOptions, func(val string) {
		for i, p := range positionFilters {
			if p.Label == val {
				currentPosIdx = i
				state.PositionLabel = p.Label
				break
			}
		}
		rebuild()
	})
	if state.PositionLabel == "" {
		state.PositionLabel = "All"
	}
	posSelect.SetSelected(state.PositionLabel)

	legend := buildActionLegend()
	positionRow := container.NewHBox(
		container.NewCenter(widget.NewLabel("Position:")),
		container.NewCenter(posSelect),
	)
	topBarContent := container.NewHBox(
		positionRow,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Action Palette", fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
		legend,
		widget.NewSeparator(),
		container.NewCenter(clearSelectionButton),
	)
	topBar := container.NewCenter(topBarContent)

	split := container.NewHSplit(leftWrap, container.NewScroll(rightWrap))
	split.Offset = 0.66

	rebuild()

	return container.NewBorder(
		container.NewVBox(topBar, canvas.NewLine(theme.ShadowColor())),
		nil,
		nil,
		nil,
		split,
	)
}
