package ui

import (
	"fmt"
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

const (
	rangeCellW   float32 = 46
	rangeCellH   float32 = 33
	rangeHeaderH float32 = 24
	rangeHeaderW float32 = 24
)

type actionVisual struct {
	Buckets       []stats.RangeActionBucket
	LabelKey      string
	LabelFallback string
	Color         color.NRGBA
}

type HandRangeViewState struct {
	PositionKey   string
	SelectedCombo string
}

var actionVisuals = []actionVisual{
	{Buckets: []stats.RangeActionBucket{stats.RangeActionCheck}, LabelKey: "hand_range.action.check", LabelFallback: "Check", Color: color.NRGBA{R: 0x4F, G: 0xC3, B: 0xF7, A: 0xFF}},
	{Buckets: []stats.RangeActionBucket{stats.RangeActionCall}, LabelKey: "hand_range.action.call", LabelFallback: "Call", Color: color.NRGBA{R: 0x4C, G: 0xAF, B: 0x50, A: 0xFF}},
	{Buckets: []stats.RangeActionBucket{stats.RangeActionBetSmall, stats.RangeActionBetHalf, stats.RangeActionBetTwoThird, stats.RangeActionBetPot, stats.RangeActionBetOver}, LabelKey: "hand_range.action.raise", LabelFallback: "Raise", Color: color.NRGBA{R: 0xE5, G: 0x39, B: 0x35, A: 0xFF}},
	{Buckets: []stats.RangeActionBucket{stats.RangeActionFold}, LabelKey: "hand_range.action.fold", LabelFallback: "Fold", Color: color.NRGBA{R: 0xB0, G: 0xBE, B: 0xC5, A: 0xFF}},
}

var positionFilters = []struct {
	Key           string
	LabelKey      string
	LabelFallback string
	Pos           parser.Position
	IsAll         bool
}{
	{Key: "all", LabelKey: "hand_range.filter.all", LabelFallback: "All", IsAll: true},
	{Key: "sb", LabelKey: "hand_range.filter.sb", LabelFallback: "SB", Pos: parser.PosSB},
	{Key: "bb", LabelKey: "hand_range.filter.bb", LabelFallback: "BB", Pos: parser.PosBB},
	{Key: "utg", LabelKey: "hand_range.filter.utg", LabelFallback: "UTG", Pos: parser.PosUTG},
	{Key: "utg1", LabelKey: "hand_range.filter.utg1", LabelFallback: "UTG+1", Pos: parser.PosUTG1},
	{Key: "mp", LabelKey: "hand_range.filter.mp", LabelFallback: "MP", Pos: parser.PosMP},
	{Key: "hj", LabelKey: "hand_range.filter.hj", LabelFallback: "HJ", Pos: parser.PosHJ},
	{Key: "co", LabelKey: "hand_range.filter.co", LabelFallback: "CO", Pos: parser.PosCO},
	{Key: "btn", LabelKey: "hand_range.filter.btn", LabelFallback: "BTN", Pos: parser.PosBTN},
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

func actionVisualLabel(av actionVisual) string {
	return lang.X(av.LabelKey, av.LabelFallback)
}

func actionCountForVisual(counts [stats.RangeActionBucketCount]int, av actionVisual) int {
	total := 0
	for _, b := range av.Buckets {
		total += counts[b]
	}
	return total
}

func positionFilterLabel(index int) string {
	if index < 0 || index >= len(positionFilters) {
		return "" //i18n:ignore invalid index fallback
	}
	p := positionFilters[index]
	return lang.X(p.LabelKey, p.LabelFallback)
}

type rangeCellWidget struct {
	widget.BaseWidget

	counts [stats.RangeActionBucketCount]int
	dealt  int

	comboKey string
	label    string

	isSelected bool
	isDimmed   bool

	onSelect func()
}

func newRangeCellWidget(cell *stats.HandRangeCell, posIdx int, selectedCombo string, onSelect func(*stats.HandRangeCell)) *rangeCellWidget {
	w := &rangeCellWidget{}
	w.counts, w.dealt = actionCountsForCell(cell, posIdx)
	w.label = "---"
	if cell != nil {
		w.comboKey = comboDisplayName(cell)
		w.label = w.comboKey
		if onSelect != nil {
			selectedCell := cell
			w.onSelect = func() { onSelect(selectedCell) }
		}
	}
	if selectedCombo != "" {
		w.isSelected = w.comboKey != "" && w.comboKey == selectedCombo
		w.isDimmed = w.comboKey != "" && w.comboKey != selectedCombo
	}
	w.ExtendBaseWidget(w)
	return w
}

func (w *rangeCellWidget) Tapped(*fyne.PointEvent) {
	if w.onSelect != nil {
		w.onSelect()
	}
}

func (w *rangeCellWidget) TappedSecondary(*fyne.PointEvent) {}

func (w *rangeCellWidget) MinSize() fyne.Size {
	return fyne.NewSize(rangeCellW, rangeCellH)
}

func (w *rangeCellWidget) CreateRenderer() fyne.WidgetRenderer {
	r := &rangeCellRenderer{
		owner:  w,
		bg:     canvas.NewRectangle(color.NRGBA{R: 0x2B, G: 0x2B, B: 0x2B, A: 0xFF}),
		mask:   canvas.NewRectangle(color.Transparent),
		border: canvas.NewRectangle(color.Transparent),
		label:  canvas.NewText(w.label, color.White),
	}
	r.border.StrokeColor = color.NRGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xFF}
	r.border.StrokeWidth = 1
	if w.isSelected {
		r.border.StrokeColor = color.NRGBA{R: 0xF5, G: 0xF5, B: 0xF5, A: 0xFF}
		r.border.StrokeWidth = 2
	}
	if w.isDimmed {
		r.mask.FillColor = color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xB5}
	}

	r.label.TextStyle = fyne.TextStyle{Bold: true}
	r.label.TextSize = 13.5
	r.label.Alignment = fyne.TextAlignCenter
	r.labelOutline = []*canvas.Text{
		canvas.NewText(w.label, color.NRGBA{R: 0x2E, G: 0x2E, B: 0x2E, A: 0xD0}),
		canvas.NewText(w.label, color.NRGBA{R: 0x2E, G: 0x2E, B: 0x2E, A: 0xD0}),
		canvas.NewText(w.label, color.NRGBA{R: 0x2E, G: 0x2E, B: 0x2E, A: 0xD0}),
		canvas.NewText(w.label, color.NRGBA{R: 0x2E, G: 0x2E, B: 0x2E, A: 0xD0}),
	}
	outlineObjects := make([]fyne.CanvasObject, 0, len(r.labelOutline)+1)
	for _, o := range r.labelOutline {
		o.TextStyle = fyne.TextStyle{Bold: true}
		o.TextSize = r.label.TextSize
		o.Alignment = fyne.TextAlignCenter
		outlineObjects = append(outlineObjects, o)
	}
	outlineObjects = append(outlineObjects, r.label)
	r.labelLayer = container.NewWithoutLayout(outlineObjects...)

	r.barStops, r.barColors = buildRangeCellStops(w.counts, w.dealt)
	r.barRaster = canvas.NewRasterWithPixels(func(x, _y, w, h int) color.Color {
		if w <= 0 || h <= 0 || len(r.barStops) == 0 || len(r.barStops) != len(r.barColors) {
			return color.Transparent
		}

		t := (float32(x) + 0.5) / float32(w)
		for i, stop := range r.barStops {
			if t <= stop {
				return r.barColors[i]
			}
		}

		return color.Transparent
	})

	r.objects = make([]fyne.CanvasObject, 0, 5)
	r.objects = append(r.objects, r.bg)
	r.objects = append(r.objects, r.barRaster)
	r.objects = append(r.objects, r.mask, r.border, r.labelLayer)
	return r
}

