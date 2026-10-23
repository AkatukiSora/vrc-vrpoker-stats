package compat

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/application"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/watcher"
)

const (
	baselineCommit = "426d4d89cb6241b94542b7d5e2f123686d50e576"
	goldenPath     = "../../testdata/compat/legacy-v1.json"
)

type compatibilityGolden struct {
	Baseline string                        `json:"baseline_commit"`
	Inputs   map[string]parserObservation  `json:"inputs"`
	Corpus   []corpusObservation           `json:"corpus"`
	Service  serviceObservation            `json:"service"`
	Failures map[string]failureObservation `json:"save_failures"`
	Sources  []sourceObservation           `json:"source_cases"`
	Watcher  watcherObservation            `json:"watcher"`
}

type parserObservation struct {
	ParseReader json.RawMessage `json:"parse_reader"`
	Prefixes    []prefixRecord  `json:"prefixes"`
}

type prefixRecord struct {
	Line              int    `json:"line"`
	LineError         string `json:"line_error"`
	StateSHA256       string `json:"state_sha256"`
	CloneSuffixSHA256 string `json:"clone_suffix_sha256"`
	OriginalUnchanged bool   `json:"original_unchanged"`
}

type corpusObservation struct {
	Name       string   `json:"name"`
	Size       int      `json:"size"`
	Lines      int      `json:"lines"`
	Boundaries []string `json:"boundary_state_sha256"`
	Final      string   `json:"final_state_sha256"`
}

type serviceObservation struct {
	Operations   []repoEvent `json:"repository_operations"`
	Before       serviceView `json:"after_import"`
	AfterRestart serviceView `json:"after_restart"`
}

type serviceView struct {
	Snapshot     json.RawMessage `json:"snapshot"`
	Stats        json.RawMessage `json:"stats"`
	Summaries    json.RawMessage `json:"summaries"`
	SummaryCount int             `json:"summary_count"`
	NextOffset   int64           `json:"next_offset"`
	ActivePath   string          `json:"active_path"`
}

type failureObservation struct {
	Error      string      `json:"error"`
	Operations []repoEvent `json:"repository_operations"`
	HandCount  int         `json:"persisted_hand_count"`
	ActivePath string      `json:"active_path"`
}

type sourceObservation struct {
	Name       string      `json:"name"`
	Error      string      `json:"error,omitempty"`
	Operations []repoEvent `json:"repository_operations"`
	HandCount  int         `json:"persisted_hand_count"`
	NextOffset int64       `json:"next_offset"`
	ActivePath string      `json:"active_path"`
}

type watcherObservation struct {
	ScannerError string            `json:"scanner_error"`
	Callbacks    []watcherCallback `json:"callbacks"`
	NewLogFile   string            `json:"new_log_file"`
}

type watcherCallback struct {
	Lines       []string `json:"lines,omitempty"`
	StartOffset int64    `json:"start_offset"`
	EndOffset   int64    `json:"end_offset"`
	Error       string   `json:"error,omitempty"`
}

type stateSnapshot struct {
	Hands       []*parser.Hand      `json:"hands"`
	CurrentHand *parser.Hand        `json:"current_hand"`
	LocalSeat   int                 `json:"local_seat"`
	World       parser.WorldContext `json:"world_context"`
}

type repoEvent struct {
	Operation string                      `json:"operation"`
	Hands     []persistence.PersistedHand `json:"hands,omitempty"`
	Cursor    *persistence.ImportCursor   `json:"cursor,omitempty"`
	Result    persistence.UpsertResult    `json:"result,omitempty"`
	Error     string                      `json:"error,omitempty"`
}

type repoRecorder struct {
	inner  *persistence.MemoryRepository
	events []repoEvent
	failAt string
	calls  map[string]int
}

type batchRecorder struct {
	persistence.ImportRepository
	state *repoRecorder
}

type fallbackRecorder struct {
	persistence.ImportRepository
	state *repoRecorder
}

func (r *repoRecorder) record(op string, hands []persistence.PersistedHand, cursor *persistence.ImportCursor) int {
	r.calls[op]++
	event := repoEvent{Operation: op}
	if hands != nil {
		event.Hands = cloneRows(hands)
	}
	if cursor != nil {
		c := normalizeCursor(*cursor)
		event.Cursor = &c
	}
	r.events = append(r.events, event)
	return len(r.events) - 1
}

