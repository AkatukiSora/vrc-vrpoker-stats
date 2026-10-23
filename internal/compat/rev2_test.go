package compat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/application"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
)

const rev2GoldenPath = "../../testdata/compat/legacy-rev2.json"

type rev2CompatibilityGolden struct {
	Baseline          string                       `json:"baseline_commit"`
	ServiceQueries    serviceQueryObservation      `json:"service_queries"`
	Resume            resumeObservation            `json:"resume"`
	ContextlessResume contextlessResumeObservation `json:"contextless_resume"`
	StaleSource       staleSourceObservation       `json:"stale_source"`
	SeekFallback      seekFallbackObservation      `json:"seek_fallback"`
}

type handIdentity struct {
	UID       string    `json:"uid"`
	StartTime time.Time `json:"start_time"`
	WorldID   string    `json:"world_id,omitempty"`
}

type serviceQueryObservation struct {
	AllHands             []handIdentity `json:"all_hands"`
	FilteredSummaryCount int            `json:"filtered_summary_count"`
	FilteredSummaries    []handIdentity `json:"filtered_summaries"`
	FilteredStatsTotal   int            `json:"filtered_stats_total"`
	FilteredLastNTotal   int            `json:"filtered_last_n_total"`
}

type resumeObservation struct {
	OldFullyImported    bool                 `json:"old_fully_imported"`
	ActiveInitialOffset int64                `json:"active_initial_offset"`
	ActiveWorldContext  *parser.WorldContext `json:"active_world_context"`
	ResumeProgressPaths []string             `json:"resume_progress_paths"`
	SkippedOldOnResume  bool                 `json:"skipped_old_on_resume"`
	ResumedHands        []handIdentity       `json:"resumed_hands"`
	RepeatProgressPaths []string             `json:"repeat_progress_paths"`
	SkippedOldOnRepeat  bool                 `json:"skipped_old_on_repeat"`
	RepeatedHandCount   int                  `json:"repeated_hand_count"`
	RepeatStable        bool                 `json:"repeat_stable"`
}

type contextlessResumeObservation struct {
	InitialHandCount           int   `json:"initial_hand_count"`
	NonEmptyBatchRows          int   `json:"non_empty_batch_rows"`
	HandCountAfterRescan       int   `json:"hand_count_after_rescan"`
	PriorCursorOffset          int64 `json:"prior_cursor_offset"`
	CursorWorldContextRestored bool  `json:"cursor_world_context_restored"`
	HandIdentitiesStable       bool  `json:"hand_identities_stable"`
}

type staleSourceObservation struct {
	ActivePath string         `json:"active_path"`
	Before     []handIdentity `json:"before"`
	After      []handIdentity `json:"after"`
	Unchanged  bool           `json:"unchanged"`
}

type seekFallbackObservation struct {
	FallbackWarning bool   `json:"fallback_warning"`
	ImportError     string `json:"import_error"`
}

