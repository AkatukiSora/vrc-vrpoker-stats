package application

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

// AppService is the interface that the UI layer depends on for log import and stats queries.
// application.Service satisfies this interface.
type AppService interface {
	BootstrapImportAllLogs(ctx context.Context) (string, error)
	BootstrapImportAllLogsWithProgress(ctx context.Context, onProgress func(BootstrapProgress)) (string, error)
	ChangeLogFile(ctx context.Context, path string) error
	ImportLines(ctx context.Context, sourcePath string, lines []string, startOffset int64, endOffset int64) error
	Snapshot(ctx context.Context) (*stats.Stats, []*parser.Hand, int, error)
	Stats(ctx context.Context, filter persistence.HandFilter) (*stats.Stats, int, error)
	ListHandSummaries(ctx context.Context, f persistence.HandFilter) ([]persistence.HandSummary, int, error)
	// GetHandByUID returns the full hand data for a single hand UID (for detail view).
	// Returns nil, nil if not found.
	GetHandByUID(ctx context.Context, uid string) (*parser.Hand, error)
	NextOffset(ctx context.Context, path string) (int64, error)
	MarkLogFullyImported(ctx context.Context, path string)
	Close() error
}

type LogFileLocator func() ([]string, error)

type Service struct {
	mu             sync.RWMutex
	repo           persistence.ImportRepository
	parser         *parser.Parser
	logPath        string
	localSeat      int
	detectLogFiles LogFileLocator

	parsedHands          int
	currentLineNumber    int64
	currentHandStartLn   int64
	currentByteOffset    int64
	currentHandStartByte int64

	// Incremental AllTime calculator
	incMu        sync.Mutex
	incCalc      *stats.IncrementalCalculator
	incLocalSeat int       // localSeat that incCalc was built for
	watermark    time.Time // zero value = not initialized

	// Period-filter cache (keyed by filter + localSeat + handCount)
	cacheMu    sync.Mutex
	statsCache map[statsCacheKey]*stats.Stats
}

type statsCacheKey struct {
	fromTime  time.Time
	toTime    time.Time
	localSeat int
	handCount int
}

func NewService(repo persistence.ImportRepository, locator LogFileLocator) *Service {
	if locator == nil {
		locator = func() ([]string, error) {
			return nil, fmt.Errorf("log file locator is not configured")
		}
	}

	return &Service{
		repo:               repo,
		parser:             parser.NewParser(),
		localSeat:          -1,
		currentHandStartLn: 0,
		detectLogFiles:     locator,
	}
}

// BootstrapProgress carries per-file progress information during bootstrap import.
type BootstrapProgress struct {
	// Current is the 1-based index of the file currently being imported.
	Current int
	// Total is the total number of files to import (skipped files excluded).
	Total int
	// Path is the absolute path of the file being imported.
	Path string
	// Skipped is the number of files skipped (already fully imported).
	Skipped int
}

func (s *Service) BootstrapImportAllLogs(ctx context.Context) (string, error) {
	return s.BootstrapImportAllLogsWithProgress(ctx, nil)
}

// parseResult holds the outcome of parsing a single log file.
type parseResult struct {
	path          string
	hands         []persistence.PersistedHand
	parser        *parser.Parser
	byteOffset    int64
	lineNumber    int64
	handStartLn   int64
	handStartByte int64
	parsedHands   int
	err           error
}

// parseWorker parses a single log file and sends the result on out.
// It does not touch the database.
func parseWorker(ctx context.Context, path string, out chan<- parseResult) {
	result := parseResult{path: path}

	f, err := os.Open(path)
	if err != nil {
		result.err = err
		out <- result
		return
	}
	defer f.Close()

	p := parser.NewParser()
	lineNo := int64(0)
	handStartLn := int64(0)
	byteOffset := int64(0)
	handStartByte := int64(0)
	parsedHands := 0

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for scanner.Scan() {
		if ctx.Err() != nil {
			result.err = ctx.Err()
			out <- result
			return
		}
		line := scanner.Text()
		lineStartByte := byteOffset
		lineNo++
		byteOffset += int64(len(line)) + 1

		markHandStart(line, lineNo, lineStartByte, &handStartLn, &handStartByte)

		_ = p.ParseLine(line)
		if p.HandCount() > parsedHands {
			hands := p.GetHands()
			newRows := collectNewPersistedHands(path, hands, &parsedHands, lineNo, &handStartLn, &handStartByte, lineStartByte, byteOffset)
			result.hands = append(result.hands, newRows...)
		}
	}
	if err := scanner.Err(); err != nil {
		result.err = err
		out <- result
		return
	}

	result.parser = p
	result.byteOffset = byteOffset
	result.lineNumber = lineNo
	result.handStartLn = handStartLn
	result.handStartByte = handStartByte
	result.parsedHands = parsedHands
	out <- result
}

