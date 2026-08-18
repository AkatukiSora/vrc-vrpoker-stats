package parser

// normalizeHand is the sole authority for blind and position normalization.
// It intentionally refuses to reconstruct two missing blind posts from action
// order: a parser can start mid-hand and that would turn missing evidence into
// fabricated blind seats.  A single missing counterpart is inferred only when
// the complete observed preflop rotation corroborates it.
func (p *Parser) normalizeHand(h *Hand) {
	if h == nil {
		return
	}
	sbAmount, bbAmount := p.blindAmounts(h)
	p.inferBlindsFromPreflop(h)
	if !hasCompletePreflopRotation(h) {
		for _, pi := range h.Players {
			pi.Position = PosUnknown
		}
		h.addAnomaly("PREFLOP_ROTATION_INCOMPLETE", "warn", "not every observed seat made a non-blind preflop decision")
	} else {
		p.assignPositions(h)
	}
	if h.SBSeat < 0 || h.BBSeat < 0 {
		h.addAnomaly("BLIND_SEATS_UNKNOWN", "warn", "missing explicit or corroborated blind seat")
	}
	if h.SBSeat >= 0 && sbAmount <= 0 {
		h.addAnomaly("SMALL_BLIND_AMOUNT_UNKNOWN", "warn", "small blind seat is known but its posted amount was not observed")
	}
	if h.BBSeat >= 0 && bbAmount <= 0 {
		h.addAnomaly("BIG_BLIND_AMOUNT_UNKNOWN", "warn", "big blind seat is known but its posted amount was not observed")
	}
}

func hasCompletePreflopRotation(h *Hand) bool {
	if h == nil || len(h.ActiveSeats) < 2 {
		return false
	}
	seen := make(map[int]bool, len(h.ActiveSeats))
	for seat, pi := range h.Players {
		for _, a := range pi.Actions {
			if a.Street == StreetPreFlop && a.Action != ActionBlindSB && a.Action != ActionBlindBB {
				seen[seat] = true
				break
			}
		}
	}
	if len(seen) == len(h.ActiveSeats) {
		return true
	}
	// A fold-win ends the round immediately. Its sole surviving player has no
	// legal response to log, so accept exactly that one missing decision only
	// when the winner is explicit and every recorded decision folded.
	if len(seen) != len(h.ActiveSeats)-1 || h.WinType != "fold" || h.WinnerSeat < 0 || seen[h.WinnerSeat] {
		return false
	}
	for seat, pi := range h.Players {
		if seat == h.WinnerSeat || pi == nil {
			continue
		}
		for _, a := range pi.Actions {
			if a.Street == StreetPreFlop && a.Action != ActionBlindSB && a.Action != ActionBlindBB && a.Action != ActionFold {
				return false
			}
		}
	}
	return true
}

func (p *Parser) blindAmounts(h *Hand) (int, int) {
	sbAmount, bbAmount := 0, 0
	for _, pi := range h.Players {
		for _, a := range pi.Actions {
			if a.Action == ActionBlindSB && a.Amount > 0 {
				sbAmount = a.Amount
			}
			if a.Action == ActionBlindBB && a.Amount > 0 {
				bbAmount = a.Amount
			}
		}
	}
	return sbAmount, bbAmount
}

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

	// Do not infer when both posts are absent. The first observed action may be
	// after a join, a dropped line, or a seat change.
	if h.SBSeat < 0 && h.BBSeat < 0 {
		return false
	}
	first := -1
	seen := make(map[int]bool)
	for _, a := range p.pfActions {
		if a.action != ActionBlindSB && a.action != ActionBlindBB {
			if first < 0 {
				first = a.seatID
			}
			seen[a.seatID] = true
		}
	}
	if first < 0 || len(seen) != len(seats) {
		return false
	}
	index := func(seat int) int {
		for i, s := range seats {
			if s == seat {
				return i
			}
		}
		return -1
	}
	if h.SBSeat >= 0 {
		i := index(h.SBSeat)
		if i < 0 {
			return false
		}
		candidate := seats[(i+1)%len(seats)]
		expected := seats[(i+2)%len(seats)]
		if len(seats) == 2 {
			expected = h.SBSeat
		}
		if first == expected {
			h.BBSeat = candidate
			return true
		}
	}
	if h.BBSeat >= 0 {
		i := index(h.BBSeat)
		if i < 0 {
			return false
		}
		candidate := seats[(i-1+len(seats))%len(seats)]
		expected := seats[(i+1)%len(seats)]
		if len(seats) == 2 {
			expected = candidate
		}
		if first == expected {
			h.SBSeat = candidate
			return true
		}
	}
	return false
}
