package parser

// calculatePreflopStats computes VPIP/PFR/3Bet/FoldTo3Bet for all players.
//
// VRPoker log structure without SB/BB lines:
//
//	First End Turn with amount > 0 is treated as the "open" (like BB).
//	Subsequent raises are 2bet, 3bet, etc.
//
// With SB/BB lines:
//
//	BlindSB/BlindBB = level 1 (not voluntary)
//	First raise/bet = 2bet (PFR)
//	Second raise = 3bet
func (p *Parser) calculatePreflopStats(h *Hand) {
	if len(p.pfActions) == 0 {
		return
	}

	hasBlinds := h.SBSeat >= 0 || h.BBSeat >= 0

	// betLevel tracks aggression:
	// hasBlinds:   0=preblind, 1=after SB, 2=after BB (=open), 3=3bet, ...
	// !hasBlinds:  1=before first aggression, 2=first open raise (PFR), 3=3bet, ...
	betLevel := 0
	if hasBlinds {
		betLevel = 1 // SB counts as level 1 even if not logged
	} else {
		// No explicit blind logs in this hand. Treat first aggression as open raise (PFR).
		betLevel = 1
	}

	// Track who raised at each level for FoldTo3Bet
	raiserAtLevel := make(map[int]int) // level -> seatID of raiser

	for _, act := range p.pfActions {
		pi, ok := h.Players[act.seatID]
		if !ok {
			continue
		}

		isSB := act.seatID == h.SBSeat
		isBB := act.seatID == h.BBSeat

		switch act.action {
		case ActionBlindSB:
			betLevel = 1

		case ActionBlindBB:
			if betLevel < 2 {
				betLevel = 2
			}

		case ActionRaise, ActionBet:
			betLevel++
			raiserAtLevel[betLevel] = act.seatID
			switch betLevel {
			case 2: // open raise / first aggression
				pi.PFR = true
				pi.VPIP = true
			case 3: // 3-bet
				pi.ThreeBet = true
				pi.VPIP = true
			default: // 4bet+
				pi.VPIP = true
			}

		case ActionCall:
			// SB completing to BB is not VPIP
			// BB checking / calling raise is VPIP
			if isSB && act.amount <= p.bbAmount(h) {
				// SB completing the BB without a raise = not voluntary beyond blind
				// But calling a raise from SB = VPIP
				if betLevel > 2 {
					pi.VPIP = true
				}
				// SB posting and completing = still VPIP? No: SB is forced.
				// Only raises beyond BB are voluntary for SB
			} else if isBB && betLevel <= 2 {
				// BB calling their own blind or checking = not additional VPIP
				// BB calling a raise = VPIP
			} else {
				pi.VPIP = true
			}

		case ActionCheck:
			// BB checks when no raise = not VPIP

		case ActionFold:
			// FoldTo3Bet: player folds when facing a 3-bet
			// = they had raised (level 2 raiser) and someone 3-bet (level 3)
			if betLevel >= 3 {
				if raiserSeat, ok := raiserAtLevel[betLevel-1]; ok && raiserSeat == act.seatID {
					pi.FoldTo3Bet = true
				}
			}
		}
		_ = isSB
		_ = isBB
	}
}

// bbAmount estimates the BB amount from the hand's blind structure
func (p *Parser) bbAmount(h *Hand) int {
	if h.BBSeat >= 0 {
		if pi, ok := h.Players[h.BBSeat]; ok {
			for _, act := range pi.Actions {
				if act.Action == ActionBlindBB {
					return act.Amount
				}
			}
		}
	}
	return 0
}