func (r *repoRecorder) failure(op string) error {
	if r.failAt == op && r.calls[op] == 1 {
		return fmt.Errorf("injected %s failure", op)
	}
	return nil
}

func (r *repoRecorder) result(index int, result persistence.UpsertResult, err error) {
	r.events[index].Result = result
	if err != nil {
		r.events[index].Error = err.Error()
	}
}

func (r *batchRecorder) UpsertHands(ctx context.Context, hands []persistence.PersistedHand) (persistence.UpsertResult, error) {
	i := r.state.record("upsert", hands, nil)
	if err := r.state.failure("upsert"); err != nil {
		r.state.result(i, persistence.UpsertResult{}, err)
		return persistence.UpsertResult{}, err
	}
	result, err := r.state.inner.UpsertHands(ctx, hands)
	r.state.result(i, result, err)
	return result, err
}

func (r *batchRecorder) SaveCursor(ctx context.Context, cursor persistence.ImportCursor) error {
	i := r.state.record("cursor", nil, &cursor)
	if err := r.state.failure("cursor"); err != nil {
		r.state.result(i, persistence.UpsertResult{}, err)
		return err
	}
	err := r.state.inner.SaveCursor(ctx, cursor)
	r.state.result(i, persistence.UpsertResult{}, err)
	return err
}

func (r *batchRecorder) SaveImportBatch(ctx context.Context, hands []persistence.PersistedHand, cursor persistence.ImportCursor) (persistence.UpsertResult, error) {
	i := r.state.record("batch", hands, &cursor)
	if err := r.state.failure("batch"); err != nil {
		r.state.result(i, persistence.UpsertResult{}, err)
		return persistence.UpsertResult{}, err
	}
	result, err := r.state.inner.SaveImportBatch(ctx, hands, cursor)
	r.state.result(i, result, err)
	return result, err
}

func (r *fallbackRecorder) UpsertHands(ctx context.Context, hands []persistence.PersistedHand) (persistence.UpsertResult, error) {
	i := r.state.record("upsert", hands, nil)
	if err := r.state.failure("upsert"); err != nil {
		r.state.result(i, persistence.UpsertResult{}, err)
		return persistence.UpsertResult{}, err
	}
	result, err := r.state.inner.UpsertHands(ctx, hands)
	r.state.result(i, result, err)
	return result, err
}

func (r *fallbackRecorder) SaveCursor(ctx context.Context, cursor persistence.ImportCursor) error {
	i := r.state.record("cursor", nil, &cursor)
	if err := r.state.failure("cursor"); err != nil {
		r.state.result(i, persistence.UpsertResult{}, err)
		return err
	}
	err := r.state.inner.SaveCursor(ctx, cursor)
	r.state.result(i, persistence.UpsertResult{}, err)
	return err
}

func TestLegacyGolden(t *testing.T) {
	goldenBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	var want compatibilityGolden
	if err := json.Unmarshal(goldenBytes, &want); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	got, err := captureCompatibility(t)
	if err != nil {
		t.Fatal(err)
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
		t.Fatalf("legacy compatibility output differs from frozen oracle %s; regenerate only after explicit baseline review", baselineCommit)
	}
}

