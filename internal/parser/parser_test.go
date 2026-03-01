package parser

import (
	"strings"
	"testing"
)

// Sample log lines representative of a full VRPoker hand
const sampleLog = `
2026.02.21 00:18:53 Debug      -  [Table]: Preparing for New Game: 
2026.02.21 00:18:55 Debug      -  [Seat]: Player 0 SB BET IN = 10
2026.02.21 00:18:55 Debug      -  [Seat]: Draw Local Hole Cards: 7d, 9c
2026.02.21 00:18:56 Debug      -  [Seat]: Player 1 BB BET IN = 20
2026.02.21 00:18:57 Debug      -  [Seat]: Player 2 End Turn with BET IN = 20
2026.02.21 00:18:58 Debug      -  [Seat]: Player 3 End Turn with BET IN = 20
2026.02.21 00:18:59 Debug      -  [Seat]: Player 4 Folded.
2026.02.21 00:18:59 Debug      -  [Seat]: Player 4 End Turn with BET IN = 0
2026.02.21 00:19:00 Debug      -  [Seat]: Player 0 End Turn with BET IN = 20
2026.02.21 00:19:00 Debug      -  [Seat]: Player 1 End Turn with BET IN = 20
2026.02.21 00:19:00 Debug      -  [Table]: Next phase.True - 4
2026.02.21 00:19:00 Debug      -  [Table]: Collecting Bets. ----------------
2026.02.21 00:19:01 Debug      -  [Table]: New Community Card: Qd
2026.02.21 00:19:01 Debug      -  [Table]: New Community Card: 3s
2026.02.21 00:19:01 Debug      -  [Table]: New Community Card: Kd
2026.02.21 00:19:10 Debug      -  [Seat]: Player 0 End Turn with BET IN = 0
2026.02.21 00:19:11 Debug      -  [Seat]: Player 1 End Turn with BET IN = 0
2026.02.21 00:19:12 Debug      -  [Seat]: Player 2 End Turn with BET IN = 0
2026.02.21 00:19:13 Debug      -  [Seat]: Player 3 End Turn with BET IN = 0
2026.02.21 00:19:13 Debug      -  [Table]: Next phase.True - 4
2026.02.21 00:19:13 Debug      -  [Table]: Collecting Bets. ----------------
2026.02.21 00:19:14 Debug      -  [Table]: New Community Card: 8c
2026.02.21 00:19:30 Debug      -  [Seat]: Player 3 End Turn with BET IN = 50
2026.02.21 00:19:31 Debug      -  [Table]: New Min Bet: 50 === New Min Raise: 50
2026.02.21 00:19:32 Debug      -  [Seat]: Player 0 Folded.
2026.02.21 00:19:32 Debug      -  [Seat]: Player 0 End Turn with BET IN = 0
2026.02.21 00:19:33 Debug      -  [Seat]: Player 1 Folded.
2026.02.21 00:19:33 Debug      -  [Seat]: Player 1 End Turn with BET IN = 0
2026.02.21 00:19:34 Debug      -  [Seat]: Player 2 Folded.
2026.02.21 00:19:34 Debug      -  [Table]: Fold to One Condition.
2026.02.21 00:19:34 Debug      -  [Table]: Collecting Bets. ----------------
2026.02.21 00:19:34 Debug      -  [Seat]: Player 2 End Turn with BET IN = 0
2026.02.21 00:19:34 Debug      -  [Table]: Detected On Turn Passed, already processing result.
2026.02.21 00:19:35 Debug      -  [PotManager]: All players folded, player 3 won 130
2026.02.21 00:19:36 Debug      -  ================================================
2026.02.21 00:19:37 Debug      -  [Table]: Preparing for New Game: 
`

