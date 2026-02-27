package parser

func (p *Parser) Clone() *Parser {
	if p == nil {
		return nil
	}

	clone := &Parser{
		result:                cloneParseResult(p.result),
		handIDCounter:         p.handIDCounter,
		inPokerWorld:          p.inPokerWorld,
		worldDetected:         p.worldDetected,
		currentStreet:         p.currentStreet,
		streetBetAmount:       p.streetBetAmount,
		streetBets:            cloneMap(p.streetBets),
		foldedThisHand:        cloneMap(p.foldedThisHand),
		pendingWinners:        append([]pendingWin(nil), p.pendingWinners...),
		lastTimestamp:         p.lastTimestamp,
		pendingLocalSeat:      p.pendingLocalSeat,
		lastBlindSeat:         p.lastBlindSeat,
		pfActions:             append([]pfAction(nil), p.pfActions...),
		currentWorldID:        p.currentWorldID,
		currentWorldName:      p.currentWorldName,
		currentInstanceUID:    p.currentInstanceUID,
		currentInstanceType:   p.currentInstanceType,
		currentInstanceOwner:  p.currentInstanceOwner,
		currentInstanceRegion: p.currentInstanceRegion,
		currentInstanceUsers:  cloneMap(p.currentInstanceUsers),
	}
	clone.pendingLocalCards = append([]Card(nil), p.pendingLocalCards...)
	clone.currentHand = cloneHand(p.currentHand)
	if clone.currentHand != nil {
		clone.result.CurrentHand = clone.currentHand
	}
	return clone
}

func cloneParseResult(in ParseResult) ParseResult {
	out := in
	out.Hands = make([]*Hand, 0, len(in.Hands))
	for _, hand := range in.Hands {
		out.Hands = append(out.Hands, cloneHand(hand))
	}
	if in.CurrentHand != nil {
		out.CurrentHand = cloneHand(in.CurrentHand)
	}
	return out
}

// CloneHand returns a fully independent deep copy of h.
// It is exported so that packages that already import parser (e.g. persistence)
// can reuse this instead of maintaining their own copy of the same logic.
func CloneHand(h *Hand) *Hand { return cloneHand(h) }

func cloneHand(h *Hand) *Hand {
	if h == nil {
		return nil
	}
	copyHand := *h
	copyHand.CommunityCards = append([]Card(nil), h.CommunityCards...)
	copyHand.ActiveSeats = append([]int(nil), h.ActiveSeats...)
	if len(h.ActiveSeatSet) > 0 {
		copyHand.ActiveSeatSet = make(map[int]struct{}, len(h.ActiveSeatSet))
		for seat := range h.ActiveSeatSet {
			copyHand.ActiveSeatSet[seat] = struct{}{}
		}
	}
	copyHand.InstanceUsers = append([]InstanceUser(nil), h.InstanceUsers...)
	copyHand.Anomalies = append([]HandAnomaly(nil), h.Anomalies...)
	copyHand.Players = make(map[int]*PlayerHandInfo, len(h.Players))
	for seat, pi := range h.Players {
		copyHand.Players[seat] = clonePlayerInfo(pi)
	}
	return &copyHand
}

func clonePlayerInfo(pi *PlayerHandInfo) *PlayerHandInfo {
	if pi == nil {
		return nil
	}
	copyPI := *pi
	copyPI.HoleCards = append([]Card(nil), pi.HoleCards...)
	copyPI.Actions = append([]PlayerAction(nil), pi.Actions...)
	return &copyPI
}

func cloneMap[K comparable, V any](in map[K]V) map[K]V {
	if len(in) == 0 {
		return make(map[K]V)
	}
	out := make(map[K]V, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
