package stats

import "github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"

type seqAction struct {
	seat int
	act  parser.PlayerAction
}

func preflopActionSequence(h *parser.Hand) []seqAction {
	if h == nil {
		return nil
	}
	out := make([]seqAction, 0)
	for seat, p := range h.Players {
		if p == nil {
			continue
		}
		for _, a := range p.Actions {
			if a.Street != parser.StreetPreFlop {
				continue
			}
			if a.Action == parser.ActionBlindSB || a.Action == parser.ActionBlindBB {
				continue
			}
			out = append(out, seqAction{seat: seat, act: a})
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].act.Timestamp.Before(out[j-1].act.Timestamp) || (out[j].act.Timestamp.Equal(out[j-1].act.Timestamp) && out[j].seat < out[j-1].seat)); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func isAggressivePreflop(a parser.ActionType) bool {
	return a == parser.ActionBet || a == parser.ActionRaise || a == parser.ActionAllIn
}

func hasRFIOpportunityApprox(pi *parser.PlayerHandInfo, h *parser.Hand) bool {
	if pi == nil || h == nil {
		return false
	}
	seq := preflopActionSequence(h)
	for _, sa := range seq {
		if sa.seat == pi.SeatID {
			return true
		}
		if sa.act.Action == parser.ActionCall || isAggressivePreflop(sa.act.Action) {
			return false
		}
	}
	return false
}

func didRFIApprox(pi *parser.PlayerHandInfo, h *parser.Hand) bool {
	if !hasRFIOpportunityApprox(pi, h) {
		return false
	}
	for _, a := range pi.Actions {
		if a.Street != parser.StreetPreFlop {
			continue
		}
		if isAggressivePreflop(a.Action) {
			return true
		}
		if a.Action == parser.ActionCall || a.Action == parser.ActionFold || a.Action == parser.ActionCheck {
			return false
		}
	}
	return false
}

func hasThreeBetOpportunityApprox(pi *parser.PlayerHandInfo, h *parser.Hand) bool {
	if pi == nil || h == nil {
		return false
	}
	if pi.ThreeBet {
		return true
	}
	for seat, p := range h.Players {
		if p == nil || seat == pi.SeatID {
			continue
		}
		if p.PFR {
			return hasCallOnStreet(pi, parser.StreetPreFlop) || pi.FoldedPF
		}
	}
	return false
}

func hasFoldToThreeBetOpportunityApprox(pi *parser.PlayerHandInfo, h *parser.Hand) bool {
	if pi == nil || h == nil {
		return false
	}
	if pi.FoldTo3Bet {
		return true
	}
	if !pi.PFR {
		return false
	}
	for seat, p := range h.Players {
		if p == nil || seat == pi.SeatID {
			continue
		}
		if p.ThreeBet {
			return true
		}
	}
	return false
}

func hasFourBetOpportunityApprox(pi *parser.PlayerHandInfo, h *parser.Hand) bool {
	return hasFoldToThreeBetOpportunityApprox(pi, h)
}

func didFourBetApprox(pi *parser.PlayerHandInfo, h *parser.Hand) bool {
	if !hasFourBetOpportunityApprox(pi, h) {
		return false
	}
	level, ok := firstPreflopAggressionLevel(h, pi.SeatID)
	return ok && level >= 3
}

func hasSqueezeOpportunityApprox(pi *parser.PlayerHandInfo, h *parser.Hand) bool {
	if pi == nil || h == nil {
		return false
	}
	seq := preflopActionSequence(h)
	openSeen := false
	openCalls := 0
	raiseCount := 0
	for _, sa := range seq {
		if sa.seat == pi.SeatID {
			return openSeen && openCalls > 0 && raiseCount == 1
		}
		if isAggressivePreflop(sa.act.Action) {
			raiseCount++
			if raiseCount == 1 {
				openSeen = true
				continue
			}
			return false
		}
		if openSeen && sa.act.Action == parser.ActionCall {
			openCalls++
		}
	}
	return false
}

func didSqueezeApprox(pi *parser.PlayerHandInfo, h *parser.Hand) bool {
	if !hasSqueezeOpportunityApprox(pi, h) {
		return false
	}
	for _, a := range pi.Actions {
		if a.Street != parser.StreetPreFlop {
			continue
		}
		if a.Action == parser.ActionBlindSB || a.Action == parser.ActionBlindBB {
			continue
		}
		return isAggressivePreflop(a.Action)
	}
	return false
}

func isStealOpportunity(pi *parser.PlayerHandInfo, h *parser.Hand) bool {
	if pi == nil || h == nil {
		return false
	}
	if pi.Position != parser.PosCO && pi.Position != parser.PosBTN && pi.Position != parser.PosSB {
		return false
	}
	return hasRFIOpportunityApprox(pi, h)
}

func isStealAttempt(pi *parser.PlayerHandInfo) bool {
	if pi == nil {
		return false
	}
	for _, a := range pi.Actions {
		if a.Street != parser.StreetPreFlop {
			continue
		}
		if a.Action == parser.ActionBet || a.Action == parser.ActionRaise || a.Action == parser.ActionAllIn {
			return true
		}
		if a.Action == parser.ActionCall || a.Action == parser.ActionFold || a.Action == parser.ActionCheck {
			return false
		}
	}
	return false
}

func isFoldToStealOpportunity(pi *parser.PlayerHandInfo, h *parser.Hand) bool {
	if pi == nil {
		return false
	}
	if pi.Position != parser.PosSB && pi.Position != parser.PosBB {
		return false
	}
	return isFoldToStealOpportunityByPosition(pi, h, pi.Position)
}

func isFoldToStealOpportunityByPosition(pi *parser.PlayerHandInfo, h *parser.Hand, pos parser.Position) bool {
	if pi == nil || h == nil {
		return false
	}
	if pi.Position != pos {
		return false
	}
	openSeat, ok := detectStealOpenSeat(h)
	if !ok || openSeat == pi.SeatID {
		return false
	}
	seq := preflopActionSequence(h)
	seenOpen := false
	for _, sa := range seq {
		if sa.seat == openSeat && isAggressivePreflop(sa.act.Action) {
			seenOpen = true
			continue
		}
		if !seenOpen {
			continue
		}
		if sa.seat == pi.SeatID {
			return true
		}
	}
	return false
}

func isThreeBetVsStealOpportunity(pi *parser.PlayerHandInfo, h *parser.Hand) bool {
	if pi == nil || h == nil {
		return false
	}
	if pi.Position != parser.PosSB && pi.Position != parser.PosBB {
		return false
	}
	openSeat, ok := detectStealOpenSeat(h)
	if !ok || openSeat == pi.SeatID {
		return false
	}
	seq := preflopActionSequence(h)
	seenOpen := false
	for _, sa := range seq {
		if sa.seat == openSeat && isAggressivePreflop(sa.act.Action) {
			seenOpen = true
			continue
		}
		if !seenOpen {
			continue
		}
		if sa.seat == pi.SeatID {
			return true
		}
		if isAggressivePreflop(sa.act.Action) {
			return false
		}
	}
	return false
}

func didThreeBetVsSteal(pi *parser.PlayerHandInfo, h *parser.Hand) bool {
	if !isThreeBetVsStealOpportunity(pi, h) {
		return false
	}
	seq := preflopActionSequence(h)
	openSeat, ok := detectStealOpenSeat(h)
	if !ok {
		return false
	}
	seenOpen := false
	for _, sa := range seq {
		if sa.seat == openSeat && isAggressivePreflop(sa.act.Action) {
			seenOpen = true
			continue
		}
		if !seenOpen {
			continue
		}
		if sa.seat == pi.SeatID {
			return isAggressivePreflop(sa.act.Action)
		}
		if isAggressivePreflop(sa.act.Action) {
			return false
		}
	}
	return false
}

func firstPreflopAggressionLevel(h *parser.Hand, seat int) (int, bool) {
	if h == nil {
		return 0, false
	}
	level := 0
	for _, sa := range preflopActionSequence(h) {
		if !isAggressivePreflop(sa.act.Action) {
			continue
		}
		if sa.seat == seat {
			return level + 1, true
		}
		level++
	}
	return 0, false
}

func isColdCallApprox(pi *parser.PlayerHandInfo, h *parser.Hand) bool {
	if pi == nil || h == nil {
		return false
	}
	if pi.PFR || pi.ThreeBet || !pi.VPIP {
		return false
	}
	if pi.Position == parser.PosSB {
		if hasCallOnStreet(pi, parser.StreetPreFlop) {
			bb := bbAmountFromHand(h)
			if bb <= 0 {
				return false
			}
			for _, a := range pi.Actions {
				if a.Street == parser.StreetPreFlop && a.Action == parser.ActionCall && a.Amount > bb {
					return true
				}
			}
		}
		return false
	}
	if pi.Position == parser.PosBB {
		for seat, p := range h.Players {
			if p == nil || seat == pi.SeatID {
				continue
			}
			if p.PFR {
				return hasCallOnStreet(pi, parser.StreetPreFlop)
			}
		}
		return false
	}
	return hasCallOnStreet(pi, parser.StreetPreFlop)
}

func detectStealOpenSeat(h *parser.Hand) (int, bool) {
	if h == nil {
		return -1, false
	}
	seq := preflopActionSequence(h)
	for _, sa := range seq {
		if sa.act.Action == parser.ActionCall {
			return -1, false
		}
		if isAggressivePreflop(sa.act.Action) {
			pi := h.Players[sa.seat]
			if pi == nil {
				return -1, false
			}
			if pi.Position == parser.PosCO || pi.Position == parser.PosBTN || pi.Position == parser.PosSB {
				return sa.seat, true
			}
			return -1, false
		}
	}
	return -1, false
}