func TestGoldenManifest(t *testing.T) {
	f, err := os.Open("../../testdata/compat/SHA256SUMS")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	seen := map[string]bool{}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != 64 {
			t.Fatalf("invalid SHA256SUMS line %q", scanner.Text())
		}
		name := fields[1]
		if filepath.IsAbs(name) || strings.Contains(name, "..") || seen[name] {
			t.Fatalf("invalid or duplicate manifest path %q", name)
		}
		seen[name] = true
		data, err := os.ReadFile(filepath.Join("../../testdata/compat", name))
		if err != nil {
			t.Fatalf("read manifest entry %s: %v", name, err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != fields[0] {
			t.Errorf("SHA-256 mismatch for %s", name)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"sample_log.txt", "showdown_log.txt", "characterization_cases.txt", "legacy-v1.json",
		"generator-input/output_log_sample.txt", "generator-input/output_log_showdown.txt",
	} {
		if !seen[required] {
			t.Errorf("manifest does not include %s", required)
		}
	}
	root := "../../testdata/compat"
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Clean(path) == filepath.Join(root, "SHA256SUMS") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !seen[filepath.ToSlash(rel)] {
			t.Errorf("fixture %s is missing from SHA256SUMS", rel)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func captureCompatibility(t *testing.T) (compatibilityGolden, error) {
	t.Helper()
	golden := compatibilityGolden{Baseline: baselineCommit, Inputs: map[string]parserObservation{}, Failures: map[string]failureObservation{}}
	for _, name := range []string{"sample_log.txt", "showdown_log.txt", "characterization_cases.txt"} {
		data, err := os.ReadFile(filepath.Join("../../testdata/compat", name))
		if err != nil {
			return golden, err
		}
		golden.Inputs[name] = observeParser(data)
	}
	corpusDir := "../../testdata/compat/corpus"
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		return golden, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "output_log_") || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(corpusDir, entry.Name()))
		if err != nil {
			return golden, err
		}
		golden.Corpus = append(golden.Corpus, observeCorpus(entry.Name(), data))
	}
	sort.Slice(golden.Corpus, func(i, j int) bool { return golden.Corpus[i].Name < golden.Corpus[j].Name })
	if len(golden.Corpus) != 2 {
		return golden, fmt.Errorf("generated corpus contains %d logs, want 2", len(golden.Corpus))
	}
	golden.Service, err = observeService(t)
	if err != nil {
		return golden, err
	}
	for _, scenario := range []struct {
		name, op string
		batch    bool
	}{
		{name: "batch_save", op: "batch", batch: true},
		{name: "fallback_upsert", op: "upsert"},
		{name: "fallback_cursor", op: "cursor"},
	} {
		golden.Failures[scenario.name], err = observeFailure(t, scenario.op, scenario.batch)
		if err != nil {
			return golden, err
		}
	}
	golden.Sources, err = observeSources(t)
	if err != nil {
		return golden, err
	}
	golden.Watcher, err = observeWatcher(t)
	if err != nil {
		return golden, err
	}
	return golden, nil
}

func observeParser(data []byte) parserObservation {
	readerResult, readerErr := parser.ParseReader(bytes.NewReader(data))
	readerPayload := struct {
		Result *parser.ParseResult `json:"result"`
		Error  string              `json:"error"`
	}{Result: readerResult}
	if readerErr != nil {
		readerPayload.Error = readerErr.Error()
	}
	readerPayload.Result = normalizeParseResult(readerPayload.Result)
	readerJSON, _ := json.Marshal(readerPayload)

	lines := splitLines(data)
	p := parser.NewParser()
	prefixes := make([]prefixRecord, 0, len(lines))
	for i, line := range lines {
		err := p.ParseLine(line)
		before := digestSnapshot(p)
		clone := p.Clone()
		for _, suffixLine := range lines[i+1:] {
			_ = clone.ParseLine(suffixLine)
		}
		cloneDigest := digestSnapshot(clone)
		unchanged := before == digestSnapshot(p)
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		prefixes = append(prefixes, prefixRecord{
			Line: i + 1, LineError: errText, StateSHA256: before,
			CloneSuffixSHA256: cloneDigest, OriginalUnchanged: unchanged,
		})
	}
	return parserObservation{ParseReader: readerJSON, Prefixes: prefixes}
}

func observeCorpus(name string, data []byte) corpusObservation {
	lines := splitLines(data)
	p := parser.NewParser()
	boundaries := make([]string, 0)
	for _, line := range lines {
		_ = p.ParseLine(line)
		if parser.IsNewHandLine(line) {
			boundaries = append(boundaries, digestSnapshot(p))
		}
	}
	return corpusObservation{Name: name, Size: len(data), Lines: len(lines), Boundaries: boundaries, Final: digestSnapshot(p)}
}

func observeService(t *testing.T) (serviceObservation, error) {
	t.Helper()
	ctx := context.Background()
	sample, err := os.ReadFile("../../testdata/compat/sample_log.txt")
	if err != nil {
		return serviceObservation{}, err
	}
	showdown, err := os.ReadFile("../../testdata/compat/showdown_log.txt")
	if err != nil {
		return serviceObservation{}, err
	}
	logBytes := append(append(append([]byte{}, sample...), showdown...), []byte("2026.02.21 00:20:40 Debug      -  [Table]: Preparing for New Game: \n")...)
	path := filepath.Join(t.TempDir(), "service.log")
	if err := os.WriteFile(path, logBytes, 0o600); err != nil {
		return serviceObservation{}, err
	}
	state := &repoRecorder{inner: persistence.NewMemoryRepository(), calls: map[string]int{}}
	repo := &batchRecorder{ImportRepository: state.inner, state: state}
	svc := application.NewService(repo, nil)
	if err := svc.ChangeLogFile(ctx, path); err != nil {
		return serviceObservation{}, err
	}
	before, err := viewService(ctx, svc, path)
	if err != nil {
		return serviceObservation{}, err
	}

	restart := application.NewService(repo, func() ([]string, error) { return []string{path}, nil })
	if _, err := restart.BootstrapImportAllLogs(ctx); err != nil {
		return serviceObservation{}, err
	}
	after, err := viewService(ctx, restart, path)
	if err != nil {
		return serviceObservation{}, err
	}
	return serviceObservation{Operations: state.events, Before: before, AfterRestart: after}, nil
}

func viewService(ctx context.Context, svc *application.Service, path string) (serviceView, error) {
	_, hands, seat, err := svc.Snapshot(ctx)
	if err != nil {
		return serviceView{}, err
	}
	normalized := normalizeHands(hands)
	snapshotBytes, _ := json.Marshal(struct {
		Hands []*parser.Hand `json:"hands"`
		Seat  int            `json:"local_seat"`
	}{Hands: normalized, Seat: seat})
	metric, _, err := svc.Stats(ctx, persistence.HandFilter{})
	if err != nil {
		return serviceView{}, err
	}
	statsJSON, _ := json.Marshal(metric)
	summaries, count, err := svc.ListHandSummaries(ctx, persistence.HandFilter{})
	if err != nil {
		return serviceView{}, err
	}
	summaryJSON, _ := json.Marshal(summaries)
	offset, err := svc.NextOffset(ctx, path)
	if err != nil {
		return serviceView{}, err
	}
	return serviceView{Snapshot: snapshotBytes, Stats: statsJSON, Summaries: summaryJSON, SummaryCount: count, NextOffset: offset, ActivePath: "compat/source.log"}, nil
}

func observeFailure(t *testing.T, failAt string, batch bool) (failureObservation, error) {
	t.Helper()
	ctx := context.Background()
	sample, err := os.ReadFile("../../testdata/compat/sample_log.txt")
	if err != nil {
		return failureObservation{}, err
	}
	path := filepath.Join(t.TempDir(), "failure.log")
	if err := os.WriteFile(path, sample, 0o600); err != nil {
		return failureObservation{}, err
	}
	state := &repoRecorder{inner: persistence.NewMemoryRepository(), failAt: failAt, calls: map[string]int{}}
	var svc *application.Service
	if batch {
		svc = application.NewService(&batchRecorder{ImportRepository: state.inner, state: state}, nil)
	} else {
		svc = application.NewService(&fallbackRecorder{ImportRepository: state.inner, state: state}, nil)
	}
	callErr := svc.ChangeLogFile(ctx, path)
	if callErr == nil {
		return failureObservation{}, fmt.Errorf("expected %s failure", failAt)
	}
	count, err := state.inner.CountHands(ctx, persistence.HandFilter{})
	if err != nil {
		return failureObservation{}, err
	}
	return failureObservation{Error: callErr.Error(), Operations: state.events, HandCount: count, ActivePath: svc.LogPath()}, nil
}

func observeSources(t *testing.T) ([]sourceObservation, error) {
	t.Helper()
	base := strings.Join([]string{
		"2026.02.21 01:00:00 Debug      -  [Table]: Preparing for New Game: ",
		"2026.02.21 01:00:01 Debug      -  [Seat]: Player 0 SB BET IN = 10",
		"2026.02.21 01:00:02 Debug      -  [Seat]: Player 1 BB BET IN = 20",
		"2026.02.21 01:00:03 Debug      -  [Behaviour] Entering Room: Café 東京",
		"",
		"2026.02.21 01:00:04 Debug      -  [PotManager]: All players folded, player 0 won 30",
		"2026.02.21 01:00:05 Debug      -  [Table]: Preparing for New Game: ",
	}, "\n") + "\n"
	inputs := []struct {
		name string
		data []byte
	}{
		{name: "lf-final-newline", data: []byte(base)},
		{name: "crlf-final-newline", data: bytes.ReplaceAll([]byte(base), []byte("\n"), []byte("\r\n"))},
		{name: "lf-no-final-newline", data: []byte(strings.TrimSuffix(base, "\n"))},
	}
	var observations []sourceObservation
	for _, input := range inputs {
		got, err := observeSourceFile(t, input.name, input.data, false)
		if err != nil {
			return nil, err
		}
		observations = append(observations, got)
	}
	for _, endOffset := range []int64{1, 9000} {
		got, err := observeImportLines(t, endOffset)
		if err != nil {
			return nil, err
		}
		observations = append(observations, got)
	}
	cancelled, err := observeSourceFile(t, "cancelled", []byte(base), true)
	if err != nil {
		return nil, err
	}
	observations = append(observations, cancelled)
	tooLong := append(bytes.Repeat([]byte{'x'}, 4*1024*1024+1), '\n')
	scanLimit, err := observeSourceFile(t, "scan-limit", tooLong, false)
	if err != nil {
		return nil, err
	}
	observations = append(observations, scanLimit)
	return observations, nil
}

func observeSourceFile(t *testing.T, name string, data []byte, cancel bool) (sourceObservation, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.log")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return sourceObservation{}, err
	}
	state := &repoRecorder{inner: persistence.NewMemoryRepository(), calls: map[string]int{}}
	svc := application.NewService(&batchRecorder{ImportRepository: state.inner, state: state}, nil)
	ctx := context.Background()
	if cancel {
		cctx, cancelFn := context.WithCancel(ctx)
		cancelFn()
		ctx = cctx
	}
	callErr := svc.ChangeLogFile(ctx, path)
	count, err := state.inner.CountHands(context.Background(), persistence.HandFilter{})
	if err != nil {
		return sourceObservation{}, err
	}
	offset, err := svc.NextOffset(context.Background(), path)
	if err != nil {
		return sourceObservation{}, err
	}
	observation := sourceObservation{Name: name, Operations: state.events, HandCount: count, NextOffset: offset, ActivePath: "compat/source.log"}
	if callErr != nil {
		observation.Error = callErr.Error()
	}
	if svc.LogPath() == "" {
		observation.ActivePath = ""
	}
	return observation, nil
}