func buildRangeCellStops(counts [stats.RangeActionBucketCount]int, dealt int) ([]float32, []color.NRGBA) {
	if dealt <= 0 {
		return nil, nil
	}

	stops := make([]float32, 0, len(actionVisuals))
	colors := make([]color.NRGBA, 0, len(actionVisuals))
	cumulative := 0

	for _, av := range actionVisuals {
		count := actionCountForVisual(counts, av)
		if count <= 0 {
			continue
		}

		cumulative += count
		if cumulative > dealt {
			cumulative = dealt
		}

		stop := float32(cumulative) / float32(dealt)
		if len(stops) > 0 && stop <= stops[len(stops)-1] {
			continue
		}

		stops = append(stops, stop)
		colors = append(colors, av.Color)

		if cumulative == dealt {
			break
		}
	}

	return stops, colors
}

type rangeCellRenderer struct {
	owner *rangeCellWidget

	bg           *canvas.Rectangle
	barRaster    *canvas.Raster
	barStops     []float32
	barColors    []color.NRGBA
	mask         *canvas.Rectangle
	border       *canvas.Rectangle
	label        *canvas.Text
	labelOutline []*canvas.Text

	labelLayer fyne.CanvasObject
	objects    []fyne.CanvasObject
}

func (r *rangeCellRenderer) Layout(size fyne.Size) {
	r.bg.Move(fyne.NewPos(0, 0))
	r.bg.Resize(size)
	r.barRaster.Move(fyne.NewPos(0, 0))
	r.barRaster.Resize(size)

	r.mask.Move(fyne.NewPos(0, 0))
	r.mask.Resize(size)
	r.border.Move(fyne.NewPos(0, 0))
	r.border.Resize(size)
	r.labelLayer.Move(fyne.NewPos(0, 0))
	r.labelLayer.Resize(size)

	labelSize := r.label.MinSize()
	x := (size.Width - labelSize.Width) / 2
	y := (size.Height - labelSize.Height) / 2
	offsets := []fyne.Position{{X: -1, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: -1}, {X: 0, Y: 1}}
	for i, o := range r.labelOutline {
		o.Move(fyne.NewPos(x+offsets[i].X, y+offsets[i].Y))
		o.Resize(labelSize)
	}
	r.label.Move(fyne.NewPos(x, y))
	r.label.Resize(labelSize)
}