func TestLegacyRev2Golden(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("seek fallback characterization uses a Linux FIFO")
	}

	goldenBytes, err := os.ReadFile(rev2GoldenPath)
	if err != nil {
		t.Fatal(err)
	}
	var want rev2CompatibilityGolden
	if err := json.Unmarshal(goldenBytes, &want); err != nil {
		t.Fatalf("decode rev2 golden: %v", err)
	}
	got, err := captureRev2(t)
	if err != nil {
		t.Fatal(err)
	}
	if got.Baseline != baselineCommit {
		t.Fatalf("rev2 baseline = %q, want %q", got.Baseline, baselineCommit)
	}
	if len(got.ServiceQueries.AllHands) != 4 || got.ServiceQueries.FilteredSummaryCount != 3 || got.ServiceQueries.FilteredStatsTotal != 3 || got.ServiceQueries.FilteredLastNTotal != 2 {
		t.Fatalf("service query boundaries not exercised: %+v", got.ServiceQueries)
	}
	if !got.Resume.OldFullyImported || !got.Resume.SkippedOldOnResume || !got.Resume.SkippedOldOnRepeat || !got.Resume.RepeatStable || got.Resume.RepeatedHandCount != 3 {
		t.Fatalf("resume/full-import/idempotency boundaries not exercised: %+v", got.Resume)
	}
	if got.Resume.ActiveWorldContext == nil || got.Resume.ActiveWorldContext.WorldID != "wrld_aeba3422-1543-4e6f-bd9d-0f41ddc5c4f8" || len(got.Resume.ResumedHands) != 3 || got.Resume.ResumedHands[2].WorldID != "wrld_aeba3422-1543-4e6f-bd9d-0f41ddc5c4f8" {
		t.Fatalf("positive WorldContext resume did not preserve world metadata: %+v", got.Resume)
	}
	if got.ContextlessResume.InitialHandCount != 1 || got.ContextlessResume.NonEmptyBatchRows == 0 || got.ContextlessResume.HandCountAfterRescan != 1 || got.ContextlessResume.PriorCursorOffset <= 0 || !got.ContextlessResume.CursorWorldContextRestored || !got.ContextlessResume.HandIdentitiesStable {
		t.Fatalf("contextless cursor did not trigger stable full rescan: %+v", got.ContextlessResume)
	}
	if !got.StaleSource.Unchanged {
		t.Fatalf("stale-source import changed hands: before=%+v after=%+v", got.StaleSource.Before, got.StaleSource.After)
	}
	if !got.SeekFallback.FallbackWarning || got.SeekFallback.ImportError == "" {
		t.Fatalf("seek fallback path was not exercised: %+v", got.SeekFallback)
	}

	gotBytes, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	wantBytes, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotBytes, wantBytes) {
		t.Fatalf("rev2 compatibility output differs from frozen baseline oracle %s", baselineCommit)
	}
}

func TestCharacterizationIncludesRequiredBoundaries(t *testing.T) {
	data, err := os.ReadFile("../../testdata/compat/characterization_cases.txt")
	if err != nil {
		t.Fatal(err)
	}
	result, err := parser.ParseReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse characterization cases: %v", err)
	}

	anomalies := map[string]bool{}
	hasMissingBlind := false
	hasReversedBlinds := false
	hasThreeBet := false
	hasLocalSeat2 := false
	hasLocalSeat3 := false
	hasSidePot := false
	text := string(data)
	bbBeforeSB := strings.Index(text, "Player 3 BB BET IN = 20") >= 0 && strings.Index(text, "Player 2 SB BET IN = 10") > strings.Index(text, "Player 3 BB BET IN = 20")
	for _, hand := range result.Hands {
		hasSBAction := false
		hasBBAction := false
		for _, player := range hand.Players {
			for _, action := range player.Actions {
				if action.Action == parser.ActionBlindSB {
					hasSBAction = true
				}
				if action.Action == parser.ActionBlindBB {
					hasBBAction = true
				}
			}
		}
		if len(hand.Players) > 0 && (!hasSBAction || !hasBBAction) {
			hasMissingBlind = true
		}
		if hand.SBSeat == 2 && hand.BBSeat == 3 && hand.LocalPlayerSeat == 3 && bbBeforeSB {
			hasReversedBlinds = true
		}
		if hand.LocalPlayerSeat == 2 {
			hasLocalSeat2 = true
		}
		if hand.LocalPlayerSeat == 3 {
			hasLocalSeat3 = true
		}
		paidSeats := 0
		for _, player := range hand.Players {
			if player.ThreeBet {
				hasThreeBet = true
			}
			if player.PotWon > 0 {
				paidSeats++
			}
		}
		if paidSeats >= 2 && hand.TotalPot >= 600 {
			hasSidePot = true
		}
		for _, anomaly := range hand.Anomalies {
			anomalies[anomaly.Code] = true
		}
	}
	for _, required := range []string{"BOARD_PARSE_ERROR", "BOARD_DUPLICATE_CARD", "BOARD_OVERFLOW", "BOARD_COUNT_INVALID"} {
		if !anomalies[required] {
			t.Errorf("characterization input does not produce %s", required)
		}
	}
	if !hasMissingBlind || !hasReversedBlinds || !hasLocalSeat2 || !hasLocalSeat3 || !hasThreeBet || !hasSidePot {
		t.Errorf("characterization boundaries missing: missingBlind=%v reversedBlinds=%v seat2=%v seat3=%v threeBet=%v sidePot=%v", hasMissingBlind, hasReversedBlinds, hasLocalSeat2, hasLocalSeat3, hasThreeBet, hasSidePot)
	}
}

