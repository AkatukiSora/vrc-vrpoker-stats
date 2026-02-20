package parser

import (
	"bufio"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Log line patterns
var (
	// Timestamp prefix: "2026.02.21 00:18:29 Debug      -  "
	reTimestamp = regexp.MustCompile(`^(\d{4}\.\d{2}\.\d{2} \d{2}:\d{2}:\d{2}) \w+\s+-\s+(.+)$`)

	// World events
	reWorldJoining = regexp.MustCompile(`Joining (wrld_[a-f0-9-]+)`)
	reWorldLeaving = regexp.MustCompile(`\[Behaviour\] OnLeftRoom`)

	// Table events
	reNewGame        = regexp.MustCompile(`\[Table\]: Preparing for New Game`)
	reNewCommunity   = regexp.MustCompile(`\[Table\]: New Community Card: (.+)`)
	reFoldToOne      = regexp.MustCompile(`\[Table\]: Fold to One Condition`)
	reNextPhase      = regexp.MustCompile(`\[Table\]: Next phase\.True - (\d+)`)
	reCollectingBets = regexp.MustCompile(`\[Table\]: Collecting Bets`)

	// Seat events
	reDrawLocalHole = regexp.MustCompile(`\[Seat\]: Draw Local Hole Cards: (.+)`)
	reSBBet         = regexp.MustCompile(`\[Seat\]: Player (\d+) SB BET IN = (\d+)`)
	reBBBet         = regexp.MustCompile(`\[Seat\]: Player (\d+) BB BET IN = (\d+)`)
	rePlayerFolded  = regexp.MustCompile(`\[Seat\]: Player (\d+) Folded\.`)
	rePlayerEndTurn = regexp.MustCompile(`\[Seat\]: Player (\d+) End Turn with BET IN = (\d+)`)
	reShowHoleCards = regexp.MustCompile(`\[Seat\]: Player (\d+) Show hole cards: (.+)`)

	// Pot events
	rePotWinner  = regexp.MustCompile(`\[Pot\]: Winner: (\d+) Pot Amount: (\d+)`)
	rePotManager = regexp.MustCompile(`\[PotManager\]: All players folded, player (\d+) won (\d+)`)
)

const timeLayout = "2006.01.02 15:04:05"

// Parser holds state for incremental log parsing
type Parser struct {
	result          ParseResult
	currentHand     *Hand
	handIDCounter   int
	inPokerWorld    bool
	currentStreet   Street
	streetBetAmount int // highest bet in current street (for raise/call detection)
	foldedThisHand  map[int]bool
	pendingWinners  []pendingWin
	lastTimestamp   time.Time
	streetBets      map[int]int // per-player committed amount in current street
	pfActions       []pfAction  // pre-flop action sequence for 3bet detection
	// pending local hole cards (arrive after Draw Local Hole Cards before we know seat)
	pendingLocalCards []Card
	lastBlindSeat     int // most recent SB or BB seat logged (for local seat hinting)
}

type pendingWin struct {
	seatID int
	amount int
}

type pfAction struct {
	seatID int
	action ActionType
	amount int
}

// NewParser creates a new log parser
func NewParser() *Parser {
	return &Parser{
		result: ParseResult{
			LocalPlayerSeat: -1,
		},
		foldedThisHand: make(map[int]bool),
		streetBets:     make(map[int]int),
	}
}

// ParseLine processes a single log line and updates state
func (p *Parser) ParseLine(line string) error {
	m := reTimestamp.FindStringSubmatch(line)
	if m == nil {
		return nil
	}

	ts, err := time.Parse(timeLayout, m[1])
	if err != nil {
		return nil
	}
	p.lastTimestamp = ts
	msg := strings.TrimSpace(m[2])

	// --- World detection ---
	if wm := reWorldJoining.FindStringSubmatch(msg); wm != nil {
		worldID := wm[1]
		p.inPokerWorld = (worldID == VRPokerWorldID)
		p.result.InPokerWorld = p.inPokerWorld
		return nil
	}
	if reWorldLeaving.MatchString(msg) {
		if p.inPokerWorld {
			// Finalize any ongoing hand when leaving
			p.finalizeCurrentHand()
		}
		p.inPokerWorld = false
		p.result.InPokerWorld = false
		return nil
	}

	// Only track poker events in VRPoker world.
	// But for historical full-log parsing, we detect poker events regardless
	// and filter by VRPokerWorldID presence in the log overall.
	return p.processPokerEvent(ts, msg)
}

func (p *Parser) processPokerEvent(ts time.Time, msg string) error {
	// === New game start ===
	if reNewGame.MatchString(msg) {
		p.finalizeCurrentHand()
		p.startNewHand(ts)
		return nil
	}

	if p.currentHand == nil {
		return nil
	}

	// === Draw local hole cards ===
	// These arrive right after "Preparing for New Game" (sometimes after SB/BB lines).
	// Strategy to identify local seat:
	// 1. Already known from previous hand → use it
	// 2. SB posted before this line → SB seat might be us
	// 3. Store as pending, resolve at "Show hole cards" matching
	if m := reDrawLocalHole.FindStringSubmatch(msg); m != nil {
		cards, err := parseCards(m[1])
		if err != nil {
			return nil
		}
		p.pendingLocalCards = cards

		localSeat := p.result.LocalPlayerSeat
		if localSeat >= 0 {
			// Already identified in a previous hand
			p.assignLocalCards(localSeat, cards)
		} else if p.lastBlindSeat >= 0 {
			// The most recently logged blind (SB or BB) is likely the local player.
			// "Draw Local Hole Cards" arrives immediately after the local player's blind post,
			// so the last-logged blind seat is us.
			// This will be corrected later if "Show hole cards" reveals a different match.
			p.assignLocalCards(p.lastBlindSeat, cards)
		}
		// If neither known, wait for Show hole cards to confirm
		return nil
	}

	// === SB blind ===
	if m := reSBBet.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		amount, _ := strconv.Atoi(m[2])
		p.currentHand.SBSeat = seat
		p.ensurePlayer(seat)
		pi := p.currentHand.Players[seat]
		pi.Actions = append(pi.Actions, PlayerAction{
			Timestamp: ts, PlayerID: seat,
			Street: StreetPreFlop, Action: ActionBlindSB, Amount: amount,
		})
		p.streetBets[seat] = amount
		if p.streetBetAmount < amount {
			p.streetBetAmount = amount
		}
		p.pfActions = append(p.pfActions, pfAction{seat, ActionBlindSB, amount})
		p.lastBlindSeat = seat
		return nil
	}

	// === BB blind ===
	if m := reBBBet.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		amount, _ := strconv.Atoi(m[2])
		p.currentHand.BBSeat = seat
		p.ensurePlayer(seat)
		pi := p.currentHand.Players[seat]
		pi.Actions = append(pi.Actions, PlayerAction{
			Timestamp: ts, PlayerID: seat,
			Street: StreetPreFlop, Action: ActionBlindBB, Amount: amount,
		})
		p.streetBets[seat] = amount
		if p.streetBetAmount < amount {
			p.streetBetAmount = amount
		}
		p.pfActions = append(p.pfActions, pfAction{seat, ActionBlindBB, amount})
		p.lastBlindSeat = seat
		return nil
	}

	// === Community card ===
	if m := reNewCommunity.FindStringSubmatch(msg); m != nil {
		card, err := parseCard(strings.TrimSpace(m[1]))
		if err != nil {
			return nil
		}
		p.currentHand.CommunityCards = append(p.currentHand.CommunityCards, card)
		switch len(p.currentHand.CommunityCards) {
		case 1, 2, 3:
			if p.currentStreet < StreetFlop {
				p.currentStreet = StreetFlop
			}
		case 4:
			p.currentStreet = StreetTurn
		case 5:
			p.currentStreet = StreetRiver
		}
		return nil
	}

	// === Next phase / Collecting bets: street boundary ===
	if reNextPhase.MatchString(msg) || reCollectingBets.MatchString(msg) {
		p.streetBets = make(map[int]int)
		p.streetBetAmount = 0
		return nil
	}

	// === Player folded ===
	if m := rePlayerFolded.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		p.foldedThisHand[seat] = true
		p.ensurePlayer(seat)
		pi := p.currentHand.Players[seat]
		pi.Actions = append(pi.Actions, PlayerAction{
			Timestamp: ts, PlayerID: seat,
			Street: p.currentStreet, Action: ActionFold, Amount: 0,
		})
		if p.currentStreet == StreetPreFlop {
			pi.FoldedPF = true
			p.pfActions = append(p.pfActions, pfAction{seat, ActionFold, 0})
		}
		return nil
	}

	// === Player end turn (bet/call/check/raise) ===
	if m := rePlayerEndTurn.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		amount, _ := strconv.Atoi(m[2])

		if p.foldedThisHand[seat] {
			// Ignore "End Turn" that comes after Folded (redundant log line)
			return nil
		}

		p.ensurePlayer(seat)
		action := p.classifyAction(seat, amount)

		pi := p.currentHand.Players[seat]
		pi.Actions = append(pi.Actions, PlayerAction{
			Timestamp: ts, PlayerID: seat,
			Street: p.currentStreet, Action: action, Amount: amount,
		})

		if amount > p.streetBetAmount {
			p.streetBetAmount = amount
		}
		p.streetBets[seat] = amount

		if p.currentStreet == StreetPreFlop {
			p.pfActions = append(p.pfActions, pfAction{seat, action, amount})
		}
		return nil
	}

	// === Show hole cards (showdown) ===
	if m := reShowHoleCards.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		cards, err := parseCards(m[2])
		if err != nil {
			return nil
		}
		p.ensurePlayer(seat)
		pi := p.currentHand.Players[seat]
		pi.ShowedDown = true
		p.currentStreet = StreetShowdown

		// If this matches pending local cards → this player is us
		if p.pendingLocalCards != nil && cardsMatch(p.pendingLocalCards, cards) {
			// Local seat confirmed
			if p.result.LocalPlayerSeat < 0 {
				p.result.LocalPlayerSeat = seat
			}
			p.assignLocalCards(seat, cards)
		} else if len(pi.HoleCards) == 0 {
			pi.HoleCards = cards
		}
		return nil
	}

	// === Fold to one ===
	if reFoldToOne.MatchString(msg) {
		p.currentStreet = StreetShowdown
		return nil
	}

	// === Pot winner (showdown split pot or multi-winner) ===
	if m := rePotWinner.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		amount, _ := strconv.Atoi(m[2])
		p.pendingWinners = append(p.pendingWinners, pendingWin{seat, amount})
		if p.currentHand.WinType == "" {
			p.currentHand.WinType = "showdown"
		}
		return nil
	}

	// === PotManager (all folded, one winner) ===
	if m := rePotManager.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		amount, _ := strconv.Atoi(m[2])
		p.pendingWinners = append(p.pendingWinners, pendingWin{seat, amount})
		p.currentHand.WinType = "fold"
		return nil
	}

	return nil
}

