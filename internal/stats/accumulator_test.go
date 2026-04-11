package stats

import (
	"testing"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
)

func TestNewMetricAccumulator(t *testing.T) {
	acc := newMetricAccumulator()

	if acc == nil {
		t.Fatal("newMetricAccumulator returned nil")
	}
	if acc.counts == nil {
		t.Error("counts map is nil")
	}
	if acc.opps == nil {
		t.Error("opps map is nil")
	}
	if acc.bbNet != 0.0 {
		t.Errorf("expected bbNet=0, got %f", acc.bbNet)
	}
	if acc.bbHands != 0 {
		t.Errorf("expected bbHands=0, got %d", acc.bbHands)
	}
}

func TestMetricAccumulatorIncOpp(t *testing.T) {
	acc := newMetricAccumulator()

	// Initial state
	if count, ok := acc.opps[MetricVPIP]; ok && count != 0 {
		t.Errorf("expected 0 initial opportunities, got %d", count)
	}

	// Increment once
	acc.incOpp(MetricVPIP)
	if count, ok := acc.opps[MetricVPIP]; !ok || count != 1 {
		t.Errorf("expected 1 opportunity after incOpp, got %d", count)
	}

	// Increment multiple times
	acc.incOpp(MetricVPIP)
	acc.incOpp(MetricVPIP)
	if count := acc.opps[MetricVPIP]; count != 3 {
		t.Errorf("expected 3 opportunities, got %d", count)
	}

	// Different metrics are separate
	acc.incOpp(MetricPFR)
	if count := acc.opps[MetricPFR]; count != 1 {
		t.Errorf("expected 1 opportunity for PFR, got %d", count)
	}
}

func TestMetricAccumulatorIncCount(t *testing.T) {
	acc := newMetricAccumulator()

	// Initial state
	if count, ok := acc.counts[MetricVPIP]; ok && count != 0 {
		t.Errorf("expected 0 initial counts, got %d", count)
	}

	// Increment once
	acc.incCount(MetricVPIP)
	if count, ok := acc.counts[MetricVPIP]; !ok || count != 1 {
		t.Errorf("expected 1 count after incCount, got %d", count)
	}

	// Increment multiple times
	acc.incCount(MetricVPIP)
	acc.incCount(MetricVPIP)
	if count := acc.counts[MetricVPIP]; count != 3 {
		t.Errorf("expected 3 counts, got %d", count)
	}
}

func TestMetricAccumulatorClone(t *testing.T) {
	acc := newMetricAccumulator()
	acc.incOpp(MetricVPIP)
	acc.incCount(MetricVPIP)
	acc.incOpp(MetricPFR)
	acc.bbNet = 1.5
	acc.bbHands = 10
	acc.aggPostflop = 5
	acc.callPostflop = 3
	acc.foldPostflop = 2

	cloned := acc.clone()

	// Verify all fields are copied
	if cloned.opps[MetricVPIP] != 1 {
		t.Error("cloned opportunities not copied")
	}
	if cloned.counts[MetricVPIP] != 1 {
		t.Error("cloned counts not copied")
	}
	if cloned.bbNet != 1.5 {
		t.Error("cloned bbNet not copied")
	}
	if cloned.bbHands != 10 {
		t.Error("cloned bbHands not copied")
	}
	if cloned.aggPostflop != 5 {
		t.Error("cloned aggPostflop not copied")
	}

	// Verify maps are independent
	cloned.incOpp(MetricVPIP)
	if acc.opps[MetricVPIP] != 1 {
		t.Error("original accumulator affected by clone modification")
	}
	if cloned.opps[MetricVPIP] != 2 {
		t.Error("clone not incremented properly")
	}
}

