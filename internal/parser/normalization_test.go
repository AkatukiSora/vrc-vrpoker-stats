package parser

import (
	"strings"
	"testing"
)

const truncatedBlindOnlyLog = `
2026.08.18 12:00:00 Debug      -  [Table]: Preparing for New Game:
2026.08.18 12:00:01 Debug      -  [Seat]: Player 0 SB BET IN = 10
2026.08.18 12:00:01 Debug      -  [Seat]: Player 1 BB BET IN = 20
2026.08.18 12:00:02 Debug      -  [Table]: New Community Card: Ah
2026.08.18 12:00:02 Debug      -  [Table]: New Community Card: Kd
2026.08.18 12:00:02 Debug      -  [Table]: New Community Card: Qc
`

func normalizedHand(seats ...int) *Hand {
	players := make(map[int]*PlayerHandInfo, len(seats))
	for _, seat := range seats {
		players[seat] = &PlayerHandInfo{SeatID: seat}
	}
	return &Hand{SBSeat: -1, BBSeat: -1, ActiveSeats: seats, Players: players, StatsEligible: true}
}

func anomalyCode(h *Hand, code string) bool {
	for _, a := range h.Anomalies {
		if a.Code == code {
			return true
		}
	}
	return false
}

func TestNormalizeHandBlindAndPositionEvidence(t *testing.T) {
	tests := []struct {
		name           string
		seats          []int
		sb, bb         int
		actions        []pfAction
		wantSB, wantBB int
		wantPos        map[int]Position
		wantAnomaly    string
	}{
		{
			name: "explicit full ring preserves blind amounts and positions", seats: []int{0, 1, 2, 3, 4, 5}, sb: 0, bb: 1,
			actions: []pfAction{{0, ActionBlindSB, 10}, {1, ActionBlindBB, 20}, {2, ActionFold, 0}, {3, ActionCall, 20}, {4, ActionFold, 0}, {5, ActionRaise, 60}, {0, ActionFold, 0}, {1, ActionCall, 60}},
			wantSB:  0, wantBB: 1, wantPos: map[int]Position{0: PosSB, 1: PosBB, 2: PosUTG, 3: PosHJ, 4: PosCO, 5: PosBTN},
		},
		{
			name: "single missing post needs complete corroborating rotation", seats: []int{0, 1, 2, 3}, sb: 0, bb: -1,
			actions: []pfAction{{0, ActionBlindSB, 10}, {2, ActionFold, 0}, {3, ActionFold, 0}, {0, ActionFold, 0}, {1, ActionCheck, 0}},
			wantSB:  0, wantBB: 1, wantAnomaly: "BIG_BLIND_AMOUNT_UNKNOWN",
		},
		{
			name: "both missing remains unknown for partial log", seats: []int{0, 1, 2, 3}, sb: -1, bb: -1,
			actions: []pfAction{{2, ActionFold, 0}, {3, ActionRaise, 40}}, wantSB: -1, wantBB: -1, wantAnomaly: "BLIND_SEATS_UNKNOWN",
		},
		{
			name: "mid session join does not infer counterpart", seats: []int{0, 1, 2, 3}, sb: 0, bb: -1,
			actions: []pfAction{{0, ActionBlindSB, 10}, {3, ActionRaise, 40}}, wantSB: 0, wantBB: -1, wantAnomaly: "BLIND_SEATS_UNKNOWN",
		},
		{
			name: "heads up button is small blind", seats: []int{4, 7}, sb: 4, bb: 7,
			actions: []pfAction{{4, ActionBlindSB, 10}, {7, ActionBlindBB, 20}, {4, ActionCall, 20}, {7, ActionCheck, 0}}, wantSB: 4, wantBB: 7,
			wantPos: map[int]Position{4: PosBTN, 7: PosBB},
		},
		{
			name: "zero blind amount is distinct from position uncertainty", seats: []int{0, 1, 2}, sb: 0, bb: 1,
			actions: []pfAction{{0, ActionBlindSB, 0}, {1, ActionBlindBB, 20}, {2, ActionFold, 0}}, wantSB: 0, wantBB: 1, wantAnomaly: "SMALL_BLIND_AMOUNT_UNKNOWN",
		},
		{
			name: "seat change topology is not fabricated", seats: []int{0, 2, 5}, sb: 0, bb: 5,
			actions: []pfAction{{0, ActionBlindSB, 10}, {5, ActionBlindBB, 20}, {2, ActionFold, 0}, {0, ActionFold, 0}, {5, ActionCheck, 0}}, wantSB: 0, wantBB: 5, wantAnomaly: "BLIND_SEAT_ORDER_AMBIGUOUS",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := normalizedHand(tt.seats...)
			h.SBSeat, h.BBSeat = tt.sb, tt.bb
			p := NewParser()
			p.pfActions = tt.actions
			for i, a := range tt.actions {
				h.Players[a.seatID].Actions = append(h.Players[a.seatID].Actions, PlayerAction{Action: a.action, Amount: a.amount, Street: StreetPreFlop, Sequence: i + 1})
			}
			p.normalizeHand(h)
			if h.SBSeat != tt.wantSB || h.BBSeat != tt.wantBB {
				t.Fatalf("blinds=(%d,%d), want (%d,%d)", h.SBSeat, h.BBSeat, tt.wantSB, tt.wantBB)
			}
			if tt.wantAnomaly != "" && !anomalyCode(h, tt.wantAnomaly) {
				t.Fatalf("missing anomaly %s: %#v", tt.wantAnomaly, h.Anomalies)
			}
			for seat, want := range tt.wantPos {
				if got := h.Players[seat].Position; got != want {
					t.Errorf("seat %d position=%s want %s", seat, got, want)
				}
			}
		})
	}
}

func TestNormalizeHandDoesNotUseFirstActionToInventBothBlinds(t *testing.T) {
	h := normalizedHand(0, 1, 2, 3)
	p := NewParser()
	p.pfActions = []pfAction{{2, ActionFold, 0}, {3, ActionRaise, 40}, {0, ActionFold, 0}, {1, ActionFold, 0}}
	p.normalizeHand(h)
	if h.SBSeat != -1 || h.BBSeat != -1 {
		t.Fatalf("invented blinds: SB=%d BB=%d", h.SBSeat, h.BBSeat)
	}
	if h.Players[2].Position != PosUnknown {
		t.Fatalf("invented position: %s", h.Players[2].Position)
	}
}

func TestParsePartialBlindOnlyHandLeavesPositionsUnknown(t *testing.T) {
	result, err := ParseReader(strings.NewReader(truncatedBlindOnlyLog))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(result.Hands) != 1 {
		t.Fatalf("hands=%d want 1", len(result.Hands))
	}
	h := result.Hands[0]
	if !anomalyCode(h, "PREFLOP_ROTATION_INCOMPLETE") || h.IsStatsEligible() {
		t.Fatalf("partial hand must be anomalous and stats-ineligible: %#v", h.Anomalies)
	}
	if h.Players[0].Position != PosUnknown || h.Players[1].Position != PosUnknown {
		t.Fatalf("partial hand fabricated positions: %s, %s", h.Players[0].Position, h.Players[1].Position)
	}
}
