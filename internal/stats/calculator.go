package stats

import (
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
)

// Calculator computes poker statistics from parsed hands
type Calculator struct{}

// NewCalculator creates a new stats calculator
func NewCalculator() *Calculator {
	return &Calculator{}
}

// Calculate computes full statistics from a list of hands for the local player
func (c *Calculator) Calculate(hands []*parser.Hand, localSeat int) *Stats {
	s := &Stats{
		ByPosition: make(map[parser.Position]*PositionStats),
		HandRange:  newHandRangeTable(),
		Metrics:    make(map[MetricID]MetricValue),
	}
	ma := newMetricAccumulator()

	for _, h := range hands {
		if !h.IsComplete {
			continue
		}
		if !h.IsStatsEligible() {
			continue
		}

		handSeat := localSeat
		if h.LocalPlayerSeat >= 0 {
			handSeat = h.LocalPlayerSeat
		}
		if handSeat < 0 {
			continue
		}

		localInfo, ok := h.Players[handSeat]
		if !ok {
			// We might not be in this hand
			continue
		}

		s.TotalHands++

		pos := localInfo.Position
		ps := c.ensurePositionStats(s, pos)
		ps.Hands++

		// Financial
		invested := c.investedAmount(h, handSeat)
		ps.Invested += invested
		ps.PotWon += localInfo.PotWon
		s.TotalPotWon += localInfo.PotWon
		s.TotalInvested += invested

		// Win
		if localInfo.Won {
			s.WonHands++
			ps.Won++
		}

		// VPIP
		if localInfo.VPIP {
			s.VPIPHands++
			ps.VPIP++
		}

		// PFR
		if localInfo.PFR {
			s.PFRHands++
			ps.PFR++
		}

		// 3bet opportunity: faced preflop raise before committing as opener
		if hasThreeBetOpportunityApprox(localInfo, h) {
			s.ThreeBetOpportunities++
			ps.ThreeBetOpp++
		}
		if localInfo.ThreeBet {
			s.ThreeBetHands++
			ps.ThreeBet++
		}

		// Fold to 3bet
		if hasFoldToThreeBetOpportunityApprox(localInfo, h) {
			s.FoldTo3BetOpportunities++
			ps.FoldTo3BetOpp++
		}
		if localInfo.FoldTo3Bet {
			s.FoldTo3BetHands++
			ps.FoldTo3Bet++
		}

		// Showdown
		if localInfo.ShowedDown {
			s.ShowdownHands++
			ps.Showdowns++
			if localInfo.Won {
				s.WonShowdowns++
				ps.WonShowdowns++
			}
		}

		// Hand range table update
		if len(localInfo.HoleCards) == 2 {
			c.updateHandRange(s.HandRange, h, localInfo, pos)
		}

		ma.consumeHand(h, localInfo, invested)
	}

	ma.finalize(s)

	return s
}

// ensurePositionStats gets or creates position stats
func (c *Calculator) ensurePositionStats(s *Stats, pos parser.Position) *PositionStats {
	if ps, ok := s.ByPosition[pos]; ok {
		return ps
	}
	ps := &PositionStats{Position: pos}
	s.ByPosition[pos] = ps
	return ps
}

// investedAmount calculates total chips invested in a hand by a player
func (c *Calculator) investedAmount(h *parser.Hand, seat int) int {
	pi, ok := h.Players[seat]
	if !ok {
		return 0
	}
	total := 0
	for _, act := range pi.Actions {
		total += act.Amount
	}
	return total
}

