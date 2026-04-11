package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/application"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

func TestHandDetailRequestGenerationIncrements(t *testing.T) {
	app := &App{
		historyState: &HandHistoryViewState{SelectedHandKey: handDetailSelectionKey("hand-2")},
	}

	gen1 := app.nextHandDetailRequestGeneration()
	gen2 := app.nextHandDetailRequestGeneration()

	if gen1 == 0 {
		t.Fatal("expected first generation to be non-zero")
	}
	if gen2 != gen1+1 {
		t.Fatalf("expected generation to increment by one: first=%d second=%d", gen1, gen2)
	}
	if app.isCurrentHandDetailRequestGeneration(gen1) {
		t.Fatalf("expected first generation %d to be stale", gen1)
	}
	if !app.isCurrentHandDetailRequestGeneration(gen2) {
		t.Fatalf("expected second generation %d to be current", gen2)
	}
	if !app.canApplyHandDetailRequest(gen2, "hand-2") {
		t.Fatalf("expected current generation %d with matching uid to apply", gen2)
	}
	if got := handDetailSelectionKey("hand-2"); got != "uid:hand-2" {
		t.Fatalf("unexpected selection key: %q", got)
	}
}

func TestHandDetailRequestGenerationRejectsStale(t *testing.T) {
	app := &App{
		historyState: &HandHistoryViewState{SelectedHandKey: handDetailSelectionKey("hand-1")},
	}

	staleGen := app.nextHandDetailRequestGeneration()
	currentGen := app.nextHandDetailRequestGeneration()

	if app.canApplyHandDetailRequest(staleGen, "hand-1") {
		t.Fatalf("expected stale generation %d to be rejected", staleGen)
	}
	if !app.canApplyHandDetailRequest(currentGen, "hand-1") {
		t.Fatalf("expected current generation %d with matching uid to apply", currentGen)
	}

	app.historyState.SelectedHandKey = handDetailSelectionKey("hand-2")
	if app.canApplyHandDetailRequest(currentGen, "hand-1") {
		t.Fatalf("expected mismatched selected key to reject generation %d", currentGen)
	}
}

func TestHandHistoryHarnessCreatesWindowAndSelectsRow(t *testing.T) {
	h := newHandHistoryHarness(t)
	h.seedSummaries(handSummary("hand-1", 0), handSummary("hand-2", 1))
	h.fake.blockHand(testHand("hand-1", 0, "Ah", "Kd"))
	h.fake.blockHand(testHand("hand-2", 1, "Qs", "Qc"))

	h.selectRow(t, 1)
	h.fake.waitForDetailRequest(t, "hand-2")

	if got := h.app.historyState.SelectedHandKey; got != handDetailSelectionKey("hand-2") {
		t.Fatalf("unexpected selected hand key: %q", got)
	}
	if got := h.detailText(); !strings.Contains(got, lang.X("hand_history.detail.loading", "Loading hand details…")) {
		t.Fatalf("expected loading state after selection, got %q", got)
	}

	h.fake.releaseHand("hand-2")
	h.waitForDetailText(t, lang.X("hand_history.hole_cards", "Hole Cards"))
	if got := h.detailText(); !strings.Contains(got, "Q") {
		t.Fatalf("expected loaded detail text for released hand, got %q", got)
	}

	h.fake.releaseHand("hand-1")
}

func TestHandHistoryHarnessControlsAsyncCompletion(t *testing.T) {
	h := newHandHistoryHarness(t)
	h.seedSummaries(handSummary("hand-1", 0), handSummary("hand-2", 1))
	h.fake.blockHand(testHand("hand-1", 0, "Ah", "Kd"))
	h.fake.blockHand(testHand("hand-2", 1, "Qs", "Qc"))

	h.selectRow(t, 0)
	h.fake.waitForDetailRequest(t, "hand-1")
	h.selectRow(t, 1)
	h.fake.waitForDetailRequest(t, "hand-2")

	if got := h.fake.requestOrder(); len(got) != 2 || got[0] != "hand-1" || got[1] != "hand-2" {
		t.Fatalf("unexpected detail request order: %v", got)
	}

	h.fake.releaseHand("hand-2")
	h.waitForDetailText(t, lang.X("hand_history.hole_cards", "Hole Cards"))

	h.fake.releaseHand("hand-1")
	h.flushUI()
	if got := h.app.historyState.SelectedHandKey; got != handDetailSelectionKey("hand-2") {
		t.Fatalf("unexpected selected hand key after releases: %q", got)
	}
}