func observeImportLines(t *testing.T, endOffset int64) (sourceObservation, error) {
	t.Helper()
	name := "batch-end-offset-small"
	if endOffset > 100 {
		name = "batch-end-offset-large"
	}
	path := filepath.Join(t.TempDir(), "batch.log")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		return sourceObservation{}, err
	}
	state := &repoRecorder{inner: persistence.NewMemoryRepository(), calls: map[string]int{}}
	svc := application.NewService(&batchRecorder{ImportRepository: state.inner, state: state}, nil)
	ctx := context.Background()
	if err := svc.ChangeLogFile(ctx, path); err != nil {
		return sourceObservation{}, err
	}
	lines := []string{
		"2026.02.21 02:00:00 Debug      -  [Table]: Preparing for New Game: ",
		"2026.02.21 02:00:01 Debug      -  [Seat]: Player 0 SB BET IN = 10",
		"2026.02.21 02:00:02 Debug      -  [Seat]: Player 1 BB BET IN = 20",
		"2026.02.21 02:00:03 Debug      -  [PotManager]: All players folded, player 0 won 30",
		"2026.02.21 02:00:04 Debug      -  [Table]: Preparing for New Game: ",
	}
	if err := svc.ImportLines(ctx, path, lines, 200, endOffset); err != nil {
		return sourceObservation{}, err
	}
	count, err := state.inner.CountHands(ctx, persistence.HandFilter{})
	if err != nil {
		return sourceObservation{}, err
	}
	offset, err := svc.NextOffset(ctx, path)
	if err != nil {
		return sourceObservation{}, err
	}
	return sourceObservation{Name: name, Operations: state.events, HandCount: count, NextOffset: offset, ActivePath: "compat/source.log"}, nil
}

