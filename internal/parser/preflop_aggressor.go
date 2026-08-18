package parser

import "sort"

type preflopSeatAction struct {
	seat int
	act  PlayerAction
}

// LastPreflopAggressorSeat returns the seat of the final aggressive preflop action.
// Blind posts are ignored. If no aggressive preflop action exists, it returns -1.
func LastPreflopAggressorSeat(h *Hand) int {
	if h == nil {
		return -1
	}

	actions := make([]preflopSeatAction, 0)
	for seat, player := range h.Players {
		if player == nil {
			continue
		}
		for _, act := range player.Actions {
			if act.Street != StreetPreFlop {
				continue
			}
			if act.Action == ActionBlindSB || act.Action == ActionBlindBB {
				continue
			}
			if !isPreflopAggressiveAction(act.Action) {
				continue
			}
			actions = append(actions, preflopSeatAction{seat: seat, act: act})
		}
	}

	if len(actions) == 0 {
		return -1
	}

	useSequence := completePreflopSequence(actions)
	sort.Slice(actions, func(i, j int) bool {
		if useSequence {
			return actions[i].act.Sequence < actions[j].act.Sequence
		}
		if actions[i].act.Timestamp.Equal(actions[j].act.Timestamp) {
			if actions[i].act.Amount != actions[j].act.Amount {
				return actions[i].act.Amount < actions[j].act.Amount
			}
			return actions[i].seat < actions[j].seat
		}
		return actions[i].act.Timestamp.Before(actions[j].act.Timestamp)
	})

	return actions[len(actions)-1].seat
}

func completePreflopSequence(actions []preflopSeatAction) bool {
	if len(actions) == 0 {
		return false
	}
	seen := make(map[int]struct{}, len(actions))
	for _, action := range actions {
		if action.act.Sequence <= 0 {
			return false
		}
		if _, duplicate := seen[action.act.Sequence]; duplicate {
			return false
		}
		seen[action.act.Sequence] = struct{}{}
	}
	return true
}

func isPreflopAggressiveAction(action ActionType) bool {
	return action == ActionBet || action == ActionRaise || action == ActionAllIn
}
