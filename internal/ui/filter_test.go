package ui

import (
	"context"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

func TestAggregationFilterMapsSelectionToExpectedBounds(t *testing.T) {
	now := time.Date(2026, 8, 18, 15, 0, 0, 0, time.UTC)
	from := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		state TabFilterState
		check func(*testing.T, time.Time, time.Time, int)
	}{
		{"all", TabFilterState{Mode: FilterModeAll}, func(t *testing.T, gotFrom, gotTo time.Time, lastN int) {
			if !gotFrom.IsZero() || !gotTo.IsZero() || lastN != 0 {
				t.Fatalf("all-time filter = from=%v to=%v lastN=%d", gotFrom, gotTo, lastN)
			}
		}},
		{"trend", TabFilterState{Mode: FilterModeTrend, NHands: 500}, func(t *testing.T, gotFrom, gotTo time.Time, lastN int) {
			if !gotFrom.IsZero() || !gotTo.IsZero() || lastN != 500 {
				t.Fatalf("trend filter = from=%v to=%v lastN=%d", gotFrom, gotTo, lastN)
			}
		}},
		{"days", TabFilterState{Mode: FilterModeLastNDays, NDays: 7}, func(t *testing.T, gotFrom, gotTo time.Time, lastN int) {
			if want := now.AddDate(0, 0, -7); !gotFrom.Equal(want) || !gotTo.IsZero() || lastN != 0 {
				t.Fatalf("days filter = from=%v to=%v lastN=%d", gotFrom, gotTo, lastN)
			}
		}},
		{"months", TabFilterState{Mode: FilterModeLastNMonths, NMonths: 2}, func(t *testing.T, gotFrom, gotTo time.Time, lastN int) {
			if want := now.AddDate(0, -2, 0); !gotFrom.Equal(want) || !gotTo.IsZero() || lastN != 0 {
				t.Fatalf("months filter = from=%v to=%v lastN=%d", gotFrom, gotTo, lastN)
			}
		}},
		{"hands", TabFilterState{Mode: FilterModeLastNHands, NHands: 250}, func(t *testing.T, gotFrom, gotTo time.Time, lastN int) {
			if !gotFrom.IsZero() || !gotTo.IsZero() || lastN != 250 {
				t.Fatalf("hands filter = from=%v to=%v lastN=%d", gotFrom, gotTo, lastN)
			}
		}},
		{"custom inclusive end", TabFilterState{Mode: FilterModeCustom, From: from, To: to}, func(t *testing.T, gotFrom, gotTo time.Time, lastN int) {
			wantTo := to.AddDate(0, 0, 1).Add(-time.Nanosecond)
			if !gotFrom.Equal(from) || !gotTo.Equal(wantTo) || lastN != 0 {
				t.Fatalf("custom filter = from=%v to=%v lastN=%d", gotFrom, gotTo, lastN)
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := aggregationFilter(tt.state, now)
			var gotFrom, gotTo time.Time
			if f.FromTime != nil {
				gotFrom = *f.FromTime
			}
			if f.ToTime != nil {
				gotTo = *f.ToTime
			}
			tt.check(t, gotFrom, gotTo, f.LastN)
		})
	}
}

func TestDateEntryCommitsOnlyCompleteValidInput(t *testing.T) {
	initial := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	var committed []time.Time
	entry := newDateEntry(initial, func(value time.Time) { committed = append(committed, value) })

	entry.SetText("2026-08-") //i18n:ignore test input
	entry.onCommit(entry.Text)
	if len(committed) != 0 || entry.Text != "2026-08-" {
		t.Fatalf("incomplete input committed or was discarded: commits=%v text=%q", committed, entry.Text)
	}

	entry.SetText("20260802") //i18n:ignore test input
	entry.onCommit(entry.Text)
	if len(committed) != 1 || !committed[0].Equal(time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("valid input was not committed: %v", committed)
	}
	if got, want := entry.Text, "2026-08-02"; got != want {
		t.Fatalf("normalized text = %q, want %q", got, want)
	}

	entry.onCommit(entry.Text)
	if len(committed) != 1 {
		t.Fatalf("duplicate commit triggered refresh: %v", committed)
	}
}

func TestAggregationFilterWidgetsCommitOnlyValidInputAndRefreshDisplayedStats(t *testing.T) {
	service := &aggregationInteractionService{fakeHandHistoryAppService: newFakeHandHistoryAppService()}
	app := newAggregationInteractionApp(t, service)
	app.doUpdateStats()
	refreshAggregationApp(app)
	assertLabelText(t, app.mainContent, "100") //i18n:ignore expected all-time hand count

	selectWidget := findSelect(t, app.mainContent)
	selectWidget.SetSelected(lang.X("filter.mode.last_n_hands_select", "Last N Hands"))
	refreshAggregationApp(app)
	assertLabelText(t, app.mainContent, "500") //i18n:ignore expected rendered hand count

	selectWidget = findSelect(t, app.mainContent)
	selectWidget.SetSelected(lang.X("filter.mode.custom", "Custom Range"))
	refreshAggregationApp(app)

	entries := findCommitEntries(app.mainContent)
	if len(entries) != 2 {
		t.Fatalf("custom filter entries = %d, want 2", len(entries))
	}
	fromEntry := entries[0]
	fromEntry.FocusGained()
	test.Type(fromEntry, "2026-08-") //i18n:ignore partial test input
	fromEntry.FocusLost()
	refreshAggregationApp(app)
	if got := service.callCount(); got != 3 { // initial render + two selector changes
		t.Fatalf("incomplete date triggered stats refreshes: calls=%d", got)
	}
	if got := fromEntry.Text; got != "2026-08-" {
		t.Fatalf("incomplete date draft was discarded: %q", got)
	}
	assertLabelText(t, app.mainContent, "100") //i18n:ignore custom range remains unbounded

	test.Type(fromEntry, "18") //i18n:ignore completes partial test input
	fromEntry.OnSubmitted(fromEntry.Text)
	refreshAggregationApp(app)
	if got := service.callCount(); got != 4 {
		t.Fatalf("complete date refresh calls=%d, want 4", got)
	}
	if filter := service.lastFilter(); filter.FromTime == nil || !filter.FromTime.Equal(time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("committed date filter = %+v", filter)
	}
	assertLabelText(t, app.mainContent, "7") //i18n:ignore expected date-filtered hand count

	selectWidget = findSelect(t, app.mainContent)
	selectWidget.SetSelected(lang.X("filter.mode.last_n_hands_select", "Last N Hands"))
	refreshAggregationApp(app)
	entry := findCommitEntries(app.mainContent)[0]
	entry.SetText("")
	entry.FocusGained()
	test.Type(entry, "0") //i18n:ignore invalid test input
	entry.FocusLost()
	refreshAggregationApp(app)
	if got := service.callCount(); got != 5 { // selector change only
		t.Fatalf("invalid number triggered stats refreshes: calls=%d", got)
	}
	assertLabelText(t, app.mainContent, "500") //i18n:ignore last valid hand window remains active

	entry.SetText("")
	entry.FocusGained()
	test.Type(entry, "20") //i18n:ignore valid test input
	entry.OnSubmitted(entry.Text)
	refreshAggregationApp(app)
	if filter := service.lastFilter(); filter.LastN != 20 {
		t.Fatalf("last-N filter = %+v, want LastN=20", filter)
	}
	assertLabelText(t, app.mainContent, "20") //i18n:ignore expected latest-N hand count
}

type aggregationInteractionService struct {
	*fakeHandHistoryAppService
	mu      sync.Mutex
	filters []persistence.HandFilter
}

func (s *aggregationInteractionService) Stats(_ context.Context, filter persistence.HandFilter) (*stats.Stats, int, error) {
	s.mu.Lock()
	s.filters = append(s.filters, filter)
	s.mu.Unlock()
	count := 100
	if filter.LastN > 0 {
		count = filter.LastN
	}
	if filter.FromTime != nil {
		count = 7
	}
	return &stats.Stats{TotalHands: count}, 0, nil
}

func (s *aggregationInteractionService) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.filters)
}

