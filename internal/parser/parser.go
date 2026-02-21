package parser

import (
	"bufio"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	reTimestamp = regexp.MustCompile(`^(\d{4}\.\d{2}\.\d{2} \d{2}:\d{2}:\d{2}) \w+\s+-\s+(.+)$`)

	reWorldJoining = regexp.MustCompile(`Joining (wrld_[a-f0-9-]+)`)
	reWorldLeaving = regexp.MustCompile(`\[Behaviour\] OnLeftRoom`)

	reNewGame        = regexp.MustCompile(`\[Table\]: Preparing for New Game`)
	reNewCommunity   = regexp.MustCompile(`\[Table\]: New Community Card: (.+)`)
	reFoldToOne      = regexp.MustCompile(`\[Table\]: Fold to One Condition`)
	reNextPhase      = regexp.MustCompile(`\[Table\]: Next phase\.True - (\d+)`)
	reCollectingBets = regexp.MustCompile(`\[Table\]: Collecting Bets`)
	reNewMinBet      = regexp.MustCompile(`\[Table\]: New Min Bet: (\d+)`)

	reDrawLocalHole = regexp.MustCompile(`\[Seat\]: Draw Local Hole Cards: (.+)`)
	reSBBet         = regexp.MustCompile(`\[Seat\]: Player (\d+) SB BET IN = (\d+)`)
	reBBBet         = regexp.MustCompile(`\[Seat\]: Player (\d+) BB BET IN = (\d+)`)
	rePlayerFolded  = regexp.MustCompile(`\[Seat\]: Player (\d+) Folded\.`)
	rePlayerEndTurn = regexp.MustCompile(`\[Seat\]: Player (\d+) End Turn with BET IN = (\d+)`)
	reShowHoleCards = regexp.MustCompile(`\[Seat\]: Player (\d+) Show hole cards: (.+)`)
	reLocalSeatID   = regexp.MustCompile(`\[Manager\]: Local Seat Assigned\. ID: (\d+)`)

	rePotWinner  = regexp.MustCompile(`\[Pot\]: Winner: (\d+) Pot Amount: (\d+)`)
	rePotManager = regexp.MustCompile(`\[PotManager\]: All players folded, player (\d+) won (\d+)`)
)

const timeLayout = "2006.01.02 15:04:05"

// Parser holds state for incremental log parsing
type Parser struct {
	result        ParseResult
	currentHand   *Hand
	handIDCounter int
	inPokerWorld  bool
	worldDetected bool
	currentStreet Street

	// Per-street tracking
	streetBetAmount int         // current highest bet in the street
	streetBets      map[int]int // total committed by each player this street
	foldedThisHand  map[int]bool

	pendingWinners    []pendingWin
	lastTimestamp     time.Time
	pendingLocalCards []Card
	pendingLocalSeat  int
	lastBlindSeat     int

	// Pre-flop action sequence for 3bet/fold-to-3bet detection
	pfActions []pfAction
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

func NewParser() *Parser {
	return &Parser{
		result:           ParseResult{LocalPlayerSeat: -1},
		foldedThisHand:   make(map[int]bool),
		streetBets:       make(map[int]int),
		lastBlindSeat:    -1,
		pendingLocalSeat: -1,
	}
}

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

	// World detection
	if wm := reWorldJoining.FindStringSubmatch(msg); wm != nil {
		p.worldDetected = true
		p.inPokerWorld = (wm[1] == VRPokerWorldID)
		p.result.InPokerWorld = p.inPokerWorld
		return nil
	}
	if reWorldLeaving.MatchString(msg) {
		p.worldDetected = true
		if p.inPokerWorld {
			p.finalizeCurrentHand()
		}
		p.inPokerWorld = false
		p.result.InPokerWorld = false
		p.result.LocalPlayerSeat = -1
		return nil
	}

	if p.worldDetected && !p.inPokerWorld {
		return nil
	}

	return p.processPokerEvent(ts, msg)
}

