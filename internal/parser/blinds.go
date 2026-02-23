package parser

func (p *Parser) inferBlindsFromPreflop(h *Hand) bool {
	if h == nil {
		return false
	}
	if h.SBSeat >= 0 && h.BBSeat >= 0 {
		return false
	}

	seats := make([]int, len(h.ActiveSeats))
	copy(seats, h.ActiveSeats)
	sortInts(seats)
	if len(seats) < 2 {
		return false
	}

	if h.SBSeat >= 0 && h.BBSeat < 0 {
		sbIdx := -1
		for i, s := range seats {
			if s == h.SBSeat {
				sbIdx = i
				break
			}
		}
		if sbIdx >= 0 {
			h.BBSeat = seats[(sbIdx+1)%len(seats)]
			return true
		}
	}
	if h.BBSeat >= 0 && h.SBSeat < 0 {
		bbIdx := -1
		for i, s := range seats {
			if s == h.BBSeat {
				bbIdx = i
				break
			}
		}
		if bbIdx >= 0 {
			h.SBSeat = seats[(bbIdx-1+len(seats))%len(seats)]
			return true
		}
	}

	firstSeat := -1
	for _, act := range p.pfActions {
		switch act.action {
		case ActionBlindSB, ActionBlindBB:
			continue
		case ActionFold, ActionCheck, ActionCall, ActionBet, ActionRaise, ActionAllIn:
			firstSeat = act.seatID
		}
		if firstSeat >= 0 {
			break
		}
	}
	if firstSeat < 0 {
		return false
	}

	idx := -1
	for i, s := range seats {
		if s == firstSeat {
			idx = i
			break
		}
	}
	if idx < 0 || len(seats) < 2 {
		return false
	}

	bbIdx := (idx - 1 + len(seats)) % len(seats)
	sbIdx := (idx - 2 + len(seats)) % len(seats)

	h.BBSeat = seats[bbIdx]
	h.SBSeat = seats[sbIdx]
	return true
}
