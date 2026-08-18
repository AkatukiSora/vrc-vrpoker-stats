package ui

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
)

func TestHandExportButtonInvokesSaveAndReportsSuccess(t *testing.T) {
	service := newFakeHandHistoryAppService()
	service.exportData = []byte("variant = \"NT\"\n")
	writer := &exportTestWriter{}
	dialog := &fakeHandExportDialog{writer: writer, name: "hand.phh"}
	status := make(chan string, 1)
	flow := newHandExportFlow(nil, service, dialog, func(message string) { status <- message })
	view := newHandHistoryTabView(&HandHistoryViewState{}, nil, nil, flow.begin)
	fyne.DoAndWait(func() {
		view.UpdatePage([]persistence.HandSummary{{HandUID: "hand-1"}}, 0, 1)
		view.list.Select(0)
		test.Tap(view.exportButton)
	})
	if got, want := dialog.defaultName, "vrpoker-hand-hand-1.phh"; got != want {
		t.Fatalf("save filename = %q, want %q", got, want)
	}
	if got := waitExportStatus(t, status); got != "Hand exported: hand.phh" {
		t.Fatalf("status = %q", got)
	}
	if got := writer.String(); got != "variant = \"NT\"\n" {
		t.Fatalf("file data = %q", got)
	}
	if !writer.closed {
		t.Fatal("writer was not closed")
	}
}

func TestHandExportFlowReportsCancelAndWriteError(t *testing.T) {
	service := newFakeHandHistoryAppService()
	status := make(chan string, 1)
	flow := newHandExportFlow(nil, service, &fakeHandExportDialog{}, func(message string) { status <- message })
	flow.begin("hand-1")
	if got := waitExportStatus(t, status); got != "Hand export cancelled." {
		t.Fatalf("cancel status = %q", got)
	}

	status = make(chan string, 1)
	writer := &exportTestWriter{writeErr: errors.New("disk full")}
	flow = newHandExportFlow(nil, service, &fakeHandExportDialog{writer: writer, name: "hand.phh"}, func(message string) { status <- message })
	flow.begin("hand-1")
	if got := waitExportStatus(t, status); got != "Could not export hand: disk full" {
		t.Fatalf("error status = %q", got)
	}
	if !writer.closed {
		t.Fatal("writer was not closed after write error")
	}
}

func TestHandExportFlowReportsServiceValidationError(t *testing.T) {
	service := newFakeHandHistoryAppService()
	service.exportErr = errors.New("missing valid small and big blinds")
	status := make(chan string, 1)
	flow := newHandExportFlow(nil, service, &fakeHandExportDialog{writer: &exportTestWriter{}, name: "hand.phh"}, func(message string) { status <- message })
	flow.begin("partial")
	if got := waitExportStatus(t, status); got != "Could not export hand: missing valid small and big blinds" {
		t.Fatalf("validation status = %q", got)
	}
}

func waitExportStatus(t *testing.T, status <-chan string) string {
	t.Helper()
	select {
	case got := <-status:
		return got
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for export status")
		return ""
	}
}

type fakeHandExportDialog struct {
	writer      io.WriteCloser
	name        string
	err         error
	defaultName string
}

func (d *fakeHandExportDialog) Save(defaultName string, done func(io.WriteCloser, string, error)) {
	d.defaultName = defaultName
	done(d.writer, d.name, d.err)
}

type exportTestWriter struct {
	bytes.Buffer
	writeErr error
	closed   bool
	mu       sync.Mutex
}

func (w *exportTestWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return w.Buffer.Write(p)
}
func (w *exportTestWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}