// BootstrapImportAllLogsWithProgress imports all log files, calling onProgress after
// each file is saved to the database. Files whose is_fully_imported flag is set are
// skipped. Non-active files are parsed concurrently using a worker pool; the DB
// writes are serialized. onProgress may be nil.
func (s *Service) BootstrapImportAllLogsWithProgress(ctx context.Context, onProgress func(BootstrapProgress)) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	paths, err := s.detectLogFiles()
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("no log files found")
	}

	slog.Info("bootstrapping log import", "files", len(paths))

	// DetectAllLogFiles returns newest first. Import oldest -> newest.
	reversed := make([]string, len(paths))
	for i := range paths {
		reversed[i] = paths[len(paths)-1-i]
	}

	// Pre-check which non-active files can be skipped (is_fully_imported=1).
	skipped := 0
	toImport := make([]string, 0, len(reversed))
	for i, p := range reversed {
		isLast := i == len(reversed)-1
		if !isLast {
			cursor, cerr := s.repo.GetCursor(ctx, p)
			if cerr == nil && cursor != nil && cursor.IsFullyImported {
				slog.Debug("skipping fully-imported file", "path", p)
				skipped++
				continue
			}
		}
		toImport = append(toImport, p)
	}

	// Separate the last (active) file from historical files.
	activeFile := reversed[len(reversed)-1]
	historicalFiles := toImport
	if len(historicalFiles) > 0 && historicalFiles[len(historicalFiles)-1] == activeFile {
		historicalFiles = historicalFiles[:len(historicalFiles)-1]
	}

	prog := BootstrapProgress{Total: len(toImport), Skipped: skipped}

	// --- Parallel parse of historical files ---
	if len(historicalFiles) > 0 {
		workers := runtime.GOMAXPROCS(0)
		if workers > 4 {
			workers = 4
		}
		if workers < 1 {
			workers = 1
		}
		slog.Debug("parallel parse", "files", len(historicalFiles), "workers", workers)

		jobCh := make(chan string, len(historicalFiles))
		resultCh := make(chan parseResult, workers*2)

		// Launch workers.
		var wg sync.WaitGroup
		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for path := range jobCh {
					if ctx.Err() != nil {
						return
					}
					parseWorker(ctx, path, resultCh)
				}
			}()
		}

		// Feed jobs.
		for _, p := range historicalFiles {
			jobCh <- p
		}
		close(jobCh)

		// Wait for all workers then close resultCh.
		go func() {
			wg.Wait()
			close(resultCh)
		}()

		// Collect results and write to DB serially.
		// We collect all results first to report progress in order; results arrive
		// out-of-order because workers run in parallel.
		type indexedResult struct {
			idx int
			res parseResult
		}
		// Build a path-to-index map for progress ordering.
		pathIdx := make(map[string]int, len(historicalFiles))
		for i, p := range historicalFiles {
			pathIdx[p] = i
		}
		collected := make([]parseResult, len(historicalFiles))
		seen := 0
		for res := range resultCh {
			idx := pathIdx[res.path]
			collected[idx] = res
			seen++
		}

		// Now write to DB in order (oldest → newest).
		for i, res := range collected {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if res.err != nil {
				return "", fmt.Errorf("parse %q: %w", res.path, res.err)
			}

			cursor := buildImportCursorWithContext(res.path, res.byteOffset, res.lineNumber, res.parser)
			cursor.IsFullyImported = true
			if err := s.saveImportBatch(ctx, res.hands, cursor); err != nil {
				return "", fmt.Errorf("save %q: %w", res.path, err)
			}

			prog.Current++
			prog.Path = res.path
			prog.Skipped = skipped + i
			if onProgress != nil {
				onProgress(prog)
			}
			slog.Debug("historical file imported", "path", res.path, "hands", len(res.hands))
		}
	}

	// --- Import the active (latest) file serially to activate parser state ---
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	prog.Current++
	prog.Path = activeFile
	if onProgress != nil {
		onProgress(prog)
	}
	// Attempt to resume from existing cursor (world context + byte offset).
	activeCursor, cerr := s.repo.GetCursor(ctx, activeFile)
	if cerr != nil {
		slog.Warn("failed to load cursor for active file, scanning from start", "path", activeFile, "error", cerr)
		activeCursor = nil
	}
	if err := s.importFileFrom(ctx, activeFile, true, activeCursor); err != nil {
		return "", fmt.Errorf("import active file %q: %w", activeFile, err)
	}

	slog.Info("bootstrap import complete", "files", len(paths), "skipped", skipped)
	return paths[0], nil
}