func TestHandHistoryRapidReselectionIgnoresStaleSuccess(t *testing.T) {
	h := newHandHistoryHarness(t)
	h.seedSummaries(handSummary("hand-1", 0), handSummary("hand-2", 1))
	h.fake.blockHand(testHand("hand-1", 0, "Ah", "Kd"))
	h.fake.blockHand(testHand("hand-2", 1, "Qs", "Qc"))

	h.selectRow(t, 0)
	h.fake.waitForDetailRequest(t, "hand-1")
	h.selectRow(t, 1)
	h.fake.waitForDetailRequest(t, "hand-2")

	h.fake.releaseHand("hand-2")
	h.waitForDetailText(t, lang.X("hand_history.hole_cards", "Hole Cards"))
	if got := h.detailText(); !strings.Contains(got, "Q") {
		t.Fatalf("expected current hand detail to load second hand, got %q", got)
	}

	h.fake.releaseHand("hand-1")
	h.flushUI()
	if got := h.app.historyState.SelectedHandKey; got != handDetailSelectionKey("hand-2") {
		t.Fatalf("unexpected selected hand key after stale success release: %q", got)
	}
	if got := h.detailText(); !strings.Contains(got, "Q") || strings.Contains(got, "A") {
		t.Fatalf("expected stale success to be ignored, got %q", got)
	}
}

func TestHandHistoryStaleErrorIgnored(t *testing.T) {
	h := newHandHistoryHarness(t)
	h.seedSummaries(handSummary("hand-1", 0), handSummary("hand-2", 1))
	h.fake.blockHand(testHand("hand-1", 0, "Ah", "Kd"))
	h.fake.blockHand(testHand("hand-2", 1, "Qs", "Qc"))
	h.fake.mu.Lock()
	h.fake.blockedHands["hand-1"].err = errors.New("boom")
	h.fake.mu.Unlock()

	h.selectRow(t, 0)
	h.fake.waitForDetailRequest(t, "hand-1")
	h.selectRow(t, 1)
	h.fake.waitForDetailRequest(t, "hand-2")

	h.fake.releaseHand("hand-2")
	h.waitForDetailText(t, lang.X("hand_history.hole_cards", "Hole Cards"))

	h.fake.releaseHand("hand-1")
	h.flushUI()
	if got := h.detailText(); strings.Contains(got, lang.X("hand_history.detail.error", "Failed to load hand details.")) {
		t.Fatalf("expected stale error to be ignored, got %q", got)
	}
	if got := h.detailText(); !strings.Contains(got, "Q") || strings.Contains(got, "A") {
		t.Fatalf("expected current hand detail to remain after stale error, got %q", got)
	}
}

func TestHandHistoryRefreshRestoreKeepsMatchingDetail(t *testing.T) {
	h := newHandHistoryHarness(t)
	h.seedSummaries(handSummary("hand-1", 0), handSummary("hand-2", 1))
	h.fake.blockHand(testHand("hand-1", 0, "Ah", "Kd"))

	h.selectRow(t, 0)
	h.fake.waitForDetailRequest(t, "hand-1")
	h.fake.releaseHand("hand-1")
	h.waitForDetailText(t, lang.X("hand_history.hole_cards", "Hole Cards"))
	h.fake.blockHand(testHand("hand-1", 2, "Tc", "Td"))

	fyne.DoAndWait(func() {
		h.app.handHistoryView.UpdateDetail(container.NewCenter(widget.NewLabel("stale detail")))
		h.app.handHistoryView.UpdatePage(
			[]persistence.HandSummary{handSummary("hand-2", 1), handSummary("hand-1", 2)},
			0,
			2,
		)
	})

	h.fake.waitForDetailRequest(t, "hand-1")
	if got := h.detailText(); !strings.Contains(got, lang.X("hand_history.detail.loading", "Loading hand details…")) {
		t.Fatalf("expected loading state while converging refreshed detail, got %q", got)
	}

	h.fake.releaseHand("hand-1")
	h.waitForDetailText(t, lang.X("hand_history.hole_cards", "Hole Cards"))
	if got := h.app.historyState.SelectedHandKey; got != handDetailSelectionKey("hand-1") {
		t.Fatalf("unexpected selected hand key after refresh restore: %q", got)
	}
	if got := h.detailText(); strings.Contains(got, "stale detail") || !strings.Contains(got, "T") || strings.Contains(got, "A") {
		t.Fatalf("expected refreshed detail to converge to hand-1, got %q", got)
	}
	if got := h.fake.requestOrder(); len(got) != 2 || got[0] != "hand-1" || got[1] != "hand-1" {
		t.Fatalf("expected initial selection and refresh restore to fetch hand-1, got %v", got)
	}
}