func TestSawFlop(t *testing.T) {
	tests := []struct {
		name     string
		hand     *parser.Hand
		player   *parser.PlayerHandInfo
		expected bool
	}{
		{
			name:     "nil player",
			hand:     &parser.Hand{},
			player:   nil,
			expected: false,
		},
		{
			name:     "nil hand",
			hand:     nil,
			player:   &parser.PlayerHandInfo{},
			expected: false,
		},
		{
			name: "less than 3 community cards",
			hand: &parser.Hand{
				CommunityCards: []parser.Card{
					{Rank: "A", Suit: "h"},
					{Rank: "K", Suit: "d"},
				},
			},
			player:   &parser.PlayerHandInfo{},
			expected: false,
		},
		{
			name: "folded preflop",
			hand: &parser.Hand{
				CommunityCards: []parser.Card{
					{Rank: "A", Suit: "h"},
					{Rank: "K", Suit: "d"},
					{Rank: "Q", Suit: "c"},
				},
			},
			player:   &parser.PlayerHandInfo{FoldedPF: true},
			expected: false,
		},
		{
			name: "saw flop",
			hand: &parser.Hand{
				CommunityCards: []parser.Card{
					{Rank: "A", Suit: "h"},
					{Rank: "K", Suit: "d"},
					{Rank: "Q", Suit: "c"},
				},
			},
			player:   &parser.PlayerHandInfo{FoldedPF: false},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sawFlop(tt.player, tt.hand)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPostFlopActionCounts(t *testing.T) {
	tests := []struct {
		name         string
		player       *parser.PlayerHandInfo
		expectedAgg  int
		expectedCall int
		expectedFold int
	}{
		{
			name:         "nil player",
			player:       nil,
			expectedAgg:  0,
			expectedCall: 0,
			expectedFold: 0,
		},
		{
			name: "no postflop actions",
			player: &parser.PlayerHandInfo{
				Actions: []parser.PlayerAction{
					{Action: parser.ActionCall, Street: parser.StreetPreFlop},
				},
			},
			expectedAgg:  0,
			expectedCall: 0,
			expectedFold: 0,
		},
		{
			name: "multiple postflop actions",
			player: &parser.PlayerHandInfo{
				Actions: []parser.PlayerAction{
					{Action: parser.ActionCall, Street: parser.StreetPreFlop},
					{Action: parser.ActionBet, Street: parser.StreetFlop},
					{Action: parser.ActionCall, Street: parser.StreetFlop},
					{Action: parser.ActionRaise, Street: parser.StreetTurn},
					{Action: parser.ActionFold, Street: parser.StreetRiver},
				},
			},
			expectedAgg:  2, // Bet + Raise
			expectedCall: 1,
			expectedFold: 1,
		},
		{
			name: "all-in counts as aggression",
			player: &parser.PlayerHandInfo{
				Actions: []parser.PlayerAction{
					{Action: parser.ActionAllIn, Street: parser.StreetFlop},
				},
			},
			expectedAgg:  1,
			expectedCall: 0,
			expectedFold: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agg, call, fold := postFlopActionCounts(tt.player)
			if agg != tt.expectedAgg || call != tt.expectedCall || fold != tt.expectedFold {
				t.Errorf("expected (%d, %d, %d), got (%d, %d, %d)",
					tt.expectedAgg, tt.expectedCall, tt.expectedFold,
					agg, call, fold)
			}
		})
	}
}

func TestHasActionOnStreet(t *testing.T) {
	tests := []struct {
		name     string
		player   *parser.PlayerHandInfo
		street   parser.Street
		pred     actionPredicate
		expected bool
	}{
		{
			name:     "nil player",
			player:   nil,
			street:   parser.StreetFlop,
			pred:     anyAction,
			expected: false,
		},
		{
			name: "no actions on street",
			player: &parser.PlayerHandInfo{
				Actions: []parser.PlayerAction{
					{Action: parser.ActionCall, Street: parser.StreetPreFlop},
				},
			},
			street:   parser.StreetFlop,
			pred:     anyAction,
			expected: false,
		},
		{
			name: "action exists on street",
			player: &parser.PlayerHandInfo{
				Actions: []parser.PlayerAction{
					{Action: parser.ActionCall, Street: parser.StreetFlop},
				},
			},
			street:   parser.StreetFlop,
			pred:     anyAction,
			expected: true,
		},
		{
			name: "predicate filters action",
			player: &parser.PlayerHandInfo{
				Actions: []parser.PlayerAction{
					{Action: parser.ActionCall, Street: parser.StreetFlop},
				},
			},
			street:   parser.StreetFlop,
			pred:     isFoldAction,
			expected: false,
		},
		{
			name: "predicate matches action",
			player: &parser.PlayerHandInfo{
				Actions: []parser.PlayerAction{
					{Action: parser.ActionBet, Street: parser.StreetFlop},
					{Action: parser.ActionFold, Street: parser.StreetTurn},
				},
			},
			street:   parser.StreetFlop,
			pred:     isAggressiveAction,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasActionOnStreet(tt.player, tt.street, tt.pred)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestAnyAction(t *testing.T) {
	tests := []struct {
		action   parser.PlayerAction
		expected bool
	}{
		{parser.PlayerAction{Action: parser.ActionUnknown}, false},
		{parser.PlayerAction{Action: parser.ActionCall}, true},
		{parser.PlayerAction{Action: parser.ActionBet}, true},
		{parser.PlayerAction{Action: parser.ActionFold}, true},
		{parser.PlayerAction{Action: parser.ActionCheck}, true},
	}

	for _, tt := range tests {
		result := anyAction(tt.action)
		if result != tt.expected {
			t.Errorf("expected %v for action %v, got %v", tt.expected, tt.action.Action, result)
		}
	}
}

func TestIsFoldAction(t *testing.T) {
	tests := []struct {
		action   parser.PlayerAction
		expected bool
	}{
		{parser.PlayerAction{Action: parser.ActionFold}, true},
		{parser.PlayerAction{Action: parser.ActionCall}, false},
		{parser.PlayerAction{Action: parser.ActionBet}, false},
		{parser.PlayerAction{Action: parser.ActionCheck}, false},
	}

	for _, tt := range tests {
		result := isFoldAction(tt.action)
		if result != tt.expected {
			t.Errorf("expected %v for action %v, got %v", tt.expected, tt.action.Action, result)
		}
	}
}

func TestIsAggressiveAction(t *testing.T) {
	tests := []struct {
		action   parser.PlayerAction
		expected bool
	}{
		{parser.PlayerAction{Action: parser.ActionBet}, true},
		{parser.PlayerAction{Action: parser.ActionRaise}, true},
		{parser.PlayerAction{Action: parser.ActionAllIn}, true},
		{parser.PlayerAction{Action: parser.ActionCall}, false},
		{parser.PlayerAction{Action: parser.ActionFold}, false},
		{parser.PlayerAction{Action: parser.ActionCheck}, false},
	}

	for _, tt := range tests {
		result := isAggressiveAction(tt.action)
		if result != tt.expected {
			t.Errorf("expected %v for action %v, got %v", tt.expected, tt.action.Action, result)
		}
	}
}

func TestHasOpponentAggressionOnStreet(t *testing.T) {
	tests := []struct {
		name      string
		hand      *parser.Hand
		localSeat int
		street    parser.Street
		expected  bool
	}{
		{
			name:      "nil hand",
			hand:      nil,
			localSeat: 0,
			street:    parser.StreetFlop,
			expected:  false,
		},
		{
			name: "no other players",
			hand: &parser.Hand{
				Players: map[int]*parser.PlayerHandInfo{
					0: {
						SeatID: 0,
						Actions: []parser.PlayerAction{
							{Action: parser.ActionBet, Street: parser.StreetFlop},
						},
					},
				},
			},
			localSeat: 0,
			street:    parser.StreetFlop,
			expected:  false,
		},
		{
			name: "opponent aggression on street",
			hand: &parser.Hand{
				Players: map[int]*parser.PlayerHandInfo{
					0: {SeatID: 0},
					1: {
						SeatID: 1,
						Actions: []parser.PlayerAction{
							{Action: parser.ActionBet, Street: parser.StreetFlop},
						},
					},
				},
			},
			localSeat: 0,
			street:    parser.StreetFlop,
			expected:  true,
		},
		{
			name: "opponent passive on street",
			hand: &parser.Hand{
				Players: map[int]*parser.PlayerHandInfo{
					0: {SeatID: 0},
					1: {
						SeatID: 1,
						Actions: []parser.PlayerAction{
							{Action: parser.ActionCall, Street: parser.StreetFlop},
						},
					},
				},
			},
			localSeat: 0,
			street:    parser.StreetFlop,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasOpponentAggressionOnStreet(tt.hand, tt.localSeat, tt.street)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestCBetOpportunityUsesLastPreflopAggressor(t *testing.T) {
	h := &parser.Hand{CommunityCards: []parser.Card{{Rank: "A", Suit: "h"}, {Rank: "K", Suit: "d"}, {Rank: "Q", Suit: "c"}}, Players: map[int]*parser.PlayerHandInfo{
		1: {SeatID: 1, PFR: true, Actions: []parser.PlayerAction{{PlayerID: 1, Street: parser.StreetPreFlop, Action: parser.ActionRaise}}},
		2: {SeatID: 2, Actions: []parser.PlayerAction{{PlayerID: 2, Street: parser.StreetPreFlop, Action: parser.ActionRaise}, {PlayerID: 2, Street: parser.StreetFlop, Action: parser.ActionBet}}},
	}}

	openerAcc := newMetricAccumulator()
	openerAcc.consumeHand(h, h.Players[1], 0)
	if openerAcc.opps[MetricFlopCBet] != 0 {
		t.Fatalf("opener flop c-bet opps = %d, want 0", openerAcc.opps[MetricFlopCBet])
	}

	aggressorAcc := newMetricAccumulator()
	aggressorAcc.consumeHand(h, h.Players[2], 0)
	if aggressorAcc.opps[MetricFlopCBet] != 1 || aggressorAcc.counts[MetricFlopCBet] != 1 {
		t.Fatalf("final aggressor flop c-bet opp/count = %d/%d, want 1/1", aggressorAcc.opps[MetricFlopCBet], aggressorAcc.counts[MetricFlopCBet])
	}
}

func TestCBetMetricsDoNotTreatOpenRaiserAsAggressorAfterThreeBet(t *testing.T) {
	h := &parser.Hand{CommunityCards: []parser.Card{{Rank: "A", Suit: "h"}, {Rank: "K", Suit: "d"}, {Rank: "Q", Suit: "c"}}, Players: map[int]*parser.PlayerHandInfo{
		1: {SeatID: 1, PFR: true, Actions: []parser.PlayerAction{{PlayerID: 1, Street: parser.StreetPreFlop, Action: parser.ActionRaise}, {PlayerID: 1, Street: parser.StreetFlop, Action: parser.ActionFold}}},
		2: {SeatID: 2, ThreeBet: true, Actions: []parser.PlayerAction{{PlayerID: 2, Street: parser.StreetPreFlop, Action: parser.ActionRaise}, {PlayerID: 2, Street: parser.StreetFlop, Action: parser.ActionBet}}},
	}}

	openerAcc := newMetricAccumulator()
	openerAcc.consumeHand(h, h.Players[1], 0)
	if openerAcc.opps[MetricFlopCBet] != 0 || openerAcc.opps[MetricFoldToFlopCBet] != 1 || openerAcc.counts[MetricFoldToFlopCBet] != 1 {
		t.Fatalf("opener metrics unexpected: flopOpp=%d foldOpp=%d foldCount=%d", openerAcc.opps[MetricFlopCBet], openerAcc.opps[MetricFoldToFlopCBet], openerAcc.counts[MetricFoldToFlopCBet])
	}

	aggressorAcc := newMetricAccumulator()
	aggressorAcc.consumeHand(h, h.Players[2], 0)
	if aggressorAcc.opps[MetricFlopCBet] != 1 || aggressorAcc.counts[MetricFlopCBet] != 1 || aggressorAcc.opps[MetricFoldToFlopCBet] != 0 {
		t.Fatalf("3-bettor metrics unexpected: flopOpp=%d flopCount=%d foldOpp=%d", aggressorAcc.opps[MetricFlopCBet], aggressorAcc.counts[MetricFlopCBet], aggressorAcc.opps[MetricFoldToFlopCBet])
	}
}

func TestNoCBetOpportunityWithoutPreflopAggressor(t *testing.T) {
	h := &parser.Hand{CommunityCards: []parser.Card{{Rank: "A", Suit: "h"}, {Rank: "K", Suit: "d"}, {Rank: "Q", Suit: "c"}}, Players: map[int]*parser.PlayerHandInfo{
		0: {SeatID: 0, Actions: []parser.PlayerAction{{PlayerID: 0, Street: parser.StreetPreFlop, Action: parser.ActionCall}, {PlayerID: 0, Street: parser.StreetFlop, Action: parser.ActionCheck}}},
		1: {SeatID: 1, Actions: []parser.PlayerAction{{PlayerID: 1, Street: parser.StreetPreFlop, Action: parser.ActionCheck}, {PlayerID: 1, Street: parser.StreetFlop, Action: parser.ActionCheck}}},
	}}

	for seat, pi := range h.Players {
		acc := newMetricAccumulator()
		acc.consumeHand(h, pi, 0)
		if acc.opps[MetricFlopCBet] != 0 || acc.opps[MetricFoldToFlopCBet] != 0 {
			t.Fatalf("seat %d unexpected opps: flop=%d fold=%d", seat, acc.opps[MetricFlopCBet], acc.opps[MetricFoldToFlopCBet])
		}
	}
}

// Benchmark tests
func BenchmarkMetricAccumulatorIncOpp(b *testing.B) {
	acc := newMetricAccumulator()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		acc.incOpp(MetricVPIP)
	}
}

func BenchmarkMetricAccumulatorIncCount(b *testing.B) {
	acc := newMetricAccumulator()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		acc.incCount(MetricVPIP)
	}
}

func BenchmarkMetricAccumulatorClone(b *testing.B) {
	acc := newMetricAccumulator()
	acc.incOpp(MetricVPIP)
	acc.incCount(MetricVPIP)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		acc.clone()
	}
}

func BenchmarkPostFlopActionCounts(b *testing.B) {
	player := &parser.PlayerHandInfo{
		Actions: []parser.PlayerAction{
			{Action: parser.ActionCall, Street: parser.StreetPreFlop},
			{Action: parser.ActionBet, Street: parser.StreetFlop},
			{Action: parser.ActionCall, Street: parser.StreetFlop},
			{Action: parser.ActionRaise, Street: parser.StreetTurn},
			{Action: parser.ActionFold, Street: parser.StreetRiver},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		postFlopActionCounts(player)
	}
}