func captureRev2(t *testing.T) (rev2CompatibilityGolden, error) {
	t.Helper()
	got := rev2CompatibilityGolden{Baseline: baselineCommit}
	var err error
	got.ServiceQueries, err = observeServiceQueries(t)
	if err != nil {
		return got, err
	}
	got.Resume, err = observeResume(t)
	if err != nil {
		return got, err
	}
	got.ContextlessResume, err = observeContextlessResume(t)
	if err != nil {
		return got, err
	}
	got.StaleSource, err = observeStaleSource(t)
	if err != nil {
		return got, err
	}
	got.SeekFallback, err = observeSeekFallback(t)
	if err != nil {
		return got, err
	}
	return got, nil
}

func observeServiceQueries(t *testing.T) (serviceQueryObservation, error) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "queries.log")
	if err := os.WriteFile(path, []byte(queryLog()), 0o600); err != nil {
		return serviceQueryObservation{}, err
	}
	svc := application.NewService(persistence.NewMemoryRepository(), nil)
	if err := svc.ChangeLogFile(ctx, path); err != nil {
		return serviceQueryObservation{}, err
	}
	_, hands, _, err := svc.Snapshot(ctx)
	if err != nil {
		return serviceQueryObservation{}, err
	}

	from := time.Date(2026, 2, 21, 12, 10, 0, 0, time.UTC)
	to := time.Date(2026, 2, 21, 12, 30, 0, 0, time.UTC)
	summaries, count, err := svc.ListHandSummaries(ctx, persistence.HandFilter{FromTime: &from, ToTime: &to, Limit: 2})
	if err != nil {
		return serviceQueryObservation{}, err
	}
	filteredStats, _, err := svc.Stats(ctx, persistence.HandFilter{FromTime: &from, ToTime: &to})
	if err != nil {
		return serviceQueryObservation{}, err
	}
	lastNStats, _, err := svc.Stats(ctx, persistence.HandFilter{FromTime: &from, ToTime: &to, LastN: 2})
	if err != nil {
		return serviceQueryObservation{}, err
	}

	filtered := make([]handIdentity, 0, len(summaries))
	for _, summary := range summaries {
		filtered = append(filtered, handIdentity{UID: summary.HandUID, StartTime: summary.StartTime})
	}
	return serviceQueryObservation{
		AllHands:             handIdentities(hands),
		FilteredSummaryCount: count,
		FilteredSummaries:    filtered,
		FilteredStatsTotal:   filteredStats.TotalHands,
		FilteredLastNTotal:   lastNStats.TotalHands,
	}, nil
}

