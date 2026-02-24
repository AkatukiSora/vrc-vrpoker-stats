package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
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
	filter     TabFilterState
	lastStats  *stats.Stats
	localSeat  int
}

func newOverviewTabView(win fyne.Window, visibility *MetricVisibilityState) *overviewTabView {
	return &overviewTabView{
		tabRoot:    newTabRoot(),
		win:        win,
		visibility: visibility,
		filter:     TabFilterState{Mode: FilterModeTrend, NDays: 30, NMonths: 3, NHands: 500},
	}
}

func (v *overviewTabView) Update(s *stats.Stats, localSeat int) {
	v.lastStats = s
	v.localSeat = localSeat
	v.rebuild()
}

func (v *overviewTabView) rebuild() {
	s := v.lastStats
	if s == nil {
		loadingLabel := widget.NewLabel(lang.X("app.status.loading_stats", "Loading stats…"))
		loadingLabel.Alignment = fyne.TextAlignCenter
		replaceViewContentPreservingLayout(v.root, container.NewCenter(loadingLabel))
		return
	}
	// trendN=-1 since filtering is now done at the service layer
	filterBar := buildFilterBar(&v.filter, -1, func() {
		v.rebuild()
	})
	content := NewOverviewTab(s, v.visibility, v.win)
	inner := container.NewBorder(filterBar, nil, nil, nil, content)
	replaceViewContentPreservingLayout(v.root, inner)
}

type positionStatsTabView struct {
	tabRoot
	visibility *MetricVisibilityState
	filter     TabFilterState
	lastStats  *stats.Stats
	localSeat  int
}

func newPositionStatsTabView(visibility *MetricVisibilityState) *positionStatsTabView {
	return &positionStatsTabView{
		tabRoot:    newTabRoot(),
		visibility: visibility,
		filter:     TabFilterState{Mode: FilterModeTrend, NDays: 30, NMonths: 3, NHands: 500},
	}
}

func (v *positionStatsTabView) Update(s *stats.Stats, localSeat int) {
	v.lastStats = s
	v.localSeat = localSeat
	v.rebuild()
}

func (v *positionStatsTabView) rebuild() {
	s := v.lastStats
	if s == nil {
		loadingLabel := widget.NewLabel(lang.X("app.status.loading_stats", "Loading stats…"))
		loadingLabel.Alignment = fyne.TextAlignCenter
		replaceViewContentPreservingLayout(v.root, container.NewCenter(loadingLabel))
		return
	}
	// trendN=-1 since filtering is now done at the service layer
	filterBar := buildFilterBar(&v.filter, -1, func() {
		v.rebuild()
	})
	content := NewPositionStatsTab(s, v.visibility)
	inner := container.NewBorder(filterBar, nil, nil, nil, content)
	replaceViewContentPreservingLayout(v.root, inner)
}

type handRangeTabView struct {
	tabRoot
	win       fyne.Window
	state     *HandRangeViewState
	filter    TabFilterState
	lastStats *stats.Stats
	localSeat int
}

func newHandRangeTabView(win fyne.Window, state *HandRangeViewState) *handRangeTabView {
	return &handRangeTabView{
		tabRoot: newTabRoot(),
		win:     win,
		state:   state,
		filter:  TabFilterState{Mode: FilterModeTrend, NDays: 30, NMonths: 3, NHands: 500},
	}
}

func (v *handRangeTabView) Update(s *stats.Stats, localSeat int) {
	v.lastStats = s
	v.localSeat = localSeat
	v.rebuild()
}

func (v *handRangeTabView) rebuild() {
	s := v.lastStats
	if s == nil {
		loadingLabel := widget.NewLabel(lang.X("app.status.loading_stats", "Loading stats…"))
		loadingLabel.Alignment = fyne.TextAlignCenter
		replaceViewContentPreservingLayout(v.root, container.NewCenter(loadingLabel))
		return
	}
	// trendN=-1 since filtering is now done at the service layer
	filterBar := buildFilterBar(&v.filter, -1, func() {
		v.rebuild()
	})
	content := NewHandRangeTab(s, v.win, v.state)
	inner := container.NewBorder(filterBar, nil, nil, nil, content)
	replaceViewContentPreservingLayout(v.root, inner)
}

type handHistoryTabView struct {
	tabRoot
	state      *HandHistoryViewState
	handFilter HandHistoryFilterState

	// Pagination state
	page          int                       // 0-based current page
	totalCount    int                       // total complete hands matching current filter
	summaries     []persistence.HandSummary // current page only
	filteredCount int                       // count of hands in current page (after DB filter)

	// onLoadPage is called when the view needs a new page of data.
	// The callback runs in a goroutine; result arrives via UpdatePage.
	onLoadPage func(page int)

	// onFetchHand is called when the user selects a hand in the list.
	// The UID is passed; the controller fetches the full hand and calls UpdateDetail.
	onFetchHand func(uid string)

	// detailContent holds the right-side detail panel; kept as a typed ref so
	// UpdateDetail can replace its content from outside NewHandHistoryTabFromSummaries.
	detailContent *fyne.Container
}

func newHandHistoryTabView(state *HandHistoryViewState, onLoadPage func(page int), onFetchHand func(uid string)) *handHistoryTabView {
	return &handHistoryTabView{
		tabRoot:     newTabRoot(),
		state:       state,
		onLoadPage:  onLoadPage,
		onFetchHand: onFetchHand,
	}
}

// UpdatePage replaces the current page of summaries and total count, then rebuilds the view.
// Must be called from the Fyne main thread.
func (v *handHistoryTabView) UpdatePage(summaries []persistence.HandSummary, page, totalCount int) {
	v.summaries = summaries
	v.page = page
	v.totalCount = totalCount
	v.filteredCount = len(summaries)
	v.rebuild()
}

// UpdateDetail replaces the detail panel content with the supplied canvas object.
// Must be called from the Fyne main thread.
func (v *handHistoryTabView) UpdateDetail(obj fyne.CanvasObject) {
	if v.detailContent == nil {
		return
	}
	v.detailContent.Objects = []fyne.CanvasObject{obj}
	v.detailContent.Refresh()
}

const handHistoryPageSize = 200

func (v *handHistoryTabView) rebuild() {
	totalPages := (v.totalCount + handHistoryPageSize - 1) / handHistoryPageSize
	if totalPages < 1 {
		totalPages = 1
	}

	panel := buildHandHistoryFilterPanelSummary(
		&v.handFilter,
		v.page,
		totalPages,
		v.totalCount,
		v.filteredCount,
		func() {
			if v.onLoadPage != nil {
				v.onLoadPage(0)
			}
		},
		func(newPage int) {
			if v.onLoadPage != nil {
				v.onLoadPage(newPage)
			}
		},
	)
	content, detailContent := NewHandHistoryTabFromSummaries(v.summaries, v.state, v.onFetchHand)
	v.detailContent = detailContent
	inner := container.NewBorder(panel, nil, nil, nil, content)
	replaceViewContentPreservingLayout(v.root, inner)
}
