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
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/watcher"
)

var reNewGame = regexp.MustCompile(`\[Table\]: Preparing for New Game`)

type Service struct {
	mu        sync.RWMutex
	repo      persistence.ImportRepository
	parser    *parser.Parser
	calc      *stats.Calculator
	logPath   string
	localSeat int

	parsedHands        int
	currentLineNumber  int64
	currentHandStartLn int64
}

func NewService(repo persistence.ImportRepository) *Service {
	return &Service{
		repo:               repo,
		parser:             parser.NewParser(),
		calc:               stats.NewCalculator(),
		localSeat:          -1,
		currentHandStartLn: 1,
	}
}

func (s *Service) BootstrapImportAllLogs() (string, error) {
	paths, err := watcher.DetectAllLogFiles()
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

	for _, p := range reversed {
		if err := s.importFile(p, false); err != nil {
			return "", err
		}
	}

	latest := paths[0]
	if err := s.importFile(latest, true); err != nil {
		return "", err
	}
	return latest, nil
}

func (s *Service) ChangeLogFile(path string) error {
	return s.importFile(path, true)
}

func (s *Service) importFile(path string, activate bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	p := parser.NewParser()
	lineNo := int64(0)
	handStart := int64(1)
	parsedHands := 0

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		lineNo++

		if reNewGame.MatchString(line) {
			if handStart == 0 {
				handStart = lineNo
			}
		}

		_ = p.ParseLine(line)
		hands := p.GetHands()
		if len(hands) > parsedHands {
			newRows := make([]persistence.PersistedHand, 0, len(hands)-parsedHands)
			for i := parsedHands; i < len(hands); i++ {
				h := hands[i]
				if handStart <= 0 {
					handStart = maxInt64(1, lineNo)
				}
				source := persistence.HandSourceRef{
					SourcePath: path,
					StartByte:  handStart,
					EndByte:    lineNo,
					StartLine:  handStart,
					EndLine:    lineNo,
				}
				source.HandUID = persistence.GenerateHandUID(h, source)
				newRows = append(newRows, persistence.PersistedHand{Hand: h, Source: source})
				handStart = lineNo
			}
			if _, err := s.repo.UpsertHands(context.Background(), newRows); err != nil {
				return fmt.Errorf("save imported hands: %w", err)
			}
			parsedHands = len(hands)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if err := s.repo.SaveCursor(context.Background(), persistence.ImportCursor{
		SourcePath:     path,
		NextByteOffset: 0,
		NextLineNumber: lineNo,
		UpdatedAt:      time.Now(),
	}); err != nil {
		return err
	}

	if activate {
		s.mu.Lock()
		s.parser = p
		s.logPath = path
		s.localSeat = p.GetLocalSeat()
		s.parsedHands = parsedHands
		s.currentLineNumber = lineNo
		s.currentHandStartLn = maxInt64(handStart, 1)
		s.mu.Unlock()
	}

	return nil
}

func (s *Service) ImportLines(lines []string, _ int64, endOffset int64) error {
	s.mu.Lock()
	p := s.parser
	path := s.logPath
	parsedHands := s.parsedHands
	lineNo := s.currentLineNumber
	handStart := s.currentHandStartLn

	newRows := make([]persistence.PersistedHand, 0)
	for _, line := range lines {
		lineNo++
		if reNewGame.MatchString(line) && handStart <= 0 {
			handStart = lineNo
		}
		_ = p.ParseLine(line)
		hands := p.GetHands()
		if len(hands) > parsedHands {
			for i := parsedHands; i < len(hands); i++ {
				h := hands[i]
				if handStart <= 0 {
					handStart = lineNo
				}
				source := persistence.HandSourceRef{
					SourcePath: path,
					StartByte:  handStart,
					EndByte:    lineNo,
					StartLine:  handStart,
					EndLine:    lineNo,
				}
				source.HandUID = persistence.GenerateHandUID(h, source)
				newRows = append(newRows, persistence.PersistedHand{Hand: h, Source: source})
				handStart = lineNo
			}
			parsedHands = len(hands)
		}
	}

	s.localSeat = p.GetLocalSeat()
	s.parsedHands = parsedHands
	s.currentLineNumber = lineNo
	s.currentHandStartLn = handStart
	s.mu.Unlock()

	if len(newRows) > 0 {
		if _, err := s.repo.UpsertHands(context.Background(), newRows); err != nil {
			return err
		}
	}

	return s.repo.SaveCursor(context.Background(), persistence.ImportCursor{
		SourcePath:     path,
		NextByteOffset: endOffset,
		NextLineNumber: lineNo,
		UpdatedAt:      time.Now(),
	})
}

func (s *Service) LogPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.logPath
}

func (s *Service) Snapshot() (*stats.Stats, []*parser.Hand, int, error) {
	s.mu.RLock()
	localSeat := s.localSeat
	calc := s.calc
	s.mu.RUnlock()

	var filter persistence.HandFilter
	filter.OnlyComplete = true

	hands, err := s.repo.ListHands(context.Background(), filter)
	if err != nil {
		return nil, nil, localSeat, err
	}

	calculated := calc.Calculate(hands, localSeat)
	return calculated, hands, localSeat, nil
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