func TestHandHistoryRefreshClearsMissingSelection(t *testing.T) {
	h := newHandHistoryHarness(t)
	h.seedSummaries(handSummary("hand-1", 0), handSummary("hand-2", 1))
	h.fake.blockHand(testHand("hand-1", 0, "Ah", "Kd"))

	h.selectRow(t, 0)
	h.fake.waitForDetailRequest(t, "hand-1")
	h.fake.releaseHand("hand-1")
	h.waitForDetailText(t, lang.X("hand_history.hole_cards", "Hole Cards"))

	fyne.DoAndWait(func() {
		h.app.handHistoryView.UpdatePage([]persistence.HandSummary{handSummary("hand-2", 1)}, 0, 1)
	})

	if got := h.app.historyState.SelectedHandKey; got != "" {
		t.Fatalf("expected selection to clear when uid disappears, got %q", got)
	}
	if got := h.detailText(); !strings.Contains(got, lang.X("hand_history.select_hand", "Select a hand to see details.")) {
		t.Fatalf("expected empty detail placeholder after missing selection refresh, got %q", got)
	}
	if got := h.fake.requestOrder(); len(got) != 1 || got[0] != "hand-1" {
		t.Fatalf("expected no extra detail fetch after missing selection refresh, got %v", got)
	}
}

func TestHandHistoryRefreshLosesToNewerUserChoice(t *testing.T) {
	h := newHandHistoryHarness(t)
	h.seedSummaries(handSummary("hand-1", 0), handSummary("hand-2", 1))
	h.fake.blockHand(testHand("hand-1", 0, "Ah", "Kd"))
	h.fake.blockHand(testHand("hand-2", 1, "Qs", "Qc"))

	h.selectRow(t, 0)
	h.fake.waitForDetailRequest(t, "hand-1")
	h.fake.releaseHand("hand-1")
	h.waitForDetailText(t, lang.X("hand_history.hole_cards", "Hole Cards"))

	h.fake.blockHand(testHand("hand-1", 2, "Tc", "Td"))
	fyne.DoAndWait(func() {
		h.app.handHistoryView.UpdatePage(
			[]persistence.HandSummary{handSummary("hand-2", 1), handSummary("hand-1", 2)},
			0,
			2,
		)
	})
	h.fake.waitForDetailRequest(t, "hand-1")
	if got := h.app.historyState.SelectedHandKey; got != handDetailSelectionKey("hand-1") {
		t.Fatalf("expected refresh to preserve hand-1 before user change, got %q", got)
	}
	if got := h.detailText(); !strings.Contains(got, lang.X("hand_history.detail.loading", "Loading hand details…")) {
		t.Fatalf("expected refresh restore to show loading before newer user choice, got %q", got)
	}

	h.selectRow(t, 0)
	h.fake.waitForDetailRequest(t, "hand-2")
	if got := h.app.historyState.SelectedHandKey; got != handDetailSelectionKey("hand-2") {
		t.Fatalf("expected newer user selection to win immediately, got %q", got)
	}

	h.fake.releaseHand("hand-2")
	h.waitForDetailText(t, lang.X("hand_history.hole_cards", "Hole Cards"))
	if got := h.detailText(); !strings.Contains(got, "Q♠") || !strings.Contains(got, "Q♣") || strings.Contains(got, "T♣") || strings.Contains(got, "T♦") || strings.Contains(got, "A♥") || strings.Contains(got, "K♦") {
		t.Fatalf("expected newer user detail to load hand-2, got %q", got)
	}

	h.fake.releaseHand("hand-1")
	h.flushUI()
	if got := h.app.historyState.SelectedHandKey; got != handDetailSelectionKey("hand-2") {
		t.Fatalf("expected stale refresh completion to lose to hand-2 selection, got %q", got)
	}
	if got := h.detailText(); !strings.Contains(got, "Q♠") || !strings.Contains(got, "Q♣") || strings.Contains(got, "T♣") || strings.Contains(got, "T♦") || strings.Contains(got, "A♥") || strings.Contains(got, "K♦") {
		t.Fatalf("expected stale refresh completion to be ignored, got %q", got)
	}
	if got := h.fake.requestOrder(); len(got) != 3 || got[0] != "hand-1" || got[1] != "hand-1" || got[2] != "hand-2" {
		t.Fatalf("expected initial load, refresh restore, then newer user choice request order, got %v", got)
	}
}

