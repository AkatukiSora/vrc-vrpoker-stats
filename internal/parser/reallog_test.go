package parser

import (
	"fmt"
	"os"
	"testing"
)

// TestRealLog tests parsing the actual VRChat log file
// Run with: go test -v -run TestRealLog ./internal/parser/...
func TestRealLog(t *testing.T) {
	logPath := "/home/akatuki-sora/.local/share/Steam/steamapps/compatdata/438100/pfx/drive_c/users/steamuser/AppData/LocalLow/VRChat/VRChat/output_log_2026-02-20_22-38-17.txt"

	f, err := os.Open(logPath)
	if err != nil {
		t.Skipf("log file not found: %v", err)
	}
	defer f.Close()

	result, err := ParseReader(f)
	if err != nil {
		t.Fatalf("ParseReader error: %v", err)
	}

	t.Logf("Local player seat: %d", result.LocalPlayerSeat)
	t.Logf("Total complete hands: %d", len(result.Hands))

	winCount := 0
	showdownCount := 0
	vpipCount := 0
	pfrCount := 0

	for _, h := range result.Hands {
		localSeat := h.LocalPlayerSeat
		if pi, ok := h.Players[localSeat]; ok {
			if pi.Won {
				winCount++
			}
			if pi.ShowedDown {
				showdownCount++
			}
			if pi.VPIP {
				vpipCount++
			}
			if pi.PFR {
				pfrCount++
			}
		}
	}

	total := len(result.Hands)
	if total > 0 {
		t.Logf("Win rate: %d/%d = %.1f%%", winCount, total, float64(winCount)/float64(total)*100)
		t.Logf("VPIP: %d/%d = %.1f%%", vpipCount, total, float64(vpipCount)/float64(total)*100)
		t.Logf("PFR: %d/%d = %.1f%%", pfrCount, total, float64(pfrCount)/float64(total)*100)
		t.Logf("Showdowns: %d", showdownCount)
	}

	// Show last 5 hands
	start := 0
	if len(result.Hands) > 5 {
		start = len(result.Hands) - 5
	}
	for _, h := range result.Hands[start:] {
		localSeat := h.LocalPlayerSeat
		pi := h.Players[localSeat]
		cards := ""
		if pi != nil && len(pi.HoleCards) == 2 {
			cards = fmt.Sprintf("%s%s %s%s", pi.HoleCards[0].Rank, pi.HoleCards[0].Suit, pi.HoleCards[1].Rank, pi.HoleCards[1].Suit)
		}
		community := ""
		for _, c := range h.CommunityCards {
			community += c.Rank + c.Suit + " "
		}
		won := ""
		if pi != nil && pi.Won {
			won = fmt.Sprintf("WON %d", pi.PotWon)
		} else {
			won = "lost"
		}
		pos := PosUnknown
		if pi != nil {
			pos = pi.Position
		}
		t.Logf("Hand #%d [%s] [%s] Board:[%s] %s | pos=%s players=%d",
			h.ID, h.StartTime.Format("15:04:05"), cards, community, won, pos, h.NumPlayers)
	}
}