// assignLocalCards sets hole cards for the local player's seat
func (p *Parser) assignLocalCards(seat int, cards []Card) {
	p.ensurePlayer(seat)
	pi := p.currentHand.Players[seat]
	if pi != nil {
		pi.HoleCards = cards
	}
	p.currentHand.LocalPlayerSeat = seat
	p.result.LocalPlayerSeat = seat
	p.pendingLocalCards = nil
}

// cardsMatch returns true if two card slices have the same cards (order-insensitive)
func cardsMatch(a, b []Card) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Rank != b[i].Rank || a[i].Suit != b[i].Suit {
			return false
		}
	}
	return true
}

// classifyAction determines the action type based on bet amount vs current street state
func (p *Parser) classifyAction(seat, amount int) ActionType {
	prevCommitted := p.streetBets[seat]

	if amount == 0 {
		// Zero bet: check (no one bet before) or call of 0 (edge case)
		if p.streetBetAmount == 0 || prevCommitted == p.streetBetAmount {
			return ActionCheck
		}
		return ActionCheck
	}

	// Compare to current street max bet
	if p.streetBetAmount == 0 {
		// First bet in street
		return ActionBet
	}
	if amount > p.streetBetAmount {
		return ActionRaise
	}
	if amount == p.streetBetAmount {
		return ActionCall
	}
	// Partial call (all-in for less)
	return ActionCall
}