// MarkLogFullyImported marks the given log file path as fully imported in the
// persistence layer. This should be called when a new log file is detected so
// the previous log is never re-scanned on future startups.
func (s *Service) MarkLogFullyImported(ctx context.Context, path string) {
	if path == "" {
		return
	}
	if err := s.repo.MarkFullyImported(ctx, path); err != nil {
		slog.Warn("failed to mark log as fully imported", "path", path, "error", err)
	}
}

func (s *Service) ChangeLogFile(ctx context.Context, path string) error {
	return s.importFile(ctx, path, true)
}

func (s *Service) importFile(ctx context.Context, path string, activate bool) error {
	return s.importFileFrom(ctx, path, activate, nil)
}

// importFileFrom imports path, optionally resuming from cursor's byte offset.
// If cursor is non-nil and has WorldCtx, the parser is restored from context
// and parsing begins at cursor.NextByteOffset (skipping already-processed bytes).
func (s *Service) importFileFrom(ctx context.Context, path string, activate bool, cursor *persistence.ImportCursor) error {
	slog.Debug("importing file", "path", path, "activate", activate)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Determine start offset: resume from cursor when possible.
	startByte := int64(0)
	p := parser.NewParser()
	if cursor != nil && cursor.WorldCtx != nil && cursor.NextByteOffset > 0 {
		startByte = cursor.NextByteOffset
		p = restoreParserFromCursor(cursor)
		slog.Debug("resuming file parse from offset", "path", path, "offset", startByte)
		if _, err := f.Seek(startByte, io.SeekStart); err != nil {
			// On seek failure, fall back to scanning from beginning.
			slog.Warn("seek failed, re-scanning from start", "path", path, "error", err)
			startByte = 0
			p = parser.NewParser()
			if _, err2 := f.Seek(0, io.SeekStart); err2 != nil {
				return err2
			}
		}
	}

	lineNo := int64(0)
	handStartLn := int64(0)
	byteOffset := startByte
	handStartByte := startByte
	parsedHands := p.HandCount() // already-restored hands don't count as new

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}

		line := scanner.Text()
		lineStartByte := byteOffset
		lineNo++
		byteOffset += int64(len(line)) + 1

		markHandStart(line, lineNo, lineStartByte, &handStartLn, &handStartByte)

		_ = p.ParseLine(line)
		if p.HandCount() > parsedHands {
			hands := p.GetHands()
			newRows := collectNewPersistedHands(path, hands, &parsedHands, lineNo, &handStartLn, &handStartByte, lineStartByte, byteOffset)
			cursor := buildImportCursorWithContext(path, byteOffset, lineNo, p)
			if err := s.saveImportBatch(ctx, newRows, cursor); err != nil {
				return fmt.Errorf("save imported hands: %w", err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if err := s.saveImportBatch(ctx, nil, buildImportCursorWithContext(path, byteOffset, lineNo, p)); err != nil {
		return err
	}

	s.invalidateStatsCache()

	if activate {
		s.mu.Lock()
		s.parser = p
		s.logPath = path
		s.localSeat = p.GetLocalSeat()
		s.parsedHands = parsedHands
		s.currentLineNumber = lineNo
		s.currentHandStartLn = maxInt64(handStartLn, 1)
		s.currentByteOffset = byteOffset
		s.currentHandStartByte = maxInt64(handStartByte, 0)
		s.mu.Unlock()
	}

	slog.Debug("file import complete", "path", path, "hands", parsedHands)
	return nil
}

func (s *Service) ImportLines(ctx context.Context, sourcePath string, lines []string, startOffset int64, endOffset int64) error {
	if len(lines) == 0 {
		return nil
	}

	s.mu.RLock()
	if sourcePath == "" {
		sourcePath = s.logPath
	}
	if sourcePath == "" || sourcePath != s.logPath {
		s.mu.RUnlock()
		return nil
	}

	workingParser := s.parser.Clone()
	parsedHands := s.parsedHands
	lineNo := s.currentLineNumber
	handStartLn := s.currentHandStartLn
	byteOffset := s.currentByteOffset
	handStartByte := s.currentHandStartByte
	s.mu.RUnlock()

	if startOffset > 0 {
		byteOffset = startOffset
		if handStartByte <= 0 {
			handStartByte = startOffset
		}
	}

	newRows := make([]persistence.PersistedHand, 0)
	for i, line := range lines {
		if err := ctx.Err(); err != nil {
			return err
		}

		lineStartByte := byteOffset
		lineNo++
		lineEndByte := lineStartByte + int64(len(line))
		if i < len(lines)-1 {
			lineEndByte++
		} else if endOffset > lineEndByte {
			lineEndByte = endOffset
		}
		byteOffset = lineEndByte

		markHandStart(line, lineNo, lineStartByte, &handStartLn, &handStartByte)

		_ = workingParser.ParseLine(line)
		if workingParser.HandCount() > parsedHands {
			hands := workingParser.GetHands()
			newRows = append(newRows, collectNewPersistedHands(sourcePath, hands, &parsedHands, lineNo, &handStartLn, &handStartByte, lineStartByte, lineEndByte)...)
		}
	}

	if endOffset > byteOffset {
		byteOffset = endOffset
	}

	cursor := buildImportCursorWithContext(sourcePath, byteOffset, lineNo, workingParser)
	if err := s.saveImportBatch(ctx, newRows, cursor); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if sourcePath != s.logPath {
		return nil
	}
	s.parser = workingParser
	s.localSeat = workingParser.GetLocalSeat()
	s.parsedHands = parsedHands
	s.currentLineNumber = lineNo
	s.currentHandStartLn = maxInt64(handStartLn, 1)
	s.currentByteOffset = byteOffset
	s.currentHandStartByte = maxInt64(handStartByte, 0)

	s.invalidateStatsCache()
	return nil
}

func (s *Service) LogPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.logPath
}

func (s *Service) GetCursor(ctx context.Context, path string) (*persistence.ImportCursor, error) {
	if path == "" {
		return nil, nil
	}
	return s.repo.GetCursor(ctx, path)
}

func (s *Service) NextOffset(ctx context.Context, path string) (int64, error) {
	cursor, err := s.GetCursor(ctx, path)
	if err != nil || cursor == nil {
		return 0, err
	}
	return cursor.NextByteOffset, nil
}

func (s *Service) Snapshot(ctx context.Context) (*stats.Stats, []*parser.Hand, int, error) {
	s.mu.RLock()
	localSeat := s.localSeat
	s.mu.RUnlock()

	var filter persistence.HandFilter
	filter.OnlyComplete = true

	hands, err := s.repo.ListHands(ctx, filter)
	if err != nil {
		return nil, nil, localSeat, err
	}

	slog.Debug("snapshot", "hands", len(hands), "localSeat", localSeat)
	return nil, hands, localSeat, nil
}

// GetHandByUID returns full hand data for a single UID (used by the hand detail view).
func (s *Service) GetHandByUID(ctx context.Context, uid string) (*parser.Hand, error) {
	return s.repo.GetHandByUID(ctx, uid)
}

// ListHandSummaries returns lightweight hand summaries for list display and total count.
// Only complete hands are returned, ordered by start_time DESC (newest first).
func (s *Service) ListHandSummaries(ctx context.Context, f persistence.HandFilter) ([]persistence.HandSummary, int, error) {
	f.OnlyComplete = true
	return s.repo.ListHandSummaries(ctx, f)
}

// Stats returns aggregated stats for the given filter.
// When no time range is set (AllTime mode) it uses an IncrementalCalculator with a
// watermark so only new hands are re-processed on each call.
// For period-filter modes a small LRU-style cache (keyed by filter + hand count)
// avoids redundant full-scan calculations.
func (s *Service) Stats(ctx context.Context, filter persistence.HandFilter) (*stats.Stats, int, error) {
	s.mu.RLock()
	localSeat := s.localSeat
	s.mu.RUnlock()

	if filter.FromTime == nil && filter.ToTime == nil {
		// AllTime mode — use IncrementalCalculator.
		s.incMu.Lock()
		defer s.incMu.Unlock()

		if s.incCalc == nil || s.incLocalSeat != localSeat {
			s.incCalc = stats.NewIncrementalCalculator(localSeat)
			s.incLocalSeat = localSeat
			s.watermark = time.Time{}
		}

		newHands, err := s.repo.ListHandsAfter(ctx, s.watermark, localSeat)
		if err != nil {
			return nil, localSeat, err
		}

		for _, h := range newHands {
			s.incCalc.Feed(h)
			if !h.StartTime.IsZero() && h.StartTime.After(s.watermark) {
				s.watermark = h.StartTime
			}
		}

		return s.incCalc.Compute(), localSeat, nil
	}

	// Period-filter mode — use cache keyed by (fromTime, toTime, localSeat, handCount).
	count, err := s.repo.CountHands(ctx, filter)
	if err != nil {
		return nil, localSeat, err
	}

	fromTime := time.Time{}
	if filter.FromTime != nil {
		fromTime = *filter.FromTime
	}
	toTime := time.Time{}
	if filter.ToTime != nil {
		toTime = *filter.ToTime
	}
	key := statsCacheKey{
		fromTime:  fromTime,
		toTime:    toTime,
		localSeat: localSeat,
		handCount: count,
	}

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	if cached, ok := s.statsCache[key]; ok {
		return cached, localSeat, nil
	}

	// Cache miss: full compute.
	filter.OnlyComplete = true
	hands, err := s.repo.ListHands(ctx, filter)
	if err != nil {
		return nil, localSeat, err
	}

	calc := stats.NewCalculator()
	result := calc.Calculate(hands, localSeat)

	if s.statsCache == nil {
		s.statsCache = make(map[statsCacheKey]*stats.Stats)
	}
	// Keep cache small: evict all entries if >= 8.
	if len(s.statsCache) >= 8 {
		s.statsCache = make(map[statsCacheKey]*stats.Stats)
	}
	s.statsCache[key] = result
	return result, localSeat, nil
}

// invalidateStatsCache clears the period-filter stats cache.
// The AllTime incremental calculator is NOT reset — it picks up new hands via
// ListHandsAfter(watermark) on the next Stats() call.
func (s *Service) invalidateStatsCache() {
	s.cacheMu.Lock()
	s.statsCache = nil
	s.cacheMu.Unlock()
}

func (s *Service) Close() error {
	if c, ok := s.repo.(interface{ Close() error }); ok {
		return c.Close()
	}
	return nil
}

func (s *Service) saveImportBatch(ctx context.Context, hands []persistence.PersistedHand, cursor persistence.ImportCursor) error {
	if repo, ok := s.repo.(persistence.ImportBatchRepository); ok {
		_, err := repo.SaveImportBatch(ctx, hands, cursor)
		return err
	}

	if len(hands) > 0 {
		if _, err := s.repo.UpsertHands(ctx, hands); err != nil {
			return err
		}
	}
	return s.repo.SaveCursor(ctx, cursor)
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func markHandStart(line string, lineNo, lineStartByte int64, handStartLn, handStartByte *int64) {
	if parser.IsNewHandLine(line) && *handStartLn <= 0 {
		*handStartLn = lineNo
		*handStartByte = lineStartByte
	}
}

func collectNewPersistedHands(sourcePath string, hands []*parser.Hand, parsedHands *int, lineNo int64, handStartLn, handStartByte *int64, lineStartByte, lineEndByte int64) []persistence.PersistedHand {
	if len(hands) <= *parsedHands {
		return nil
	}
	newRows := make([]persistence.PersistedHand, 0, len(hands)-*parsedHands)
	for i := *parsedHands; i < len(hands); i++ {
		h := hands[i]
		if *handStartLn <= 0 {
			*handStartLn = maxInt64(1, lineNo)
			*handStartByte = lineStartByte
		}
		source := persistence.HandSourceRef{
			SourcePath: sourcePath,
			StartByte:  *handStartByte,
			EndByte:    lineEndByte,
			StartLine:  *handStartLn,
			EndLine:    lineNo,
		}
		source.HandUID = persistence.GenerateHandUID(h, source)
		newRows = append(newRows, persistence.PersistedHand{Hand: h, Source: source})
		*handStartLn = lineNo
		*handStartByte = lineEndByte
	}
	*parsedHands = len(hands)
	return newRows
}

func buildImportCursorWithContext(sourcePath string, nextByteOffset, nextLineNumber int64, p *parser.Parser) persistence.ImportCursor {
	wc := p.WorldContext()
	return persistence.ImportCursor{
		SourcePath:     sourcePath,
		NextByteOffset: nextByteOffset,
		NextLineNumber: nextLineNumber,
		UpdatedAt:      time.Now(),
		WorldCtx:       &wc,
	}
}

// restoreParserFromCursor creates a new Parser and restores world context from
// the cursor if available. The caller should then seek the file to
// cursor.NextByteOffset before parsing further lines.
func restoreParserFromCursor(cursor *persistence.ImportCursor) *parser.Parser {
	p := parser.NewParser()
	if cursor == nil || cursor.WorldCtx == nil {
		return p
	}
	p.RestoreWorldContext(*cursor.WorldCtx)
	return p
}
