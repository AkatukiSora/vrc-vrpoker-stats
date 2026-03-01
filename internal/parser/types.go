package parser

import "time"

type InstanceType string

const (
	InstanceTypeUnknown     InstanceType = "unknown"
	InstanceTypePublic      InstanceType = "public"
	InstanceTypeFriends     InstanceType = "friends"
	InstanceTypeFriendsPlus InstanceType = "friends_plus"
	InstanceTypeInvite      InstanceType = "invite"
	InstanceTypeInvitePlus  InstanceType = "invite_plus"
	InstanceTypeGroup       InstanceType = "group"
	InstanceTypeGroupPlus   InstanceType = "group_plus"
	InstanceTypeGroupPublic InstanceType = "group_public"
)

type HandAnomaly struct {
	Code     string
	Severity string
	Detail   string
}

type InstanceUser struct {
	UserUID     string
	DisplayName string
}

// Card represents a playing card
type Card struct {
	Rank string // "A", "K", "Q", "J", "10", "2"-"9"
	Suit string // "h", "d", "c", "s"
}

func (c Card) String() string {
	return c.Rank + c.Suit
}

// Position represents a player's position at the table
type Position int

const (
	PosUnknown Position = iota
	PosSB               // Small Blind
	PosBB               // Big Blind
	PosUTG              // Under the Gun
	PosUTG1             // UTG+1
	PosMP               // Middle Position
	PosMP1              // Hijack (legacy name: MP+1)
	PosCO               // Cutoff
	PosBTN              // Button (Dealer)
)

const PosHJ Position = PosMP1

func (p Position) String() string {
	switch p {
	case PosSB:
		return "SB"
	case PosBB:
		return "BB"
	case PosUTG:
		return "UTG"
	case PosUTG1:
		return "UTG+1"
	case PosMP:
		return "MP"
	case PosMP1:
		return "HJ"
	case PosCO:
		return "CO"
	case PosBTN:
		return "BTN"
	default:
		return "?"
	}
}

// ActionType represents a player action
type ActionType int

const (
	ActionUnknown ActionType = iota
	ActionFold
	ActionCheck
	ActionCall
	ActionRaise
	ActionBet
	ActionBlindSB
	ActionBlindBB
	ActionAllIn
)

func (a ActionType) String() string {
	switch a {
	case ActionFold:
		return "Fold"
	case ActionCheck:
		return "Check"
	case ActionCall:
		return "Call"
	case ActionRaise:
		return "Raise"
	case ActionBet:
		return "Bet"
	case ActionBlindSB:
		return "SB"
	case ActionBlindBB:
		return "BB"
	case ActionAllIn:
		return "AllIn"
	default:
		return "Unknown"
	}
}

// Street represents the betting round
type Street int

const (
	StreetPreFlop Street = iota
	StreetFlop
	StreetTurn
	StreetRiver
	StreetShowdown
)

func (s Street) String() string {
	switch s {
	case StreetPreFlop:
		return "PreFlop"
	case StreetFlop:
		return "Flop"
	case StreetTurn:
		return "Turn"
	case StreetRiver:
		return "River"
	case StreetShowdown:
		return "Showdown"
	default:
		return "Unknown"
	}
}

// PlayerAction represents a single action by a player in a hand
type PlayerAction struct {
	Timestamp time.Time
	PlayerID  int // Seat number (0-7)
	Street    Street
	Action    ActionType
	Amount    int
}

// PlayerHandInfo holds per-player data within a hand
type PlayerHandInfo struct {
	SeatID     int
	HoleCards  []Card
	Position   Position
	Actions    []PlayerAction
	ShowedDown bool
	Won        bool
	PotWon     int
	// Pre-flop action summary
	VPIP         bool // Voluntarily Put money In Pot (called or raised PF, not blind)
	PFR          bool // Pre-Flop Raised
	ThreeBet     bool // 3-bet pre-flop
	FoldTo3Bet   bool // Folded to a 3-bet
	FoldedPF     bool // Folded pre-flop
	Participated bool // Participated in hand (not a pre-flop fold)
}

// Hand represents a single poker hand
type Hand struct {
	ID               int
	HandUID          string
	StartTime        time.Time
	EndTime          time.Time
	LocalPlayerSeat  int // Which seat is the local player
	WorldID          string
	WorldDisplayName string
	InstanceUID      string
	InstanceType     InstanceType
	InstanceOwner    string
	InstanceRegion   string
	CommunityCards   []Card
	Players          map[int]*PlayerHandInfo // keyed by seat number
	InstanceUsers    []InstanceUser
	SBSeat           int
	BBSeat           int
	ActiveSeats      []int // seats with players in this hand
	ActiveSeatSet    map[int]struct{}
	NumPlayers       int
	TotalPot         int
	WinnerSeat       int
	WinType          string // "fold" or "showdown"
	IsComplete       bool
	HasAnomaly       bool
	StatsEligible    bool
	Anomalies        []HandAnomaly
}

func (h *Hand) HasDataAnomaly() bool {
	if h == nil {
		return false
	}
	return h.HasAnomaly || len(h.Anomalies) > 0
}

func (h *Hand) IsStatsEligible() bool {
	if h == nil {
		return false
	}
	if h.HasDataAnomaly() {
		return false
	}
	return h.StatsEligible
}

// VRPokerWorldID is the VRChat world ID for VR Poker
const VRPokerWorldID = "wrld_aeba3422-1543-4e6f-bd9d-0f41ddc5c4f8"

// ParseResult contains the result of parsing a log file
type ParseResult struct {
	Hands           []*Hand
	CurrentHand     *Hand // Hand in progress (may be incomplete)
	LocalPlayerSeat int   // Detected local player seat
	InPokerWorld    bool
}