func observeResume(t *testing.T) (resumeObservation, error) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "old.log")
	activePath := filepath.Join(dir, "active.log")
	oldBytes := []byte(rev2HandLog("03:00", 0, false))
	activeBytes := []byte(rev2HandLog("03:10", 0, true))
	if err := os.WriteFile(oldPath, oldBytes, 0o600); err != nil {
		return resumeObservation{}, err
	}
	if err := os.WriteFile(activePath, activeBytes, 0o600); err != nil {
		return resumeObservation{}, err
	}

	repo := persistence.NewMemoryRepository()
	locator := func() ([]string, error) { return []string{activePath, oldPath}, nil }
	first := application.NewService(repo, locator)
	if _, err := first.BootstrapImportAllLogs(ctx); err != nil {
		return resumeObservation{}, err
	}
	oldCursor, err := repo.GetCursor(ctx, oldPath)
	if err != nil {
		return resumeObservation{}, err
	}
	activeCursor, err := repo.GetCursor(ctx, activePath)
	if err != nil {
		return resumeObservation{}, err
	}
	if activeCursor == nil {
		return resumeObservation{}, fmt.Errorf("initial active cursor was not saved")
	}
	var activeWorld *parser.WorldContext
	if activeCursor.WorldCtx != nil {
		world := activeCursor.WorldCtx.Clone()
		activeWorld = &world
	}

	with, err := os.OpenFile(activePath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return resumeObservation{}, err
	}
	if _, err := with.Write([]byte(rev2HandLog("03:20", 0, false))); err != nil {
		_ = with.Close()
		return resumeObservation{}, err
	}
	if err := with.Close(); err != nil {
		return resumeObservation{}, err
	}

	var resumeProgress []application.BootstrapProgress
	second := application.NewService(repo, locator)
	if _, err := second.BootstrapImportAllLogsWithProgress(ctx, func(p application.BootstrapProgress) {
		resumeProgress = append(resumeProgress, p)
	}); err != nil {
		return resumeObservation{}, err
	}
	resumedHands, err := repo.ListHands(ctx, persistence.HandFilter{OnlyComplete: true})
	if err != nil {
		return resumeObservation{}, err
	}
	resumedIdentities := handIdentities(resumedHands)

	var repeatProgress []application.BootstrapProgress
	third := application.NewService(repo, locator)
	if _, err := third.BootstrapImportAllLogsWithProgress(ctx, func(p application.BootstrapProgress) {
		repeatProgress = append(repeatProgress, p)
	}); err != nil {
		return resumeObservation{}, err
	}
	repeatedHands, err := repo.ListHands(ctx, persistence.HandFilter{OnlyComplete: true})
	if err != nil {
		return resumeObservation{}, err
	}
	repeatedIdentities := handIdentities(repeatedHands)
	count, err := repo.CountHands(ctx, persistence.HandFilter{OnlyComplete: true})
	if err != nil {
		return resumeObservation{}, err
	}
	stable, err := identitiesEqual(resumedIdentities, repeatedIdentities)
	if err != nil {
		return resumeObservation{}, err
	}

	return resumeObservation{
		OldFullyImported:    oldCursor != nil && oldCursor.IsFullyImported,
		ActiveInitialOffset: activeCursor.NextByteOffset,
		ActiveWorldContext:  activeWorld,
		ResumeProgressPaths: normalizeProgressPaths(resumeProgress, oldPath, activePath),
		SkippedOldOnResume:  !progressContainsPath(resumeProgress, oldPath),
		ResumedHands:        resumedIdentities,
		RepeatProgressPaths: normalizeProgressPaths(repeatProgress, oldPath, activePath),
		SkippedOldOnRepeat:  !progressContainsPath(repeatProgress, oldPath),
		RepeatedHandCount:   count,
		RepeatStable:        stable,
	}, nil
}

func observeContextlessResume(t *testing.T) (contextlessResumeObservation, error) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "contextless.log")
	if err := os.WriteFile(path, []byte(rev2HandLog("05:00", 0, true)), 0o600); err != nil {
		return contextlessResumeObservation{}, err
	}
	state := &repoRecorder{inner: persistence.NewMemoryRepository(), calls: map[string]int{}}
	repo := &batchRecorder{ImportRepository: state.inner, state: state}
	locator := func() ([]string, error) { return []string{path}, nil }
	initial := application.NewService(repo, nil)
	if err := initial.ChangeLogFile(ctx, path); err != nil {
		return contextlessResumeObservation{}, err
	}
	before, err := state.inner.ListHands(ctx, persistence.HandFilter{OnlyComplete: true})
	if err != nil {
		return contextlessResumeObservation{}, err
	}
	priorCursor, err := state.inner.GetCursor(ctx, path)
	if err != nil {
		return contextlessResumeObservation{}, err
	}
	if priorCursor == nil {
		return contextlessResumeObservation{}, fmt.Errorf("initial contextless-resume cursor was not saved")
	}
	priorOffset := priorCursor.NextByteOffset
	priorCursor.WorldCtx = nil
	if err := state.inner.SaveCursor(ctx, *priorCursor); err != nil {
		return contextlessResumeObservation{}, err
	}
	state.events = nil
	state.calls = map[string]int{}

	resumed := application.NewService(repo, locator)
	if _, err := resumed.BootstrapImportAllLogs(ctx); err != nil {
		return contextlessResumeObservation{}, err
	}
	after, err := state.inner.ListHands(ctx, persistence.HandFilter{OnlyComplete: true})
	if err != nil {
		return contextlessResumeObservation{}, err
	}
	cursorAfter, err := state.inner.GetCursor(ctx, path)
	if err != nil {
		return contextlessResumeObservation{}, err
	}
	reimportedRows := 0
	for _, event := range state.events {
		if event.Operation == "batch" {
			reimportedRows += len(event.Hands)
		}
	}
	stable, err := identitiesEqual(handIdentities(before), handIdentities(after))
	if err != nil {
		return contextlessResumeObservation{}, err
	}
	worldRestored := cursorAfter != nil && cursorAfter.WorldCtx != nil && cursorAfter.WorldCtx.WorldID == "wrld_aeba3422-1543-4e6f-bd9d-0f41ddc5c4f8"
	return contextlessResumeObservation{
		InitialHandCount:           len(before),
		NonEmptyBatchRows:          reimportedRows,
		HandCountAfterRescan:       len(after),
		PriorCursorOffset:          priorOffset,
		CursorWorldContextRestored: worldRestored,
		HandIdentitiesStable:       stable,
	}, nil
}

