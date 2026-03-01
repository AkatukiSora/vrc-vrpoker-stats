package stats

import "github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"

// IncrementalCalculator accumulates hand statistics incrementally.
// Feed hands one by one, then call Compute to get the current Stats.
// This avoids re-scanning the full hand history on every update.
type IncrementalCalculator struct {
	localSeat int
	s         *Stats
	ma        *metricAccumulator
	calc      *Calculator
}

// NewIncrementalCalculator creates a new incremental calculator for the given local seat.
func NewIncrementalCalculator(localSeat int) *IncrementalCalculator {
	return &IncrementalCalculator{
		localSeat: localSeat,
		s: &Stats{
			ByPosition: make(map[parser.Position]*PositionStats),
			HandRange:  newHandRangeTable(),
			Metrics:    make(map[MetricID]MetricValue),
		},
		ma:   newMetricAccumulator(),
		calc: NewCalculator(),
	}
}

// Feed processes a single hand into the accumulator.
// Only complete, stats-eligible hands are processed; others are silently skipped.
func (ic *IncrementalCalculator) Feed(h *parser.Hand) {
	if h == nil || !h.IsComplete || !h.IsStatsEligible() {
		return
	}

	handSeat := ic.localSeat
	if h.LocalPlayerSeat >= 0 {
		handSeat = h.LocalPlayerSeat
	}
	if handSeat < 0 {
		return
	}

	localInfo, ok := h.Players[handSeat]
	if !ok {
		return
	}

	s := ic.s
	s.TotalHands++

	pos := localInfo.Position
	ps := ic.calc.ensurePositionStats(s, pos)
	ps.Hands++

	// Financial
	invested := ic.calc.investedAmount(h, handSeat)
	ps.Invested += invested
	ps.PotWon += localInfo.PotWon
	s.TotalPotWon += localInfo.PotWon
	s.TotalInvested += invested

	if localInfo.Participated && localInfo.Won {
		s.WonHands++
		ps.Won++
	}
	if localInfo.VPIP {
		s.VPIPHands++
		ps.VPIP++
	}
	if localInfo.PFR {
		s.PFRHands++
		ps.PFR++
	}
	if hasThreeBetOpportunityApprox(localInfo, h) {
		s.ThreeBetOpportunities++
		ps.ThreeBetOpp++
	}
	if localInfo.ThreeBet {
		s.ThreeBetHands++
		ps.ThreeBet++
	}
	if hasFoldToThreeBetOpportunityApprox(localInfo, h) {
		s.FoldTo3BetOpportunities++
		ps.FoldTo3BetOpp++
	}
	if localInfo.FoldTo3Bet {
		s.FoldTo3BetHands++
		ps.FoldTo3Bet++
	}
	if localInfo.ShowedDown {
		s.ShowdownHands++
		ps.Showdowns++
		if localInfo.Participated && localInfo.Won {
			s.WonShowdowns++
			ps.WonShowdowns++
		}
	}
	if len(localInfo.HoleCards) == 2 {
		ic.calc.updateHandRange(s.HandRange, h, localInfo, pos)
	}

	ic.ma.consumeHand(h, localInfo, invested)
}

// Compute returns the current aggregated Stats with all metrics finalized.
// The returned value is a snapshot; subsequent Feed calls do not affect it.
func (ic *IncrementalCalculator) Compute() *Stats {
	// Copy the accumulated stats and finalize metrics into the copy.
	// We finalize into a temporary copy so the accumulator remains valid for
	// future Feed calls.
	out := &Stats{
		TotalHands:              ic.s.TotalHands,
		WonHands:                ic.s.WonHands,
		ShowdownHands:           ic.s.ShowdownHands,
		WonShowdowns:            ic.s.WonShowdowns,
		VPIPHands:               ic.s.VPIPHands,
		PFRHands:                ic.s.PFRHands,
		ThreeBetHands:           ic.s.ThreeBetHands,
		ThreeBetOpportunities:   ic.s.ThreeBetOpportunities,
		FoldTo3BetHands:         ic.s.FoldTo3BetHands,
		FoldTo3BetOpportunities: ic.s.FoldTo3BetOpportunities,
		TotalPotWon:             ic.s.TotalPotWon,
		TotalInvested:           ic.s.TotalInvested,
		ByPosition:              clonePositionStats(ic.s.ByPosition),
		HandRange:               cloneHandRangeTable(ic.s.HandRange),
		Metrics:                 make(map[MetricID]MetricValue),
	}
	// Clone the accumulator for finalization so the original is not mutated.
	maClone := ic.ma.clone()
	maClone.finalize(out)
	return out
}

// HandCount returns the number of eligible hands processed so far.
func (ic *IncrementalCalculator) HandCount() int {
	return ic.s.TotalHands
}

func clonePositionStats(in map[parser.Position]*PositionStats) map[parser.Position]*PositionStats {
	if len(in) == 0 {
		return make(map[parser.Position]*PositionStats)
	}
	out := make(map[parser.Position]*PositionStats, len(in))
	for k, v := range in {
		if v == nil {
			out[k] = nil
			continue
		}
		copyPS := *v
		out[k] = &copyPS
	}
	return out
}

func cloneHandRangeTable(in *HandRangeTable) *HandRangeTable {
	if in == nil {
		return nil
	}
	copyTable := *in
	if in.ByHandClass != nil {
		copyTable.ByHandClass = make(map[string]*HandClassStats, len(in.ByHandClass))
		for k, v := range in.ByHandClass {
			if v == nil {
				copyTable.ByHandClass[k] = nil
				continue
			}
			copyStat := *v
			copyTable.ByHandClass[k] = &copyStat
		}
	}
	if in.ComboIndex != nil {
		copyTable.ComboIndex = make(map[string]*HandRangeCell, len(in.ComboIndex))
	}
	for r := 0; r < 13; r++ {
		for c := 0; c < 13; c++ {
			cell := in.Cells[r][c]
			if cell == nil {
				copyTable.Cells[r][c] = nil
				continue
			}
			copyCell := *cell
			if cell.ByPosition != nil {
				copyCell.ByPosition = make(map[parser.Position]*HandRangePositionCell, len(cell.ByPosition))
				for pos, v := range cell.ByPosition {
					if v == nil {
						copyCell.ByPosition[pos] = nil
						continue
					}
					copyPos := *v
					copyCell.ByPosition[pos] = &copyPos
				}
			}
			if cell.ByHandClass != nil {
				copyCell.ByHandClass = make(map[string]*HandClassStats, len(cell.ByHandClass))
				for className, v := range cell.ByHandClass {
					if v == nil {
						copyCell.ByHandClass[className] = nil
						continue
					}
					copyStat := *v
					copyCell.ByHandClass[className] = &copyStat
				}
			}
			copyTable.Cells[r][c] = &copyCell
			if copyTable.ComboIndex != nil {
				copyTable.ComboIndex[copyCell.ComboKey()] = &copyCell
			}
		}
	}
	return &copyTable
}
