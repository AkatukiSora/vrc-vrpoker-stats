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
	applyViewLayoutStateRecursive(obj, state, &scrollIdx, &splitIdx, &listIdx)
}

func applyViewLayoutStateRecursive(obj fyne.CanvasObject, state viewLayoutState, scrollIdx, splitIdx, listIdx *int) {
	if obj == nil {
		return
	}

	switch o := obj.(type) {
	case *container.Scroll:
		if *scrollIdx < len(state.scrollOffsets) {
			o.Offset = state.scrollOffsets[*scrollIdx]
		}
		(*scrollIdx)++
		applyViewLayoutStateRecursive(o.Content, state, scrollIdx, splitIdx, listIdx)
	case *container.Split:
		if *splitIdx < len(state.splitOffsets) {
			o.Offset = state.splitOffsets[*splitIdx]
		}
		(*splitIdx)++
		applyViewLayoutStateRecursive(o.Leading, state, scrollIdx, splitIdx, listIdx)
		applyViewLayoutStateRecursive(o.Trailing, state, scrollIdx, splitIdx, listIdx)
	case *fyne.Container:
		for _, child := range o.Objects {
			applyViewLayoutStateRecursive(child, state, scrollIdx, splitIdx, listIdx)
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
	root       *fyne.Container
	win        fyne.Window
	visibility *MetricVisibilityState
}

func newOverviewTabView(win fyne.Window, visibility *MetricVisibilityState) *overviewTabView {
	return &overviewTabView{root: container.NewMax(), win: win, visibility: visibility}
}

func (v *overviewTabView) Update(s *stats.Stats) {
	replaceViewContentPreservingLayout(v.root, NewOverviewTab(s, v.visibility, v.win))
}

func (v *overviewTabView) CanvasObject() fyne.CanvasObject {
	return v.root
}

type positionStatsTabView struct {
	root       *fyne.Container
	visibility *MetricVisibilityState
}

func newPositionStatsTabView(visibility *MetricVisibilityState) *positionStatsTabView {
	return &positionStatsTabView{root: container.NewMax(), visibility: visibility}
}

func (v *positionStatsTabView) Update(s *stats.Stats) {
	replaceViewContentPreservingLayout(v.root, NewPositionStatsTab(s, v.visibility))
}

func (v *positionStatsTabView) CanvasObject() fyne.CanvasObject {
	return v.root
}

type handRangeTabView struct {
	root  *fyne.Container
	win   fyne.Window
	state *HandRangeViewState
}

func newHandRangeTabView(win fyne.Window, state *HandRangeViewState) *handRangeTabView {
	return &handRangeTabView{root: container.NewMax(), win: win, state: state}
}

func (v *handRangeTabView) Update(s *stats.Stats) {
	replaceViewContentPreservingLayout(v.root, NewHandRangeTab(s, v.win, v.state))
}

func (v *handRangeTabView) CanvasObject() fyne.CanvasObject {
	return v.root
}

type handHistoryTabView struct {
	root  *fyne.Container
	state *HandHistoryViewState
}

func newHandHistoryTabView(state *HandHistoryViewState) *handHistoryTabView {
	return &handHistoryTabView{root: container.NewMax(), state: state}
}

func (v *handHistoryTabView) Update(hands []*parser.Hand, localSeat int) {
	replaceViewContentPreservingLayout(v.root, NewHandHistoryTab(hands, localSeat, v.state))
}

func (v *handHistoryTabView) CanvasObject() fyne.CanvasObject {
	return v.root
}
