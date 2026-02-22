package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
)

type HandFilter struct {
	FromTime     *time.Time
	ToTime       *time.Time
	OnlyComplete bool
	LocalSeat    *int
}

type HandSourceRef struct {
	SourcePath string
	StartByte  int64
	EndByte    int64
	StartLine  int64
	EndLine    int64
	HandUID    string
}

type PersistedHand struct {
	Hand   *parser.Hand
	Source HandSourceRef
}

type UpsertResult struct {
	Inserted int
	Updated  int
	Skipped  int
}

type ImportCursor struct {
	SourcePath      string
	NextByteOffset  int64
	NextLineNumber  int64
	LastEventTime   *time.Time
	LastHandUID     string
	ParserStateJSON []byte
	UpdatedAt       time.Time
}

type HandRepository interface {
	UpsertHands(ctx context.Context, hands []PersistedHand) (UpsertResult, error)
	ListHands(ctx context.Context, f HandFilter) ([]*parser.Hand, error)
	CountHands(ctx context.Context, f HandFilter) (int, error)
}

type CursorRepository interface {
	GetCursor(ctx context.Context, sourcePath string) (*ImportCursor, error)
	SaveCursor(ctx context.Context, c ImportCursor) error
}

type ImportRepository interface {
	HandRepository
	CursorRepository
}

type ImportBatchRepository interface {
	ImportRepository
	SaveImportBatch(ctx context.Context, hands []PersistedHand, cursor ImportCursor) (UpsertResult, error)
}

func GenerateHandUID(h *parser.Hand, src HandSourceRef) string {
	if h == nil {
		payload := fmt.Sprintf("src:%s|%d|%d|%d|%d", src.SourcePath, src.StartByte, src.EndByte, src.StartLine, src.EndLine)
		s := sha256.Sum256([]byte(payload))
		return hex.EncodeToString(s[:])
	}

	b := strings.Builder{}
	b.WriteString("v2|")
	b.WriteString(h.StartTime.UTC().Format(time.RFC3339Nano))
	b.WriteString("|")
	b.WriteString(h.EndTime.UTC().Format(time.RFC3339Nano))
	b.WriteString("|")
	b.WriteString(h.WorldID)
	b.WriteString("|")
	b.WriteString(h.InstanceUID)
	b.WriteString("|")
	b.WriteString(fmt.Sprintf("%d|%d|%d|%d|%d|%s", h.LocalPlayerSeat, h.SBSeat, h.BBSeat, h.WinnerSeat, h.TotalPot, h.WinType))

	b.WriteString("|B:")
	for _, c := range h.CommunityCards {
		b.WriteString(c.Rank)
		b.WriteString(c.Suit)
		b.WriteString(",")
	}

	seats := make([]int, 0, len(h.Players))
	for seat := range h.Players {
		seats = append(seats, seat)
	}
	sort.Ints(seats)
	b.WriteString("|P:")
	for _, seat := range seats {
		pi := h.Players[seat]
		if pi == nil {
			continue
		}
		b.WriteString(fmt.Sprintf("%d:%d:%t:%t:%d:", seat, pi.Position, pi.ShowedDown, pi.Won, pi.PotWon))
		for _, c := range pi.HoleCards {
			b.WriteString(c.Rank)
			b.WriteString(c.Suit)
		}
		b.WriteString(":")
		for _, act := range pi.Actions {
			b.WriteString(fmt.Sprintf("%s/%d/%d/%d/", act.Timestamp.UTC().Format(time.RFC3339Nano), act.Street, act.Action, act.Amount))
		}
		b.WriteString(";")
	}

	payload := b.String()
	s := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(s[:])
}
