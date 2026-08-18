package handhistory

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
)

func TestSerializePHHDeterministicAndPrivacySafe(t *testing.T) {
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	h := &parser.Hand{HandUID: "hand-1", StartTime: base, SBSeat: 3, BBSeat: 4, Players: map[int]*parser.PlayerHandInfo{
		3: {SeatID: 3, HoleCards: []parser.Card{{Rank: "A", Suit: "h"}, {Rank: "10", Suit: "d"}}, Actions: []parser.PlayerAction{{Timestamp: base, PlayerID: 3, Street: parser.StreetPreFlop, Action: parser.ActionBlindSB, Amount: 5}, {Timestamp: base.Add(time.Second), PlayerID: 3, Street: parser.StreetPreFlop, Action: parser.ActionCall, Amount: 10}}},
		4: {SeatID: 4, Actions: []parser.PlayerAction{{Timestamp: base, PlayerID: 4, Street: parser.StreetPreFlop, Action: parser.ActionBlindBB, Amount: 10}, {Timestamp: base.Add(2 * time.Second), PlayerID: 4, Street: parser.StreetPreFlop, Action: parser.ActionCheck}}},
	}, CommunityCards: []parser.Card{{Rank: "K", Suit: "c"}, {Rank: "Q", Suit: "d"}, {Rank: "2", Suit: "s"}}}
	got, err := SerializePHH(h)
	if err != nil {
		t.Fatal(err)
	}
	want := "variant = \"NT\"\nantes = [0, 0]\nblinds_or_straddles = [5, 10]\nmin_bet = 10\nstarting_stacks = [inf, inf]\nactions = [\n  \"d dh p1 AhTd\",\n  \"d dh p2 ????\",\n  \"p1 cc\",\n  \"p2 cc\",\n  \"d db KcQd2s\",\n]"
	if !strings.Contains(string(got), want) {
		t.Fatalf("unexpected PHH:\n%s", got)
	}
	if strings.Contains(string(got), "Instance") || strings.Contains(string(got), "usr_") {
		t.Fatalf("export leaked identity metadata: %s", got)
	}
}

func TestSerializePHHRejectsUnexportableHand(t *testing.T) {
	_, err := SerializePHH(&parser.Hand{Players: map[int]*parser.PlayerHandInfo{1: {SeatID: 1}}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSerializePHHArchiveKeepsHandsAsSeparateDocuments(t *testing.T) {
	hands := []*parser.Hand{testExportHand(1), testExportHand(2)}
	data, result, err := SerializePHHArchive(hands)
	if err != nil {
		t.Fatal(err)
	}
	if result.Exported != 2 || result.Skipped != 0 {
		t.Fatalf("result = %+v", result)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(archive.File), 2; got != want {
		t.Fatalf("entries = %d", got)
	}
	if archive.File[0].Name != "hand-000001.phh" || archive.File[1].Name != "hand-000002.phh" {
		t.Fatalf("unexpected archive names")
	}
}

func TestSerializePHHArchiveSkipsHandsMissingBlinds(t *testing.T) {
	data, result, err := SerializePHHArchive([]*parser.Hand{testExportHand(1), {Players: map[int]*parser.PlayerHandInfo{0: {SeatID: 0}, 1: {SeatID: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Exported != 1 || result.Skipped != 1 {
		t.Fatalf("result = %+v", result)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(archive.File) != 1 {
		t.Fatalf("archive = %v, entries=%d", err, len(archive.File))
	}
}

func testExportHand(seat int) *parser.Hand {
	return &parser.Hand{SBSeat: seat, BBSeat: seat + 1, Players: map[int]*parser.PlayerHandInfo{
		seat:     {SeatID: seat, Actions: []parser.PlayerAction{{Action: parser.ActionBlindSB, Amount: 5}}},
		seat + 1: {SeatID: seat + 1, Actions: []parser.PlayerAction{{Action: parser.ActionBlindBB, Amount: 10}}},
	}}
}