// updateHandRange updates the hand range table for a hand
func (c *Calculator) updateHandRange(table *HandRangeTable, h *parser.Hand, pi *parser.PlayerHandInfo, pos parser.Position) {
	card1 := pi.HoleCards[0]
	card2 := pi.HoleCards[1]

	ri1, ok1 := RankIndex[card1.Rank]
	ri2, ok2 := RankIndex[card2.Rank]
	if !ok1 || !ok2 {
		return
	}

	suited := card1.Suit == card2.Suit
	isPair := card1.Rank == card2.Rank

	// Determine row/col for 13x13 grid
	// Convention: row = higher rank index, col = lower rank index
	// For suited: row < col (upper triangle)
	// For offsuit: row > col (lower triangle)
	// For pairs: row == col (diagonal)
	row, col := ri1, ri2
	if ri1 > ri2 {
		row, col = ri2, ri1
	}
	// Now row <= col (row is higher card, col is lower card)

	var r, c2 int
	if isPair {
		r, c2 = row, col // diagonal
	} else if suited {
		// Suited: upper triangle (row < col in our indexing means higher rank, lower rank)
		// Convention: suited hands are in upper-right triangle
		r, c2 = row, col
	} else {
		// Offsuit: lower triangle
		r, c2 = col, row
	}

	cell := table.Cells[r][c2]
	if cell == nil {
		cell = &HandRangeCell{
			Rank1:       RankOrder[row],
			Rank2:       RankOrder[col],
			Suited:      suited,
			IsPair:      isPair,
			ByPosition:  make(map[parser.Position]*HandRangePositionCell),
			ByHandClass: make(map[string]*HandClassStats),
		}
		table.Cells[r][c2] = cell
	}

	cell.Dealt++
	if pfAction, ok := preflopRangeActionSummary(h, pi); ok {
		cell.Actions[pfAction]++
		table.TotalActions[pfAction]++
	}

	classes := handClasses(h, pi)
	if len(classes) > 0 {
		overallAction, overallOK := overallActionSummary(h, pi)
		for _, className := range classes {
			cellClass := cell.ByHandClass[className]
			if cellClass == nil {
				cellClass = &HandClassStats{}
				cell.ByHandClass[className] = cellClass
			}
			cellClass.Hands++
			if overallOK {
				cellClass.Actions[overallAction]++
			}

			hcs := table.ByHandClass[className]
			if hcs == nil {
				hcs = &HandClassStats{}
				table.ByHandClass[className] = hcs
			}
			hcs.Hands++
			if overallOK {
				hcs.Actions[overallAction]++
			}
		}
	}

	if pi.Won {
		cell.Won++
	}

	// Per-position
	ppc := cell.ByPosition[pos]
	if ppc == nil {
		ppc = &HandRangePositionCell{}
		cell.ByPosition[pos] = ppc
	}
	ppc.Dealt++
	if pfAction, ok := preflopRangeActionSummary(h, pi); ok {
		ppc.Actions[pfAction]++
	}
	if pi.Won {
		ppc.Won++
	}
}

func preflopRangeActionSummary(h *parser.Hand, pi *parser.PlayerHandInfo) (RangeActionBucket, bool) {
	if pi == nil {
		return RangeActionCheck, false
	}

	lastAction := parser.ActionUnknown
	lastAmount := 0
	for _, act := range pi.Actions {
		if act.Street != parser.StreetPreFlop {
			continue
		}
		switch act.Action {
		case parser.ActionBlindSB, parser.ActionBlindBB:
			continue
		case parser.ActionCheck, parser.ActionCall, parser.ActionBet, parser.ActionRaise, parser.ActionFold, parser.ActionAllIn:
			lastAction = act.Action
			lastAmount = act.Amount
		}
	}

	if lastAction == parser.ActionUnknown {
		if pi.FoldedPF {
			return RangeActionFold, true
		}
		if pi.PFR || pi.ThreeBet {
			return RangeActionBetHalf, true
		}
		if pi.VPIP {
			return RangeActionCall, true
		}
		return RangeActionCheck, false
	}

	switch lastAction {
	case parser.ActionFold:
		return RangeActionFold, true
	case parser.ActionCheck:
		return RangeActionCheck, true
	case parser.ActionCall:
		return RangeActionCall, true
	case parser.ActionBet, parser.ActionRaise, parser.ActionAllIn:
		return bucketByBBMultiple(lastAmount, bbAmountFromHand(h)), true
	}

	return RangeActionCheck, false
}

