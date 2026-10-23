package parser

import (
	"testing"
	"time"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/logevent"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/logformat"
)

type fixedTestLexer struct {
	record logevent.Record
}

func (l *fixedTestLexer) Decode(string) logevent.Record {
	return l.record
}

func TestNewParserWithLexerAndClone(t *testing.T) {
	ts := time.Date(2026, time.February, 1, 2, 43, 0, 0, time.UTC)
	lexer := &fixedTestLexer{record: logevent.Record{
		Timestamp:    ts,
		HasTimestamp: true,
		Event:        logevent.NewGame{},
	}}
	p := NewParserWithLexer(lexer)
	if err := p.ParseLine("synthetic input"); err != nil {
		t.Fatalf("ParseLine() error = %v", err)
	}
	if p.currentHand == nil || !p.currentHand.StartTime.Equal(ts) {
		t.Fatalf("ParseLine did not use the injected lexer: currentHand = %#v", p.currentHand)
	}

	clone := p.Clone()
	if clone.lexer != p.lexer {
		t.Fatal("Clone did not share the read-only lexer")
	}
	if clone.currentHand == p.currentHand {
		t.Fatal("Clone shared current hand state")
	}

	if err := p.ParseRecord(logevent.Record{
		Timestamp:    ts.Add(time.Second),
		HasTimestamp: true,
		Event:        logevent.LocalSeat{Seat: 4},
	}); err != nil {
		t.Fatalf("ParseRecord(LocalSeat) error = %v", err)
	}
	if p.result.LocalPlayerSeat != 4 || clone.result.LocalPlayerSeat != -1 {
		t.Fatalf("local seat leaked across clone: original=%d clone=%d", p.result.LocalPlayerSeat, clone.result.LocalPlayerSeat)
	}
	if clone.currentHand.LocalPlayerSeat != -1 {
		t.Fatalf("current hand state leaked across clone: clone seat=%d", clone.currentHand.LocalPlayerSeat)
	}

	if err := p.ParseRecord(logevent.Record{
		Timestamp:    ts.Add(2 * time.Second),
		HasTimestamp: true,
		Event:        logevent.SmallBlind{Seat: 1, Amount: 5},
	}); err != nil {
		t.Fatalf("ParseRecord(SmallBlind) error = %v", err)
	}
	if _, ok := clone.currentHand.Players[1]; ok {
		t.Fatal("player map state leaked across clone")
	}
}

func TestNewParserWithLexerNilUsesLegacyLexer(t *testing.T) {
	p := NewParserWithLexer(nil)
	if _, ok := p.lexer.(logformat.LegacyLexer); !ok {
		t.Fatalf("default lexer type = %T, want logformat.LegacyLexer", p.lexer)
	}
	record := p.DecodeLine("2026.02.01 02:43:00 INFO - [Table]: Preparing for New Game")
	if !record.HasTimestamp || !record.HandStartHint {
		t.Fatalf("default lexer record = %#v", record)
	}
}
