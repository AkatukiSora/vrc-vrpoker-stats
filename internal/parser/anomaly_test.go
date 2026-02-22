package parser

import (
	"strings"
	"testing"
)

const boardOverflowLog = `
2026.02.22 18:10:00 Debug      -  [Behaviour] Joining wrld_aeba3422-1543-4e6f-bd9d-0f41ddc5c4f8:11111~region(jp)
2026.02.22 18:10:01 Debug      -  [Table]: Preparing for New Game:
2026.02.22 18:10:02 Debug      -  [Table]: New Community Card: Ah
2026.02.22 18:10:02 Debug      -  [Table]: New Community Card: Kd
2026.02.22 18:10:02 Debug      -  [Table]: New Community Card: Qc
2026.02.22 18:10:03 Debug      -  [Table]: New Community Card: Js
2026.02.22 18:10:04 Debug      -  [Table]: New Community Card: 10h
2026.02.22 18:10:05 Debug      -  [Table]: New Community Card: 9d
2026.02.22 18:10:06 Debug      -  [Pot]: Winner: 1 Pot Amount: 100
2026.02.22 18:10:07 Debug      -  [Table]: Preparing for New Game:
`

const instanceTypeLog = `
2026.02.22 18:01:58 Debug      -  [Behaviour] Joining wrld_aeba3422-1543-4e6f-bd9d-0f41ddc5c4f8:66851~hidden(usr_7d24eeca-2652-425f-a866-3331c42c2e53)~region(jp)
2026.02.22 18:01:59 Debug      -  [Behaviour] Entering Room: Idle Cube
2026.02.22 18:02:00 Debug      -  [Table]: Preparing for New Game:
2026.02.22 18:02:01 Debug      -  [PotManager]: All players folded, player 1 won 40
2026.02.22 18:02:02 Debug      -  [Table]: Preparing for New Game:
2026.02.22 18:03:14 Debug      -  [Behaviour] Destination requested: wrld_aeba3422-1543-4e6f-bd9d-0f41ddc5c4f8:41439~friends(usr_7d24eeca-2652-425f-a866-3331c42c2e53)~region(jp)
2026.02.22 18:03:16 Debug      -  [Behaviour] Joining wrld_aeba3422-1543-4e6f-bd9d-0f41ddc5c4f8:41439~friends(usr_7d24eeca-2652-425f-a866-3331c42c2e53)~region(jp)
2026.02.22 18:03:17 Debug      -  [Table]: Preparing for New Game:
2026.02.22 18:03:18 Debug      -  [PotManager]: All players folded, player 2 won 60
`

func TestBoardOverflowIsFlaggedAndCapped(t *testing.T) {
	result, err := ParseReader(strings.NewReader(boardOverflowLog))
	if err != nil {
		t.Fatalf("ParseReader error: %v", err)
	}
	if len(result.Hands) == 0 {
		t.Fatal("expected at least one hand")
	}
	h := result.Hands[0]
	if len(h.CommunityCards) != 5 {
		t.Fatalf("expected board capped at 5 cards, got %d", len(h.CommunityCards))
	}
	if !h.HasAnomaly {
		t.Fatalf("expected anomaly flag")
	}
	if h.StatsEligible {
		t.Fatalf("expected stats ineligible when anomalous")
	}
	seenOverflow := false
	for _, a := range h.Anomalies {
		if a.Code == "BOARD_OVERFLOW" {
			seenOverflow = true
			break
		}
	}
	if !seenOverflow {
		t.Fatalf("expected BOARD_OVERFLOW anomaly")
	}
}

func TestInstanceTypeClassification(t *testing.T) {
	result, err := ParseReader(strings.NewReader(instanceTypeLog))
	if err != nil {
		t.Fatalf("ParseReader error: %v", err)
	}
	if len(result.Hands) < 2 {
		t.Fatalf("expected at least 2 hands, got %d", len(result.Hands))
	}
	hasFriendsPlus := false
	hasFriends := false
	for _, h := range result.Hands {
		if h.InstanceType == InstanceTypeFriendsPlus {
			hasFriendsPlus = true
		}
		if h.InstanceType == InstanceTypeFriends {
			hasFriends = true
		}
		if h.InstanceOwner == "" {
			t.Fatalf("expected owner user uid to be captured")
		}
	}
	if !hasFriendsPlus || !hasFriends {
		t.Fatalf("expected both friends_plus and friends instance types, got: %+v", result.Hands)
	}
}