func (p *Parser) processPokerEvent(ts time.Time, msg string) error {
	// === New game ===
	if reNewGame.MatchString(msg) {
		p.finalizeCurrentHand()
		p.startNewHand(ts)
		return nil
	}

	if m := reLocalSeatID.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		p.result.LocalPlayerSeat = seat
		if p.currentHand != nil {
			p.currentHand.LocalPlayerSeat = seat
			if p.pendingLocalCards != nil {
				p.assignLocalCards(seat, p.pendingLocalCards, true)
			}
		}
		return nil
	}

	if p.currentHand == nil {
		return nil
	}

	// === Draw local hole cards ===
	if m := reDrawLocalHole.FindStringSubmatch(msg); m != nil {
		cards, err := parseCards(m[1])
		if err != nil || len(cards) != 2 {
			return nil
		}
		p.pendingLocalCards = cards
		p.pendingLocalSeat = -1

		localSeat := p.result.LocalPlayerSeat
		if p.lastBlindSeat >= 0 {
			// Tentative: last blind poster is likely us
			// Will be corrected by Local Seat Assigned / Show hole cards.
			p.assignLocalCards(p.lastBlindSeat, cards, false)
		} else if localSeat >= 0 {
			p.assignLocalCards(localSeat, cards, false)
		}
		// If no blind seen yet, will be resolved at Show hole cards
		return nil
	}

	// === SB blind ===
	if m := reSBBet.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		amount, _ := strconv.Atoi(m[2])
		p.currentHand.SBSeat = seat
		p.lastBlindSeat = seat
		p.ensurePlayer(seat)
		pi := p.currentHand.Players[seat]
		pi.Actions = append(pi.Actions, PlayerAction{
			Timestamp: ts, PlayerID: seat,
			Street: StreetPreFlop, Action: ActionBlindSB, Amount: amount,
		})
		p.streetBets[seat] = amount
		if amount > p.streetBetAmount {
			p.streetBetAmount = amount
		}
		p.pfActions = append(p.pfActions, pfAction{seat, ActionBlindSB, amount})
		return nil
	}

	// === BB blind ===
	if m := reBBBet.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		amount, _ := strconv.Atoi(m[2])
		p.currentHand.BBSeat = seat
		p.lastBlindSeat = seat
		p.ensurePlayer(seat)
		pi := p.currentHand.Players[seat]
		pi.Actions = append(pi.Actions, PlayerAction{
			Timestamp: ts, PlayerID: seat,
			Street: StreetPreFlop, Action: ActionBlindBB, Amount: amount,
		})
		p.streetBets[seat] = amount
		if amount > p.streetBetAmount {
			p.streetBetAmount = amount
		}
		p.pfActions = append(p.pfActions, pfAction{seat, ActionBlindBB, amount})
		return nil
	}

	// === New Min Bet (raise announced) ===
	// This reliably tells us the current bet level in the street
	if m := reNewMinBet.FindStringSubmatch(msg); m != nil {
		amount, _ := strconv.Atoi(m[1])
		if amount > p.streetBetAmount {
			p.streetBetAmount = amount
		}
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

	// === Street boundary: Next phase / Collecting bets ===
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

	// === Player End Turn ===
	if m := rePlayerEndTurn.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		amount, _ := strconv.Atoi(m[2])

		// Skip redundant End Turn after Fold
		if p.foldedThisHand[seat] {
			return nil
		}

		p.ensurePlayer(seat)
		action := p.classifyAction(seat, amount)

		pi := p.currentHand.Players[seat]
		pi.Actions = append(pi.Actions, PlayerAction{
			Timestamp: ts, PlayerID: seat,
			Street: p.currentStreet, Action: action, Amount: amount,
		})

		p.streetBets[seat] = amount
		if amount > p.streetBetAmount {
			p.streetBetAmount = amount
		}

		if p.currentStreet == StreetPreFlop {
			p.pfActions = append(p.pfActions, pfAction{seat, action, amount})
		}
		return nil
	}

	// === Show hole cards ===
	if m := reShowHoleCards.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		cards, err := parseCards(m[2])
		if err != nil || len(cards) != 2 {
			return nil
		}
		p.ensurePlayer(seat)
		pi := p.currentHand.Players[seat]
		pi.ShowedDown = true
		p.currentStreet = StreetShowdown

		// If the shown cards match our pending local cards → confirm seat
		if p.pendingLocalCards != nil && cardsMatch(p.pendingLocalCards, cards) {
			if p.result.LocalPlayerSeat < 0 {
				p.result.LocalPlayerSeat = seat
			}
			p.assignLocalCards(seat, cards, true)
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

	// === Pot winner (showdown) ===
	if m := rePotWinner.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		amount, _ := strconv.Atoi(m[2])
		p.pendingWinners = append(p.pendingWinners, pendingWin{seat, amount})
		if p.currentHand.WinType == "" {
			p.currentHand.WinType = "showdown"
		}
		return nil
	}

	// === PotManager (fold win) ===
	if m := rePotManager.FindStringSubmatch(msg); m != nil {
		seat, _ := strconv.Atoi(m[1])
		amount, _ := strconv.Atoi(m[2])
		p.pendingWinners = append(p.pendingWinners, pendingWin{seat, amount})
		p.currentHand.WinType = "fold"
		return nil
	}

	return nil
}

func (p *Parser) assignLocalCards(seat int, cards []Card, confirmed bool) {
	if p.currentHand == nil {
		return
	}
	p.ensurePlayer(seat)
	pi := p.currentHand.Players[seat]
	if pi != nil {
		pi.HoleCards = cards
	}
	p.currentHand.LocalPlayerSeat = seat
	p.pendingLocalSeat = seat
	if p.result.LocalPlayerSeat < 0 {
		p.result.LocalPlayerSeat = seat
	}
	if confirmed {
		p.result.LocalPlayerSeat = seat
		p.pendingLocalCards = nil
		p.pendingLocalSeat = -1
	}
}

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