// ensurePlayer creates player entry if not exists
func (p *Parser) ensurePlayer(seat int) {
	if p.currentHand == nil {
		return
	}
	if _, ok := p.currentHand.Players[seat]; !ok {
		p.currentHand.Players[seat] = &PlayerHandInfo{SeatID: seat}
		// Only add to ActiveSeats once
		found := false
		for _, s := range p.currentHand.ActiveSeats {
			if s == seat {
				found = true
				break
			}
		}
		if !found {
			p.currentHand.ActiveSeats = append(p.currentHand.ActiveSeats, seat)
		}
	}
}

// startNewHand initializes a new hand
func (p *Parser) startNewHand(ts time.Time) {
	p.handIDCounter++
	localSeat := p.result.LocalPlayerSeat
	p.currentHand = &Hand{
		ID:              p.handIDCounter,
		StartTime:       ts,
		LocalPlayerSeat: localSeat,
		Players:         make(map[int]*PlayerHandInfo),
		SBSeat:          -1,
		BBSeat:          -1,
		WinnerSeat:      -1,
	}
	p.currentStreet = StreetPreFlop
	p.streetBets = make(map[int]int)
	p.streetBetAmount = 0
	p.pfActions = nil
	p.foldedThisHand = make(map[int]bool)
	p.pendingWinners = nil
	p.pendingLocalCards = nil
	p.lastBlindSeat = -1
}