func TestHandHistoryCanvasTapSmokeSelectsRow(t *testing.T) {
	h := newHandHistoryHarness(t)
	h.seedSummaries(handSummary("hand-1", 0), handSummary("hand-2", 1))
	h.fake.blockHand(testHand("hand-1", 0, "Ah", "Kd"))
	h.fake.blockHand(testHand("hand-2", 1, "Qs", "Qc"))

	h.tapRow(t, 1)
	h.fake.waitForDetailRequest(t, "hand-2")
	if got := h.app.historyState.SelectedHandKey; got != handDetailSelectionKey("hand-2") {
		t.Fatalf("expected canvas tap to select hand-2, got %q", got)
	}
	if got := h.detailText(); !strings.Contains(got, lang.X("hand_history.detail.loading", "Loading hand details…")) {
		t.Fatalf("expected loading state after canvas tap selection, got %q", got)
	}

	h.fake.releaseHand("hand-2")
	h.waitForDetailText(t, lang.X("hand_history.hole_cards", "Hole Cards"))
	if got := h.detailText(); !strings.Contains(got, "Q") {
		t.Fatalf("expected canvas tap path to load tapped hand detail, got %q", got)
	}

	h.fake.releaseHand("hand-1")
}

var _ application.AppService = (*fakeHandHistoryAppService)(nil)

type handHistoryHarness struct {
	app  *App
	fake *fakeHandHistoryAppService
	root *fyne.Container
}

func newHandHistoryHarness(t *testing.T) *handHistoryHarness {
	t.Helper()

	fyneApp := test.NewTempApp(t)
	fyneApp.Settings().SetTheme(newPokerTheme())
	root := container.NewStack()
	win := test.NewTempWindow(t, root)
	ctx, cancel := context.WithCancel(context.Background())

	fake := newFakeHandHistoryAppService()
	app := &App{
		ctx:                ctx,
		cancel:             cancel,
		fyneApp:            fyneApp,
		win:                win,
		service:            fake,
		rangeState:         &HandRangeViewState{},
		historyState:       &HandHistoryViewState{},
		metricState:        NewMetricVisibilityState(),
		currentTab:         tabHandHistory,
		mainContent:        root,
		pendingHistoryPage: -1,
	}

	fyne.DoAndWait(func() {
		app.doRefreshCurrentTab()
	})
	fake.waitForListCall(t)
	waitForHistoryPageIdle(t, app)
	t.Cleanup(func() {
		cancel()
		waitForHistoryPageIdle(t, app)
	})

	return &handHistoryHarness{app: app, fake: fake, root: root}
}

func (h *handHistoryHarness) seedSummaries(summaries ...persistence.HandSummary) {
	waitForHistoryPageIdle(nil, h.app)
	h.fake.setSummaries(summaries)
	h.flushUI()
	fyne.DoAndWait(func() {
		h.app.handHistoryView.UpdatePage(summaries, 0, len(summaries))
		if h.app.mainContent != nil {
			h.app.mainContent.Refresh()
		}
	})
}

func waitForHistoryPageIdle(t *testing.T, app *App) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		fyne.DoAndWait(func() {})
		if atomic.LoadInt32(&app.historyPageRunning) == 0 && atomic.LoadInt32(&app.pendingHistoryPage) == -1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if t != nil {
		t.Fatal("timed out waiting for hand history page load to go idle")
	}
}

func (h *handHistoryHarness) selectRow(t *testing.T, row int) {
	t.Helper()
	fyne.DoAndWait(func() {
		h.app.handHistoryView.list.Select(row)
	})
}

