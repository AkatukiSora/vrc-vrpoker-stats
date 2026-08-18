package persistence

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
)

func TestSaveImportBatchParity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		newRepo func(t *testing.T) ImportRepository
	}{
		{
			name: "memory",
			newRepo: func(_ *testing.T) ImportRepository {
				return NewMemoryRepository()
			},
		},
		{
			name: "sqlite",
			newRepo: func(t *testing.T) ImportRepository {
				repo, err := NewSQLiteRepository(filepath.Join(t.TempDir(), "stats.db"))
				if err != nil {
					t.Fatalf("new sqlite repo: %v", err)
				}
				t.Cleanup(func() {
					_ = repo.Close()
				})
				return repo
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := tt.newRepo(t)
			batchRepo, ok := repo.(ImportBatchRepository)
			if !ok {
				t.Fatalf("repo does not implement ImportBatchRepository")
			}

			hand := &parser.Hand{
				ID:         1,
				StartTime:  time.Date(2026, 2, 21, 0, 0, 0, 0, time.UTC),
				EndTime:    time.Date(2026, 2, 21, 0, 0, 5, 0, time.UTC),
				Players:    map[int]*parser.PlayerHandInfo{0: {SeatID: 0}},
				IsComplete: true,
			}
			source := HandSourceRef{
				SourcePath: "test.log",
				StartByte:  0,
				EndByte:    128,
				StartLine:  1,
				EndLine:    5,
			}
			source.HandUID = GenerateHandUID(hand, source)

			cursor := ImportCursor{
				SourcePath:     source.SourcePath,
				NextByteOffset: source.EndByte,
				NextLineNumber: source.EndLine,
				UpdatedAt:      time.Now(),
			}

			res, err := batchRepo.SaveImportBatch(context.Background(), []PersistedHand{{Hand: hand, Source: source}}, cursor)
			if err != nil {
				t.Fatalf("first save import batch: %v", err)
			}
			if res.Inserted != 1 || res.Updated != 0 {
				t.Fatalf("first upsert result: %+v", res)
			}

			res, err = batchRepo.SaveImportBatch(context.Background(), []PersistedHand{{Hand: hand, Source: source}}, cursor)
			if err != nil {
				t.Fatalf("second save import batch: %v", err)
			}
			if res.Updated != 1 {
				t.Fatalf("second upsert should update existing row: %+v", res)
			}

			saved, err := repo.GetCursor(context.Background(), source.SourcePath)
			if err != nil {
				t.Fatalf("get cursor: %v", err)
			}
			if saved == nil || saved.NextByteOffset != source.EndByte {
				t.Fatalf("cursor not saved correctly: %+v", saved)
			}
		})
	}
}