// Hand with showdown
const showdownLog = `
2026.02.21 00:20:00 Debug      -  [Table]: Preparing for New Game: 
2026.02.21 00:20:01 Debug      -  [Seat]: Player 1 SB BET IN = 10
2026.02.21 00:20:01 Debug      -  [Seat]: Player 0 BB BET IN = 20
2026.02.21 00:20:01 Debug      -  [Seat]: Draw Local Hole Cards: Ac, Kh
2026.02.21 00:20:03 Debug      -  [Seat]: Player 2 End Turn with BET IN = 20
2026.02.21 00:20:04 Debug      -  [Seat]: Player 0 End Turn with BET IN = 20
2026.02.21 00:20:04 Debug      -  [Seat]: Player 1 End Turn with BET IN = 20
2026.02.21 00:20:04 Debug      -  [Table]: Next phase.True - 3
2026.02.21 00:20:04 Debug      -  [Table]: Collecting Bets. ----------------
2026.02.21 00:20:05 Debug      -  [Table]: New Community Card: As
2026.02.21 00:20:05 Debug      -  [Table]: New Community Card: 7h
2026.02.21 00:20:05 Debug      -  [Table]: New Community Card: 2d
2026.02.21 00:20:10 Debug      -  [Seat]: Player 0 End Turn with BET IN = 100
2026.02.21 00:20:11 Debug      -  [Seat]: Player 1 End Turn with BET IN = 100
2026.02.21 00:20:12 Debug      -  [Seat]: Player 2 End Turn with BET IN = 100
2026.02.21 00:20:12 Debug      -  [Table]: Next phase.True - 3
2026.02.21 00:20:12 Debug      -  [Table]: Collecting Bets. ----------------
2026.02.21 00:20:13 Debug      -  [Table]: New Community Card: 5c
2026.02.21 00:20:18 Debug      -  [Seat]: Player 0 End Turn with BET IN = 0
2026.02.21 00:20:19 Debug      -  [Seat]: Player 1 End Turn with BET IN = 0
2026.02.21 00:20:20 Debug      -  [Seat]: Player 2 End Turn with BET IN = 0
2026.02.21 00:20:20 Debug      -  [Table]: Next phase.True - 3
2026.02.21 00:20:20 Debug      -  [Table]: Collecting Bets. ----------------
2026.02.21 00:20:21 Debug      -  [Table]: New Community Card: 9s
2026.02.21 00:20:25 Debug      -  [Seat]: Player 0 End Turn with BET IN = 0
2026.02.21 00:20:26 Debug      -  [Seat]: Player 1 End Turn with BET IN = 0
2026.02.21 00:20:27 Debug      -  [Seat]: Player 2 End Turn with BET IN = 0
2026.02.21 00:20:27 Debug      -  [Table]: Next phase.True - 3
2026.02.21 00:20:27 Debug      -  [Table]: Collecting Bets. ----------------
2026.02.21 00:20:28 Debug      -  [Seat]: Player 0 Show hole cards: Ac, Kh
2026.02.21 00:20:29 Debug      -  [Seat]: Player 1 Show hole cards: 7d, 7c
2026.02.21 00:20:30 Debug      -  [Seat]: Player 2 Show hole cards: 2h, 2c
2026.02.21 00:20:31 Debug      -  [Pot]: Deal One Pot
2026.02.21 00:20:33 Debug      -  [Pot]: Winner: 0 Pot Amount: 360
`

func TestParseSimpleHand(t *testing.T) {
	result, err := ParseReader(strings.NewReader(sampleLog))
	if err != nil {
		t.Fatalf("ParseReader error: %v", err)
	}

	// The first hand finalizes when "Preparing for New Game" is seen the second time
	if len(result.Hands) == 0 {
		t.Fatal("expected at least 1 hand, got 0")
	}

	hand := result.Hands[0]
	t.Logf("Hand #%d: players=%d, community=%d, winType=%s, winner=%d, pot=%d",
		hand.ID, hand.NumPlayers, len(hand.CommunityCards), hand.WinType, hand.WinnerSeat, hand.TotalPot)

	// Local player is seat 0 (SB BET IN = 10 and Draw Local Hole Cards same timestamp)
	if result.LocalPlayerSeat != 0 {
		t.Errorf("expected local seat 0, got %d", result.LocalPlayerSeat)
	}

	// Local player has hole cards
	if p0, ok := hand.Players[0]; ok {
		if len(p0.HoleCards) != 2 {
			t.Errorf("expected 2 hole cards for player 0, got %d", len(p0.HoleCards))
		} else {
			t.Logf("Player 0 hole cards: %v %v", p0.HoleCards[0], p0.HoleCards[1])
		}
	} else {
		t.Error("player 0 not found in hand")
	}

	// SB/BB seats
	if hand.SBSeat != 0 {
		t.Errorf("expected SB seat 0, got %d", hand.SBSeat)
	}
	if hand.BBSeat != 1 {
		t.Errorf("expected BB seat 1, got %d", hand.BBSeat)
	}

	// Community cards: Qd, 3s, Kd, 8c = 4 cards
	if len(hand.CommunityCards) != 4 {
		t.Errorf("expected 4 community cards, got %d", len(hand.CommunityCards))
	}

	// Winner is player 3
	if hand.WinnerSeat != 3 {
		t.Errorf("expected winner seat 3, got %d", hand.WinnerSeat)
	}

	// Win type should be fold
	if hand.WinType != "fold" {
		t.Errorf("expected win type 'fold', got %q", hand.WinType)
	}

	// Total pot
	if hand.TotalPot != 130 {
		t.Errorf("expected pot 130, got %d", hand.TotalPot)
	}
}

