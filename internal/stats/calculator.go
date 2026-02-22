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

type metricAccumulator struct {
	counts       map[MetricID]int
	opps         map[MetricID]int
	aggPostflop  int
	callPostflop int
	foldPostflop int
	bbNet        float64
	bbHands      int
}

func newMetricAccumulator() *metricAccumulator {
	return &metricAccumulator{
		counts: make(map[MetricID]int),
		opps:   make(map[MetricID]int),
	}
}

func (m *metricAccumulator) incOpp(id MetricID) {
	m.opps[id]++
}

func (m *metricAccumulator) incCount(id MetricID) {
	m.counts[id]++
}

func (m *metricAccumulator) consumeHand(h *parser.Hand, pi *parser.PlayerHandInfo, invested int) {
	if h == nil || pi == nil {
		return
	}

	// Hand-frequency metrics
	m.incOpp(MetricVPIP)
	if pi.VPIP {
		m.incCount(MetricVPIP)
	}

	m.incOpp(MetricPFR)
	if pi.PFR {
		m.incCount(MetricPFR)
	}

	if hasRFIOpportunityApprox(pi, h) {
		m.incOpp(MetricRFI)
	}
	if didRFIApprox(pi, h) {
		m.incCount(MetricRFI)
	}

	m.incOpp(MetricWonWithoutSD)
	if pi.Won && !pi.ShowedDown {
		m.incCount(MetricWonWithoutSD)
	}

	bb := bbAmountFromHand(h)
	if bb > 0 {
		m.bbNet += float64(pi.PotWon-invested) / float64(bb)
		m.bbHands++
	}

	// Situational metrics already explicitly tracked by parser
	if hasThreeBetOpportunityApprox(pi, h) {
		m.incOpp(MetricThreeBet)
	}
	if pi.ThreeBet {
		m.incCount(MetricThreeBet)
	}

	if hasFourBetOpportunityApprox(pi, h) {
		m.incOpp(MetricFourBet)
	}
	if didFourBetApprox(pi, h) {
		m.incCount(MetricFourBet)
	}

	if hasSqueezeOpportunityApprox(pi, h) {
		m.incOpp(MetricSqueeze)
	}
	if didSqueezeApprox(pi, h) {
		m.incCount(MetricSqueeze)
	}

	if hasFoldToThreeBetOpportunityApprox(pi, h) {
		m.incOpp(MetricFoldToThreeBet)
	}
	if pi.FoldTo3Bet {
		m.incCount(MetricFoldToThreeBet)
	}

	if isStealOpportunity(pi, h) {
		m.incOpp(MetricSteal)
		if isStealAttempt(pi) {
			m.incCount(MetricSteal)
		}
	}

	if isFoldToStealOpportunity(pi, h) {
		m.incOpp(MetricFoldToSteal)
		if pi.FoldedPF {
			m.incCount(MetricFoldToSteal)
		}
	}
	if isFoldToStealOpportunityByPosition(pi, h, parser.PosBB) {
		m.incOpp(MetricFoldBBToSteal)
		if pi.FoldedPF {
			m.incCount(MetricFoldBBToSteal)
		}
	}
	if isFoldToStealOpportunityByPosition(pi, h, parser.PosSB) {
		m.incOpp(MetricFoldSBToSteal)
		if pi.FoldedPF {
			m.incCount(MetricFoldSBToSteal)
		}
	}

	if isThreeBetVsStealOpportunity(pi, h) {
		m.incOpp(MetricThreeBetVsSteal)
		if didThreeBetVsSteal(pi, h) {
			m.incCount(MetricThreeBetVsSteal)
		}
	}

	sawFlop := sawFlop(pi, h)
	if sawFlop {
		m.incOpp(MetricWTSD)
		if pi.ShowedDown {
			m.incCount(MetricWTSD)
		}

		m.incOpp(MetricWWSF)
		if pi.Won {
			m.incCount(MetricWWSF)
		}
	}

	if pi.ShowedDown {
		m.incOpp(MetricWSD)
		if pi.Won {
			m.incCount(MetricWSD)
		}
	}

	flopAgg := hasAggressionOnStreet(pi, parser.StreetFlop)
	turnAgg := hasAggressionOnStreet(pi, parser.StreetTurn)

	if pi.PFR && len(h.CommunityCards) >= 3 {
		m.incOpp(MetricFlopCBet)
		if flopAgg {
			m.incCount(MetricFlopCBet)
		}
	}

	if pi.PFR && len(h.CommunityCards) >= 4 && flopAgg {
		m.incOpp(MetricTurnCBet)
		if turnAgg {
			m.incCount(MetricTurnCBet)
		}
	}

	if pi.PFR && len(h.CommunityCards) >= 4 && !flopAgg {
		m.incOpp(MetricDelayedCBet)
		if turnAgg {
			m.incCount(MetricDelayedCBet)
		}
	}

	if !pi.PFR && len(h.CommunityCards) >= 3 && hasOpponentAggressionOnStreet(h, pi.SeatID, parser.StreetFlop) && actedOnStreet(pi, parser.StreetFlop) {
		m.incOpp(MetricFoldToFlopCBet)
		if hasFoldOnStreet(pi, parser.StreetFlop) {
			m.incCount(MetricFoldToFlopCBet)
		}
	}
	if !pi.PFR && len(h.CommunityCards) >= 4 && hasOpponentAggressionOnStreet(h, pi.SeatID, parser.StreetTurn) && actedOnStreet(pi, parser.StreetTurn) {
		m.incOpp(MetricFoldToTurnCBet)
		if hasFoldOnStreet(pi, parser.StreetTurn) {
			m.incCount(MetricFoldToTurnCBet)
		}
	}
	agg, call, fold := postFlopActionCounts(pi)
	m.aggPostflop += agg
	m.callPostflop += call
	m.foldPostflop += fold
}

