package stats

import "github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"

// Stats holds aggregated poker statistics
type Stats struct {
	// Hand counts
	TotalHands    int
	WonHands      int
	ShowdownHands int
	WonShowdowns  int

	// Pre-flop stats
	VPIPHands     int // Hands where player voluntarily put money in pot
	PFRHands      int // Hands where player raised pre-flop
	ThreeBetHands int // Hands where player 3-bet
	ThreeBetOpportunities int
	FoldTo3BetHands int
	FoldTo3BetOpportunities int

	// Financial
	TotalPotWon  int
	TotalInvested int // total chips put into pots

	// Position breakdown
	ByPosition map[parser.Position]*PositionStats

	// Hand range data
	HandRange *HandRangeTable
}

// PositionStats holds stats for a specific position
type PositionStats struct {
	Position  parser.Position
	Hands     int
	Won       int
	VPIP      int
	PFR       int
	ThreeBet  int
	ThreeBetOpp int
	FoldTo3Bet int
	FoldTo3BetOpp int
	Showdowns int
	WonShowdowns int
	PotWon    int
	Invested  int
}

// HandRangeTable is a 13x13 matrix of hand combo statistics
type HandRangeTable struct {
	// Cells[rank1][rank2] where rank1 >= rank2 (suited) or rank1 < rank2 (offsuit)
	// Standard 13x13 poker range table
	// Rows/cols: A, K, Q, J, T, 9, 8, 7, 6, 5, 4, 3, 2
	Cells [13][13]*HandRangeCell
}

// HandRangeCell holds statistics for a specific hand combo (e.g., AKs, 77, T9o)
type HandRangeCell struct {
	Rank1   string // higher rank
	Rank2   string // lower rank (same as Rank1 for pairs)
	Suited  bool   // true if suited, false if offsuit (only applies when Rank1 != Rank2)
	IsPair  bool

	// Action counts
	Dealt    int // times dealt this hand
	VPIP     int // times voluntarily entered pot
	PFR      int // times raised PF
	ThreeBet int // times 3-bet
	Fold     int // times folded PF
	Won      int // times won

	// Position breakdown for this combo
	ByPosition map[parser.Position]*HandRangePositionCell
}

// HandRangePositionCell holds per-position stats for a hand combo
type HandRangePositionCell struct {
	Dealt    int
	VPIP     int
	PFR      int
	ThreeBet int
	Fold     int
	Won      int
}

// RankOrder is the canonical rank order for the 13x13 grid (A=0, 2=12)
var RankOrder = []string{"A", "K", "Q", "J", "10", "9", "8", "7", "6", "5", "4", "3", "2"}

// RankIndex maps rank string to its index in RankOrder
var RankIndex = map[string]int{
	"A": 0, "K": 1, "Q": 2, "J": 3, "10": 4,
	"9": 5, "8": 6, "7": 7, "6": 8, "5": 9,
	"4": 10, "3": 11, "2": 12,
}

// ComboKey returns the hand name for a cell (e.g., "AKs", "77", "T9o")
func (c *HandRangeCell) ComboKey() string {
	if c.IsPair {
		return c.Rank1 + c.Rank2
	}
	if c.Suited {
		return c.Rank1 + c.Rank2 + "s"
	}
	return c.Rank1 + c.Rank2 + "o"
}

// Rates returns computed percentage rates
func (c *HandRangeCell) VPIPRate() float64 {
	if c.Dealt == 0 {
		return 0
	}
	return float64(c.VPIP) / float64(c.Dealt) * 100
}

func (c *HandRangeCell) PFRRate() float64 {
	if c.Dealt == 0 {
		return 0
	}
	return float64(c.PFR) / float64(c.Dealt) * 100
}

func (c *HandRangeCell) FoldRate() float64 {
	if c.Dealt == 0 {
		return 0
	}
	return float64(c.Fold) / float64(c.Dealt) * 100
}

func (c *HandRangeCell) ThreeBetRate() float64 {
	if c.Dealt == 0 {
		return 0
	}
	return float64(c.ThreeBet) / float64(c.Dealt) * 100
}

// Overall rate helpers for Stats
func (s *Stats) VPIPRate() float64 {
	if s.TotalHands == 0 {
		return 0
	}
	return float64(s.VPIPHands) / float64(s.TotalHands) * 100
}

func (s *Stats) PFRRate() float64 {
	if s.TotalHands == 0 {
		return 0
	}
	return float64(s.PFRHands) / float64(s.TotalHands) * 100
}

func (s *Stats) ThreeBetRate() float64 {
	if s.ThreeBetOpportunities == 0 {
		return 0
	}
	return float64(s.ThreeBetHands) / float64(s.ThreeBetOpportunities) * 100
}

func (s *Stats) FoldTo3BetRate() float64 {
	if s.FoldTo3BetOpportunities == 0 {
		return 0
	}
	return float64(s.FoldTo3BetHands) / float64(s.FoldTo3BetOpportunities) * 100
}

func (s *Stats) WinRate() float64 {
	if s.TotalHands == 0 {
		return 0
	}
	return float64(s.WonHands) / float64(s.TotalHands) * 100
}

func (s *Stats) WSDRate() float64 {
	if s.ShowdownHands == 0 {
		return 0
	}
	return float64(s.WonShowdowns) / float64(s.ShowdownHands) * 100
}

// PositionStats rate helpers
func (ps *PositionStats) VPIPRate() float64 {
	if ps.Hands == 0 {
		return 0
	}
	return float64(ps.VPIP) / float64(ps.Hands) * 100
}

func (ps *PositionStats) PFRRate() float64 {
	if ps.Hands == 0 {
		return 0
	}
	return float64(ps.PFR) / float64(ps.Hands) * 100
}

func (ps *PositionStats) WinRate() float64 {
	if ps.Hands == 0 {
		return 0
	}
	return float64(ps.Won) / float64(ps.Hands) * 100
}

func (ps *PositionStats) WSDRate() float64 {
	if ps.Showdowns == 0 {
		return 0
	}
	return float64(ps.WonShowdowns) / float64(ps.Showdowns) * 100
}

func (ps *PositionStats) ThreeBetRate() float64 {
	if ps.ThreeBetOpp == 0 {
		return 0
	}
	return float64(ps.ThreeBet) / float64(ps.ThreeBetOpp) * 100
}

func (ps *PositionStats) FoldTo3BetRate() float64 {
	if ps.FoldTo3BetOpp == 0 {
		return 0
	}
	return float64(ps.FoldTo3Bet) / float64(ps.FoldTo3BetOpp) * 100
}
