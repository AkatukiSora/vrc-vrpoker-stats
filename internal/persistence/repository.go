package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
)

type HandFilter struct {
	FromTime          *time.Time
	ToTime            *time.Time
	OnlyComplete      bool
	LocalSeat         *int
	PocketCategoryIDs []int
	FinalClassIDs     []int
	// Limit and Offset are used by ListHandSummaries for pagination.
	// Limit == 0 means no limit (return all matching rows).
	Limit  int
	Offset int
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

// HandSummary is a lightweight hand record for list display.
// It avoids loading the full hand_actions join needed by parser.Hand.
type HandSummary struct {
	HandUID    string
	StartTime  time.Time
	NumPlayers int
	TotalPot   int
	IsComplete bool

	// Local player fields (may be zero if local player not in hand)
	LocalSeat int
	HoleCard0 string // e.g. "Ah" — empty if not known
	HoleCard1 string
	Position  string // position string, empty if unknown
	PotWon    int
	NetChips  int // PotWon - sum(actions.amount) for local player
	Won       bool

	// Community cards as space-separated string, e.g. "Ah Kd 2c"
	CommunityCards string
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
	IsFullyImported bool
	// WorldCtx holds the parser world/instance context at the cursor position.
	// When set, the parser can resume from NextByteOffset without full re-scan.
	// Uses parser.WorldContext directly to avoid duplicating the struct definition.
	WorldCtx  *parser.WorldContext
	UpdatedAt time.Time
}

type HandRepository interface {
	UpsertHands(ctx context.Context, hands []PersistedHand) (UpsertResult, error)
	ListHands(ctx context.Context, f HandFilter) ([]*parser.Hand, error)
	CountHands(ctx context.Context, f HandFilter) (int, error)
	// ListHandsAfter returns complete, stats-eligible hands with start_time > after,
	// ordered by start_time ASC. Used for incremental stats updates.
	ListHandsAfter(ctx context.Context, after time.Time, localSeat int) ([]*parser.Hand, error)
	// ListHandSummaries returns lightweight hand summaries for list display and
	// the total count of matching hands (ignoring Limit/Offset).
	// Only complete hands are returned, ordered by start_time DESC (newest first).
	ListHandSummaries(ctx context.Context, f HandFilter) ([]HandSummary, int, error)
	// GetHandByUID returns the full hand data for a single hand UID.
	// Returns nil, nil if not found.
	GetHandByUID(ctx context.Context, uid string) (*parser.Hand, error)
}

type CursorRepository interface {
	GetCursor(ctx context.Context, sourcePath string) (*ImportCursor, error)
	SaveCursor(ctx context.Context, c ImportCursor) error
	// MarkFullyImported atomically sets is_fully_imported=1 on an existing cursor.
	// If no cursor row exists yet the call is a no-op.
	MarkFullyImported(ctx context.Context, sourcePath string) error
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
	appendInt(&b, h.LocalPlayerSeat)
	b.WriteByte('|')
	appendInt(&b, h.SBSeat)
	b.WriteByte('|')
	appendInt(&b, h.BBSeat)
	b.WriteByte('|')
	appendInt(&b, h.WinnerSeat)
	b.WriteByte('|')
	appendInt(&b, h.TotalPot)
	b.WriteByte('|')
	b.WriteString(h.WinType)

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
		appendInt(&b, seat)
		b.WriteByte(':')
		appendInt(&b, int(pi.Position))
		b.WriteByte(':')
		b.WriteString(strconv.FormatBool(pi.ShowedDown))
		b.WriteByte(':')
		b.WriteString(strconv.FormatBool(pi.Won))
		b.WriteByte(':')
		appendInt(&b, pi.PotWon)
		b.WriteByte(':')
		for _, c := range pi.HoleCards {
			b.WriteString(c.Rank)
			b.WriteString(c.Suit)
		}
		b.WriteString(":")
		for _, act := range pi.Actions {
			b.WriteString(act.Timestamp.UTC().Format(time.RFC3339Nano))
			b.WriteByte('/')
			appendInt(&b, int(act.Street))
			b.WriteByte('/')
			appendInt(&b, int(act.Action))
			b.WriteByte('/')
			appendInt(&b, act.Amount)
			b.WriteByte('/')
		}
		b.WriteString(";")
	}

	payload := b.String()
	s := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(s[:])
}

func appendInt(b *strings.Builder, v int) {
	b.WriteString(strconv.Itoa(v))
}