func (m *metricAccumulator) finalize(s *Stats) {
	if s == nil {
		return
	}

	// Derived count for gap (VPIP - PFR)
	m.opps[MetricGap] = m.opps[MetricVPIP]
	if vpipCount, ok := m.counts[MetricVPIP]; ok {
		m.counts[MetricGap] = vpipCount - m.counts[MetricPFR]
	}

	for _, def := range metricRegistry {
		threshold := confidenceThreshold(def.SampleClass)
		rate := metricRate(def.ID, m.counts[def.ID], m.opps[def.ID], m)
		count := m.counts[def.ID]
		opp := m.opps[def.ID]
		s.Metrics[def.ID] = MetricValue{
			ID:          def.ID,
			Count:       count,
			Opportunity: opp,
			Rate:        rate,
			Confident:   opp >= threshold,
			MinSample:   threshold,
			Format:      def.Format,
		}
	}
}

func metricRate(id MetricID, count, opp int, m *metricAccumulator) float64 {
	switch id {
	case MetricAFq:
		total := m.aggPostflop + m.callPostflop + m.foldPostflop
		if total == 0 {
			return 0
		}
		m.opps[MetricAFq] = total
		m.counts[MetricAFq] = m.aggPostflop
		return float64(m.aggPostflop) / float64(total) * 100
	case MetricAF:
		total := m.aggPostflop + m.callPostflop
		m.opps[MetricAF] = total
		m.counts[MetricAF] = m.aggPostflop
		if m.callPostflop == 0 {
			if m.aggPostflop > 0 {
				return float64(m.aggPostflop)
			}
			return 0
		}
		return float64(m.aggPostflop) / float64(m.callPostflop)
	case MetricGap:
		if opp == 0 {
			return 0
		}
		return float64(m.counts[MetricVPIP]-m.counts[MetricPFR]) / float64(opp) * 100
	case MetricBBPer100:
		m.opps[MetricBBPer100] = m.bbHands
		if m.bbHands == 0 {
			return 0
		}
		return m.bbNet / float64(m.bbHands) * 100
	default:
		if opp == 0 {
			return 0
		}
		return float64(count) / float64(opp) * 100
	}
}

func sawFlop(pi *parser.PlayerHandInfo, h *parser.Hand) bool {
	if pi == nil || h == nil {
		return false
	}
	if len(h.CommunityCards) < 3 {
		return false
	}
	if pi.FoldedPF {
		return false
	}
	return true
}

func hasAggressionOnStreet(pi *parser.PlayerHandInfo, street parser.Street) bool {
	if pi == nil {
		return false
	}
	for _, a := range pi.Actions {
		if a.Street != street {
			continue
		}
		if a.Action == parser.ActionBet || a.Action == parser.ActionRaise || a.Action == parser.ActionAllIn {
			return true
		}
	}
	return false
}

func hasCallOnStreet(pi *parser.PlayerHandInfo, street parser.Street) bool {
	if pi == nil {
		return false
	}
	for _, a := range pi.Actions {
		if a.Street == street && a.Action == parser.ActionCall {
			return true
		}
	}
	return false
}

func hasFoldOnStreet(pi *parser.PlayerHandInfo, street parser.Street) bool {
	if pi == nil {
		return false
	}
	for _, a := range pi.Actions {
		if a.Street == street && a.Action == parser.ActionFold {
			return true
		}
	}
	return false
}

func actedOnStreet(pi *parser.PlayerHandInfo, street parser.Street) bool {
	if pi == nil {
		return false
	}
	for _, a := range pi.Actions {
		if a.Street == street {
			return true
		}
	}
	return false
}

func hasOpponentAggressionOnStreet(h *parser.Hand, localSeat int, street parser.Street) bool {
	if h == nil {
		return false
	}
	for seat, p := range h.Players {
		if p == nil || seat == localSeat {
			continue
		}
		for _, a := range p.Actions {
			if a.Street != street {
				continue
			}
			if a.Action == parser.ActionBet || a.Action == parser.ActionRaise || a.Action == parser.ActionAllIn {
				return true
			}
		}
	}
	return false
}

func postFlopActionCounts(pi *parser.PlayerHandInfo) (agg, call, fold int) {
	if pi == nil {
		return 0, 0, 0
	}
	for _, a := range pi.Actions {
		if a.Street == parser.StreetPreFlop || a.Street == parser.StreetShowdown {
			continue
		}
		switch a.Action {
		case parser.ActionBet, parser.ActionRaise, parser.ActionAllIn:
			agg++
		case parser.ActionCall:
			call++
		case parser.ActionFold:
			fold++
		}
	}
	return agg, call, fold
}

func participatedPostFlop(pi *parser.PlayerHandInfo) bool {
	if pi == nil {
		return false
	}
	for _, a := range pi.Actions {
		if a.Street != parser.StreetPreFlop && a.Street != parser.StreetShowdown {
			return true
		}
	}
	return false
}

func hasCheckRaiseApprox(pi *parser.PlayerHandInfo) bool {
	if pi == nil {
		return false
	}
	checkedByStreet := map[parser.Street]bool{}
	for _, a := range pi.Actions {
		if a.Street == parser.StreetPreFlop || a.Street == parser.StreetShowdown {
			continue
		}
		if a.Action == parser.ActionCheck {
			checkedByStreet[a.Street] = true
		}
		if checkedByStreet[a.Street] && (a.Action == parser.ActionRaise || a.Action == parser.ActionBet || a.Action == parser.ActionAllIn) {
			return true
		}
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