func (r *rangeCellRenderer) MinSize() fyne.Size {
	return fyne.NewSize(rangeCellW, rangeCellH)
}

func (r *rangeCellRenderer) Refresh() {
	r.barStops, r.barColors = buildRangeCellStops(r.owner.counts, r.owner.dealt)
	r.barRaster.Refresh()

	r.label.Text = r.owner.label
	r.label.Refresh()
	for _, o := range r.labelOutline {
		o.Text = r.owner.label
		o.Refresh()
	}
	r.Layout(r.owner.Size())
	for _, obj := range r.objects {
		canvas.Refresh(obj)
	}
}

func (r *rangeCellRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *rangeCellRenderer) Destroy() {}

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
			items = append(items, newRangeCellWidget(cell, posIdx, selectedCombo, onSelect))
		}
	}

	return container.NewGridWithColumns(14, items...)
}

func buildActionFrequencyCards(counts [stats.RangeActionBucketCount]int, total int) fyne.CanvasObject {
	cards := make([]fyne.CanvasObject, 0, len(actionVisuals))
	for _, av := range actionVisuals {
		rate := 0.0
		count := actionCountForVisual(counts, av)
		if total > 0 {
			rate = float64(count) / float64(total) * 100
		}

		title := canvas.NewText(actionVisualLabel(av), color.NRGBA{R: 0xF8, G: 0xFA, B: 0xFC, A: 0xFF})
		title.TextStyle = fyne.TextStyle{Bold: true}
		title.Alignment = fyne.TextAlignLeading

		percent := canvas.NewText(fmt.Sprintf("%.1f%%", rate), color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF})
		percent.TextStyle = fyne.TextStyle{Bold: true}
		percent.TextSize = theme.TextSize() * 1.2
		percent.Alignment = fyne.TextAlignCenter

		note := canvas.NewText(lang.X("metric.footnote.normal", "n={{.N}}", map[string]any{"N": count}), color.NRGBA{R: 0xF0, G: 0xF3, B: 0xF6, A: 0xBF})
		note.TextSize = theme.TextSize() * 0.86
		note.Alignment = fyne.TextAlignTrailing

		bg := canvas.NewRectangle(color.NRGBA{R: av.Color.R, G: av.Color.G, B: av.Color.B, A: 0xCC})
		bg.CornerRadius = 10
		border := canvas.NewRectangle(color.Transparent)
		border.CornerRadius = 10
		border.StrokeColor = color.NRGBA{R: 0x20, G: 0x23, B: 0x29, A: 0xAA}
		border.StrokeWidth = 1

		cardBody := container.NewBorder(title, container.NewHBox(layout.NewSpacer(), note), nil, nil, container.NewCenter(percent))
		cards = append(cards, container.NewStack(bg, border, container.NewPadded(cardBody)))
	}

	samples := widget.NewLabel(lang.X("hand_range.samples", "Samples: {{.N}}", map[string]any{"N": total}))
	return container.NewVBox(container.NewGridWithColumns(2, cards...), container.NewHBox(layout.NewSpacer(), samples))
}

