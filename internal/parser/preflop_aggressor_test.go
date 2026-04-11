package parser

import (
	"testing"
	"time"
)

func TestLastPreflopAggressorSeat(t *testing.T) {
	base := time.Date(2026, time.February, 21, 0, 0, 0, 0, time.UTC)
	ts := func(second int) time.Time {
		return base.Add(time.Duration(second) * time.Second)
	}

	tests := []struct {
		name string
		hand *Hand
		want int
	}{
		{
			name: "no preflop raise returns none",
			hand: testHandWithActions(map[int][]PlayerAction{
				0: {
					{Timestamp: ts(1), Street: StreetPreFlop, Action: ActionBlindSB, Amount: 10},
					{Timestamp: ts(4), Street: StreetPreFlop, Action: ActionCall, Amount: 20},
				},
				1: {
					{Timestamp: ts(2), Street: StreetPreFlop, Action: ActionBlindBB, Amount: 20},
					{Timestamp: ts(5), Street: StreetPreFlop, Action: ActionCheck, Amount: 20},
				},
				2: {{Timestamp: ts(3), Street: StreetPreFlop, Action: ActionCall, Amount: 20}},
				3: {{Timestamp: ts(6), Street: StreetPreFlop, Action: ActionFold, Amount: 0}},
			}),
			want: -1,
		},
		{
			name: "single raise returns raiser seat",
			hand: testHandWithActions(map[int][]PlayerAction{
				0: {{Timestamp: ts(1), Street: StreetPreFlop, Action: ActionBlindSB, Amount: 10}},
				1: {{Timestamp: ts(2), Street: StreetPreFlop, Action: ActionBlindBB, Amount: 20}},
				2: {{Timestamp: ts(3), Street: StreetPreFlop, Action: ActionRaise, Amount: 60}},
				3: {{Timestamp: ts(4), Street: StreetPreFlop, Action: ActionFold, Amount: 0}},
			}),
			want: 2,
		},
		{
			name: "three bet returns final aggressor seat",
			hand: testHandWithActions(map[int][]PlayerAction{
				0: {{Timestamp: ts(1), Street: StreetPreFlop, Action: ActionBlindSB, Amount: 10}},
				1: {{Timestamp: ts(2), Street: StreetPreFlop, Action: ActionBlindBB, Amount: 20}},
				2: {{Timestamp: ts(3), Street: StreetPreFlop, Action: ActionRaise, Amount: 60}},
				3: {{Timestamp: ts(4), Street: StreetPreFlop, Action: ActionRaise, Amount: 180}},
			}),
			want: 3,
		},
		{
			name: "four bet returns final aggressor seat",
			hand: testHandWithActions(map[int][]PlayerAction{
				0: {{Timestamp: ts(1), Street: StreetPreFlop, Action: ActionBlindSB, Amount: 10}},
				1: {{Timestamp: ts(2), Street: StreetPreFlop, Action: ActionBlindBB, Amount: 20}},
				2: {{Timestamp: ts(3), Street: StreetPreFlop, Action: ActionRaise, Amount: 60}},
				3: {{Timestamp: ts(4), Street: StreetPreFlop, Action: ActionRaise, Amount: 180}},
				4: {{Timestamp: ts(5), Street: StreetPreFlop, Action: ActionRaise, Amount: 420}},
			}),
			want: 4,
		},
		{
			name: "preflop all in can be final aggressor",
			hand: testHandWithActions(map[int][]PlayerAction{
				0: {{Timestamp: ts(1), Street: StreetPreFlop, Action: ActionBlindSB, Amount: 10}},
				1: {{Timestamp: ts(2), Street: StreetPreFlop, Action: ActionBlindBB, Amount: 20}},
				2: {{Timestamp: ts(3), Street: StreetPreFlop, Action: ActionRaise, Amount: 60}},
				3: {{Timestamp: ts(4), Street: StreetPreFlop, Action: ActionAllIn, Amount: 500}},
				4: {{Timestamp: ts(5), Street: StreetPreFlop, Action: ActionCall, Amount: 500}},
			}),
			want: 3,
		},
		{
			name: "same timestamp breaks ties by seat",
			hand: testHandWithActions(map[int][]PlayerAction{
				0: {{Timestamp: ts(1), Street: StreetPreFlop, Action: ActionBlindSB, Amount: 10}},
				1: {{Timestamp: ts(2), Street: StreetPreFlop, Action: ActionBlindBB, Amount: 20}},
				4: {{Timestamp: ts(3), Street: StreetPreFlop, Action: ActionRaise, Amount: 60}},
				2: {{Timestamp: ts(4), Street: StreetPreFlop, Action: ActionRaise, Amount: 180}},
				5: {{Timestamp: ts(4), Street: StreetPreFlop, Action: ActionRaise, Amount: 420}},
			}),
			want: 5,
		},
		{
			name: "same timestamp uses amount before seat",
			hand: testHandWithActions(map[int][]PlayerAction{
				1: {{Timestamp: ts(1), Street: StreetPreFlop, Action: ActionBlindSB, Amount: 10}},
				2: {
					{Timestamp: ts(2), Street: StreetPreFlop, Action: ActionBlindBB, Amount: 20},
					{Timestamp: ts(3), Street: StreetPreFlop, Action: ActionRaise, Amount: 180},
				},
				5: {{Timestamp: ts(3), Street: StreetPreFlop, Action: ActionRaise, Amount: 60}},
			}),
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LastPreflopAggressorSeat(tt.hand)
			if got != tt.want {
				t.Fatalf("LastPreflopAggressorSeat() = %d, want %d", got, tt.want)
			}
		})
	}
}

func testHandWithActions(actionsBySeat map[int][]PlayerAction) *Hand {
	players := make(map[int]*PlayerHandInfo, len(actionsBySeat))
	for seat, actions := range actionsBySeat {
		copied := append([]PlayerAction(nil), actions...)
		players[seat] = &PlayerHandInfo{SeatID: seat, Actions: copied}
	}
	return &Hand{Players: players}
}