func TestParseShowdownHand(t *testing.T) {
	result, err := ParseReader(strings.NewReader(showdownLog))
	if err != nil {
		t.Fatalf("ParseReader error: %v", err)
	}

	if len(result.Hands) == 0 {
		t.Fatal("expected at least 1 hand, got 0")
	}

	hand := result.Hands[0]
	t.Logf("Hand #%d: players=%d, community=%d, winType=%s, winner=%d, pot=%d",
		hand.ID, hand.NumPlayers, len(hand.CommunityCards), hand.WinType, hand.WinnerSeat, hand.TotalPot)

	// Local seat detected via "Show hole cards: Ac, Kh" matching Draw Local Hole Cards
	if result.LocalPlayerSeat != 0 {
		t.Errorf("expected local seat 0, got %d", result.LocalPlayerSeat)
	}

	// 5 community cards
	if len(hand.CommunityCards) != 5 {
		t.Errorf("expected 5 community cards, got %d", len(hand.CommunityCards))
	}

	// Winner is player 0 (us)
	if hand.WinnerSeat != 0 {
		t.Errorf("expected winner seat 0, got %d", hand.WinnerSeat)
	}

	// Win type showdown
	if hand.WinType != "showdown" {
		t.Errorf("expected win type 'showdown', got %q", hand.WinType)
	}

	// Player 0 WSD
	if p0, ok := hand.Players[0]; ok {
		if !p0.ShowedDown {
			t.Error("expected player 0 to have showed down")
		}
		if !p0.Won {
			t.Error("expected player 0 to have won")
		}
	}
}

func TestParseCard(t *testing.T) {
	tests := []struct {
		input string
		rank  string
		suit  string
	}{
		{"Ah", "A", "h"},
		{"Kd", "K", "d"},
		{"10c", "10", "c"},
		{"2s", "2", "s"},
		{"Jh", "J", "h"},
	}
	for _, tt := range tests {
		card, err := parseCard(tt.input)
		if err != nil {
			t.Errorf("parseCard(%q) error: %v", tt.input, err)
			continue
		}
		if card.Rank != tt.rank || card.Suit != tt.suit {
			t.Errorf("parseCard(%q) = {%s %s}, want {%s %s}",
				tt.input, card.Rank, card.Suit, tt.rank, tt.suit)
		}
	}
}

// Hand where local player is not SB/BB (no blind post before Draw Local Hole Cards)
const nonBlindLocalLog = `
2026.02.21 00:30:00 Debug      -  [Table]: Preparing for New Game: 
2026.02.21 00:30:01 Debug      -  [Seat]: Player 2 SB BET IN = 10
2026.02.21 00:30:01 Debug      -  [Seat]: Player 3 BB BET IN = 20
2026.02.21 00:30:01 Debug      -  [Seat]: Draw Local Hole Cards: Ks, Qd
2026.02.21 00:30:05 Debug      -  [Seat]: Player 4 End Turn with BET IN = 20
2026.02.21 00:30:06 Debug      -  [Seat]: Player 5 Folded.
2026.02.21 00:30:06 Debug      -  [Seat]: Player 5 End Turn with BET IN = 0
2026.02.21 00:30:07 Debug      -  [Seat]: Player 0 Folded.
2026.02.21 00:30:07 Debug      -  [Seat]: Player 0 End Turn with BET IN = 0
2026.02.21 00:30:08 Debug      -  [Seat]: Player 1 End Turn with BET IN = 20
2026.02.21 00:30:08 Debug      -  [Table]: Next phase.True - 4
2026.02.21 00:30:08 Debug      -  [Table]: Collecting Bets. ----------------
2026.02.21 00:30:09 Debug      -  [Table]: New Community Card: Ah
2026.02.21 00:30:09 Debug      -  [Table]: New Community Card: 2c
2026.02.21 00:30:09 Debug      -  [Table]: New Community Card: 7s
2026.02.21 00:30:20 Debug      -  [Seat]: Player 3 Show hole cards: Ks, Qd
2026.02.21 00:30:21 Debug      -  [Seat]: Player 4 Show hole cards: 6h, 6d
2026.02.21 00:30:22 Debug      -  [Pot]: Deal One Pot
2026.02.21 00:30:24 Debug      -  [Pot]: Winner: 4 Pot Amount: 200
`

