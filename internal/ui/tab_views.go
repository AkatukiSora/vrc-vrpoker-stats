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

func applyFilterLayout(root *fyne.Container, filter *TabFilterState, rebuild func(), buildContent func() fyne.CanvasObject) {
	if root == nil || filter == nil || rebuild == nil || buildContent == nil {
		return
	}
	// trendN=-1 since filtering is now done at the service layer
	filterBar := buildFilterBar(filter, -1, func() {
		rebuild()
	})
	inner := container.NewBorder(filterBar, nil, nil, nil, buildContent())
	replaceViewContentPreservingLayout(root, inner)
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
	applyFilterLayout(v.root, &v.filter, v.rebuild, func() fyne.CanvasObject {
		return NewOverviewTab(s, v.visibility, v.win)
	})
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
	applyFilterLayout(v.root, &v.filter, v.rebuild, func() fyne.CanvasObject {
		return NewPositionStatsTab(s, v.visibility)
	})
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
	applyFilterLayout(v.root, &v.filter, v.rebuild, func() fyne.CanvasObject {
		return NewHandRangeTab(s, v.win, v.state)
	})
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
	// UpdateDetail can replace its content while reusing the same container.
	detailContent  *fyne.Container
	list           *widget.List
	split          *container.Split
	suppressSelect bool
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
	v.ensureInitialized()
	if v.list != nil {
		v.list.Refresh()
	}
	v.restoreSelection()
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
	v.ensureInitialized()
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
	var content fyne.CanvasObject
	if len(v.summaries) == 0 {
		content = newCenteredEmptyState(lang.X("hand_history.no_hands", "No hands recorded yet.\nStart playing in the VR Poker world!"))
		v.showEmptyDetail()
	} else {
		content = v.split
	}

	title := widget.NewLabelWithStyle(lang.X("hand_history.title", "Recent Hands"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	subtitle := widget.NewLabel(lang.X("hand_history.subtitle", "Select a hand to inspect street-by-street action flow."))
	subtitle.Wrapping = fyne.TextWrapWord
	content = container.NewBorder(container.NewVBox(title, subtitle, newSectionDivider()), nil, nil, nil, content)
	inner := container.NewBorder(panel, nil, nil, nil, content)
	replaceViewContentPreservingLayout(v.root, inner)
}

func (v *handHistoryTabView) ensureInitialized() {
	if v.state == nil {
		v.state = &HandHistoryViewState{}
	}
	if v.detailContent == nil {
		v.detailContent = container.NewStack()
		v.detailContent.Objects = []fyne.CanvasObject{buildDetailPanelEmpty(lang.X("hand_history.select_hand", "Select a hand to see details."))}
	}
	if v.list != nil {
		return
	}
	rowRefs := make(map[fyne.CanvasObject]*handListEntryRefs)
	v.list = widget.NewList(
		func() int { return len(v.summaries) },
		func() fyne.CanvasObject {
			row, refs := newHandListEntryRow()
			rowRefs[row] = refs
			return row
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(v.summaries) {
				return
			}
			s := v.summaries[id]
			line1, line2 := handSummaryEntryFieldsFromSummary(s)
			if refs, ok := rowRefs[obj]; ok {
				refs.setFields(line1, line2)
			}
		},
	)

	v.list.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(v.summaries) {
			return
		}
		s := v.summaries[id]
		v.state.SelectedHandKey = "uid:" + s.HandUID
		if v.suppressSelect {
			v.suppressSelect = false
			return
		}

		loadingLabel := widget.NewLabel(lang.X("hand_history.detail.loading", "Loading hand details…"))
		loadingLabel.Alignment = fyne.TextAlignCenter
		v.detailContent.Objects = []fyne.CanvasObject{container.NewCenter(loadingLabel)}
		v.detailContent.Refresh()

		if v.onFetchHand != nil {
			go v.onFetchHand(s.HandUID)
		}
	}

	v.split = container.NewHSplit(v.list, v.detailContent)
	v.split.Offset = 0.48
}

func (v *handHistoryTabView) restoreSelection() {
	if v.list == nil {
		return
	}
	if v.state == nil || v.state.SelectedHandKey == "" {
		v.list.UnselectAll()
		v.showEmptyDetail()
		return
	}
	for i, s := range v.summaries {
		if "uid:"+s.HandUID == v.state.SelectedHandKey {
			v.suppressSelect = true
			v.list.Select(i)
			return
		}
	}
	v.list.UnselectAll()
	v.state.SelectedHandKey = ""
	v.showEmptyDetail()
}

func (v *handHistoryTabView) showEmptyDetail() {
	if v.detailContent == nil {
		return
	}
	v.detailContent.Objects = []fyne.CanvasObject{buildDetailPanelEmpty(lang.X("hand_history.select_hand", "Select a hand to see details."))}
	v.detailContent.Refresh()
}
