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
	VPIPHands               int // Hands where player voluntarily put money in pot
	PFRHands                int // Hands where player raised pre-flop
	ThreeBetHands           int // Hands where player 3-bet
	ThreeBetOpportunities   int
	FoldTo3BetHands         int
	FoldTo3BetOpportunities int

	// Financial
	TotalPotWon   int
	TotalInvested int // total chips put into pots

	// Position breakdown
	ByPosition map[parser.Position]*PositionStats

	// Hand range data
	HandRange *HandRangeTable

	// Registry-driven metrics (new framework, legacy fields remain above)
	Metrics map[MetricID]MetricValue
}

// PositionStats holds stats for a specific position
type PositionStats struct {
	Position      parser.Position
	Hands         int
	Won           int
	VPIP          int
	PFR           int
	ThreeBet      int
	ThreeBetOpp   int
	FoldTo3Bet    int
	FoldTo3BetOpp int
	Showdowns     int
	WonShowdowns  int
	PotWon        int
	Invested      int
}

// HandRangeTable is a 13x13 matrix of hand combo statistics
type HandRangeTable struct {
	// Cells[rank1][rank2] where rank1 >= rank2 (suited) or rank1 < rank2 (offsuit)
	// Standard 13x13 poker range table
	// Rows/cols: A, K, Q, J, T, 9, 8, 7, 6, 5, 4, 3, 2
	Cells [13][13]*HandRangeCell

	// Action totals across the whole range
	TotalActions [RangeActionBucketCount]int

	// Action totals grouped by hand classes/draw classes
	ByHandClass map[string]*HandClassStats
}

// RangeActionBucket is the action taxonomy used in range visualization.
type RangeActionBucket int

const (
	RangeActionCheck RangeActionBucket = iota
	RangeActionCall
	RangeActionBetSmall
	RangeActionBetHalf
	RangeActionBetTwoThird
	RangeActionBetPot
	RangeActionBetOver
	RangeActionFold
	RangeActionBucketCount
)

var RangeActionLabels = [RangeActionBucketCount]string{
	"Check",
	"Call",
	"Bet <=1/3",
	"Bet ~1/2",
	"Bet ~2/3",
	"Bet ~Pot",
	"Overbet",
	"Fold",
}

type HandClassStats struct {
	Hands   int
	Actions [RangeActionBucketCount]int
}

// HandRangeCell holds statistics for a specific hand combo (e.g., AKs, 77, T9o)
type HandRangeCell struct {
	Rank1  string // higher rank
	Rank2  string // lower rank (same as Rank1 for pairs)
	Suited bool   // true if suited, false if offsuit (only applies when Rank1 != Rank2)
	IsPair bool

	// Action counts
	Dealt   int
	Actions [RangeActionBucketCount]int
	Won     int // times won

	// Position breakdown for this combo
	ByPosition map[parser.Position]*HandRangePositionCell

	// Action totals grouped by hand classes/draw classes for this combo only
	ByHandClass map[string]*HandClassStats
}

// HandRangePositionCell holds per-position stats for a hand combo
type HandRangePositionCell struct {
	Dealt   int
	Actions [RangeActionBucketCount]int
	Won     int
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
func (c *HandRangeCell) ActionRate(action RangeActionBucket) float64 {
	if c == nil || c.Dealt == 0 {
		return 0
	}
	if action < 0 || action >= RangeActionBucketCount {
		return 0
	}
	return float64(c.Actions[action]) / float64(c.Dealt) * 100
}

func (hcs *HandClassStats) ActionRate(action RangeActionBucket) float64 {
	if hcs == nil || hcs.Hands == 0 {
		return 0
	}
	if action < 0 || action >= RangeActionBucketCount {
		return 0
	}
	return float64(hcs.Actions[action]) / float64(hcs.Hands) * 100
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

// Metric returns a computed metric by id.
func (s *Stats) Metric(id MetricID) (MetricValue, bool) {
	if s == nil || s.Metrics == nil {
		return MetricValue{}, false
	}
	m, ok := s.Metrics[id]
	return m, ok
}