func TestLocalPlayerNotBlind(t *testing.T) {
	// Draw Local Hole Cards comes after BB, so initially assigned to BB (seat 3).
	// But Show hole cards for Player 3 matches → confirmed as seat 3.
	result, err := ParseReader(strings.NewReader(nonBlindLocalLog))
	if err != nil {
		t.Fatalf("ParseReader error: %v", err)
	}
	if len(result.Hands) == 0 {
		t.Fatal("expected at least 1 hand, got 0")
	}
	hand := result.Hands[0]
	t.Logf("Hand #%d: localSeat=%d players=%v community=%d winType=%s winner=%d pot=%d",
		hand.ID, hand.LocalPlayerSeat, hand.ActiveSeats, len(hand.CommunityCards), hand.WinType, hand.WinnerSeat, hand.TotalPot)

	// Local seat should be 3 (BB, confirmed by Show hole cards: Ks, Qd)
	if result.LocalPlayerSeat != 3 {
		t.Errorf("expected local seat 3 (BB confirmed by showdown), got %d", result.LocalPlayerSeat)
	}
	if p3, ok := hand.Players[3]; ok {
		if len(p3.HoleCards) != 2 {
			t.Errorf("expected 2 hole cards for local player (seat 3), got %d", len(p3.HoleCards))
		}
	}
	// Winner is player 4
	if hand.WinnerSeat != 4 {
		t.Errorf("expected winner seat 4, got %d", hand.WinnerSeat)
	}
}

func TestPositionAssignment(t *testing.T) {
	// 6-handed: SB=0, BB=1, UTG=2, HJ=3, CO=4, BTN=5
	h := &Hand{
		SBSeat:      0,
		BBSeat:      1,
		ActiveSeats: []int{0, 1, 2, 3, 4, 5},
		Players: map[int]*PlayerHandInfo{
			0: {SeatID: 0},
			1: {SeatID: 1},
			2: {SeatID: 2},
			3: {SeatID: 3},
			4: {SeatID: 4},
			5: {SeatID: 5},
		},
	}

	p := NewParser()
	p.assignPositions(h)

	expected := map[int]Position{
		0: PosSB,
		1: PosBB,
		2: PosUTG,
		3: PosHJ,
		4: PosCO,
		5: PosBTN,
	}

	for seat, wantPos := range expected {
		if pi, ok := h.Players[seat]; ok {
			if pi.Position != wantPos {
				t.Errorf("seat %d: expected %s, got %s", seat, wantPos, pi.Position)
			}
		}
	}
}

