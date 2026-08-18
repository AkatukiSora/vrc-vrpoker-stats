package persistence_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

func TestSQLiteRoundTripUsesSourceSequenceForRFI(t *testing.T) {
	repo, err := persistence.NewSQLiteRepository(filepath.Join(t.TempDir(), "stats.db"))
	if err != nil {
		t.Fatalf("new sqlite repo: %v", err)
	}
	defer repo.Close()
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	h := &parser.Hand{StartTime: base, EndTime: base, IsComplete: true, StatsEligible: true, LocalPlayerSeat: 5, Players: map[int]*parser.PlayerHandInfo{
		5: {SeatID: 5, Position: parser.PosBTN, PFR: true, Actions: []parser.PlayerAction{{Timestamp: base, Street: parser.StreetPreFlop, Action: parser.ActionRaise, Sequence: 1}}},
		2: {SeatID: 2, Position: parser.PosBB, FoldedPF: true, Actions: []parser.PlayerAction{{Timestamp: base, Street: parser.StreetPreFlop, Action: parser.ActionFold, Sequence: 2}}},
	}}
	src := persistence.HandSourceRef{SourcePath: "sequence.log", EndByte: 1, EndLine: 1}
	src.HandUID = persistence.GenerateHandUID(h, src)
	if _, err := repo.UpsertHands(context.Background(), []persistence.PersistedHand{{Hand: h, Source: src}}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := repo.GetHandByUID(context.Background(), src.HandUID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	v := stats.NewCalculator().Calculate([]*parser.Hand{got}, 5).Metrics[stats.MetricRFI]
	if v.Opportunity != 1 || v.Count != 1 {
		t.Fatalf("RFI after round-trip = %+v, want count/opportunity 1/1", v)
	}
}