func overallActionSummary(h *parser.Hand, pi *parser.PlayerHandInfo) (RangeActionBucket, bool) {
	if pi == nil {
		return RangeActionCheck, false
	}

	lastAction := parser.ActionUnknown
	lastAmount := 0
	for _, act := range pi.Actions {
		switch act.Action {
		case parser.ActionBlindSB, parser.ActionBlindBB:
			continue
		case parser.ActionCheck, parser.ActionCall, parser.ActionBet, parser.ActionRaise, parser.ActionFold, parser.ActionAllIn:
			lastAction = act.Action
			lastAmount = act.Amount
		}
	}

	if lastAction == parser.ActionUnknown {
		return RangeActionCheck, false
	}

	switch lastAction {
	case parser.ActionFold:
		return RangeActionFold, true
	case parser.ActionCheck:
		return RangeActionCheck, true
	case parser.ActionCall:
		return RangeActionCall, true
	case parser.ActionBet, parser.ActionRaise, parser.ActionAllIn:
		return bucketByPotFraction(lastAmount, h), true
	default:
		return RangeActionCheck, false
	}
}

func bbAmountFromHand(h *parser.Hand) int {
	if h == nil || h.BBSeat < 0 {
		return 0
	}
	bb := h.Players[h.BBSeat]
	if bb == nil {
		return 0
	}
	for _, act := range bb.Actions {
		if act.Action == parser.ActionBlindBB && act.Amount > 0 {
			return act.Amount
		}
	}
	return 0
}

func bucketByBBMultiple(amount, bb int) RangeActionBucket {
	if amount <= 0 {
		return RangeActionCheck
	}
	if bb <= 0 {
		bb = 20
	}
	multiple := float64(amount) / float64(bb)
	switch {
	case multiple <= 2.5:
		return RangeActionBetSmall
	case multiple <= 4.0:
		return RangeActionBetHalf
	case multiple <= 6.0:
		return RangeActionBetTwoThird
	case multiple <= 10.0:
		return RangeActionBetPot
	default:
		return RangeActionBetOver
	}
}

func bucketByPotFraction(amount int, h *parser.Hand) RangeActionBucket {
	if amount <= 0 {
		return RangeActionCheck
	}
	pot := 0
	if h != nil {
		pot = h.TotalPot
	}
	if pot <= 0 {
		return RangeActionBetHalf
	}
	ratio := float64(amount) / float64(pot)
	switch {
	case ratio <= 0.38:
		return RangeActionBetSmall
	case ratio <= 0.58:
		return RangeActionBetHalf
	case ratio <= 0.78:
		return RangeActionBetTwoThird
	case ratio <= 1.15:
		return RangeActionBetPot
	default:
		return RangeActionBetOver
	}
}

// newHandRangeTable initializes the 13x13 hand range table with empty cells
func newHandRangeTable() *HandRangeTable {
	t := &HandRangeTable{}
	for i := 0; i < 13; i++ {
		for j := 0; j < 13; j++ {
			r1, r2 := i, j
			// Ensure r1 <= r2 (higher rank first in rank index)
			suited := false
			isPair := i == j
			if !isPair {
				if i < j {
					suited = true // upper triangle = suited
				} else {
					suited = false // lower triangle = offsuit
					// For display, ranks: row=higher rank idx, col=lower rank idx
				}
			}
			rank1 := RankOrder[min13(i, j)]
			rank2 := RankOrder[max13(i, j)]
			_ = r1
			_ = r2
			t.Cells[i][j] = &HandRangeCell{
				Rank1:       rank1,
				Rank2:       rank2,
				Suited:      suited && !isPair,
				IsPair:      isPair,
				ByPosition:  make(map[parser.Position]*HandRangePositionCell),
				ByHandClass: make(map[string]*HandClassStats),
			}
		}
	}
	t.ByHandClass = make(map[string]*HandClassStats)
	return t
}

func min13(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max13(a, b int) int {
	if a > b {
		return a
	}
	return b
}