// finalizeCurrentHand completes the current hand and adds to results
func (p *Parser) finalizeCurrentHand() {
	if p.currentHand == nil {
		return
	}

	h := p.currentHand

	// If local cards are still pending (no showdown match), assign to known local seat
	if p.pendingLocalCards != nil && p.result.LocalPlayerSeat >= 0 {
		p.assignLocalCards(p.result.LocalPlayerSeat, p.pendingLocalCards)
	}

	// Apply pending winners
	totalPot := 0
	for _, pw := range p.pendingWinners {
		p.ensurePlayer(pw.seatID)
		h.Players[pw.seatID].Won = true
		h.Players[pw.seatID].PotWon += pw.amount
		totalPot += pw.amount
		h.WinnerSeat = pw.seatID
	}
	h.TotalPot = totalPot

	if h.WinType == "" {
		if len(h.CommunityCards) >= 3 {
			h.WinType = "showdown"
		} else {
			h.WinType = "fold"
		}
	}

	// Count players
	h.NumPlayers = len(h.ActiveSeats)

	// Assign positions (requires SB/BB to be known)
	p.assignPositions(h)

	// Calculate pre-flop stats for all players
	p.calculatePreflopStats(h)

	h.EndTime = p.lastTimestamp
	// Mark as complete only if we have winners or community cards
	h.IsComplete = len(p.pendingWinners) > 0 || len(h.CommunityCards) > 0

	// Only keep hands where local player participated
	localSeat := h.LocalPlayerSeat
	if localSeat >= 0 {
		if _, ok := h.Players[localSeat]; ok {
			p.result.Hands = append(p.result.Hands, h)
		}
	}

	p.currentHand = nil
	p.pendingLocalCards = nil
}

// assignPositions assigns poker positions based on SB/BB
func (p *Parser) assignPositions(h *Hand) {
	if h.SBSeat < 0 || h.BBSeat < 0 {
		// Try to infer from actions
		return
	}

	allSeats := make([]int, len(h.ActiveSeats))
	copy(allSeats, h.ActiveSeats)
	sortInts(allSeats)

	// Find SB in sorted list
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

	// Rotate so SB is first
	rotated := append(allSeats[sbIdx:], allSeats[:sbIdx]...)
	n := len(rotated)
	positions := positionOrder(n)

	for i, seat := range rotated {
		if pi, ok := h.Players[seat]; ok {
			if i < len(positions) {
				pi.Position = positions[i]
			}
		}
	}
}

