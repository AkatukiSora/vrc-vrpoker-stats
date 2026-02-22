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
