package logsource

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "log.txt")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func collect(t *testing.T, path string, offset int64) (int64, []Line) {
	t.Helper()
	st, err := FileSource{}.Open(context.Background(), path, offset)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	var got []Line
	if err := Consume(context.Background(), st, func(l Line) error { got = append(got, l); return nil }); err != nil {
		t.Fatalf("consume: %v", err)
	}
	return st.StartOffset(), got
}

func TestFileSourceLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		offset  int64
		start   int64
		want    []Line
	}{
		{"LF", "ab\ncd\n", 0, 0, []Line{{"ab", 1, 0, 3}, {"cd", 2, 3, 6}}},
		{"CRLF stripped, +1 arithmetic kept", "ab\r\ncd\r\n", 0, 0, []Line{{"ab", 1, 0, 3}, {"cd", 2, 3, 6}}},
		{"no trailing newline", "ab\ncd", 0, 0, []Line{{"ab", 1, 0, 3}, {"cd", 2, 3, 6}}},
		{"offset resume numbers from 0", "ab\ncd\nef\n", 3, 3, []Line{{"cd", 1, 3, 6}, {"ef", 2, 6, 9}}},
		{"seek past end still opens", "ab\n", 100, 100, nil},
		{"empty", "", 0, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, got := collect(t, writeTemp(t, tt.content), tt.offset)
			if start != tt.start {
				t.Fatalf("StartOffset = %d, want %d", start, tt.start)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("lines = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFileSourceOpenMissing(t *testing.T) {
	t.Parallel()
	if _, err := (FileSource{}).Open(context.Background(), filepath.Join(t.TempDir(), "nope"), 0); err == nil {
		t.Fatal("expected error")
	}
}

func TestFileSourceNegativeOffsetNoSeek(t *testing.T) {
	t.Parallel()
	start, got := collect(t, writeTemp(t, "ab\n"), -5)
	if start != 0 || len(got) != 1 {
		t.Fatalf("start=%d lines=%v", start, got)
	}
}

func TestConsumeCancelAndSinkError(t *testing.T) {
	t.Parallel()
	path := writeTemp(t, "a\nb\nc\n")

	ctx, cancel := context.WithCancel(context.Background())
	st, err := FileSource{}.Open(ctx, path, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	n := 0
	err = Consume(ctx, st, func(Line) error { n++; cancel(); return nil })
	if !errors.Is(err, context.Canceled) || n != 1 {
		t.Fatalf("err=%v n=%d", err, n)
	}

	st2, _ := FileSource{}.Open(context.Background(), path, 0)
	defer st2.Close()
	boom := errors.New("boom")
	n = 0
	if err := Consume(context.Background(), st2, func(Line) error { n++; return boom }); !errors.Is(err, boom) || n != 1 {
		t.Fatalf("err=%v n=%d", err, n)
	}
}

func TestConsumeBatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		lines           []string
		start, end      int64
		initial         int64
		wantLines       []Line
		wantOff, wantNo int64
	}{
		{"basic", []string{"ab", "c"}, 10, 0, 5,
			[]Line{{"ab", 6, 10, 13}, {"c", 7, 13, 14}}, 14, 7},
		{"end offset extends last line", []string{"ab", "c"}, 10, 20, 0,
			[]Line{{"ab", 1, 10, 13}, {"c", 2, 13, 20}}, 20, 2},
		{"smaller end offset ignored", []string{"ab"}, 0, 1, 0,
			[]Line{{"ab", 1, 0, 2}}, 2, 1},
		{"empty", nil, 7, 9, 3, nil, 9, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []Line
			off, no, err := ConsumeBatch(context.Background(), tt.lines, tt.start, tt.end, tt.initial,
				func(l Line) error { got = append(got, l); return nil })
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.wantLines) || off != tt.wantOff || no != tt.wantNo {
				t.Fatalf("got %#v off=%d no=%d", got, off, no)
			}
		})
	}
}

func TestConsumeBatchCancelAndError(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := ConsumeBatch(ctx, []string{"a"}, 0, 0, 0, func(Line) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	boom := errors.New("boom")
	if _, _, err := ConsumeBatch(context.Background(), []string{"a", "b"}, 0, 0, 0, func(Line) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
}