// positionOrder returns position assignments for n players starting from SB
func positionOrder(n int) []Position {
	switch n {
	case 2:
		return []Position{PosSB, PosBTN} // HU: SB = BTN acts first PF
	case 3:
		return []Position{PosSB, PosBB, PosBTN}
	case 4:
		return []Position{PosSB, PosBB, PosUTG, PosBTN}
	case 5:
		return []Position{PosSB, PosBB, PosUTG, PosMP, PosBTN}
	case 6:
		return []Position{PosSB, PosBB, PosUTG, PosMP, PosCO, PosBTN}
	case 7:
		return []Position{PosSB, PosBB, PosUTG, PosUTG1, PosMP, PosCO, PosBTN}
	case 8:
		return []Position{PosSB, PosBB, PosUTG, PosUTG1, PosMP, PosMP1, PosCO, PosBTN}
	default:
		result := make([]Position, n)
		if n > 0 {
			result[0] = PosSB
		}
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

// calculatePreflopStats computes VPIP/PFR/3bet for all players
func (p *Parser) calculatePreflopStats(h *Hand) {
	if len(p.pfActions) == 0 {
		return
	}

	// Track bet levels:
	// 1 = blind level (SB/BB)
	// 2 = first open raise (PFR)
	// 3 = 3-bet
	// 4 = 4-bet, etc.
	betLevel := 1 // blinds count as level 1
	firstRaiserSeat := -1

	for _, act := range p.pfActions {
		pi, ok := h.Players[act.seatID]
		if !ok {
			continue
		}
		isBB := act.seatID == h.BBSeat
		isSB := act.seatID == h.SBSeat

		switch act.action {
		case ActionBlindSB, ActionBlindBB:
			// blinds = level 1, no voluntary action

		case ActionBet, ActionRaise, ActionAllIn:
			betLevel++
			switch betLevel {
			case 2: // open raise
				firstRaiserSeat = act.seatID
				pi.PFR = true
				pi.VPIP = true
			case 3: // 3-bet
				pi.ThreeBet = true
				pi.VPIP = true
			default:
				pi.VPIP = true
			}

		case ActionCall:
			// Calling is VPIP unless it's BB calling a limp (amount == BB)
			if isSB || !isBB {
				pi.VPIP = true
			} else if isBB && act.amount > 0 {
				// BB calling a raise = VPIP
				pi.VPIP = true
			}

		case ActionCheck:
			// BB checking is NOT VPIP

		case ActionFold:
			// Fold to 3-bet: if the first raiser folds after a 3-bet
			if betLevel >= 3 && act.seatID == firstRaiserSeat {
				pi.FoldTo3Bet = true
			}
		}
		_ = isSB
	}
}

// ParseReader parses all lines from a reader (first pass: full file)
func ParseReader(r io.Reader) (*ParseResult, error) {
	pr := NewParser()

	// Pre-scan to detect if VRPoker world is visited
	// For simplicity: just parse all poker events regardless of world
	// (the watcher filters by world; ParseReader is for historical analysis)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		_ = pr.ParseLine(scanner.Text())
	}

	pr.finalizeCurrentHand()
	result := pr.result
	return &result, scanner.Err()
}

// parseCards parses a comma-separated card string like "7d, 9c"
func parseCards(s string) ([]Card, error) {
	parts := strings.Split(s, ",")
	cards := make([]Card, 0, len(parts))
	for _, part := range parts {
		card, err := parseCard(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	return cards, nil
}

// parseCard parses a single card like "7d", "10h", "Ah"
func parseCard(s string) (Card, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return Card{}, nil
	}
	suit := string(s[len(s)-1])
	rank := s[:len(s)-1]

	validSuits := map[string]bool{"h": true, "d": true, "c": true, "s": true}
	validRanks := map[string]bool{
		"2": true, "3": true, "4": true, "5": true, "6": true,
		"7": true, "8": true, "9": true, "10": true,
		"J": true, "Q": true, "K": true, "A": true,
	}

	if !validSuits[suit] || !validRanks[rank] {
		return Card{}, nil
	}
	return Card{Rank: rank, Suit: suit}, nil
}

func sortInts(s []int) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// GetLocalSeat returns the detected local player's seat number
func (p *Parser) GetLocalSeat() int {
	return p.result.LocalPlayerSeat
}

// GetHands returns all completed hands
func (p *Parser) GetHands() []*Hand {
	hands := make([]*Hand, len(p.result.Hands))
	copy(hands, p.result.Hands)
	return hands
}

// GetCurrentHand returns the hand currently in progress (may be nil)
func (p *Parser) GetCurrentHand() *Hand {
	return p.currentHand
}