func (p *Parser) classifyAction(seat, amount int) ActionType {
	prevCommitted := p.streetBets[seat]
	if amount == 0 {
		return ActionCheck
	}
	if p.streetBetAmount == 0 {
		return ActionBet // first aggressor
	}
	if amount > p.streetBetAmount {
		return ActionRaise
	}
	if amount == p.streetBetAmount && amount > prevCommitted {
		return ActionCall
	}
	// amount <= streetBetAmount but player already committed some:
	// Could be completing a blind or partial call
	if amount > prevCommitted {
		return ActionCall
	}
	return ActionCheck
}

func (p *Parser) ensurePlayer(seat int) {
	if p.currentHand == nil {
		return
	}
	if _, ok := p.currentHand.Players[seat]; !ok {
		p.currentHand.Players[seat] = &PlayerHandInfo{SeatID: seat}
		for _, s := range p.currentHand.ActiveSeats {
			if s == seat {
				return
			}
		}
		p.currentHand.ActiveSeats = append(p.currentHand.ActiveSeats, seat)
	}
}

func (p *Parser) startNewHand(ts time.Time) {
	p.handIDCounter++
	p.currentHand = &Hand{
		ID:              p.handIDCounter,
		StartTime:       ts,
		LocalPlayerSeat: p.result.LocalPlayerSeat,
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

func (p *Parser) finalizeCurrentHand() {
	if p.currentHand == nil {
		return
	}
	h := p.currentHand

	// Resolve pending local cards
	if p.pendingLocalCards != nil && p.result.LocalPlayerSeat >= 0 {
		seat := p.pendingLocalSeat
		if seat < 0 {
			seat = p.result.LocalPlayerSeat
		}
		p.assignLocalCards(seat, p.pendingLocalCards, true)
	}

	// Apply winners
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

	h.NumPlayers = len(h.ActiveSeats)
	if h.SBSeat < 0 || h.BBSeat < 0 {
		p.inferBlindsFromPreflop(h)
	}
	p.assignPositions(h)
	p.calculatePreflopStats(h)

	h.EndTime = p.lastTimestamp
	h.IsComplete = len(p.pendingWinners) > 0 || len(h.CommunityCards) > 0

	p.result.Hands = append(p.result.Hands, h)

	p.currentHand = nil
	p.pendingLocalCards = nil
	p.pendingLocalSeat = -1
}

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

	if h.SBSeat >= 0 && h.BBSeat < 0 {
		sbIdx := -1
		for i, s := range seats {
			if s == h.SBSeat {
				sbIdx = i
				break
			}
		}
		if sbIdx >= 0 {
			h.BBSeat = seats[(sbIdx+1)%len(seats)]
			return true
		}
	}
	if h.BBSeat >= 0 && h.SBSeat < 0 {
		bbIdx := -1
		for i, s := range seats {
			if s == h.BBSeat {
				bbIdx = i
				break
			}
		}
		if bbIdx >= 0 {
			h.SBSeat = seats[(bbIdx-1+len(seats))%len(seats)]
			return true
		}
	}

	firstSeat := -1
	for _, act := range p.pfActions {
		switch act.action {
		case ActionBlindSB, ActionBlindBB:
			continue
		case ActionFold, ActionCheck, ActionCall, ActionBet, ActionRaise, ActionAllIn:
			firstSeat = act.seatID
		}
		if firstSeat >= 0 {
			break
		}
	}
	if firstSeat < 0 {
		return false
	}

	idx := -1
	for i, s := range seats {
		if s == firstSeat {
			idx = i
			break
		}
	}
	if idx < 0 || len(seats) < 2 {
		return false
	}

	bbIdx := (idx - 1 + len(seats)) % len(seats)
	sbIdx := (idx - 2 + len(seats)) % len(seats)

	h.BBSeat = seats[bbIdx]
	h.SBSeat = seats[sbIdx]
	return true
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
		return []Position{PosSB, PosBB, PosUTG, PosMP, PosCO, PosBTN}
	case 7:
		return []Position{PosSB, PosBB, PosUTG, PosUTG1, PosMP, PosCO, PosBTN}
	case 8:
		return []Position{PosSB, PosBB, PosUTG, PosUTG1, PosMP, PosMP1, PosCO, PosBTN}
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

func ParseReader(r io.Reader) (*ParseResult, error) {
	pr := NewParser()
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		_ = pr.ParseLine(scanner.Text())
	}
	pr.finalizeCurrentHand()
	result := pr.result
	return &result, scanner.Err()
}

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

func (p *Parser) GetLocalSeat() int { return p.result.LocalPlayerSeat }
func (p *Parser) GetHands() []*Hand {
	h := make([]*Hand, len(p.result.Hands))
	copy(h, p.result.Hands)
	return h
}
func (p *Parser) GetCurrentHand() *Hand { return p.currentHand }
