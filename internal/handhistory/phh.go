// Package handhistory serializes parsed VR Poker hands for interchange.
package handhistory

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
)

// SerializePHH returns a Poker Hand History (PHH) TOML document. PHH is an
// open, tool-neutral interchange format; unknown cards and stacks are emitted
// as ?? and inf rather than being guessed from VRChat logs.
func SerializePHH(hand *parser.Hand) ([]byte, error) {
	if hand == nil {
		return nil, fmt.Errorf("serialize PHH: hand is nil")
	}
	seats, err := playerOrder(hand)
	if err != nil {
		return nil, fmt.Errorf("serialize PHH: %w", err)
	}
	sb := blindAmount(hand, hand.SBSeat, parser.ActionBlindSB)
	bb := blindAmount(hand, hand.BBSeat, parser.ActionBlindBB)
	if sb <= 0 || bb <= 0 {
		return nil, fmt.Errorf("missing valid small and big blinds")
	}
	playerIndex := make(map[int]int, len(seats))
	for i, seat := range seats {
		playerIndex[seat] = i + 1
	}

	actions := make([]string, 0, len(seats)+16)
	for _, seat := range seats {
		actions = append(actions, fmt.Sprintf("d dh p%d %s", playerIndex[seat], holeCards(hand.Players[seat])))
	}
	ordered := orderedActions(hand)
	for _, street := range []parser.Street{parser.StreetPreFlop, parser.StreetFlop, parser.StreetTurn, parser.StreetRiver} {
		switch street {
		case parser.StreetFlop:
			if len(hand.CommunityCards) >= 3 {
				actions = append(actions, "d db "+cards(hand.CommunityCards[:3]))
			}
		case parser.StreetTurn:
			if len(hand.CommunityCards) >= 4 {
				actions = append(actions, "d db "+cards(hand.CommunityCards[3:4]))
			}
		case parser.StreetRiver:
			if len(hand.CommunityCards) >= 5 {
				actions = append(actions, "d db "+cards(hand.CommunityCards[4:5]))
			}
		}
		for _, action := range ordered {
			if action.Street != street || action.Action == parser.ActionBlindSB || action.Action == parser.ActionBlindBB {
				continue
			}
			if idx, ok := playerIndex[action.PlayerID]; ok {
				if encoded, ok := encodeAction(idx, action); ok {
					actions = append(actions, encoded)
				}
			}
		}
	}
	for _, seat := range seats {
		pi := hand.Players[seat]
		if pi != nil && pi.ShowedDown {
			actions = append(actions, fmt.Sprintf("p%d sm %s", playerIndex[seat], holeCards(pi)))
		}
	}

	var b strings.Builder
	b.WriteString("# VRPoker Stats PHH export. Unknown stacks and cards are explicit.\n")
	b.WriteString("variant = \"NT\"\n")
	b.WriteString("antes = [")
	writeRepeated(&b, "0", len(seats))
	b.WriteString("]\nblinds_or_straddles = [")
	for i, seat := range seats {
		if i > 0 {
			b.WriteString(", ")
		}
		switch seat {
		case hand.SBSeat:
			b.WriteString(strconv.Itoa(sb))
		case hand.BBSeat:
			b.WriteString(strconv.Itoa(bb))
		default:
			b.WriteByte('0')
		}
	}
	b.WriteString("]\nmin_bet = ")
	b.WriteString(strconv.Itoa(bb))
	b.WriteString("\nstarting_stacks = [")
	writeRepeated(&b, "inf", len(seats))
	b.WriteString("]\nactions = [\n")
	for _, action := range actions {
		b.WriteString("  ")
		b.WriteString(strconv.Quote(action))
		b.WriteString(",\n")
	}
	b.WriteString("]\nevent = \"VRChat VR Poker\"\n")
	if hand.HandUID != "" {
		b.WriteString("hand = ")
		b.WriteString(strconv.Quote(hand.HandUID))
		b.WriteByte('\n')
	}
	if !hand.StartTime.IsZero() {
		b.WriteString("time = ")
		b.WriteString(hand.StartTime.Format("2006-01-02T15:04:05"))
		b.WriteByte('\n')
		b.WriteString("time_zone = \"UTC\"\n")
	}
	b.WriteString("players = [")
	for i, seat := range seats {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Quote(fmt.Sprintf("Seat %d", seat)))
	}
	b.WriteString("]\nseats = [")
	for i, seat := range seats {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Itoa(seat))
	}
	b.WriteString("]\nseat_count = ")
	b.WriteString(strconv.Itoa(len(seats)))
	b.WriteByte('\n')
	return []byte(b.String()), nil
}

func playerOrder(h *parser.Hand) ([]int, error) {
	seats := make([]int, 0, len(h.Players))
	for seat, pi := range h.Players {
		if pi != nil {
			seats = append(seats, seat)
		}
	}
	if len(seats) < 2 {
		return nil, fmt.Errorf("need at least two players")
	}
	sort.Ints(seats)
	if h.SBSeat < 0 {
		return nil, fmt.Errorf("missing small blind seat")
	}
	for i, seat := range seats {
		if seat == h.SBSeat {
			return append(append([]int(nil), seats[i:]...), seats[:i]...), nil
		}
	}
	return nil, fmt.Errorf("small blind seat is not a player")
}

func blindAmount(h *parser.Hand, seat int, kind parser.ActionType) int {
	if pi := h.Players[seat]; pi != nil {
		for _, a := range pi.Actions {
			if a.Action == kind && a.Amount > 0 {
				return a.Amount
			}
		}
	}
	return 0
}
func holeCards(pi *parser.PlayerHandInfo) string {
	if pi == nil || len(pi.HoleCards) != 2 {
		return "????"
	}
	return cards(pi.HoleCards)
}
func cards(cs []parser.Card) string {
	var b strings.Builder
	for _, c := range cs {
		rank := c.Rank
		if rank == "10" {
			rank = "T"
		}
		b.WriteString(rank)
		b.WriteString(c.Suit)
	}
	return b.String()
}
func writeRepeated(b *strings.Builder, v string, n int) {
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(v)
	}
}

func orderedActions(h *parser.Hand) []parser.PlayerAction {
	var out []parser.PlayerAction
	for _, pi := range h.Players {
		if pi != nil {
			out = append(out, pi.Actions...)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Timestamp.Equal(out[j].Timestamp) {
			if out[i].Street == out[j].Street {
				return out[i].PlayerID < out[j].PlayerID
			}
			return out[i].Street < out[j].Street
		}
		return out[i].Timestamp.Before(out[j].Timestamp)
	})
	return out
}
func encodeAction(index int, a parser.PlayerAction) (string, bool) {
	switch a.Action {
	case parser.ActionFold:
		return fmt.Sprintf("p%d f", index), true
	case parser.ActionCheck, parser.ActionCall:
		return fmt.Sprintf("p%d cc", index), true
	case parser.ActionBet, parser.ActionRaise, parser.ActionAllIn:
		if a.Amount > 0 {
			return fmt.Sprintf("p%d cbr %d", index, a.Amount), true
		}
	}
	return "", false
}
