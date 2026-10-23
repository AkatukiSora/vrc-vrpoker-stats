package application

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/logevent"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/logformat"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/logsource"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
)

type countingLexer struct{ calls atomic.Int64 }

func (l *countingLexer) Decode(line string) logevent.Record {
	l.calls.Add(1)
	return logformat.LegacyLexer{}.Decode(line)
}

func TestImportSessionDecodesEachLineOnce(t *testing.T) {
	t.Parallel()

	lex := &countingLexer{}
	sess := newImportSession("x.log", parser.NewParserWithLexer(lex), 0, 0, 0, 0)

	lines := strings.Split(strings.TrimSuffix(testHandLog("00:00"), "\n"), "\n")
	var rows []persistence.PersistedHand
	var off int64
	for i, text := range lines {
		l := logsource.Line{Text: text, Number: int64(i + 1), StartByte: off, EndByte: off + int64(len(text)) + 1}
		off = l.EndByte
		rows = append(rows, sess.feed(l)...)
	}
	if got := lex.calls.Load(); got != int64(len(lines)) {
		t.Fatalf("Decode calls = %d, want %d", got, len(lines))
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	src := rows[0].Source
	if src.StartLine != 1 || src.StartByte != 0 || src.EndLine != int64(len(lines)) || src.HandUID == "" {
		t.Fatalf("unexpected source span: %+v", src)
	}
	if sess.lineNo != int64(len(lines)) || sess.byteOffset != off {
		t.Fatalf("session position = %d/%d, want %d/%d", sess.lineNo, sess.byteOffset, len(lines), off)
	}
}

// fakeSource serves fixed content and records the requested offsets.
type fakeSource struct {
	content map[string]string
	offsets []int64
}

func (f *fakeSource) Open(_ context.Context, path string, startOffset int64) (logsource.Stream, error) {
	text, ok := f.content[path]
	if !ok {
		return nil, errors.New("fake: not found")
	}
	f.offsets = append(f.offsets, startOffset)
	var lines []string
	if text != "" {
		lines = strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	}
	return &fakeStream{lines: lines}, nil
}

type fakeStream struct {
	lines  []string
	i      int
	offset int64
}

func (s *fakeStream) StartOffset() int64 { return 0 }
func (s *fakeStream) Close() error       { return nil }
func (s *fakeStream) Next(ctx context.Context) (logsource.Line, error) {
	if s.i >= len(s.lines) {
		return logsource.Line{}, io.EOF
	}
	if err := ctx.Err(); err != nil {
		return logsource.Line{}, err
	}
	text := s.lines[s.i]
	s.i++
	l := logsource.Line{Text: text, Number: int64(s.i), StartByte: s.offset, EndByte: s.offset + int64(len(text)) + 1}
	s.offset = l.EndByte
	return l, nil
}

func TestServiceImportsThroughInjectedSource(t *testing.T) {
	t.Parallel()

	repo := persistence.NewMemoryRepository()
	svc := NewService(repo, nil)
	svc.source = &fakeSource{content: map[string]string{"fake.log": testHandLog("00:00")}}

	if err := svc.ChangeLogFile(context.Background(), "fake.log"); err != nil {
		t.Fatalf("ChangeLogFile: %v", err)
	}
	_, hands, _, err := svc.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(hands) != 1 {
		t.Fatalf("hands = %d, want 1", len(hands))
	}
	cur, err := repo.GetCursor(context.Background(), "fake.log")
	if err != nil || cur == nil || cur.NextLineNumber != 5 {
		t.Fatalf("cursor = %+v, err = %v", cur, err)
	}
}

// failingRepo hides any batch interface so saveImportBatch uses UpsertHands/SaveCursor.
type failingRepo struct {
	persistence.ImportRepository
	failUpsert bool
}

func (r *failingRepo) UpsertHands(ctx context.Context, hands []persistence.PersistedHand) (persistence.UpsertResult, error) {
	if r.failUpsert {
		return persistence.UpsertResult{}, errors.New("upsert failed")
	}
	return r.ImportRepository.UpsertHands(ctx, hands)
}

func TestImportSaveFailureDoesNotAdoptStateOrCursor(t *testing.T) {
	t.Parallel()

	inner := persistence.NewMemoryRepository()
	svc := NewService(&failingRepo{ImportRepository: inner, failUpsert: true}, nil)
	svc.source = &fakeSource{content: map[string]string{"fake.log": testHandLog("00:00")}}

	err := svc.ChangeLogFile(context.Background(), "fake.log")
	if err == nil || !strings.Contains(err.Error(), "upsert failed") {
		t.Fatalf("err = %v, want upsert failure", err)
	}
	if got := svc.LogPath(); got != "" {
		t.Fatalf("LogPath adopted after save failure: %q", got)
	}
	if cur, _ := inner.GetCursor(context.Background(), "fake.log"); cur != nil {
		t.Fatalf("cursor advanced despite save failure: %+v", cur)
	}
}

func TestImportLinesSaveFailureKeepsServiceState(t *testing.T) {
	t.Parallel()

	repo := &failingRepo{ImportRepository: persistence.NewMemoryRepository()}
	svc := NewService(repo, nil)
	svc.source = &fakeSource{content: map[string]string{"fake.log": ""}}
	if err := svc.ChangeLogFile(context.Background(), "fake.log"); err != nil {
		t.Fatal(err)
	}

	repo.failUpsert = true
	lines := strings.Split(strings.TrimSuffix(testHandLog("00:00"), "\n"), "\n")
	if err := svc.ImportLines(context.Background(), "fake.log", lines, 0, 0); err == nil {
		t.Fatal("expected save failure")
	}
	svc.mu.RLock()
	line, hands, off := svc.currentLineNumber, svc.parsedHands, svc.currentByteOffset
	svc.mu.RUnlock()
	if line != 0 || hands != 0 || off != 0 {
		t.Fatalf("state advanced after failed save: line=%d hands=%d off=%d", line, hands, off)
	}

	repo.failUpsert = false
	if err := svc.ImportLines(context.Background(), "fake.log", lines, 0, 0); err != nil {
		t.Fatalf("retry: %v", err)
	}
	_, got, _, err := svc.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("hands after retry = %d, want 1", len(got))
	}
}