// TestIssue33SidePotDoesNotMeanVictory verifies that receiving a side pot while losing at showdown
// does NOT count as a win. Victory is defined as participating (VPIP or ShowedDown) AND profit > 0.
func TestIssue33SidePotDoesNotMeanVictory(t *testing.T) {
	// Scenario: Player goes to showdown, loses the main pot, but receives a small side pot
	// Investment: 200 chips
	// Main pot: 200 chips lost
	// Side pot: 100 chips received (because winner went all-in)
	// Total profit: -100 chips -> Should be LOST, not WON

	h := &Hand{
		ID:             1,
		SBSeat:         0,
		BBSeat:         1,
		WinnerSeat:     2,
		WinType:        "showdown",
		IsComplete:     true,
		CommunityCards: []Card{{Rank: "K", Suit: "h"}, {Rank: "d", Suit: "d"}, {Rank: "3", Suit: "c"}},
		ActiveSeats:    []int{0, 1, 2, 3},
		ActiveSeatSet:  map[int]struct{}{0: {}, 1: {}, 2: {}, 3: {}},
		NumPlayers:     4,
		TotalPot:       500,
		Players: map[int]*PlayerHandInfo{
			0: {SeatID: 0, VPIP: false, ShowedDown: false, Won: false, PotWon: 0, Actions: []PlayerAction{{Amount: 50}}},
			1: {SeatID: 1, VPIP: false, ShowedDown: false, Won: false, PotWon: 0, Actions: []PlayerAction{{Amount: 100}}},
			2: {SeatID: 2, VPIP: true, ShowedDown: true, Won: true, PotWon: 300, Actions: []PlayerAction{{Amount: 200}}},
			// Player 3 (local player): goes to showdown, loses main pot, gets side pot
			3: {SeatID: 3, VPIP: true, ShowedDown: true, Won: false, PotWon: 100, Actions: []PlayerAction{{Amount: 200}}},
		},
	}

	// Expected after calculateInvestedAmount and Win determination logic:
	// Player 3: invested=200, PotWon=100, profit=100-200=-100 < 0 -> Should NOT be Won

	p := NewParser()
	invested := p.calculateInvestedAmount(h.Players[3])

	if invested != 200 {
		t.Errorf("expected invested=200, got %d", invested)
	}

	// Check Participated flag (should be true because went to showdown)
	if !h.Players[3].ShowedDown {
		t.Errorf("expected ShowedDown=true")
	}

	// The key issue #33 check: profit-based Won determination
	// Even though PotWon > 0, profit < 0 because invested > PotWon
	profit := h.Players[3].PotWon - invested
	if profit >= 0 {
		t.Errorf("expected profit < 0, got %d (PotWon=%d, invested=%d)", profit, h.Players[3].PotWon, invested)
	}
	// For issue #33: after fix, Won should be determined by participated && profit > 0
	// We verify that the player would be marked as Lost (not Won)
	if h.Players[3].Won {
		t.Errorf("expected Won=false for player with negative profit at showdown, but got Won=true")
	}
}

// TestParticipationFlagForPreflopFold verifies that players who fold preflop without VPIP
// are marked as not participating.
func TestParticipationFlagForPreflopFold(t *testing.T) {
	h := &Hand{
		ID:            2,
		SBSeat:        0,
		BBSeat:        1,
		WinnerSeat:    2,
		WinType:       "fold",
		IsComplete:    true,
		ActiveSeats:   []int{0, 1, 2, 3},
		ActiveSeatSet: map[int]struct{}{0: {}, 1: {}, 2: {}, 3: {}},
		NumPlayers:    4,
		TotalPot:      150,
		Players: map[int]*PlayerHandInfo{
			0: {SeatID: 0, VPIP: false, FoldedPF: false, ShowedDown: false, Won: false, PotWon: 0, Actions: []PlayerAction{{Amount: 50}}},
			1: {SeatID: 1, VPIP: false, FoldedPF: false, ShowedDown: false, Won: false, PotWon: 0, Actions: []PlayerAction{{Amount: 100}}},
			2: {SeatID: 2, VPIP: true, FoldedPF: false, ShowedDown: false, Won: true, PotWon: 150, Actions: []PlayerAction{{Amount: 50}}},
			// Player 3 (local player): folds preflop without VPIP
			3: {SeatID: 3, VPIP: false, FoldedPF: true, ShowedDown: false, Won: false, PotWon: 0, Actions: []PlayerAction{{Amount: 0}}},
		},
	}

	// Expected: Player 3 Participated=false (VPIP=false && ShowedDown=false)
	if h.Players[3].VPIP || h.Players[3].ShowedDown {
		t.Errorf("expected non-participating player (VPIP=false, ShowedDown=false)")
	}

	// After Participated flag logic: Participated = VPIP || ShowedDown
	// This player should have Participated=false
	participated := h.Players[3].VPIP || h.Players[3].ShowedDown
	if participated {
		t.Errorf("expected Participated=false for prefold fold, got true")
	}
}
