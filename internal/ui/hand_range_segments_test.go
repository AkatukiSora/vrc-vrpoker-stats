package ui

import (
	"image/color"
	"math"
	"math/rand"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

func TestBuildRangeCellStopsZeroDealt(t *testing.T) {
	var counts [stats.RangeActionBucketCount]int
	counts[stats.RangeActionCheck] = 5

	stops, colors := buildRangeCellStops(counts, 0)
	if len(stops) != 0 {
		t.Fatalf("expected no stops when dealt=0, got %d", len(stops))
	}
	if len(colors) != 0 {
		t.Fatalf("expected no colors when dealt=0, got %d", len(colors))
	}
}

func TestBuildRangeCellStopsMonotonicAndPartialCoverage(t *testing.T) {
	var counts [stats.RangeActionBucketCount]int
	counts[stats.RangeActionCheck] = 8
	counts[stats.RangeActionCall] = 3
	counts[stats.RangeActionBetHalf] = 5
	counts[stats.RangeActionFold] = 2

	stops, colors := buildRangeCellStops(counts, 20)
	if len(stops) != 4 {
		t.Fatalf("expected 4 stops, got %d", len(stops))
	}
	if len(colors) != 4 {
		t.Fatalf("expected 4 colors, got %d", len(colors))
	}

	prev := float32(0)
	for i, stop := range stops {
		if stop <= prev {
			t.Fatalf("stops must be strictly increasing: stop[%d]=%f prev=%f", i, stop, prev)
		}
		if stop > 1 {
			t.Fatalf("stop[%d] must be <= 1, got %f", i, stop)
		}
		prev = stop
	}

	if math.Abs(float64(stops[len(stops)-1]-0.9)) > 1e-6 {
		t.Fatalf("expected last stop to be 0.9, got %f", stops[len(stops)-1])
	}
}

func TestBuildRangeCellStopsClampsAtOne(t *testing.T) {
	var counts [stats.RangeActionBucketCount]int
	counts[stats.RangeActionCheck] = 10
	counts[stats.RangeActionCall] = 10
	counts[stats.RangeActionFold] = 10

	stops, _ := buildRangeCellStops(counts, 12)
	if len(stops) == 0 {
		t.Fatal("expected at least one stop")
	}
	if stops[len(stops)-1] != 1 {
		t.Fatalf("expected last stop to clamp to 1, got %f", stops[len(stops)-1])
	}
}

func TestBuildRangeCellStopsRandomizedInvariants(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	for i := 0; i < 2000; i++ {
		dealt := rng.Intn(400) + 1
		var counts [stats.RangeActionBucketCount]int
		for b := 0; b < int(stats.RangeActionBucketCount); b++ {
			counts[b] = rng.Intn(400)
		}

		stops, colors := buildRangeCellStops(counts, dealt)
		if len(stops) != len(colors) {
			t.Fatalf("stops/colors length mismatch: %d vs %d", len(stops), len(colors))
		}
		if len(stops) > len(actionVisuals) {
			t.Fatalf("unexpected stop count: %d", len(stops))
		}

		prev := float32(0)
		for idx, stop := range stops {
			if math.IsNaN(float64(stop)) || math.IsInf(float64(stop), 0) {
				t.Fatalf("invalid stop value at %d: %f", idx, stop)
			}
			if stop <= prev {
				t.Fatalf("stops not strictly increasing at %d: %f <= %f", idx, stop, prev)
			}
			if stop < 0 || stop > 1 {
				t.Fatalf("stop out of range at %d: %f", idx, stop)
			}
			prev = stop
		}

		totalVisual := 0
		for _, av := range actionVisuals {
			totalVisual += actionCountForVisual(counts, av)
		}
		expectedLast := float32(totalVisual) / float32(dealt)
		if expectedLast > 1 {
			expectedLast = 1
		}

		if len(stops) == 0 {
			if expectedLast > 0 {
				t.Fatalf("expected at least one stop, got none (expectedLast=%f)", expectedLast)
			}
			continue
		}

		if math.Abs(float64(stops[len(stops)-1]-expectedLast)) > 1e-6 {
			t.Fatalf("last stop mismatch: got=%f expected=%f", stops[len(stops)-1], expectedLast)
		}
	}
}

func TestRangeCellTransitionFixtureReusesWidgetAndRenderer(t *testing.T) {
	fx := newRangeCellTransitionFixture(t, testRangeCell("A", "K", true, map[parser.Position][stats.RangeActionBucketCount]int{
		parser.PosBTN: rangeCellActions(0, 3, 7, 0),
	}), parser.PosBTN, "")

	initialWidget := fx.widget
	initialRenderer := fx.renderer

	fx.apply(t, testRangeCell("7", "2", false, map[parser.Position][stats.RangeActionBucketCount]int{
		parser.PosBTN: rangeCellActions(1, 1, 1, 1),
	}), parser.PosBTN, "")

	if fx.widget != initialWidget {
		t.Fatal("expected fixture to reuse the same widget instance")
	}
	if fx.renderer != initialRenderer {
		t.Fatal("expected fixture to reuse the same renderer instance")
	}

	assertColorNear(t, "mixed-left", fx.sampleRasterColor(t, 0.125), actionVisuals[0].Color)
	assertColorNear(t, "mixed-mid-left", fx.sampleRasterColor(t, 0.375), actionVisuals[1].Color)
	assertColorNear(t, "mixed-mid-right", fx.sampleRasterColor(t, 0.625), actionVisuals[2].Color)
	assertColorNear(t, "mixed-right", fx.sampleRasterColor(t, 0.875), actionVisuals[3].Color)
	if fx.widget.label != "72o" {
		t.Fatalf("expected label to update on reused widget, got %q", fx.widget.label)
	}
}

func TestRangeCellTransitionRaiseHeavyToMixed(t *testing.T) {
	fx := newRangeCellTransitionFixture(t, testRangeCell("A", "K", true, map[parser.Position][stats.RangeActionBucketCount]int{
		parser.PosBTN: rangeCellActions(0, 3, 7, 0),
	}), parser.PosBTN, "")

	fx.apply(t, testRangeCell("Q", "J", true, map[parser.Position][stats.RangeActionBucketCount]int{
		parser.PosBTN: rangeCellActions(1, 1, 1, 1),
	}), parser.PosBTN, "")

	assertColorNear(t, "check segment", fx.sampleRasterColor(t, 0.125), actionVisuals[0].Color)
	assertColorNear(t, "call segment", fx.sampleRasterColor(t, 0.375), actionVisuals[1].Color)
	assertColorNear(t, "raise segment", fx.sampleRasterColor(t, 0.625), actionVisuals[2].Color)
	assertColorNear(t, "fold segment", fx.sampleRasterColor(t, 0.875), actionVisuals[3].Color)
}

func TestRangeCellTransitionRaiseHeavyToSingleSegment(t *testing.T) {
	fx := newRangeCellTransitionFixture(t, testRangeCell("A", "K", true, map[parser.Position][stats.RangeActionBucketCount]int{
		parser.PosBTN: rangeCellActions(0, 3, 7, 0),
	}), parser.PosBTN, "")

	fx.apply(t, testRangeCell("Q", "Q", false, map[parser.Position][stats.RangeActionBucketCount]int{
		parser.PosBTN: rangeCellActions(0, 0, 5, 0),
	}), parser.PosBTN, "")

	assertColorNear(t, "single-left", fx.sampleRasterColor(t, 0.125), actionVisuals[2].Color)
	assertColorNear(t, "single-middle", fx.sampleRasterColor(t, 0.500), actionVisuals[2].Color)
	assertColorNear(t, "single-right", fx.sampleRasterColor(t, 0.875), actionVisuals[2].Color)
}

func TestRangeCellTransitionPopulatedToEmptyPosition(t *testing.T) {
	cell := testRangeCell("A", "K", true, map[parser.Position][stats.RangeActionBucketCount]int{
		parser.PosBTN: rangeCellActions(0, 2, 4, 0),
	})
	fx := newRangeCellTransitionFixture(t, cell, parser.PosBTN, "")

	fx.apply(t, cell, parser.PosSB, "")

	if len(fx.renderer.barStops) != 0 {
		t.Fatalf("expected no bar stops after empty position switch, got %d", len(fx.renderer.barStops))
	}
	if len(fx.renderer.barColors) != 0 {
		t.Fatalf("expected no bar colors after empty position switch, got %d", len(fx.renderer.barColors))
	}
	if fx.widget.label != "AKs" {
		t.Fatalf("expected combo label to remain on reused widget, got %q", fx.widget.label)
	}
	assertColorNear(t, "empty background", color.NRGBAModel.Convert(fx.renderer.bg.FillColor).(color.NRGBA), color.NRGBA{R: 0x2B, G: 0x2B, B: 0x2B, A: 0xFF})
}

func TestRangeCellTransitionPositionSwitchSelectionReset(t *testing.T) {
	cell := testRangeCell("A", "K", true, map[parser.Position][stats.RangeActionBucketCount]int{
		parser.PosBTN: rangeCellActions(1, 1, 2, 0),
		parser.PosSB:  rangeCellActions(0, 0, 3, 1),
	})
	fx := newRangeCellTransitionFixture(t, cell, parser.PosBTN, cell.ComboKey())

	assertColorNear(t, "selected border", color.NRGBAModel.Convert(fx.renderer.border.StrokeColor).(color.NRGBA), color.NRGBA{R: 0xF5, G: 0xF5, B: 0xF5, A: 0xFF})

	fx.apply(t, cell, parser.PosSB, "")

	if fx.widget.isSelected {
		t.Fatal("expected selection to reset after position switch")
	}
	if fx.widget.isDimmed {
		t.Fatal("expected dimming to reset after position switch")
	}
	assertColorNear(t, "cleared border", color.NRGBAModel.Convert(fx.renderer.border.StrokeColor).(color.NRGBA), color.NRGBA{R: 0x22, G: 0x22, B: 0x22, A: 0xFF})
	assertColorNear(t, "sb raise segment", fx.sampleRasterColor(t, 0.500), actionVisuals[2].Color)
}

type rangeCellTransitionFixture struct {
	win      fyne.Window
	widget   *rangeCellWidget
	renderer *rangeCellRenderer
}

func newRangeCellTransitionFixture(t *testing.T, cell *stats.HandRangeCell, pos parser.Position, selectedCombo string) *rangeCellTransitionFixture {
	t.Helper()

	fyneApp := test.NewTempApp(t)
	fyneApp.Settings().SetTheme(newPokerTheme())
	widget := newRangeCellWidget(cell, positionIndexForTest(t, pos), selectedCombo, nil)
	win := test.NewTempWindow(t, widget)
	fx := &rangeCellTransitionFixture{win: win, widget: widget}
	fx.refresh(t)

	return fx
}

func (f *rangeCellTransitionFixture) apply(t *testing.T, cell *stats.HandRangeCell, pos parser.Position, selectedCombo string) {
	t.Helper()

	fyne.DoAndWait(func() {
		f.widget.applyCellState(cell, positionIndexForTest(t, pos), selectedCombo)
	})
	f.refresh(t)
}

func (f *rangeCellTransitionFixture) refresh(t *testing.T) {
	t.Helper()

	fyne.DoAndWait(func() {
		size := fyne.NewSize(rangeCellW, rangeCellH)
		f.win.Resize(size)
		f.widget.Resize(size)
		f.widget.Refresh()
		f.renderer = test.WidgetRenderer(f.widget).(*rangeCellRenderer)
	})
}

func (f *rangeCellTransitionFixture) sampleColor(t *testing.T, xFraction float32, y float32) color.NRGBA {
	t.Helper()

	return f.sampleColorAt(t, 2+xFraction*(rangeCellW-4), y)
}

func (f *rangeCellTransitionFixture) sampleBarColor(t *testing.T, xFraction float32) color.NRGBA {
	t.Helper()

	return f.sampleColor(t, xFraction, rangeCellH-4)
}

func (f *rangeCellTransitionFixture) sampleRasterColor(t *testing.T, xFraction float32) color.NRGBA {
	t.Helper()

	var sampled color.NRGBA
	fyne.DoAndWait(func() {
		img := f.renderer.barRaster.Generator(int(rangeCellW), int(rangeCellH))
		x := int((2 + xFraction*(rangeCellW-4)))
		y := int(rangeCellH) / 2
		sampled = color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
	})

	return sampled
}

func (f *rangeCellTransitionFixture) sampleColorAt(t *testing.T, x float32, y float32) color.NRGBA {
	t.Helper()

	var sampled color.NRGBA
	fyne.DoAndWait(func() {
		size := fyne.NewSize(rangeCellW, rangeCellH)
		f.win.Resize(size)
		f.widget.Resize(size)
		f.widget.Refresh()
		px, py := f.win.Canvas().PixelCoordinateForPosition(fyne.NewPos(x, y))
		captured := f.win.Canvas().Capture()
		sampled = color.NRGBAModel.Convert(captured.At(px, py)).(color.NRGBA)
	})

	return sampled
}

func assertColorNear(t *testing.T, label string, got color.NRGBA, want color.NRGBA) {
	t.Helper()

	const tolerance = 3
	if !channelNear(got.R, want.R, tolerance) || !channelNear(got.G, want.G, tolerance) || !channelNear(got.B, want.B, tolerance) || !channelNear(got.A, want.A, tolerance) {
		t.Fatalf("%s: got=%+v want=%+v", label, got, want)
	}
}

func channelNear(got uint8, want uint8, tolerance int) bool {
	delta := int(got) - int(want)
	if delta < 0 {
		delta = -delta
	}
	return delta <= tolerance
}

func testRangeCell(rank1 string, rank2 string, suited bool, positions map[parser.Position][stats.RangeActionBucketCount]int) *stats.HandRangeCell {
	cell := &stats.HandRangeCell{
		Rank1:      rank1,
		Rank2:      rank2,
		Suited:     suited,
		IsPair:     rank1 == rank2,
		ByPosition: make(map[parser.Position]*stats.HandRangePositionCell, len(positions)),
	}

	for pos, actions := range positions {
		dealt := 0
		for i := 0; i < int(stats.RangeActionBucketCount); i++ {
			cell.Actions[i] += actions[i]
			dealt += actions[i]
		}
		cell.Dealt += dealt
		cell.ByPosition[pos] = &stats.HandRangePositionCell{Dealt: dealt, Actions: actions}
	}

	return cell
}

func rangeCellActions(check int, call int, raise int, fold int) [stats.RangeActionBucketCount]int {
	var counts [stats.RangeActionBucketCount]int
	counts[stats.RangeActionCheck] = check
	counts[stats.RangeActionCall] = call
	counts[stats.RangeActionBetHalf] = raise
	counts[stats.RangeActionFold] = fold
	return counts
}

func positionIndexForTest(t *testing.T, pos parser.Position) int {
	t.Helper()

	for i, filter := range positionFilters {
		if !filter.IsAll && filter.Pos == pos {
			return i
		}
	}

	t.Fatalf("position %q not found in filters", pos)
	return -1
}