func observeStaleSource(t *testing.T) (staleSourceObservation, error) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.log")
	pathB := filepath.Join(dir, "b.log")
	if err := os.WriteFile(pathA, []byte(rev2HandLog("04:00", 0, false)), 0o600); err != nil {
		return staleSourceObservation{}, err
	}
	if err := os.WriteFile(pathB, []byte(rev2HandLog("04:10", 0, false)), 0o600); err != nil {
		return staleSourceObservation{}, err
	}
	repo := persistence.NewMemoryRepository()
	svc := application.NewService(repo, nil)
	if err := svc.ChangeLogFile(ctx, pathA); err != nil {
		return staleSourceObservation{}, err
	}
	if err := svc.ChangeLogFile(ctx, pathB); err != nil {
		return staleSourceObservation{}, err
	}
	before, err := repo.ListHands(ctx, persistence.HandFilter{OnlyComplete: true})
	if err != nil {
		return staleSourceObservation{}, err
	}
	staleLines := strings.Split(strings.TrimSuffix(rev2HandLog("04:20", 0, false), "\n"), "\n")
	if err := svc.ImportLines(ctx, pathA, staleLines, 0, 128); err != nil {
		return staleSourceObservation{}, err
	}
	after, err := repo.ListHands(ctx, persistence.HandFilter{OnlyComplete: true})
	if err != nil {
		return staleSourceObservation{}, err
	}
	beforeIDs := handIdentities(before)
	afterIDs := handIdentities(after)
	unchanged, err := identitiesEqual(beforeIDs, afterIDs)
	if err != nil {
		return staleSourceObservation{}, err
	}
	return staleSourceObservation{ActivePath: "compat/b.log", Before: beforeIDs, After: afterIDs, Unchanged: unchanged}, nil
}

func observeSeekFallback(t *testing.T) (seekFallbackObservation, error) {
	t.Helper()
	if runtime.GOOS != "linux" {
		return seekFallbackObservation{}, fmt.Errorf("seek fallback characterization requires Linux FIFO")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "nonseekable.log")
	if output, err := exec.Command("mkfifo", path).CombinedOutput(); err != nil {
		return seekFallbackObservation{}, fmt.Errorf("mkfifo: %w: %s", err, output)
	}

	messages := &slogMessageCollector{}
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(messages))
	defer slog.SetDefault(previousLogger)

	repo := persistence.NewMemoryRepository()
	world := parser.WorldContext{WorldID: "wrld_aeba3422-1543-4e6f-bd9d-0f41ddc5c4f8", WorldDisplayName: "Compat FIFO"}
	if err := repo.SaveCursor(context.Background(), persistence.ImportCursor{SourcePath: path, NextByteOffset: 1, WorldCtx: &world}); err != nil {
		return seekFallbackObservation{}, err
	}
	writerDone := make(chan error, 1)
	go func() {
		file, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			writerDone <- err
			return
		}
		writerDone <- file.Close()
	}()

	svc := application.NewService(repo, func() ([]string, error) { return []string{path}, nil })
	_, importErr := svc.BootstrapImportAllLogs(context.Background())
	writerErr := <-writerDone
	if writerErr != nil {
		return seekFallbackObservation{}, fmt.Errorf("finish FIFO writer: %w", writerErr)
	}
	if importErr == nil {
		return seekFallbackObservation{}, fmt.Errorf("non-seekable source unexpectedly imported successfully")
	}
	errorText := strings.ReplaceAll(importErr.Error(), path, "compat/nonseekable.log")
	return seekFallbackObservation{
		FallbackWarning: messages.contains("seek failed, re-scanning from start"),
		ImportError:     errorText,
	}, nil
}

