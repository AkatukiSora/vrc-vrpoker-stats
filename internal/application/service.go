package application

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

var reNewGame = regexp.MustCompile(`\[Table\]: Preparing for New Game`)

type LogFileLocator func() ([]string, error)

type Service struct {
	mu             sync.RWMutex
	repo           persistence.ImportRepository
	parser         *parser.Parser
	calc           *stats.Calculator
	logPath        string
	localSeat      int
	detectLogFiles LogFileLocator

	parsedHands          int
	currentLineNumber    int64
	currentHandStartLn   int64
	currentByteOffset    int64
	currentHandStartByte int64
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
		calc:               stats.NewCalculator(),
		localSeat:          -1,
		currentHandStartLn: 0,
		detectLogFiles:     locator,
	}
}

func (s *Service) BootstrapImportAllLogs(ctx context.Context) (string, error) {
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

	// DetectAllLogFiles returns newest first. Import oldest -> newest.
	reversed := make([]string, len(paths))
	for i := range paths {
		reversed[i] = paths[len(paths)-1-i]
	}

	for i, p := range reversed {
		activate := i == len(reversed)-1
		if err := s.importFile(ctx, p, activate); err != nil {
			return "", err
		}
	}

	return paths[0], nil
}

func (s *Service) ChangeLogFile(ctx context.Context, path string) error {
	return s.importFile(ctx, path, true)
}

func (s *Service) importFile(ctx context.Context, path string, activate bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
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
		if err := ctx.Err(); err != nil {
			return err
		}

		line := scanner.Text()
		lineStartByte := byteOffset
		lineNo++
		byteOffset += int64(len(line)) + 1

		markHandStart(line, lineNo, lineStartByte, &handStartLn, &handStartByte)

		_ = p.ParseLine(line)
		hands := p.GetHands()
		if len(hands) > parsedHands {
			newRows := collectNewPersistedHands(path, hands, &parsedHands, lineNo, &handStartLn, &handStartByte, lineStartByte, byteOffset)
			cursor := buildImportCursor(path, byteOffset, lineNo)
			if err := s.saveImportBatch(ctx, newRows, cursor); err != nil {
				return fmt.Errorf("save imported hands: %w", err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if err := s.saveImportBatch(ctx, nil, buildImportCursor(path, byteOffset, lineNo)); err != nil {
		return err
	}

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
		hands := workingParser.GetHands()
		if len(hands) > parsedHands {
			newRows = append(newRows, collectNewPersistedHands(sourcePath, hands, &parsedHands, lineNo, &handStartLn, &handStartByte, lineStartByte, lineEndByte)...)
		}
	}

	if endOffset > byteOffset {
		byteOffset = endOffset
	}

	cursor := buildImportCursor(sourcePath, byteOffset, lineNo)
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
	calc := s.calc
	s.mu.RUnlock()

	var filter persistence.HandFilter
	filter.OnlyComplete = true

	hands, err := s.repo.ListHands(ctx, filter)
	if err != nil {
		return nil, nil, localSeat, err
	}

	calculated := calc.Calculate(hands, localSeat)
	return calculated, hands, localSeat, nil
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
	if reNewGame.MatchString(line) && *handStartLn <= 0 {
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

func buildImportCursor(sourcePath string, nextByteOffset, nextLineNumber int64) persistence.ImportCursor {
	return persistence.ImportCursor{
		SourcePath:     sourcePath,
		NextByteOffset: nextByteOffset,
		NextLineNumber: nextLineNumber,
		UpdatedAt:      time.Now(),
	}
}
