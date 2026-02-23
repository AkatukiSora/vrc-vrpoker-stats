package parser

func (p *Parser) assignPositions(h *Hand) {
	if h.SBSeat < 0 || h.BBSeat < 0 {
		return
	}
	allSeats := make([]int, len(h.ActiveSeats))
	copy(allSeats, h.ActiveSeats)
	sortInts(allSeats)

	sbIdx := -1
	for i, s := range allSeats {
		if s == h.SBSeat {
			sbIdx = i
			break
		}
	}
	if sbIdx < 0 {
		return
	}

	rotated := append(allSeats[sbIdx:], allSeats[:sbIdx]...)
	positions := positionOrder(len(rotated))
	for i, seat := range rotated {
		if pi, ok := h.Players[seat]; ok && i < len(positions) {
			pi.Position = positions[i]
		}
	}
}

func positionOrder(n int) []Position {
	switch n {
	case 2:
		return []Position{PosSB, PosBTN}
	case 3:
		return []Position{PosSB, PosBB, PosBTN}
	case 4:
		return []Position{PosSB, PosBB, PosUTG, PosBTN}
	case 5:
		return []Position{PosSB, PosBB, PosUTG, PosMP, PosBTN}
	case 6:
		return []Position{PosSB, PosBB, PosUTG, PosHJ, PosCO, PosBTN}
	case 7:
		return []Position{PosSB, PosBB, PosUTG, PosUTG1, PosHJ, PosCO, PosBTN}
	case 8:
		return []Position{PosSB, PosBB, PosUTG, PosUTG1, PosMP, PosHJ, PosCO, PosBTN}
	default:
		result := make([]Position, n)
		result[0] = PosSB
		if n > 1 {
			result[1] = PosBB
		}
		if n > 2 {
			result[n-1] = PosBTN
		}
		if n > 3 {
			result[n-2] = PosCO
		}
		if n > 4 {
			result[n-3] = PosMP
		}
		return result
	}
}

func sortInts(s []int) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