func (s *aggregationInteractionService) lastFilter() persistence.HandFilter {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.filters[len(s.filters)-1]
}

func newAggregationInteractionApp(t *testing.T, service *aggregationInteractionService) *App {
	t.Helper()
	root := container.NewMax()
	win := test.NewWindow(root)
	t.Cleanup(win.Close)
	return &App{
		ctx:         context.Background(),
		service:     service,
		win:         win,
		mainContent: root,
		metricState: NewMetricVisibilityState(),
		rangeState:  &HandRangeViewState{},
		currentTab:  tabOverview,
	}
}

func flushFyne() { fyne.DoAndWait(func() {}) }

func refreshAggregationApp(app *App) {
	fyne.DoAndWait(app.doRefreshCurrentTab)
}

func findSelect(t *testing.T, root fyne.CanvasObject) *widget.Select {
	t.Helper()
	var found *widget.Select
	walkCanvasObjects(root, func(object fyne.CanvasObject) {
		if selectWidget, ok := object.(*widget.Select); ok {
			found = selectWidget
		}
	})
	if found == nil {
		t.Fatal("filter selector not found")
	}
	return found
}

func findCommitEntries(root fyne.CanvasObject) []*commitEntry {
	var entries []*commitEntry
	walkCanvasObjects(root, func(object fyne.CanvasObject) {
		if entry, ok := object.(*commitEntry); ok {
			entries = append(entries, entry)
		}
	})
	return entries
}

func assertLabelText(t *testing.T, root fyne.CanvasObject, want string) {
	t.Helper()
	for range 20 {
		if canvasObjectHasText(root, want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
		flushFyne()
	}
	t.Fatalf("displayed label %q not found", want)
}

func canvasObjectHasText(root fyne.CanvasObject, want string) bool {
	found := false
	walkCanvasObjects(root, func(object fyne.CanvasObject) {
		if label, ok := object.(*widget.Label); ok && label.Text == want {
			found = true
		}
		if richText, ok := object.(*widget.RichText); ok {
			for _, segment := range richText.Segments {
				if text, ok := segment.(*widget.TextSegment); ok && text.Text == want {
					found = true
				}
			}
		}
	})
	return found
}

func walkCanvasObjects(object fyne.CanvasObject, visit func(fyne.CanvasObject)) {
	if object == nil {
		return
	}
	visit(object)
	if parent, ok := object.(*fyne.Container); ok {
		for _, child := range parent.Objects {
			walkCanvasObjects(child, visit)
		}
	}
	if scroll, ok := object.(*container.Scroll); ok {
		walkCanvasObjects(scroll.Content, visit)
	}
}