func observeWatcher(t *testing.T) (watcherObservation, error) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "output_log_2026-02-21_00-00-00.txt")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		return watcherObservation{}, err
	}
	dataCh := make(chan watcherCallback, 4)
	errorCh := make(chan error, 2)
	logFileCh := make(chan string, 1)
	lw, err := watcher.NewLogWatcher(path, watcher.WatcherConfig{
		OnNewData: func(lines []string, start, end int64) {
			dataCh <- watcherCallback{Lines: append([]string(nil), lines...), StartOffset: start, EndOffset: end}
		},
		OnNewLogFile: func(newPath string) { logFileCh <- filepath.Base(newPath) },
		OnError:      func(err error) { errorCh <- err },
	})
	if err != nil {
		return watcherObservation{}, err
	}
	defer lw.Stop()
	if err := lw.Start(); err != nil {
		return watcherObservation{}, err
	}
	var observation watcherObservation
	appendData := func(data []byte) error {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		if _, err := f.Write(data); err != nil {
			_ = f.Close()
			return err
		}
		return f.Close()
	}
	if err := appendData(append(bytes.Repeat([]byte{'x'}, 64*1024+1), '\n')); err != nil {
		return watcherObservation{}, err
	}
	select {
	case scannerErr := <-errorCh:
		observation.ScannerError = scannerErr.Error()
	case <-time.After(3 * time.Second):
		return watcherObservation{}, fmt.Errorf("watcher scanner error callback timed out")
	}
	if err := appendData([]byte("\n")); err != nil {
		return watcherObservation{}, err
	}
	select {
	case callback := <-dataCh:
		observation.Callbacks = append(observation.Callbacks, callback)
	case <-time.After(3 * time.Second):
		return watcherObservation{}, fmt.Errorf("watcher callback after scanner error timed out")
	}
	if err := os.WriteFile(path, []byte("short\n"), 0o600); err != nil {
		return watcherObservation{}, err
	}
	select {
	case callback := <-dataCh:
		observation.Callbacks = append(observation.Callbacks, callback)
	case <-time.After(3 * time.Second):
		return watcherObservation{}, fmt.Errorf("watcher callback after truncation timed out")
	}
	newLog := filepath.Join(dir, "output_log_2026-02-21_00-10-00.txt")
	if err := os.WriteFile(newLog, nil, 0o600); err != nil {
		return watcherObservation{}, err
	}
	select {
	case observation.NewLogFile = <-logFileCh:
	case <-time.After(3 * time.Second):
		return watcherObservation{}, fmt.Errorf("watcher new-log callback timed out")
	}
	return observation, nil
}

