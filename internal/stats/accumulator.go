package stats

import "github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"

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

	// Compute the preflop action sequence once and reuse it for all checks
	// that need it, avoiding redundant iteration and re-sorting.
	pfc := newPreflopHandContext(h)

	// Hand-frequency metrics
	m.incOpp(MetricVPIP)
	if pi.VPIP {
		m.incCount(MetricVPIP)
	}

	m.incOpp(MetricPFR)
	if pi.PFR {
		m.incCount(MetricPFR)
	}

	if hasRFIOpportunityApprox(pi, pfc.seq) {
		m.incOpp(MetricRFI)
	}
	if didRFIApprox(pi, pfc.seq) {
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
	if didFourBetApprox(pi, h, pfc.seq) {
		m.incCount(MetricFourBet)
	}

	if hasSqueezeOpportunityApprox(pi, pfc.seq) {
		m.incOpp(MetricSqueeze)
	}
	if didSqueezeApprox(pi, pfc.seq) {
		m.incCount(MetricSqueeze)
	}

	if hasFoldToThreeBetOpportunityApprox(pi, h) {
		m.incOpp(MetricFoldToThreeBet)
	}
	if pi.FoldTo3Bet {
		m.incCount(MetricFoldToThreeBet)
	}

	if isStealOpportunity(pi, pfc.seq) {
		m.incOpp(MetricSteal)
		if isStealAttempt(pi) {
			m.incCount(MetricSteal)
		}
	}

	if isFoldToStealOpportunity(pi, pfc) {
		m.incOpp(MetricFoldToSteal)
		if pi.FoldedPF {
			m.incCount(MetricFoldToSteal)
		}
	}
	if isFoldToStealOpportunityByPosition(pi, pfc, parser.PosBB) {
		m.incOpp(MetricFoldBBToSteal)
		if pi.FoldedPF {
			m.incCount(MetricFoldBBToSteal)
		}
	}
	if isFoldToStealOpportunityByPosition(pi, pfc, parser.PosSB) {
		m.incOpp(MetricFoldSBToSteal)
		if pi.FoldedPF {
			m.incCount(MetricFoldSBToSteal)
		}
	}

	if isThreeBetVsStealOpportunity(pi, pfc) {
		m.incOpp(MetricThreeBetVsSteal)
		if didThreeBetVsSteal(pi, pfc) {
			m.incCount(MetricThreeBetVsSteal)
		}
	}

	sawFlopResult := sawFlop(pi, h)
	if sawFlopResult {
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

	flopAgg := hasActionOnStreet(pi, parser.StreetFlop, isAggressiveAction)
	turnAgg := hasActionOnStreet(pi, parser.StreetTurn, isAggressiveAction)

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

	if !pi.PFR && len(h.CommunityCards) >= 3 && hasOpponentAggressionOnStreet(h, pi.SeatID, parser.StreetFlop) && hasActionOnStreet(pi, parser.StreetFlop, anyAction) {
		m.incOpp(MetricFoldToFlopCBet)
		if hasActionOnStreet(pi, parser.StreetFlop, isFoldAction) {
			m.incCount(MetricFoldToFlopCBet)
		}
	}
	if !pi.PFR && len(h.CommunityCards) >= 4 && hasOpponentAggressionOnStreet(h, pi.SeatID, parser.StreetTurn) && hasActionOnStreet(pi, parser.StreetTurn, anyAction) {
		m.incOpp(MetricFoldToTurnCBet)
		if hasActionOnStreet(pi, parser.StreetTurn, isFoldAction) {
			m.incCount(MetricFoldToTurnCBet)
		}
	}
	agg, call, fold := postFlopActionCounts(pi)
	m.aggPostflop += agg
	m.callPostflop += call
	m.foldPostflop += fold
}

type actionPredicate func(parser.PlayerAction) bool

func hasActionOnStreet(pi *parser.PlayerHandInfo, street parser.Street, pred actionPredicate) bool {
	if pi == nil || pred == nil {
		return false
	}
	for _, act := range pi.Actions {
		if act.Street != street {
			continue
		}
		if pred(act) {
			return true
		}
	}
	return false
}

func anyAction(act parser.PlayerAction) bool {
	return act.Action != parser.ActionUnknown
}

func isFoldAction(act parser.PlayerAction) bool {
	return act.Action == parser.ActionFold
}

func isAggressiveAction(act parser.PlayerAction) bool {
	return act.Action == parser.ActionBet || act.Action == parser.ActionRaise || act.Action == parser.ActionAllIn
}

// clone returns a shallow copy of the accumulator suitable for finalize().
// The counts and opps maps are copied so finalize() mutations do not affect the original.
func (m *metricAccumulator) clone() *metricAccumulator {
	c := &metricAccumulator{
		counts:       make(map[MetricID]int, len(m.counts)),
		opps:         make(map[MetricID]int, len(m.opps)),
		aggPostflop:  m.aggPostflop,
		callPostflop: m.callPostflop,
		foldPostflop: m.foldPostflop,
		bbNet:        m.bbNet,
		bbHands:      m.bbHands,
	}
	for k, v := range m.counts {
		c.counts[k] = v
	}
	for k, v := range m.opps {
		c.opps[k] = v
	}
	return c
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
	return hasActionOnStreet(pi, street, isAggressiveAction)
}

func hasCallOnStreet(pi *parser.PlayerHandInfo, street parser.Street) bool {
	return hasActionOnStreet(pi, street, func(act parser.PlayerAction) bool {
		return act.Action == parser.ActionCall
	})
}

func hasFoldOnStreet(pi *parser.PlayerHandInfo, street parser.Street) bool {
	return hasActionOnStreet(pi, street, isFoldAction)
}

func actedOnStreet(pi *parser.PlayerHandInfo, street parser.Street) bool {
	return hasActionOnStreet(pi, street, anyAction)
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
