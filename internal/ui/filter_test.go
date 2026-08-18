package ui

import (
	"testing"
	"time"
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
