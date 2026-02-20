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
	}

	for _, h := range hands {
		if !h.IsComplete {
			continue
		}

		localInfo, ok := h.Players[localSeat]
		if !ok {
			// We might not be in this hand
			continue
		}

		s.TotalHands++

		pos := localInfo.Position
		ps := c.ensurePositionStats(s, pos)
		ps.Hands++

		// Financial
		ps.Invested += c.investedAmount(h, localSeat)
		ps.PotWon += localInfo.PotWon
		s.TotalPotWon += localInfo.PotWon
		s.TotalInvested += ps.Invested

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

		// 3bet opportunity: any time there was an open raise before us PF
		if localInfo.ThreeBet || localInfo.FoldTo3Bet {
			s.ThreeBetOpportunities++
			ps.ThreeBetOpp++
		}
		if localInfo.ThreeBet {
			s.ThreeBetHands++
			ps.ThreeBet++
		}

		// Fold to 3bet
		if localInfo.FoldTo3Bet {
			s.FoldTo3BetOpportunities++
			s.FoldTo3BetHands++
			ps.FoldTo3BetOpp++
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
			c.updateHandRange(s.HandRange, localInfo, pos)
		}
	}

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
func (c *Calculator) updateHandRange(table *HandRangeTable, pi *parser.PlayerHandInfo, pos parser.Position) {
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
			Rank1:      RankOrder[row],
			Rank2:      RankOrder[col],
			Suited:     suited,
			IsPair:     isPair,
			ByPosition: make(map[parser.Position]*HandRangePositionCell),
		}
		table.Cells[r][c2] = cell
	}

	cell.Dealt++
	if pi.VPIP {
		cell.VPIP++
	}
	if pi.PFR {
		cell.PFR++
	}
	if pi.ThreeBet {
		cell.ThreeBet++
	}
	if pi.FoldedPF {
		cell.Fold++
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
	if pi.VPIP {
		ppc.VPIP++
	}
	if pi.PFR {
		ppc.PFR++
	}
	if pi.ThreeBet {
		ppc.ThreeBet++
	}
	if pi.FoldedPF {
		ppc.Fold++
	}
	if pi.Won {
		ppc.Won++
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
				Rank1:      rank1,
				Rank2:      rank2,
				Suited:     suited && !isPair,
				IsPair:     isPair,
				ByPosition: make(map[parser.Position]*HandRangePositionCell),
			}
		}
	}
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
