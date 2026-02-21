package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

func GenerateHandUID(h *parser.Hand, src HandSourceRef) string {
	payload := fmt.Sprintf(
		"%s|%d|%d|%d|%d|%d|%d|%s",
		src.SourcePath,
		src.StartByte,
		src.EndByte,
		src.StartLine,
		src.EndLine,
		h.ID,
		h.StartTime.UnixNano(),
		h.WinType,
	)
	s := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(s[:])
}
