package application

import (
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/logsource"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
)

// importSession carries the parser and source-coordinate bookkeeping shared by
// file import, bootstrap parse workers and tail ImportLines. It returns the
// rows it produces to the caller and never touches persistence itself.
type importSession struct {
	path          string
	parser        *parser.Parser
	parsedHands   int
	lineNo        int64
	handStartLn   int64
	handStartByte int64
	byteOffset    int64
}

// newImportSession starts a session on p. parsedHands is p.HandCount() at start
// so restored hands are not reported as new.
func newImportSession(path string, p *parser.Parser, lineNo, handStartLn, handStartByte, byteOffset int64) *importSession {
	return &importSession{
		path:          path,
		parser:        p,
		parsedHands:   p.HandCount(),
		lineNo:        lineNo,
		handStartLn:   handStartLn,
		handStartByte: handStartByte,
		byteOffset:    byteOffset,
	}
}

// feed decodes the line once, records the hand-start hint, applies the record
// and returns any newly completed hands.
func (s *importSession) feed(line logsource.Line) []persistence.PersistedHand {
	s.lineNo = line.Number
	s.byteOffset = line.EndByte

	rec := s.parser.DecodeLine(line.Text)
	markHandStartHint(rec.HandStartHint, line.Number, line.StartByte, &s.handStartLn, &s.handStartByte)
	_ = s.parser.ParseRecord(rec)
	if s.parser.HandCount() > s.parsedHands {
		return collectNewPersistedHands(s.path, s.parser.GetHands(), &s.parsedHands, line.Number,
			&s.handStartLn, &s.handStartByte, line.StartByte, line.EndByte)
	}
	return nil
}

func markHandStartHint(hint bool, lineNo, lineStartByte int64, handStartLn, handStartByte *int64) {
	if hint && *handStartLn <= 0 {
		*handStartLn = lineNo
		*handStartByte = lineStartByte
	}
}
