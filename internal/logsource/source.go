// Package logsource collects raw log lines with byte/line coordinates.
// It knows nothing about log formats, hands, parsers or persistence.
package logsource

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"os"
)

const (
	scannerInitialBuffer = 1024 * 1024
	scannerMaxBuffer     = 4 * 1024 * 1024
)

// Line is a single log line with its position in the source.
type Line struct {
	Text string
	// Number is the 1-based line count since the stream started
	// (independent of any restored cursor).
	Number int64
	// StartByte/EndByte delimit the line; EndByte counts one terminator byte
	// per line (len(Text)+1), matching the historical import arithmetic.
	StartByte, EndByte int64
}

// Stream yields lines from one opened source. A Stream is used by one worker.
type Stream interface {
	// StartOffset is the offset the stream actually started from.
	StartOffset() int64
	// Next returns the next line; io.EOF marks the end.
	Next(ctx context.Context) (Line, error)
	Close() error
}

// Source opens streams. Implementations are stateless and reentrant.
type Source interface {
	Open(ctx context.Context, path string, startOffset int64) (Stream, error)
}

// LineSink receives lines in order; a returned error stops consumption.
type LineSink func(Line) error

// FileSource reads lines from the local filesystem.
type FileSource struct{}

type fileStream struct {
	f           *os.File
	scanner     *bufio.Scanner
	startOffset int64
	byteOffset  int64
	lineNo      int64
}

// Open opens path and seeks to startOffset when it is positive. If that seek
// fails the file is re-read from offset 0; StartOffset reports the adopted value.
func (FileSource) Open(_ context.Context, path string, startOffset int64) (Stream, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	adopted := int64(0)
	if startOffset > 0 {
		if _, err := f.Seek(startOffset, io.SeekStart); err != nil {
			// On seek failure, fall back to scanning from the beginning.
			slog.Warn("seek failed, re-scanning from start", "path", path, "error", err)
			if _, err2 := f.Seek(0, io.SeekStart); err2 != nil {
				_ = f.Close()
				return nil, err2
			}
		} else {
			adopted = startOffset
		}
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, scannerInitialBuffer), scannerMaxBuffer)
	return &fileStream{f: f, scanner: scanner, startOffset: adopted, byteOffset: adopted}, nil
}

func (s *fileStream) StartOffset() int64 { return s.startOffset }

func (s *fileStream) Next(ctx context.Context) (Line, error) {
	if !s.scanner.Scan() {
		if err := s.scanner.Err(); err != nil {
			return Line{}, err
		}
		return Line{}, io.EOF
	}
	if err := ctx.Err(); err != nil {
		return Line{}, err
	}
	text := s.scanner.Text()
	start := s.byteOffset
	s.lineNo++
	s.byteOffset += int64(len(text)) + 1
	return Line{Text: text, Number: s.lineNo, StartByte: start, EndByte: s.byteOffset}, nil
}

func (s *fileStream) Close() error { return s.f.Close() }

// Consume feeds every line of stream to sink serially. io.EOF ends cleanly.
func Consume(ctx context.Context, stream Stream, sink LineSink) error {
	for {
		line, err := stream.Next(ctx)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if err := sink(line); err != nil {
			return err
		}
	}
}

// ConsumeBatch feeds in-memory lines to sink, numbering them from
// initialLineNumber+1 and advancing offsets from startOffset. The last line
// extends to endOffset when that is larger. It returns the final byte offset
// and line number (also on error, reflecting the lines processed so far).
func ConsumeBatch(ctx context.Context, lines []string,
	startOffset, endOffset, initialLineNumber int64, sink LineSink) (int64, int64, error) {
	byteOffset := startOffset
	lineNo := initialLineNumber
	for i, text := range lines {
		if err := ctx.Err(); err != nil {
			return byteOffset, lineNo, err
		}
		start := byteOffset
		lineNo++
		end := start + int64(len(text))
		if i < len(lines)-1 {
			end++
		} else if endOffset > end {
			end = endOffset
		}
		byteOffset = end
		if err := sink(Line{Text: text, Number: lineNo, StartByte: start, EndByte: end}); err != nil {
			return byteOffset, lineNo, err
		}
	}
	if endOffset > byteOffset {
		byteOffset = endOffset
	}
	return byteOffset, lineNo, nil
}
