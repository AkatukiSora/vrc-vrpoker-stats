package parser

import "testing"

func TestPFRRemainsOpenRaiser(t *testing.T) {
	t.Run("single_raise_opener_keeps_pfr", func(t *testing.T) {
		h := newPreflopStatsTestHand(0, 1, 2, 3)
		p := NewParser()
		p.pfActions = []pfAction{{seatID: 0, action: ActionBlindSB, amount: 50}, {seatID: 1, action: ActionBlindBB, amount: 100}, {seatID: 2, action: ActionRaise, amount: 300}, {seatID: 3, action: ActionFold, amount: 0}}
		p.calculatePreflopStats(h)
		assertPreflopSummary(t, h.Players[2], preflopSummaryExpect{vpip: true, pfr: true, threeBet: false, foldTo3Bet: false, description: "open raiser"})
		assertPreflopSummary(t, h.Players[3], preflopSummaryExpect{vpip: false, pfr: false, threeBet: false, foldTo3Bet: false, description: "preflop folder"})
	})

	t.Run("three_bet_does_not_reassign_pfr", func(t *testing.T) {
		h := newPreflopStatsTestHand(0, 1, 2, 3)
		p := NewParser()
		p.pfActions = []pfAction{{seatID: 0, action: ActionBlindSB, amount: 50}, {seatID: 1, action: ActionBlindBB, amount: 100}, {seatID: 2, action: ActionRaise, amount: 300}, {seatID: 3, action: ActionRaise, amount: 900}}
		p.calculatePreflopStats(h)
		assertPreflopSummary(t, h.Players[2], preflopSummaryExpect{vpip: true, pfr: true, threeBet: false, foldTo3Bet: false, description: "opener after facing a 3-bet"})
		assertPreflopSummary(t, h.Players[3], preflopSummaryExpect{vpip: true, pfr: false, threeBet: true, foldTo3Bet: false, description: "3-bettor"})
	})

	t.Run("four_bet_does_not_move_pfr_or_three_bet", func(t *testing.T) {
		h := newPreflopStatsTestHand(0, 1, 2, 3, 4)
		p := NewParser()
		p.pfActions = []pfAction{{seatID: 0, action: ActionBlindSB, amount: 50}, {seatID: 1, action: ActionBlindBB, amount: 100}, {seatID: 2, action: ActionRaise, amount: 300}, {seatID: 3, action: ActionRaise, amount: 900}, {seatID: 4, action: ActionRaise, amount: 2500}}
		p.calculatePreflopStats(h)
		assertPreflopSummary(t, h.Players[2], preflopSummaryExpect{vpip: true, pfr: true, threeBet: false, foldTo3Bet: false, description: "open raiser in 4-bet pot"})
		assertPreflopSummary(t, h.Players[3], preflopSummaryExpect{vpip: true, pfr: false, threeBet: true, foldTo3Bet: false, description: "3-bettor in 4-bet pot"})
		assertPreflopSummary(t, h.Players[4], preflopSummaryExpect{vpip: true, pfr: false, threeBet: false, foldTo3Bet: false, description: "final aggressor in 4-bet pot"})
	})

	t.Run("opener_can_still_fold_to_three_bet", func(t *testing.T) {
		h := newPreflopStatsTestHand(0, 1, 2, 3)
		p := NewParser()
		p.pfActions = []pfAction{{seatID: 0, action: ActionBlindSB, amount: 50}, {seatID: 1, action: ActionBlindBB, amount: 100}, {seatID: 2, action: ActionRaise, amount: 300}, {seatID: 3, action: ActionRaise, amount: 900}, {seatID: 2, action: ActionFold, amount: 0}}
		p.calculatePreflopStats(h)
		assertPreflopSummary(t, h.Players[2], preflopSummaryExpect{vpip: true, pfr: true, threeBet: false, foldTo3Bet: true, description: "opener who folds to a 3-bet"})
		assertPreflopSummary(t, h.Players[3], preflopSummaryExpect{vpip: true, pfr: false, threeBet: true, foldTo3Bet: false, description: "3-bettor facing opener fold"})
	})
}

type preflopSummaryExpect struct {
	vpip        bool
	pfr         bool
	threeBet    bool
	foldTo3Bet  bool
	description string
}

func newPreflopStatsTestHand(sbSeat, bbSeat int, seats ...int) *Hand {
	players := make(map[int]*PlayerHandInfo, len(seats))
	for _, seat := range seats {
		players[seat] = &PlayerHandInfo{SeatID: seat}
	}
	return &Hand{SBSeat: sbSeat, BBSeat: bbSeat, Players: players}
}

func assertPreflopSummary(t *testing.T, pi *PlayerHandInfo, want preflopSummaryExpect) {
	t.Helper()
	if pi == nil {
		t.Fatalf("missing player info for %s", want.description)
	}
	if pi.VPIP != want.vpip {
		t.Fatalf("%s: VPIP=%v, want %v", want.description, pi.VPIP, want.vpip)
	}
	if pi.PFR != want.pfr {
		t.Fatalf("%s: PFR=%v, want %v", want.description, pi.PFR, want.pfr)
	}
	if pi.ThreeBet != want.threeBet {
		t.Fatalf("%s: ThreeBet=%v, want %v", want.description, pi.ThreeBet, want.threeBet)
	}
	if pi.FoldTo3Bet != want.foldTo3Bet {
		t.Fatalf("%s: FoldTo3Bet=%v, want %v", want.description, pi.FoldTo3Bet, want.foldTo3Bet)
	}
}