func queryLog() string {
	var log strings.Builder
	for _, minute := range []string{"12:00", "12:10", "12:20", "12:30"} {
		log.WriteString(rev2HandLog(minute, 0, false))
	}
	return log.String()
}

func rev2HandLog(minute string, localSeat int, includeWorld bool) string {
	lines := make([]string, 0, 8)
	if includeWorld {
		lines = append(lines,
			"2026.02.21 02:59:50 Debug      -  [Behaviour] Joining wrld_aeba3422-1543-4e6f-bd9d-0f41ddc5c4f8:12345~region(us)",
			"2026.02.21 02:59:51 Debug      -  [Behaviour] Entering Room: Compat Resume World",
		)
	}
	lines = append(lines,
		fmt.Sprintf("2026.02.21 %s:00 Debug      -  [Table]: Preparing for New Game: ", minute),
		fmt.Sprintf("2026.02.21 %s:01 Debug      -  [Manager]: Local Seat Assigned. ID: %d", minute, localSeat),
		fmt.Sprintf("2026.02.21 %s:02 Debug      -  [Seat]: Player %d SB BET IN = 10", minute, localSeat),
		fmt.Sprintf("2026.02.21 %s:03 Debug      -  [Seat]: Player %d BB BET IN = 20", minute, localSeat+1),
		fmt.Sprintf("2026.02.21 %s:04 Debug      -  [PotManager]: All players folded, player %d won 30", minute, localSeat),
		fmt.Sprintf("2026.02.21 %s:05 Debug      -  [Table]: Preparing for New Game: ", minute),
	)
	return strings.Join(lines, "\n") + "\n"
}

func handIdentities(hands []*parser.Hand) []handIdentity {
	identities := make([]handIdentity, 0, len(hands))
	for _, hand := range hands {
		identities = append(identities, handIdentity{UID: hand.HandUID, StartTime: hand.StartTime, WorldID: hand.WorldID})
	}
	return identities
}

func identitiesEqual(left, right []handIdentity) (bool, error) {
	leftJSON, err := json.Marshal(left)
	if err != nil {
		return false, err
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		return false, err
	}
	return bytes.Equal(leftJSON, rightJSON), nil
}

func normalizeProgressPaths(progress []application.BootstrapProgress, oldPath, activePath string) []string {
	paths := make([]string, 0, len(progress))
	for _, event := range progress {
		switch event.Path {
		case oldPath:
			paths = append(paths, "compat/old.log")
		case activePath:
			paths = append(paths, "compat/active.log")
		default:
			paths = append(paths, event.Path)
		}
	}
	return paths
}

func progressContainsPath(progress []application.BootstrapProgress, path string) bool {
	for _, event := range progress {
		if event.Path == path {
			return true
		}
	}
	return false
}

type slogMessageCollector struct {
	messages []string
}

func (h *slogMessageCollector) Enabled(context.Context, slog.Level) bool { return true }
func (h *slogMessageCollector) Handle(_ context.Context, record slog.Record) error {
	h.messages = append(h.messages, record.Message)
	return nil
}
func (h *slogMessageCollector) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *slogMessageCollector) WithGroup(string) slog.Handler      { return h }
func (h *slogMessageCollector) contains(want string) bool {
	for _, message := range h.messages {
		if message == want {
			return true
		}
	}
	return false
}