func (h *handHistoryHarness) tapRow(t *testing.T, row int) {
	t.Helper()
	fyne.DoAndWait(func() {
		h.app.win.Resize(fyne.NewSize(1280, 820))
		h.root.Resize(fyne.NewSize(1280, 820))
		h.root.Refresh()
		tappables := collectTappables(test.WidgetRenderer(h.app.handHistoryView.list).Objects())
		if row < 0 || row >= len(tappables) {
			t.Fatalf("row %d out of visible tappable range %d", row, len(tappables))
		}
		if tappable, ok := tappables[row].(fyne.Tappable); ok {
			test.Tap(tappable)
		}
	})
}

func collectTappables(objects []fyne.CanvasObject) []fyne.CanvasObject {
	var out []fyne.CanvasObject
	for _, obj := range objects {
		out = append(out, collectTappablesFromObject(obj)...)
	}
	return out
}

func collectTappablesFromObject(obj fyne.CanvasObject) []fyne.CanvasObject {
	if obj == nil {
		return nil
	}

	var out []fyne.CanvasObject
	if _, ok := obj.(fyne.Tappable); ok {
		out = append(out, obj)
	}

	switch v := obj.(type) {
	case *fyne.Container:
		for _, child := range v.Objects {
			out = append(out, collectTappablesFromObject(child)...)
		}
	case *container.Scroll:
		out = append(out, collectTappablesFromObject(v.Content)...)
	}

	return out
}

func (h *handHistoryHarness) flushUI() {
	fyne.DoAndWait(func() {})
}

func (h *handHistoryHarness) waitForDetailText(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		h.flushUI()
		if strings.Contains(h.detailText(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for detail text %q; got %q", want, h.detailText())
}

func (h *handHistoryHarness) detailText() string {
	if h.app.handHistoryView == nil || h.app.handHistoryView.detailContent == nil {
		return ""
	}
	return strings.Join(canvasObjectTexts(h.app.handHistoryView.detailContent), " | ")
}

func canvasObjectTexts(obj fyne.CanvasObject) []string {
	switch v := obj.(type) {
	case *fyne.Container:
		var out []string
		for _, child := range v.Objects {
			out = append(out, canvasObjectTexts(child)...)
		}
		return out
	case *container.Scroll:
		if v.Content == nil {
			return nil
		}
		return canvasObjectTexts(v.Content)
	case *widget.Label:
		if v.Text == "" {
			return nil
		}
		return []string{v.Text}
	case *canvas.Text:
		if v.Text == "" {
			return nil
		}
		return []string{v.Text}
	default:
		return nil
	}
}

type fakeHandHistoryAppService struct {
	mu           sync.Mutex
	summaries    []persistence.HandSummary
	totalCount   int
	listCalls    chan struct{}
	detailCalls  chan string
	requestLog   []string
	blockedHands map[string]*blockedHandResult
}

type blockedHandResult struct {
	hand    *parser.Hand
	err     error
	release chan struct{}
	once    sync.Once
}

func newFakeHandHistoryAppService() *fakeHandHistoryAppService {
	return &fakeHandHistoryAppService{
		listCalls:    make(chan struct{}, 8),
		detailCalls:  make(chan string, 16),
		blockedHands: make(map[string]*blockedHandResult),
	}
}

func (f *fakeHandHistoryAppService) setSummaries(summaries []persistence.HandSummary) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.summaries = append([]persistence.HandSummary(nil), summaries...)
	f.totalCount = len(summaries)
}

func (f *fakeHandHistoryAppService) blockHand(hand *parser.Hand) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blockedHands[hand.HandUID] = &blockedHandResult{
		hand:    hand,
		release: make(chan struct{}),
	}
}

func (f *fakeHandHistoryAppService) releaseHand(uid string) {
	f.mu.Lock()
	blocked := f.blockedHands[uid]
	f.mu.Unlock()
	if blocked == nil {
		return
	}
	blocked.once.Do(func() {
		close(blocked.release)
	})
}

func (f *fakeHandHistoryAppService) waitForListCall(t *testing.T) {
	t.Helper()
	select {
	case <-f.listCalls:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ListHandSummaries call")
	}
}

func (f *fakeHandHistoryAppService) waitForDetailRequest(t *testing.T, uid string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case got := <-f.detailCalls:
			if got == uid {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for GetHandByUID(%q)", uid)
		}
	}
}

func (f *fakeHandHistoryAppService) requestOrder() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requestLog...)
}

