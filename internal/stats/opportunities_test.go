package stats

import (
	"testing"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
)

func TestPreflopActionSequencePrefersNormalizedSequence(t *testing.T) {
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	h := &parser.Hand{Players: map[int]*parser.PlayerHandInfo{
		5: {SeatID: 5, Actions: []parser.PlayerAction{{Timestamp: base, Street: parser.StreetPreFlop, Action: parser.ActionRaise, Sequence: 1}}},
		2: {SeatID: 2, Actions: []parser.PlayerAction{{Timestamp: base, Street: parser.StreetPreFlop, Action: parser.ActionFold, Sequence: 2}}},
	}}
	seq := preflopActionSequence(h)
	if len(seq) != 2 || seq[0].seat != 5 || seq[1].seat != 2 {
		t.Fatalf("sequence=%#v, want seats [5 2]", seq)
	}
}
