package stats

import "github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"

const (
	handClassHighCard      = "High Card"
	handClassOnePair       = "One Pair"
	handClassTwoPair       = "Two Pair"
	handClassTrips         = "Trips"
	handClassStraight      = "Straight"
	handClassFlush         = "Flush"
	handClassFullHouse     = "Full House"
	handClassQuads         = "Quads"
	handClassStraightFlush = "Straight Flush"

	handClassFlushDraw        = "Flush Draw"
	handClassOESD             = "Open-Ended Draw"
	handClassGutshot          = "Gutshot"
	handClassBackdoorFlush    = "Backdoor Flush"
	handClassBackdoorStraight = "Backdoor Straight"
)

func handClasses(h *parser.Hand, pi *parser.PlayerHandInfo) []string {
	if h == nil || pi == nil || len(pi.HoleCards) != 2 {
		return nil
	}

	cards := make([]parser.Card, 0, 2+len(h.CommunityCards))
	cards = append(cards, pi.HoleCards...)
	cards = append(cards, h.CommunityCards...)

	classes := []string{madeHandClass(cards)}

	if hasFlushDraw(cards) {
		classes = append(classes, handClassFlushDraw)
	}
	if hasOpenEndedDraw(cards) {
		classes = append(classes, handClassOESD)
	} else if hasGutshotDraw(cards) {
		classes = append(classes, handClassGutshot)
	}
	if len(h.CommunityCards) == 3 {
		if hasBackdoorFlushDraw(cards) {
			classes = append(classes, handClassBackdoorFlush)
		}
		if hasBackdoorStraightDraw(cards) {
			classes = append(classes, handClassBackdoorStraight)
		}
	}

	return uniqueStrings(classes)
}

func madeHandClass(cards []parser.Card) string {
	rankCounts := make(map[int]int)
	suitToRanks := make(map[string]map[int]bool)

	for _, c := range cards {
		r := rankValue(c.Rank)
		if r == 0 {
			continue
		}
		rankCounts[r]++
		if suitToRanks[c.Suit] == nil {
			suitToRanks[c.Suit] = make(map[int]bool)
		}
		suitToRanks[c.Suit][r] = true
		if r == 14 {
			suitToRanks[c.Suit][1] = true
		}
	}

	hasTrips := false
	pairCount := 0
	hasQuads := false
	for _, c := range rankCounts {
		switch {
		case c >= 4:
			hasQuads = true
		case c >= 3:
			hasTrips = true
		case c >= 2:
			pairCount++
		}
	}

	if hasStraightFlush(suitToRanks) {
		return handClassStraightFlush
	}
	if hasQuads {
		return handClassQuads
	}
	if hasTrips && pairCount > 0 {
		return handClassFullHouse
	}
	if hasFlush(cards) {
		return handClassFlush
	}
	if hasStraight(cards) {
		return handClassStraight
	}
	if hasTrips {
		return handClassTrips
	}
	if pairCount >= 2 {
		return handClassTwoPair
	}
	if pairCount == 1 {
		return handClassOnePair
	}
	return handClassHighCard
}

func hasStraightFlush(suitToRanks map[string]map[int]bool) bool {
	for _, ranks := range suitToRanks {
		if len(ranks) < 5 {
			continue
		}
		if hasStraightInSet(ranks) {
			return true
		}
	}
	return false
}

func hasFlush(cards []parser.Card) bool {
	suitCounts := make(map[string]int)
	for _, c := range cards {
		suitCounts[c.Suit]++
		if suitCounts[c.Suit] >= 5 {
			return true
		}
	}
	return false
}

func hasFlushDraw(cards []parser.Card) bool {
	suitCounts := make(map[string]int)
	for _, c := range cards {
		suitCounts[c.Suit]++
		if suitCounts[c.Suit] == 4 {
			return true
		}
	}
	return false
}

func hasBackdoorFlushDraw(cards []parser.Card) bool {
	suitCounts := make(map[string]int)
	for _, c := range cards {
		suitCounts[c.Suit]++
	}
	for _, n := range suitCounts {
		if n == 3 {
			return true
		}
	}
	return false
}

func hasStraight(cards []parser.Card) bool {
	ranks := rankSet(cards)
	return hasStraightInSet(ranks)
}

func hasOpenEndedDraw(cards []parser.Card) bool {
	ranks := rankSet(cards)
	for start := 1; start <= 10; start++ {
		if ranks[start] && ranks[start+1] && ranks[start+2] && ranks[start+3] {
			if (start > 1 && !ranks[start-1]) || (start+4 <= 14 && !ranks[start+4]) {
				return true
			}
		}
	}
	return false
}

func hasGutshotDraw(cards []parser.Card) bool {
	ranks := rankSet(cards)
	for start := 1; start <= 10; start++ {
		missing := 0
		for i := 0; i < 5; i++ {
			if !ranks[start+i] {
				missing++
			}
		}
		if missing == 1 {
			return true
		}
	}
	return false
}

func hasBackdoorStraightDraw(cards []parser.Card) bool {
	ranks := rankSet(cards)
	for start := 1; start <= 10; start++ {
		hits := 0
		for i := 0; i < 5; i++ {
			if ranks[start+i] {
				hits++
			}
		}
		if hits >= 3 {
			return true
		}
	}
	return false
}

func rankSet(cards []parser.Card) map[int]bool {
	ranks := make(map[int]bool)
	for _, c := range cards {
		r := rankValue(c.Rank)
		if r == 0 {
			continue
		}
		ranks[r] = true
		if r == 14 {
			ranks[1] = true
		}
	}
	return ranks
}

func hasStraightInSet(ranks map[int]bool) bool {
	for start := 1; start <= 10; start++ {
		if ranks[start] && ranks[start+1] && ranks[start+2] && ranks[start+3] && ranks[start+4] {
			return true
		}
	}
	return false
}

func rankValue(rank string) int {
	switch rank {
	case "A":
		return 14
	case "K":
		return 13
	case "Q":
		return 12
	case "J":
		return 11
	case "10":
		return 10
	case "9":
		return 9
	case "8":
		return 8
	case "7":
		return 7
	case "6":
		return 6
	case "5":
		return 5
	case "4":
		return 4
	case "3":
		return 3
	case "2":
		return 2
	default:
		return 0
	}
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