func snapshot(p *parser.Parser) stateSnapshot {
	return stateSnapshot{Hands: normalizeHands(p.GetHands()), CurrentHand: normalizeHand(p.GetCurrentHand()), LocalSeat: p.GetLocalSeat(), World: p.WorldContext()}
}

func normalizeParseResult(result *parser.ParseResult) *parser.ParseResult {
	if result == nil {
		return nil
	}
	copyResult := *result
	copyResult.Hands = normalizeHands(result.Hands)
	copyResult.CurrentHand = normalizeHand(result.CurrentHand)
	return &copyResult
}

func normalizeHands(hands []*parser.Hand) []*parser.Hand {
	if hands == nil {
		return nil
	}
	out := make([]*parser.Hand, len(hands))
	for i, hand := range hands {
		out[i] = normalizeHand(hand)
	}
	return out
}

func normalizeHand(hand *parser.Hand) *parser.Hand {
	if hand == nil {
		return nil
	}
	copyHand := parser.CloneHand(hand)
	sort.Slice(copyHand.InstanceUsers, func(i, j int) bool { return copyHand.InstanceUsers[i].UserUID < copyHand.InstanceUsers[j].UserUID })
	return copyHand
}

func normalizeCursor(cursor persistence.ImportCursor) persistence.ImportCursor {
	cursor.SourcePath = "compat/source.log"
	cursor.UpdatedAt = time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	if cursor.WorldCtx != nil {
		world := cursor.WorldCtx.Clone()
		cursor.WorldCtx = &world
	}
	return cursor
}

func cloneRows(rows []persistence.PersistedHand) []persistence.PersistedHand {
	out := make([]persistence.PersistedHand, len(rows))
	for i, row := range rows {
		out[i] = row
		out[i].Hand = normalizeHand(row.Hand)
		out[i].Source.SourcePath = "compat/source.log"
	}
	return out
}

func digestSnapshot(p *parser.Parser) string {
	payload, _ := json.Marshal(snapshot(p))
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func splitLines(data []byte) []string {
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return []string{""}
	}
	return strings.Split(text, "\n")
}