func buildRightPanel(table *stats.HandRangeTable, posIdx int, selected *stats.HandRangeCell) fyne.CanvasObject {
	allCounts, allTotal := aggregateActions(table, posIdx)
	frequencyTitle := widget.NewLabelWithStyle(lang.X("hand_range.preflop_title", "Preflop Range Action Frequency"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	frequencyBody := fyne.CanvasObject(buildActionFrequencyCards(allCounts, allTotal))

	if selected != nil {
		frequencyTitle = widget.NewLabelWithStyle(lang.X("hand_range.combo_action_title_named", "{{.Combo}} Action Frequency", map[string]any{"Combo": comboDisplayName(selected)}), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		counts, total := actionCountsForCell(selected, posIdx)
		if total == 0 {
			noData := widget.NewLabel(lang.X("hand_range.no_combo_records", "No records for this combo in the selected position filter."))
			noData.Wrapping = fyne.TextWrapWord
			frequencyBody = newSectionCard(noData)
		} else {
			frequencyBody = buildActionFrequencyCards(counts, total)
		}
	}

	return container.NewVBox(frequencyTitle, frequencyBody)
}

func buildPositionFilterBar(currentPosIdx int, onSelect func(int)) fyne.CanvasObject {
	buttons := make([]fyne.CanvasObject, 0, len(positionFilters))
	for i := range positionFilters {
		idx := i
		btn := widget.NewButton(positionFilterLabel(idx), func() {
			onSelect(idx)
		})
		if idx == currentPosIdx {
			btn.Importance = widget.HighImportance
		} else {
			btn.Importance = widget.LowImportance
		}
		buttons = append(buttons, btn)
	}
	return container.NewHScroll(container.NewHBox(buttons...))
}

// handRangeView holds mutable render state for the hand-range tab so that
// the rebuild logic can be split into named methods instead of a closure.
type handRangeView struct {
	s             *stats.Stats
	state         *HandRangeViewState
	currentPosIdx int
	selected      *stats.HandRangeCell

	leftWrap  *fyne.Container
	rightWrap *fyne.Container
	topWrap   *fyne.Container
}

func (v *handRangeView) rebuild() {
	selectedCombo := ""
	if v.selected != nil {
		selectedCombo = comboDisplayName(v.selected)
	}

	filterBar := withMinHeight(buildPositionFilterBar(v.currentPosIdx, func(nextPos int) {
		v.currentPosIdx = nextPos
		v.state.PositionKey = positionFilters[nextPos].Key
		v.selected = nil
		v.state.SelectedCombo = ""
		v.rebuild()
	}), 36)

	var rightControl fyne.CanvasObject
	if v.selected != nil {
		clearBtn := widget.NewButtonWithIcon(lang.X("hand_range.show_all", "Show All Range"), theme.ViewRefreshIcon(), func() {
			v.selected = nil
			v.state.SelectedCombo = ""
			v.rebuild()
		})
		clearBtn.Importance = widget.HighImportance
		rightControl = clearBtn
	}

	positionRow := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle(lang.X("hand_range.position_label", "Position:"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		rightControl,
		filterBar,
	)
	v.topWrap.Objects = []fyne.CanvasObject{newSectionCard(positionRow)}
	v.topWrap.Refresh()

	v.rebuildGrid(selectedCombo)
	v.rebuildRightPanel()
}

func (v *handRangeView) rebuildGrid(selectedCombo string) {
	grid := buildRangeGrid(v.s, v.currentPosIdx, selectedCombo, func(cell *stats.HandRangeCell) {
		v.selected = cell
		v.state.SelectedCombo = comboDisplayName(cell)
		v.rebuild()
	})
	gridSize := grid.MinSize()
	gridFixed := container.NewWithoutLayout()
	gridLock := canvas.NewRectangle(color.Transparent)
	gridLock.SetMinSize(gridSize)
	gridLock.Resize(gridSize)
	gridFixed.Add(gridLock)
	grid.Move(fyne.NewPos(0, 0))
	grid.Resize(gridSize)
	gridFixed.Add(grid)

	v.leftWrap.Objects = []fyne.CanvasObject{container.NewScroll(gridFixed)}
	v.leftWrap.Refresh()
}

func (v *handRangeView) rebuildRightPanel() {
	rightMin := canvas.NewRectangle(color.Transparent)
	rightMin.SetMinSize(fyne.NewSize(360, 0))
	rightContent := container.NewPadded(buildRightPanel(v.s.HandRange, v.currentPosIdx, v.selected))
	v.rightWrap.Objects = []fyne.CanvasObject{container.NewStack(rightMin, rightContent)}
	v.rightWrap.Refresh()
}

// NewHandRangeTab renders a GTO-style mixed-strategy range view.
func NewHandRangeTab(s *stats.Stats, _ fyne.Window, state *HandRangeViewState) fyne.CanvasObject {
	if s == nil || s.HandRange == nil {
		return newCenteredEmptyState(lang.X("hand_range.no_hand_data", "No hand data yet."))
	}
	if state == nil {
		state = &HandRangeViewState{}
	}

	currentPosIdx := 0
	if state.PositionKey != "" {
		for i, p := range positionFilters {
			if p.Key == state.PositionKey {
				currentPosIdx = i
				break
			}
		}
	}

	v := &handRangeView{
		s:             s,
		state:         state,
		currentPosIdx: currentPosIdx,
		selected:      findCellByCombo(s.HandRange, state.SelectedCombo),
		leftWrap:      container.NewMax(),
		rightWrap:     container.NewMax(),
		topWrap:       container.NewMax(),
	}

	if state.PositionKey == "" {
		state.PositionKey = positionFilters[currentPosIdx].Key
	}

	v.rebuild()

	split := container.NewHSplit(v.leftWrap, container.NewScroll(v.rightWrap))
	split.Offset = 0.60

	return container.NewBorder(
		container.NewVBox(v.topWrap, newSectionDivider()),
		nil,
		nil,
		nil,
		split,
	)
}
