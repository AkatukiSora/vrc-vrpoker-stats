package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

type viewLayoutState struct {
	scrollOffsets []fyne.Position
	splitOffsets  []float64
	listOffsets   []float32
	accordionOpen [][]bool
}

// tabRoot is shared by all tab view structs; it holds the Fyne container and
// provides a common CanvasObject() accessor.
type tabRoot struct {
	root *fyne.Container
}

func newTabRoot() tabRoot {
	return tabRoot{root: container.NewMax()}
}

func (t *tabRoot) CanvasObject() fyne.CanvasObject {
	return t.root
}

func captureViewLayoutState(obj fyne.CanvasObject) viewLayoutState {
	state := viewLayoutState{}
	collectViewLayoutState(obj, &state)
	return state
}

func collectViewLayoutState(obj fyne.CanvasObject, state *viewLayoutState) {
	if obj == nil || state == nil {
		return
	}

	switch o := obj.(type) {
	case *container.Scroll:
		state.scrollOffsets = append(state.scrollOffsets, o.Offset)
		collectViewLayoutState(o.Content, state)
	case *container.Split:
		state.splitOffsets = append(state.splitOffsets, o.Offset)
		collectViewLayoutState(o.Leading, state)
		collectViewLayoutState(o.Trailing, state)
	case *widget.Accordion:
		open := make([]bool, len(o.Items))
		for i, item := range o.Items {
			if item == nil {
				continue
			}
			open[i] = item.Open
			collectViewLayoutState(item.Detail, state)
		}
		state.accordionOpen = append(state.accordionOpen, open)
	case *fyne.Container:
		for _, child := range o.Objects {
			collectViewLayoutState(child, state)
		}
	case *widget.List:
		state.listOffsets = append(state.listOffsets, o.GetScrollOffset())
	}
}

func applyViewLayoutState(obj fyne.CanvasObject, state viewLayoutState) {
	scrollIdx := 0
	splitIdx := 0
	listIdx := 0
	accordionIdx := 0
	applyViewLayoutStateRecursive(obj, state, &scrollIdx, &splitIdx, &listIdx, &accordionIdx)
}

func applyViewLayoutStateRecursive(obj fyne.CanvasObject, state viewLayoutState, scrollIdx, splitIdx, listIdx, accordionIdx *int) {
	if obj == nil {
		return
	}

	switch o := obj.(type) {
	case *container.Scroll:
		if *scrollIdx < len(state.scrollOffsets) {
			o.Offset = state.scrollOffsets[*scrollIdx]
		}
		(*scrollIdx)++
		applyViewLayoutStateRecursive(o.Content, state, scrollIdx, splitIdx, listIdx, accordionIdx)
	case *container.Split:
		if *splitIdx < len(state.splitOffsets) {
			o.Offset = state.splitOffsets[*splitIdx]
		}
		(*splitIdx)++
		applyViewLayoutStateRecursive(o.Leading, state, scrollIdx, splitIdx, listIdx, accordionIdx)
		applyViewLayoutStateRecursive(o.Trailing, state, scrollIdx, splitIdx, listIdx, accordionIdx)
	case *widget.Accordion:
		if *accordionIdx < len(state.accordionOpen) {
			open := state.accordionOpen[*accordionIdx]
			for i := 0; i < len(o.Items) && i < len(open); i++ {
				if o.Items[i] == nil {
					continue
				}
				o.Items[i].Open = open[i]
			}
			o.Refresh()
		}
		(*accordionIdx)++
		for _, item := range o.Items {
			if item == nil {
				continue
			}
			applyViewLayoutStateRecursive(item.Detail, state, scrollIdx, splitIdx, listIdx, accordionIdx)
		}
	case *fyne.Container:
		for _, child := range o.Objects {
			applyViewLayoutStateRecursive(child, state, scrollIdx, splitIdx, listIdx, accordionIdx)
		}
	case *widget.List:
		if *listIdx < len(state.listOffsets) {
			o.ScrollToOffset(state.listOffsets[*listIdx])
		}
		(*listIdx)++
	}
}

func replaceViewContentPreservingLayout(root *fyne.Container, next fyne.CanvasObject) {
	if root == nil {
		return
	}

	var state viewLayoutState
	if len(root.Objects) > 0 {
		state = captureViewLayoutState(root.Objects[0])
	}

	root.Objects = []fyne.CanvasObject{next}
	applyViewLayoutState(next, state)
	root.Refresh()
}

type overviewTabView struct {
	tabRoot
	win        fyne.Window
	visibility *MetricVisibilityState
}

func newOverviewTabView(win fyne.Window, visibility *MetricVisibilityState) *overviewTabView {
	return &overviewTabView{tabRoot: newTabRoot(), win: win, visibility: visibility}
}

func (v *overviewTabView) Update(s *stats.Stats) {
	replaceViewContentPreservingLayout(v.root, NewOverviewTab(s, v.visibility, v.win))
}

type positionStatsTabView struct {
	tabRoot
	visibility *MetricVisibilityState
}

func newPositionStatsTabView(visibility *MetricVisibilityState) *positionStatsTabView {
	return &positionStatsTabView{tabRoot: newTabRoot(), visibility: visibility}
}

func (v *positionStatsTabView) Update(s *stats.Stats) {
	replaceViewContentPreservingLayout(v.root, NewPositionStatsTab(s, v.visibility))
}

type handRangeTabView struct {
	tabRoot
	win   fyne.Window
	state *HandRangeViewState
}

func newHandRangeTabView(win fyne.Window, state *HandRangeViewState) *handRangeTabView {
	return &handRangeTabView{tabRoot: newTabRoot(), win: win, state: state}
}

func (v *handRangeTabView) Update(s *stats.Stats) {
	replaceViewContentPreservingLayout(v.root, NewHandRangeTab(s, v.win, v.state))
}

type handHistoryTabView struct {
	tabRoot
	state *HandHistoryViewState
}

func newHandHistoryTabView(state *HandHistoryViewState) *handHistoryTabView {
	return &handHistoryTabView{tabRoot: newTabRoot(), state: state}
}

func (v *handHistoryTabView) Update(hands []*parser.Hand, localSeat int) {
	replaceViewContentPreservingLayout(v.root, NewHandHistoryTab(hands, localSeat, v.state))
}
