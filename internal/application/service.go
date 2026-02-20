package application

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

type Service struct {
	mu        sync.RWMutex
	repo      persistence.ImportRepository
	parser    *parser.Parser
	calc      *stats.Calculator
	logPath   string
	localSeat int
}

func NewService(repo persistence.ImportRepository) *Service {
	return &Service{
		repo:      repo,
		parser:    parser.NewParser(),
		calc:      stats.NewCalculator(),
		localSeat: -1,
	}
}

func (s *Service) ChangeLogFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	nextParser := parser.NewParser()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		_ = nextParser.ParseLine(scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	hands := nextParser.GetHands()
	upserts := make([]persistence.PersistedHand, 0, len(hands))
	for _, h := range hands {
		upserts = append(upserts, persistence.PersistedHand{
			Hand: h,
			Source: persistence.HandSourceRef{
				SourcePath: path,
			},
		})
	}
	_, err = s.repo.UpsertHands(context.Background(), upserts)
	if err != nil {
		return fmt.Errorf("save imported hands: %w", err)
	}

	s.mu.Lock()
	s.parser = nextParser
	s.logPath = path
	s.localSeat = nextParser.GetLocalSeat()
	s.mu.Unlock()

	return nil
}

func (s *Service) ImportLines(lines []string) error {
	s.mu.Lock()
	p := s.parser
	for _, line := range lines {
		_ = p.ParseLine(line)
	}
	s.localSeat = p.GetLocalSeat()
	path := s.logPath
	hands := p.GetHands()
	s.mu.Unlock()

	upserts := make([]persistence.PersistedHand, 0, len(hands))
	for _, h := range hands {
		upserts = append(upserts, persistence.PersistedHand{
			Hand: h,
			Source: persistence.HandSourceRef{
				SourcePath: path,
			},
		})
	}
	_, err := s.repo.UpsertHands(context.Background(), upserts)
	return err
}

func (s *Service) LogPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.logPath
}

func (s *Service) Snapshot() (*stats.Stats, []*parser.Hand, int, error) {
	s.mu.RLock()
	localSeat := s.localSeat
	s.mu.RUnlock()

	var filter persistence.HandFilter
	if localSeat >= 0 {
		filter.LocalSeat = &localSeat
	}
	filter.OnlyComplete = true

	hands, err := s.repo.ListHands(context.Background(), filter)
	if err != nil {
		return nil, nil, localSeat, err
	}

	s.mu.RLock()
	calc := s.calc
	s.mu.RUnlock()

	calculated := calc.Calculate(hands, localSeat)
	return calculated, hands, localSeat, nil
}

func (s *Service) SaveCursor(offset int64) error {
	path := s.LogPath()
	if path == "" {
		return nil
	}
	return s.repo.SaveCursor(context.Background(), persistence.ImportCursor{
		SourcePath:     path,
		NextByteOffset: offset,
		NextLineNumber: 0,
		UpdatedAt:      time.Now(),
	})
}