func TestSQLiteRoundTripPreservesNormalizedActionOrder(t *testing.T) {
	repo, err := NewSQLiteRepository(filepath.Join(t.TempDir(), "stats.db"))
	if err != nil {
		t.Fatalf("new sqlite repo: %v", err)
	}
	defer repo.Close()
	base := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	h := &parser.Hand{StartTime: base, EndTime: base, IsComplete: true, StatsEligible: true, LocalPlayerSeat: 0, SBSeat: 0, BBSeat: 1, NumPlayers: 2, ActiveSeats: []int{0, 1}, Players: map[int]*parser.PlayerHandInfo{
		0: {SeatID: 0, Position: parser.PosBTN, Actions: []parser.PlayerAction{{Timestamp: base, Street: parser.StreetPreFlop, Action: parser.ActionBlindSB, Amount: 10, Sequence: 1}, {Timestamp: base, Street: parser.StreetPreFlop, Action: parser.ActionCall, Amount: 20, Sequence: 3}}},
		1: {SeatID: 1, Position: parser.PosBB, Actions: []parser.PlayerAction{{Timestamp: base, Street: parser.StreetPreFlop, Action: parser.ActionBlindBB, Amount: 20, Sequence: 2}, {Timestamp: base, Street: parser.StreetPreFlop, Action: parser.ActionCheck, Sequence: 4}}},
	}}
	src := HandSourceRef{SourcePath: "order.log", StartByte: 0, EndByte: 1, StartLine: 1, EndLine: 1}
	src.HandUID = GenerateHandUID(h, src)
	if _, err := repo.UpsertHands(context.Background(), []PersistedHand{{Hand: h, Source: src}}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := repo.GetHandByUID(context.Background(), src.HandUID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got == nil {
		t.Fatal("missing hand")
	}
	if got.Players[0].Actions[0].Sequence != 1 || got.Players[1].Actions[0].Sequence != 2 || got.Players[0].Actions[1].Sequence != 3 {
		t.Fatalf("action sequence was not retained: %#v %#v", got.Players[0].Actions, got.Players[1].Actions)
	}
}

func TestSaveImportBatchSQLiteOverwriteHandChildren(t *testing.T) {
	t.Parallel()

	repo, err := NewSQLiteRepository(filepath.Join(t.TempDir(), "stats.db"))
	if err != nil {
		t.Fatalf("new sqlite repo: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})

	hand := &parser.Hand{
		ID:              10,
		StartTime:       time.Date(2026, 2, 21, 0, 0, 0, 0, time.UTC),
		EndTime:         time.Date(2026, 2, 21, 0, 0, 5, 0, time.UTC),
		LocalPlayerSeat: 0,
		Players: map[int]*parser.PlayerHandInfo{
			0: {
				SeatID:    0,
				HoleCards: []parser.Card{{Rank: "A", Suit: "h"}, {Rank: "K", Suit: "d"}},
				Actions: []parser.PlayerAction{
					{Timestamp: time.Date(2026, 2, 21, 0, 0, 1, 0, time.UTC), PlayerID: 0, Street: parser.StreetPreFlop, Action: parser.ActionCall, Amount: 20},
				},
			},
		},
		CommunityCards: []parser.Card{{Rank: "Q", Suit: "s"}, {Rank: "7", Suit: "d"}, {Rank: "2", Suit: "c"}},
		SBSeat:         0,
		BBSeat:         1,
		NumPlayers:     2,
		WinnerSeat:     0,
		WinType:        "showdown",
		IsComplete:     true,
	}
	source := HandSourceRef{
		SourcePath: "test.log",
		StartByte:  0,
		EndByte:    128,
		StartLine:  1,
		EndLine:    5,
	}
	source.HandUID = GenerateHandUID(hand, source)

	cursor := ImportCursor{
		SourcePath:     source.SourcePath,
		NextByteOffset: source.EndByte,
		NextLineNumber: source.EndLine,
		UpdatedAt:      time.Now(),
	}

	if _, err := repo.SaveImportBatch(context.Background(), []PersistedHand{{Hand: hand, Source: source}}, cursor); err != nil {
		t.Fatalf("first save import batch: %v", err)
	}
	if _, err := repo.SaveImportBatch(context.Background(), []PersistedHand{{Hand: hand, Source: source}}, cursor); err != nil {
		t.Fatalf("second save import batch should overwrite child rows: %v", err)
	}
}

func TestSaveImportBatchSQLiteUsesSourceSpanToAvoidDuplicateHands(t *testing.T) {
	t.Parallel()

	repo, err := NewSQLiteRepository(filepath.Join(t.TempDir(), "stats.db"))
	if err != nil {
		t.Fatalf("new sqlite repo: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})

	hand := &parser.Hand{
		ID:              7,
		StartTime:       time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
		EndTime:         time.Date(2026, 2, 22, 12, 0, 5, 0, time.UTC),
		LocalPlayerSeat: 0,
		Players: map[int]*parser.PlayerHandInfo{
			0: {SeatID: 0},
		},
		IsComplete: true,
	}

	legacySource := HandSourceRef{
		HandUID:    "legacy-uid-0001",
		SourcePath: "legacy.log",
		StartByte:  100,
		EndByte:    220,
		StartLine:  11,
		EndLine:    19,
	}
	newSource := legacySource
	newSource.HandUID = "new-v2-uid-9999"

	cursor := ImportCursor{
		SourcePath:     legacySource.SourcePath,
		NextByteOffset: legacySource.EndByte,
		NextLineNumber: legacySource.EndLine,
		UpdatedAt:      time.Now(),
	}

	if _, err := repo.SaveImportBatch(context.Background(), []PersistedHand{{Hand: hand, Source: legacySource}}, cursor); err != nil {
		t.Fatalf("first save import batch: %v", err)
	}
	if _, err := repo.SaveImportBatch(context.Background(), []PersistedHand{{Hand: hand, Source: newSource}}, cursor); err != nil {
		t.Fatalf("second save import batch: %v", err)
	}

	hands, err := repo.ListHands(context.Background(), HandFilter{OnlyComplete: true})
	if err != nil {
		t.Fatalf("list hands: %v", err)
	}
	if len(hands) != 1 {
		t.Fatalf("hands count = %d, want 1", len(hands))
	}
	if hands[0].HandUID != legacySource.HandUID {
		t.Fatalf("hand uid = %q, want %q", hands[0].HandUID, legacySource.HandUID)
	}
}

func TestSQLiteRoundTripPreservesLastPreflopAggressorDerivation(t *testing.T) {
	t.Parallel()

	repo, err := NewSQLiteRepository(filepath.Join(t.TempDir(), "stats.db"))
	if err != nil {
		t.Fatalf("new sqlite repo: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
	})

	base := time.Date(2026, 2, 23, 12, 0, 0, 0, time.UTC)
	hand := &parser.Hand{
		ID:              41,
		StartTime:       base,
		EndTime:         base.Add(15 * time.Second),
		LocalPlayerSeat: 0,
		Players: map[int]*parser.PlayerHandInfo{
			0: {SeatID: 0, Actions: []parser.PlayerAction{{Timestamp: base.Add(1 * time.Second), PlayerID: 0, Street: parser.StreetPreFlop, Action: parser.ActionBlindSB, Amount: 10}, {Timestamp: base.Add(7 * time.Second), PlayerID: 0, Street: parser.StreetPreFlop, Action: parser.ActionFold, Amount: 0}}},
			1: {SeatID: 1, PFR: true, Actions: []parser.PlayerAction{{Timestamp: base.Add(2 * time.Second), PlayerID: 1, Street: parser.StreetPreFlop, Action: parser.ActionBlindBB, Amount: 20}, {Timestamp: base.Add(4 * time.Second), PlayerID: 1, Street: parser.StreetPreFlop, Action: parser.ActionRaise, Amount: 60}}},
			2: {SeatID: 2, Actions: []parser.PlayerAction{{Timestamp: base.Add(5 * time.Second), PlayerID: 2, Street: parser.StreetPreFlop, Action: parser.ActionRaise, Amount: 140}}},
			3: {SeatID: 3, Actions: []parser.PlayerAction{{Timestamp: base.Add(3 * time.Second), PlayerID: 3, Street: parser.StreetPreFlop, Action: parser.ActionCall, Amount: 20}, {Timestamp: base.Add(6 * time.Second), PlayerID: 3, Street: parser.StreetPreFlop, Action: parser.ActionFold, Amount: 0}}},
		},
		SBSeat:        0,
		BBSeat:        1,
		NumPlayers:    4,
		IsComplete:    true,
		StatsEligible: true,
	}

	expectedSeat := parser.LastPreflopAggressorSeat(hand)
	if expectedSeat != 2 {
		t.Fatalf("original last preflop aggressor seat = %d, want 2", expectedSeat)
	}
	if !hand.Players[1].PFR {
		t.Fatalf("fixture must keep the opener marked as PFR")
	}
	if hand.Players[2].PFR {
		t.Fatalf("fixture must keep the final aggressor distinct from the stored PFR flag")
	}

	source := HandSourceRef{SourcePath: "test.log", StartByte: 0, EndByte: 256, StartLine: 1, EndLine: 12}
	source.HandUID = GenerateHandUID(hand, source)
	cursor := ImportCursor{SourcePath: source.SourcePath, NextByteOffset: source.EndByte, NextLineNumber: source.EndLine, UpdatedAt: time.Now()}

	if _, err := repo.SaveImportBatch(context.Background(), []PersistedHand{{Hand: hand, Source: source}}, cursor); err != nil {
		t.Fatalf("save import batch: %v", err)
	}

	var actionCount int
	if err := repo.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM hand_actions WHERE hand_uid = ?`, source.HandUID).Scan(&actionCount); err != nil {
		t.Fatalf("count persisted hand actions: %v", err)
	}
	if actionCount != 7 {
		t.Fatalf("persisted action rows = %d, want 7", actionCount)
	}

	reloaded, err := repo.GetHandByUID(context.Background(), source.HandUID)
	if err != nil {
		t.Fatalf("get hand by uid: %v", err)
	}
	if reloaded == nil {
		t.Fatalf("reloaded hand is nil")
	}
	if !reloaded.Players[1].PFR {
		t.Fatalf("reloaded opener must retain stored PFR flag")
	}
	if reloaded.Players[2].PFR {
		t.Fatalf("reloaded final aggressor should still differ from the stored PFR flag")
	}
	if actual := parser.LastPreflopAggressorSeat(reloaded); actual != expectedSeat {
		t.Fatalf("reloaded last preflop aggressor seat = %d, want %d", actual, expectedSeat)
	}
}
