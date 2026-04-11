package ui

import (
	"testing"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
)

func TestOriginalRaiserSeat(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)

	t.Run("four bet final raiser highlighted as or", func(t *testing.T) {
		h := &parser.Hand{Players: map[int]*parser.PlayerHandInfo{
			1: {SeatID: 1, Actions: []parser.PlayerAction{{Timestamp: base.Add(1 * time.Second), Street: parser.StreetPreFlop, Action: parser.ActionRaise}}},
			2: {SeatID: 2, Actions: []parser.PlayerAction{{Timestamp: base.Add(2 * time.Second), Street: parser.StreetPreFlop, Action: parser.ActionRaise}}},
			3: {SeatID: 3, Actions: []parser.PlayerAction{{Timestamp: base.Add(3 * time.Second), Street: parser.StreetPreFlop, Action: parser.ActionRaise}}},
			4: {SeatID: 4, Actions: []parser.PlayerAction{{Timestamp: base.Add(4 * time.Second), Street: parser.StreetPreFlop, Action: parser.ActionRaise}}},
		}}

		if got := originalRaiserSeat(h); got != 4 {
			t.Fatalf("originalRaiserSeat() = %d, want 4", got)
		}
	})

	t.Run("limped pot has no or", func(t *testing.T) {
		h := &parser.Hand{Players: map[int]*parser.PlayerHandInfo{
			1: {SeatID: 1, Actions: []parser.PlayerAction{{Timestamp: base.Add(1 * time.Second), Street: parser.StreetPreFlop, Action: parser.ActionCall}}},
			2: {SeatID: 2, Actions: []parser.PlayerAction{{Timestamp: base.Add(2 * time.Second), Street: parser.StreetPreFlop, Action: parser.ActionCheck}}},
		}}

		if got := originalRaiserSeat(h); got != -1 {
			t.Fatalf("originalRaiserSeat() = %d, want -1", got)
		}
	})
}