func (f *fakeHandHistoryAppService) BootstrapImportAllLogs(context.Context) (string, error) {
	return "", nil
}

func (f *fakeHandHistoryAppService) BootstrapImportAllLogsWithProgress(context.Context, func(application.BootstrapProgress)) (string, error) {
	return "", nil
}

func (f *fakeHandHistoryAppService) ChangeLogFile(context.Context, string) error {
	return nil
}

func (f *fakeHandHistoryAppService) ImportLines(context.Context, string, []string, int64, int64) error {
	return nil
}

func (f *fakeHandHistoryAppService) Snapshot(context.Context) (*stats.Stats, []*parser.Hand, int, error) {
	return nil, nil, 0, nil
}

func (f *fakeHandHistoryAppService) Stats(context.Context, persistence.HandFilter) (*stats.Stats, int, error) {
	return nil, -1, nil
}

func (f *fakeHandHistoryAppService) ListHandSummaries(context.Context, persistence.HandFilter) ([]persistence.HandSummary, int, error) {
	select {
	case f.listCalls <- struct{}{}:
	default:
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]persistence.HandSummary(nil), f.summaries...), f.totalCount, nil
}

func (f *fakeHandHistoryAppService) GetHandByUID(_ context.Context, uid string) (*parser.Hand, error) {
	f.mu.Lock()
	blocked, ok := f.blockedHands[uid]
	if ok {
		f.requestLog = append(f.requestLog, uid)
	}
	f.mu.Unlock()

	if ok {
		select {
		case f.detailCalls <- uid:
		default:
		}
		<-blocked.release
		return blocked.hand, blocked.err
	}

	return nil, fmt.Errorf("unexpected uid: %s", uid)
}

func (f *fakeHandHistoryAppService) NextOffset(context.Context, string) (int64, error) {
	return 0, nil
}

func (f *fakeHandHistoryAppService) MarkLogFullyImported(context.Context, string) {}

func (f *fakeHandHistoryAppService) Close() error {
	return nil
}

func handSummary(uid string, offsetMinutes int) persistence.HandSummary {
	base := time.Date(2026, time.April, 10, 21, 0, 0, 0, time.UTC)
	return persistence.HandSummary{
		HandUID:    uid,
		StartTime:  base.Add(time.Duration(offsetMinutes) * time.Minute),
		NumPlayers: 6,
		TotalPot:   120,
		IsComplete: true,
		LocalSeat:  1,
		HoleCard0:  "Ah",
		HoleCard1:  "Kd",
		Position:   parser.PosBTN.String(),
		PotWon:     120,
		NetChips:   60,
		Won:        true,
	}
}

func testHand(uid string, offsetMinutes int, hole0, hole1 string) *parser.Hand {
	base := time.Date(2026, time.April, 10, 21, 0, 0, 0, time.UTC).Add(time.Duration(offsetMinutes) * time.Minute)
	return &parser.Hand{
		HandUID:         uid,
		StartTime:       base,
		LocalPlayerSeat: 1,
		CommunityCards:  []parser.Card{{Rank: "2", Suit: "h"}, {Rank: "7", Suit: "d"}, {Rank: "J", Suit: "c"}},
		Players: map[int]*parser.PlayerHandInfo{
			1: {
				SeatID:    1,
				HoleCards: []parser.Card{parseTestCard(hole0), parseTestCard(hole1)},
				Position:  parser.PosBTN,
				Won:       true,
				PotWon:    120,
				Actions: []parser.PlayerAction{
					{Timestamp: base.Add(1 * time.Second), PlayerID: 1, Street: parser.StreetPreFlop, Action: parser.ActionRaise, Amount: 20},
					{Timestamp: base.Add(2 * time.Second), PlayerID: 1, Street: parser.StreetFlop, Action: parser.ActionBet, Amount: 40},
				},
			},
		},
		SBSeat:        2,
		BBSeat:        3,
		NumPlayers:    6,
		TotalPot:      120,
		WinnerSeat:    1,
		WinType:       "showdown",
		IsComplete:    true,
		StatsEligible: true,
	}
}

func parseTestCard(raw string) parser.Card {
	if len(raw) < 2 {
		panic("invalid test card")
	}
	return parser.Card{Rank: raw[:len(raw)-1], Suit: raw[len(raw)-1:]}
}
