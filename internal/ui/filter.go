package ui

import (
	"log/slog"
	"sort"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

// TabFilterMode represents the type of filter applied to a tab's hand data.
type TabFilterMode int

const (
	FilterModeAll TabFilterMode = iota
	FilterModeTrend
	FilterModeLastNDays
	FilterModeLastNMonths
	FilterModeLastNHands
	FilterModeCustom
)

// TabFilterState holds the current filter configuration for a tab.
type TabFilterState struct {
	Mode    TabFilterMode
	NDays   int       // for FilterModeLastNDays
	NMonths int       // for FilterModeLastNMonths
	NHands  int       // for FilterModeLastNHands
	From    time.Time // for Custom
	To      time.Time // for Custom
}

// commitEntry is a widget.Entry that fires onCommit only when the user
// confirms the value — either by pressing Enter (OnSubmitted) or by moving
// focus away (FocusLost). It does NOT call onChange on every keystroke.
type commitEntry struct {
	widget.Entry
	onCommit func(string)
}

func newCommitEntry() *commitEntry {
	e := &commitEntry{}
	e.ExtendBaseWidget(e)
	return e
}

// FocusLost fires onCommit when keyboard focus leaves the entry.
func (e *commitEntry) FocusLost() {
	e.Entry.FocusLost()
	if e.onCommit != nil {
		e.onCommit(e.Text)
	}
}

// filterModeLabel returns a short display label for the current filter mode.
func filterModeLabel(mode TabFilterMode, n int, trendN int) string {
	switch mode {
	case FilterModeAll:
		return lang.X("filter.mode.all", "All Time")
	case FilterModeTrend:
		if trendN > 0 {
			return lang.X("filter.trend.window", "Trend: {{.N}} hands", map[string]any{"N": trendN})
		}
		return lang.X("filter.mode.trend", "Trend")
	case FilterModeLastNDays:
		return lang.X("filter.mode.last_n_days", "Last {{.N}} Days", map[string]any{"N": n})
	case FilterModeLastNMonths:
		return lang.X("filter.mode.last_n_months", "Last {{.N}} Months", map[string]any{"N": n})
	case FilterModeLastNHands:
		return lang.X("filter.mode.last_n_hands", "Last {{.N}} Hands", map[string]any{"N": n})
	case FilterModeCustom:
		return lang.X("filter.mode.custom", "Custom Range")
	default:
		return lang.X("filter.mode.all", "All Time")
	}
}

// applyTabFilter filters hands based on the filter state.
// For FilterModeTrend, the caller is expected to pass already-sliced hands; this function returns all hands.
func applyTabFilter(hands []*parser.Hand, f TabFilterState) []*parser.Hand {
	switch f.Mode {
	case FilterModeAll, FilterModeTrend:
		return hands
	case FilterModeLastNDays:
		cutoff := time.Now().AddDate(0, 0, -f.NDays)
		out := hands[:0:0]
		for _, h := range hands {
			if !h.StartTime.IsZero() && h.StartTime.After(cutoff) {
				out = append(out, h)
			}
		}
		return out
	case FilterModeLastNMonths:
		cutoff := time.Now().AddDate(0, -f.NMonths, 0)
		out := hands[:0:0]
		for _, h := range hands {
			if !h.StartTime.IsZero() && h.StartTime.After(cutoff) {
				out = append(out, h)
			}
		}
		return out
	case FilterModeLastNHands:
		if f.NHands <= 0 {
			return nil
		}
		if f.NHands >= len(hands) {
			return hands
		}
		return hands[len(hands)-f.NHands:]
	case FilterModeCustom:
		out := hands[:0:0]
		for _, h := range hands {
			t := h.StartTime
			if t.IsZero() {
				continue
			}
			if !f.From.IsZero() && t.Before(f.From) {
				continue
			}
			if !f.To.IsZero() && t.After(f.To.Truncate(24*time.Hour).Add(24*time.Hour-time.Second)) {
				continue
			}
			out = append(out, h)
		}
		return out
	default:
		return hands
	}
}

// trendMetricIDs lists the 23 key metrics used by trendWindowSize (excluding MetricWonWithoutSD and MetricBBPer100).
var trendMetricIDs = []stats.MetricID{
	stats.MetricVPIP,
	stats.MetricPFR,
	stats.MetricGap,
	stats.MetricRFI,
	stats.MetricThreeBet,
	stats.MetricFourBet,
	stats.MetricSqueeze,
	stats.MetricFoldToThreeBet,
	stats.MetricSteal,
	stats.MetricFoldToSteal,
	stats.MetricFoldBBToSteal,
	stats.MetricFoldSBToSteal,
	stats.MetricThreeBetVsSteal,
	stats.MetricFlopCBet,
	stats.MetricTurnCBet,
	stats.MetricFoldToFlopCBet,
	stats.MetricFoldToTurnCBet,
	stats.MetricWTSD,
	stats.MetricWSD,
	stats.MetricWWSF,
	stats.MetricAFq,
	stats.MetricAF,
	stats.MetricDelayedCBet,
}

// trendWindowSize binary-searches for the minimum tail of hands such that all 23 key
// metrics have Opportunity >= their Good threshold.
func trendWindowSize(hands []*parser.Hand, localSeat int) int {
	if len(hands) == 0 {
		return 0
	}

	slog.Debug("trendWindowSize computing", "inputHands", len(hands))

	calc := stats.NewCalculator()

	// thresholds is pre-fetched once.
	type metricThresholdPair struct {
		id        stats.MetricID
		threshold int
	}
	pairs := make([]metricThresholdPair, len(trendMetricIDs))
	for i, id := range trendMetricIDs {
		pairs[i] = metricThresholdPair{
			id:        id,
			threshold: metricCatalogEntryForID(string(id)).Threshold.Good,
		}
	}

	allPass := func(n int) bool {
		tail := hands[len(hands)-n:]
		s := calc.Calculate(tail, localSeat)
		for _, p := range pairs {
			mv, ok := s.Metrics[p.id]
			if !ok || mv.Opportunity < p.threshold {
				return false
			}
		}
		return true
	}

	// Binary search: find minimum N in [1, len(hands)] where allPass is true.
	// Property: once allPass is true, it remains true for larger N (monotone).
	// sort.Search searches [0, hi) — n is the smallest i such that allPass(i+1).
	hi := len(hands)
	found := -1

	n := sort.Search(hi, func(i int) bool {
		candidate := i + 1 // i is 0-based; candidate in [1, len(hands)]
		return allPass(candidate)
	})
	// n is in [0, hi): the smallest index satisfying allPass(n+1).
	if n < hi {
		found = n + 1
	}

	if found < 0 {
		slog.Debug("trendWindowSize result", "n", len(hands), "reason", "no passing window found")
		return len(hands)
	}
	slog.Debug("trendWindowSize result", "n", found)
	return found
}

// filterHands applies the filter state to hands and returns the filtered slice plus trendN.
// trendN is -1 for non-Trend modes.
// cachedTrendN and cachedLen are optional pointers used to memoize the trend window computation.
func filterHands(hands []*parser.Hand, localSeat int, f *TabFilterState, cachedTrendN *int, cachedLen *int) (filtered []*parser.Hand, trendN int) {
	if hands == nil {
		return nil, -1
	}
	if f == nil {
		return hands, -1
	}

	slog.Debug("filterHands", "mode", f.Mode, "inputHands", len(hands))

	if f.Mode == FilterModeTrend {
		var n int
		if cachedLen != nil && cachedTrendN != nil && *cachedTrendN >= 0 && *cachedLen == len(hands) {
			n = *cachedTrendN
		} else {
			n = trendWindowSize(hands, localSeat)
			if cachedTrendN != nil {
				*cachedTrendN = n
			}
			if cachedLen != nil {
				*cachedLen = len(hands)
			}
		}
		take := n
		if take < 0 {
			take = 0
		}
		if take > len(hands) {
			take = len(hands)
		}
		result := hands[len(hands)-take:]
		slog.Debug("filterHands result", "mode", f.Mode, "resultHands", len(result), "trendN", n)
		return result, n
	}

	result := applyTabFilter(hands, *f)
	slog.Debug("filterHands result", "mode", f.Mode, "resultHands", len(result))
	return result, -1
}

// buildFilterBar constructs a horizontal filter bar widget.
func buildFilterBar(state *TabFilterState, trendN int, onChange func()) fyne.CanvasObject {
	if state == nil {
		return container.NewHBox()
	}

	// Option list for the select widget — labels do NOT embed N so the
	// select box stays stable when the user types a new value.
	options := []string{
		lang.X("filter.mode.all", "All Time"),
		lang.X("filter.mode.trend", "Trend"),
		lang.X("filter.mode.last_n_days_select", "Last N Days"),
		lang.X("filter.mode.last_n_months_select", "Last N Months"),
		lang.X("filter.mode.last_n_hands_select", "Last N Hands"),
		lang.X("filter.mode.custom", "Custom Range"),
	}

	modeSelect := widget.NewSelect(options, nil)
	modeSelect.Selected = options[int(state.Mode)]

	// Build conditional widgets based on mode.
	buildConditional := func() []fyne.CanvasObject {
		switch state.Mode {
		case FilterModeLastNDays:
			label := widget.NewLabel(lang.X("filter.last_n.days_label", "Days:"))
			entry := newCommitEntry()
			entry.SetText(strconv.Itoa(state.NDays))
			commit := func(s string) {
				v, err := strconv.Atoi(s)
				if err != nil || v <= 0 {
					entry.SetText(strconv.Itoa(state.NDays)) // reset to last valid
					return
				}
				state.NDays = v
				entry.SetText(strconv.Itoa(v)) // normalize (e.g. strip leading zeros)
				onChange()
			}
			entry.onCommit = commit
			entry.OnSubmitted = commit
			return []fyne.CanvasObject{label, container.NewGridWrap(fyne.NewSize(80, entry.MinSize().Height), entry)}
		case FilterModeLastNMonths:
			label := widget.NewLabel(lang.X("filter.last_n.months_label", "Months:"))
			entry := newCommitEntry()
			entry.SetText(strconv.Itoa(state.NMonths))
			commit := func(s string) {
				v, err := strconv.Atoi(s)
				if err != nil || v <= 0 {
					entry.SetText(strconv.Itoa(state.NMonths))
					return
				}
				state.NMonths = v
				entry.SetText(strconv.Itoa(v))
				onChange()
			}
			entry.onCommit = commit
			entry.OnSubmitted = commit
			return []fyne.CanvasObject{label, container.NewGridWrap(fyne.NewSize(80, entry.MinSize().Height), entry)}
		case FilterModeLastNHands:
			label := widget.NewLabel(lang.X("filter.last_n.hands_label", "Hands:"))
			entry := newCommitEntry()
			entry.SetText(strconv.Itoa(state.NHands))
			commit := func(s string) {
				v, err := strconv.Atoi(s)
				if err != nil || v <= 0 {
					entry.SetText(strconv.Itoa(state.NHands))
					return
				}
				state.NHands = v
				entry.SetText(strconv.Itoa(v))
				onChange()
			}
			entry.onCommit = commit
			entry.OnSubmitted = commit
			return []fyne.CanvasObject{label, container.NewGridWrap(fyne.NewSize(80, entry.MinSize().Height), entry)}
		case FilterModeCustom:
			fromLabel := widget.NewLabel(lang.X("filter.custom.from", "From:"))
			fromEntry := newDateEntry(state.From, func(t time.Time) {
				state.From = t
				onChange()
			})
			toLabel := widget.NewLabel(lang.X("filter.custom.to", "To:"))
			toEntry := newDateEntry(state.To, func(t time.Time) {
				state.To = t
				onChange()
			})
			entryH := fromEntry.MinSize().Height
			return []fyne.CanvasObject{
				fromLabel,
				container.NewGridWrap(fyne.NewSize(130, entryH), fromEntry),
				toLabel,
				container.NewGridWrap(fyne.NewSize(130, entryH), toEntry),
			}
		case FilterModeTrend:
			if trendN > 0 {
				trendLabel := widget.NewLabel(lang.X("filter.trend.window", "Trend: {{.N}} hands", map[string]any{"N": trendN}))
				return []fyne.CanvasObject{trendLabel}
			}
			return nil
		default:
			return nil
		}
	}

	conditionalWidgets := buildConditional()

	periodLabel := widget.NewLabel(lang.X("filter.mode.label", "Period"))

	// Assemble HBox items.
	items := make([]fyne.CanvasObject, 0, 3+len(conditionalWidgets))
	items = append(items, periodLabel, modeSelect)
	items = append(items, conditionalWidgets...)

	hbox := container.NewHBox(items...)

	// Wire up mode select after hbox is created so we can refresh it.
	modeSelect.OnChanged = func(selected string) {
		for i, opt := range options {
			if opt == selected {
				state.Mode = TabFilterMode(i)
				break
			}
		}
		onChange()
	}

	return hbox
}

// newDateEntry creates a commitEntry for YYYY-MM-DD date input.
// No modification is made during typing; onChange fires only on Enter or focus-lost.
func newDateEntry(initial time.Time, onChange func(time.Time)) *commitEntry {
	entry := newCommitEntry()
	entry.SetPlaceHolder(lang.X("filter.custom.date_hint", "YYYY-MM-DD"))

	lastValid := initial
	if !initial.IsZero() {
		entry.SetText(initial.Format("2006-01-02"))
	}

	committing := false // re-entry guard: SetText inside commit must not re-trigger commit

	commit := func(s string) {
		if committing {
			return
		}
		for _, layout := range []string{"2006-01-02", "20060102"} {
			if t, err := time.Parse(layout, s); err == nil {
				lastValid = t
				committing = true
				entry.SetText(t.Format("2006-01-02")) // normalize
				committing = false
				onChange(t)
				return
			}
		}
		// Invalid: reset to last valid
		committing = true
		if !lastValid.IsZero() {
			entry.SetText(lastValid.Format("2006-01-02"))
		} else {
			entry.SetText("")
		}
		committing = false
	}
	entry.onCommit = commit
	entry.OnSubmitted = commit

	return entry
}
