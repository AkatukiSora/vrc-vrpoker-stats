package parser

import (
	"bufio"
	"fmt"
	"io"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/logevent"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/logformat"
)

// Parser holds state for incremental log parsing
type Parser struct {
	lexer         logformat.Lexer
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

	currentWorldID        string
	currentWorldName      string
	currentInstanceUID    string
	currentInstanceType   InstanceType
	currentInstanceOwner  string
	currentInstanceRegion string
	currentInstanceUsers  map[string]string
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
	return NewParserWithLexer(nil)
}

func NewParserWithLexer(lexer logformat.Lexer) *Parser {
	if lexer == nil {
		lexer = logformat.LegacyLexer{}
	}
	return &Parser{
		lexer:                lexer,
		result:               ParseResult{LocalPlayerSeat: -1},
		foldedThisHand:       make(map[int]bool),
		streetBets:           make(map[int]int),
		lastBlindSeat:        -1,
		pendingLocalSeat:     -1,
		currentInstanceType:  InstanceTypeUnknown,
		currentInstanceUsers: make(map[string]string),
	}
}

func (p *Parser) DecodeLine(line string) logevent.Record {
	return p.lexer.Decode(line)
}

func (p *Parser) ParseLine(line string) error {
	return p.ParseRecord(p.DecodeLine(line))
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
		if p.currentHand.ActiveSeatSet == nil {
			p.currentHand.ActiveSeatSet = make(map[int]struct{})
		}
		if _, ok := p.currentHand.ActiveSeatSet[seat]; !ok {
			p.currentHand.ActiveSeats = append(p.currentHand.ActiveSeats, seat)
			p.currentHand.ActiveSeatSet[seat] = struct{}{}
		}
	}
}

func (p *Parser) startNewHand(ts time.Time) {
	p.handIDCounter++
	p.currentHand = &Hand{
		ID:               p.handIDCounter,
		StartTime:        ts,
		LocalPlayerSeat:  p.result.LocalPlayerSeat,
		WorldID:          p.currentWorldID,
		WorldDisplayName: p.currentWorldName,
		InstanceUID:      p.currentInstanceUID,
		InstanceType:     p.currentInstanceType,
		InstanceOwner:    p.currentInstanceOwner,
		InstanceRegion:   p.currentInstanceRegion,
		Players:          make(map[int]*PlayerHandInfo),
		ActiveSeatSet:    make(map[int]struct{}),
		SBSeat:           -1,
		BBSeat:           -1,
		WinnerSeat:       -1,
		StatsEligible:    true,
	}
	if len(p.currentInstanceUsers) > 0 {
		p.currentHand.InstanceUsers = make([]InstanceUser, 0, len(p.currentInstanceUsers))
		for uid, name := range p.currentInstanceUsers {
			p.currentHand.InstanceUsers = append(p.currentHand.InstanceUsers, InstanceUser{UserUID: uid, DisplayName: name})
		}
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
		h.Players[pw.seatID].PotWon += pw.amount
		totalPot += pw.amount
		h.WinnerSeat = pw.seatID
	}

	// Determine won/lost status based on profit/loss and participation
	for _, pi := range h.Players {
		invested := p.calculateInvestedAmount(pi)
		profit := pi.PotWon - invested

		// Determine participation: participated if VPIP is true OR showed down
		participated := pi.VPIP || pi.ShowedDown
		pi.Participated = participated

		// Won if: participated AND profit > 0
		if participated {
			pi.Won = profit > 0
		} else {
			pi.Won = false
		}
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
	if h.ActiveSeatSet != nil {
		h.ActiveSeatSet = nil
	}
	if h.SBSeat < 0 || h.BBSeat < 0 {
		p.inferBlindsFromPreflop(h)
	}
	p.assignPositions(h)
	p.calculatePreflopStats(h)

	h.EndTime = p.lastTimestamp
	h.IsComplete = len(p.pendingWinners) > 0 || len(h.CommunityCards) > 0
	if !validBoardCount(len(h.CommunityCards)) {
		h.addAnomaly("BOARD_COUNT_INVALID", "warn", fmt.Sprintf("board_count=%d", len(h.CommunityCards)))
	}
	if h.HasAnomaly {
		h.StatsEligible = false
	}

	p.result.Hands = append(p.result.Hands, h)

	p.currentHand = nil
	p.pendingLocalCards = nil
	p.pendingLocalSeat = -1
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
	parsed, err := logformat.ParseCards(s)
	if err != nil {
		return nil, err
	}
	cards := make([]Card, len(parsed))
	for i, card := range parsed {
		cards[i] = cardFromLogEvent(card)
	}
	return cards, nil
}

func parseCard(s string) (Card, error) {
	card, err := logformat.ParseCard(s)
	if err != nil {
		return Card{}, err
	}
	return cardFromLogEvent(card), nil
}

func (p *Parser) GetLocalSeat() int { return p.result.LocalPlayerSeat }

// IsNewHandLine reports whether line is the log line that begins a poker
// hand. Callers that need to track hand-start byte offsets can use this instead
// of duplicating the regex.
func IsNewHandLine(line string) bool {
	return logformat.LegacyLexer{}.Decode(line).HandStartHint
}

// HandCount returns the number of completed hands without allocating a slice.
// Use this for cheap change-detection before calling GetHands.
func (p *Parser) HandCount() int { return len(p.result.Hands) }

func (p *Parser) GetHands() []*Hand {
	h := make([]*Hand, len(p.result.Hands))
	copy(h, p.result.Hands)
	return h
}
func (p *Parser) GetCurrentHand() *Hand { return p.currentHand }

func (h *Hand) addAnomaly(code, severity, detail string) {
	if h == nil {
		return
	}
	for _, a := range h.Anomalies {
		if a.Code == code && a.Detail == detail {
			return
		}
	}
	h.Anomalies = append(h.Anomalies, HandAnomaly{Code: code, Severity: severity, Detail: detail})
	h.HasAnomaly = true
	h.StatsEligible = false
}

func validBoardCount(n int) bool {
	switch n {
	case 0, 3, 4, 5:
		return true
	default:
		return false
	}
}

// calculateInvestedAmount calculates the total amount a player invested in a hand
func (p *Parser) calculateInvestedAmount(pi *PlayerHandInfo) int {
	if pi == nil {
		return 0
	}
	total := 0
	for _, act := range pi.Actions {
		total += act.Amount
	}
	return total
}
