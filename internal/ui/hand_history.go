package ui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
)

// suitSymbol returns the unicode suit symbol for a card suit letter.
func suitSymbol(suit string) string {
	switch suit {
	case "h":
		return "♥"
	case "d":
		return "♦"
	case "c":
		return "♣"
	case "s":
		return "♠"
	default:
		return suit
	}
}

// suitColor returns the display color for a card suit.
func suitColor(suit string) color.Color {
	switch suit {
	case "h", "d":
		return color.NRGBA{R: 0xe5, G: 0x39, B: 0x35, A: 0xff} // red
	default:
		return color.NRGBA{R: 0x21, G: 0x21, B: 0x21, A: 0xff} // near-black
	}
}

// cardLabel returns a canvas.Text for a single Card using suit unicode and color.
func cardLabel(c parser.Card, textSize float32) *canvas.Text {
	text := canvas.NewText(c.Rank+suitSymbol(c.Suit), suitColor(c.Suit))
	text.TextStyle = fyne.TextStyle{Bold: true}
	text.TextSize = textSize
	return text
}

// cardChip wraps a cardLabel in a padded, rounded background.
func cardChip(c parser.Card, textSize float32) fyne.CanvasObject {
	lbl := cardLabel(c, textSize)
	bg := canvas.NewRectangle(color.NRGBA{R: 0xf5, G: 0xf5, B: 0xf5, A: 0xff})
	bg.CornerRadius = 4
	return container.NewStack(bg, container.NewPadded(lbl))
}

// cardsRow builds a horizontal row of card chips for a slice of cards.
func cardsRow(cards []parser.Card, textSize float32, placeholder string) fyne.CanvasObject {
	if len(cards) == 0 {
		lbl := widget.NewLabel(placeholder)
		lbl.TextStyle = fyne.TextStyle{Italic: true}
		return lbl
	}
	items := make([]fyne.CanvasObject, len(cards))
	for i, c := range cards {
		items[i] = cardChip(c, textSize)
	}
	return container.NewHBox(items...)
}

type weightedMinRowLayout struct {
	weights   []float32
	minWidths []float32
	padding   float32
}

func newWeightedMinRowLayout(weights, minWidths []float32, padding float32) fyne.Layout {
	return &weightedMinRowLayout{weights: weights, minWidths: minWidths, padding: padding}
}

func brightenColor(c color.Color, amount float64) color.Color {
	r, g, b, a := c.RGBA()
	to8 := func(v uint32) float64 { return float64(v >> 8) }
	clamp := func(v float64) uint8 {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return uint8(v)
	}

	return color.NRGBA{
		R: clamp(to8(r) * (1 + amount)),
		G: clamp(to8(g) * (1 + amount)),
		B: clamp(to8(b) * (1 + amount)),
		A: uint8(a >> 8),
	}
}

func (l *weightedMinRowLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	n := len(objects)
	if len(l.weights) < n {
		n = len(l.weights)
	}
	if len(l.minWidths) < n {
		n = len(l.minWidths)
	}
	if n == 0 {
		return fyne.NewSize(0, 0)
	}

	var w float32
	var h float32
	for i := 0; i < n; i++ {
		w += l.minWidths[i]
		if objH := objects[i].MinSize().Height; objH > h {
			h = objH
		}
	}
	w += l.padding * float32(n-1)
	return fyne.NewSize(w, h)
}

func (l *weightedMinRowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	n := len(objects)
	if len(l.weights) < n {
		n = len(l.weights)
	}
	if len(l.minWidths) < n {
		n = len(l.minWidths)
	}
	if n == 0 {
		return
	}

	var totalMin float32
	var totalWeight float32
	for i := 0; i < n; i++ {
		totalMin += l.minWidths[i]
		totalWeight += l.weights[i]
	}
	totalMin += l.padding * float32(n-1)

	extra := size.Width - totalMin
	if extra < 0 {
		extra = 0
	}

	x := float32(0)
	for i := 0; i < n; i++ {
		w := l.minWidths[i]
		if totalWeight > 0 {
			w += extra * (l.weights[i] / totalWeight)
		}
		if i == n-1 {
			w = size.Width - x
			if w < l.minWidths[i] {
				w = l.minWidths[i]
			}
		}
		objects[i].Move(fyne.NewPos(x, 0))
		objects[i].Resize(fyne.NewSize(w, size.Height))
		x += w + l.padding
	}
}

func localSeatForHand(h *parser.Hand, fallback int) int {
	if h == nil {
		return fallback
	}
	if h.LocalPlayerSeat >= 0 {
		if _, ok := h.Players[h.LocalPlayerSeat]; ok {
			return h.LocalPlayerSeat
		}
	}
	if fallback >= 0 {
		if _, ok := h.Players[fallback]; ok {
			return fallback
		}
	}
	return -1
}

// handSummaryLine1 formats the first summary line for a hand list item.
func handSummaryLine1(h *parser.Hand, localSeat int) string {
	timeStr := h.StartTime.Format("15:04")
	seat := localSeatForHand(h, localSeat)

	// Hole cards
	var holeStr string
	if lp, ok := h.Players[seat]; ok && len(lp.HoleCards) == 2 {
		c1 := lp.HoleCards[0]
		c2 := lp.HoleCards[1]
		holeStr = c1.Rank + suitSymbol(c1.Suit) + " " + c2.Rank + suitSymbol(c2.Suit)
	} else {
		holeStr = "??"
	}

	// Board
	var boardParts []string
	for _, c := range h.CommunityCards {
		boardParts = append(boardParts, c.Rank+suitSymbol(c.Suit))
	}
	boardStr := strings.Join(boardParts, " ")
	if boardStr == "" {
		boardStr = "-"
	}

	return fmt.Sprintf("Hand #%d | %s | Cards: %s | Board: %s", h.ID, timeStr, holeStr, boardStr)
}

// handSummaryLine2 formats the second summary line for a hand list item.
func handSummaryLine2(h *parser.Hand, localSeat int) string {
	var result string
	var posStr string
	net := 0
	seat := localSeatForHand(h, localSeat)
	if lp, ok := h.Players[seat]; ok {
		invested := 0
		for _, act := range lp.Actions {
			invested += act.Amount
		}
		net = lp.PotWon - invested

		if lp.Won {
			result = lang.X("hand_history.result_won_simple", "Won")
		} else {
			result = lang.X("hand_history.result_lost_simple", "Lost")
		}
		if lp.Position != parser.PosUnknown {
			posStr = lp.Position.String()
		} else if seat == h.SBSeat {
			posStr = "SB"
		} else if seat == h.BBSeat {
			posStr = "BB"
		} else {
			posStr = lang.X("hand_history.seat", "Seat {{.N}}", map[string]any{"N": seat})
		}
	} else {
		result = lang.X("hand_history.na", "N/A")
		posStr = "?"
	}
	return fmt.Sprintf("Result: %-4s | Net: %6s | Pot: %4d | Pos: %-4s | Players: %2d",
		result, signedChips(net), h.TotalPot, posStr, h.NumPlayers)
}

func handSummaryEntryFields(h *parser.Hand, localSeat int) ([]string, []string) {
	timeStr := h.StartTime.Format("15:04")
	seat := localSeatForHand(h, localSeat)

	holeStr := "??"
	if lp, ok := h.Players[seat]; ok && len(lp.HoleCards) == 2 {
		c1 := lp.HoleCards[0]
		c2 := lp.HoleCards[1]
		holeStr = c1.Rank + suitSymbol(c1.Suit) + " " + c2.Rank + suitSymbol(c2.Suit)
	}

	boardParts := make([]string, 0, len(h.CommunityCards))
	for _, c := range h.CommunityCards {
		boardParts = append(boardParts, c.Rank+suitSymbol(c.Suit))
	}
	boardStr := strings.Join(boardParts, " ")
	if boardStr == "" {
		boardStr = "-"
	}

	outcome := buildHandOutcomeSummary(h, seat)

	line1 := []string{
		lang.X("hand_history.summary.time", "Time: {{.Time}}", map[string]any{"Time": timeStr}),
		lang.X("hand_history.summary.cards", "Cards: {{.Cards}}", map[string]any{"Cards": holeStr}),
		lang.X("hand_history.summary.board", "Board: {{.Board}}", map[string]any{"Board": boardStr}),
	}

	line2 := []string{
		lang.X("hand_history.summary.result", "Result: {{.Value}}", map[string]any{"Value": outcome.Result}),
		lang.X("hand_history.summary.net", "Net: {{.Value}}", map[string]any{"Value": outcome.NetValue}),
		lang.X("hand_history.summary.pot", "Pot: {{.Value}}", map[string]any{"Value": outcome.PotValue}),
		lang.X("hand_history.summary.pos", "Pos: {{.Value}}", map[string]any{"Value": outcome.PositionValue}),
		lang.X("hand_history.summary.players", "Players: {{.Value}}", map[string]any{"Value": outcome.PlayersValue}),
	}
	if h.HasDataAnomaly() {
		line2 = append(line2, lang.X("hand_history.summary.anomaly", "Quality: Flagged"))
	}

	return line1, line2
}

type handListEntryRefs struct {
	line1 []*widget.Label
	line2 []*widget.Label
}

func newHandListEntryRow() (fyne.CanvasObject, *handListEntryRefs) {
	newCell := func(style fyne.TextStyle) *widget.Label {
		lbl := widget.NewLabel("")
		lbl.Alignment = fyne.TextAlignLeading
		lbl.TextStyle = style
		lbl.Wrapping = fyne.TextWrapOff
		lbl.Truncation = fyne.TextTruncateEllipsis
		return lbl
	}

	line1Refs := make([]*widget.Label, 0, 3)
	line1Objs := make([]fyne.CanvasObject, 0, 3)
	for i := 0; i < 3; i++ {
		lbl := newCell(fyne.TextStyle{Bold: true})
		line1Refs = append(line1Refs, lbl)
		line1Objs = append(line1Objs, lbl)
	}

	line2Refs := make([]*widget.Label, 0, 5)
	line2Objs := make([]fyne.CanvasObject, 0, 5)
	for i := 0; i < 5; i++ {
		lbl := newCell(fyne.TextStyle{Italic: true})
		line2Refs = append(line2Refs, lbl)
		line2Objs = append(line2Objs, lbl)
	}

	line1Wrap := container.New(newWeightedMinRowLayout(
		[]float32{1.1, 1.9, 3.6},
		[]float32{88, 136, 220},
		theme.Padding(),
	), line1Objs...)
	line2Wrap := container.New(newWeightedMinRowLayout(
		[]float32{1.0, 1.2, 1.0, 0.8, 1.0},
		[]float32{88, 96, 84, 72, 88},
		theme.Padding(),
	), line2Objs...)

	body := container.NewVBox(line1Wrap, line2Wrap)
	root := container.NewPadded(newSectionCard(body))

	return root, &handListEntryRefs{line1: line1Refs, line2: line2Refs}
}

func (r *handListEntryRefs) setFields(line1, line2 []string) {
	for i, lbl := range r.line1 {
		if i < len(line1) {
			lbl.SetText(line1[i])
		} else {
			lbl.SetText("")
		}
	}
	for i, lbl := range r.line2 {
		if i < len(line2) {
			lbl.SetText(line2[i])
		} else {
			lbl.SetText("")
		}
	}
}

func signedChips(v int) string {
	if v > 0 {
		return fmt.Sprintf("+%d", v)
	}
	return fmt.Sprintf("%d", v)
}

type HandHistoryViewState struct {
	SelectedHandKey string
}

type handOutcomeSummary struct {
	Result        string
	NetValue      string
	PotValue      string
	PositionValue string
	PlayersValue  string
	InHand        bool
	Won           bool
}

func buildHandOutcomeSummary(h *parser.Hand, seat int) handOutcomeSummary {
	out := handOutcomeSummary{
		Result:        lang.X("hand_history.na", "N/A"),
		NetValue:      "0",
		PotValue:      fmt.Sprintf("%d", h.TotalPot),
		PositionValue: "?",
		PlayersValue:  fmt.Sprintf("%d", h.NumPlayers),
	}

	lp, ok := h.Players[seat]
	if !ok {
		out.Result = lang.X("hand_history.not_in_hand", "Not in this hand")
		out.NetValue = "-"
		out.PotValue = "-"
		out.PositionValue = "-"
		out.PlayersValue = "-"
		return out
	}

	out.InHand = true
	invested := 0
	for _, act := range lp.Actions {
		invested += act.Amount
	}
	out.NetValue = signedChips(lp.PotWon - invested)
	out.Won = lp.Won
	if lp.Won {
		out.Result = lang.X("hand_history.result_won_simple", "Won")
	} else {
		out.Result = lang.X("hand_history.result_lost_simple", "Lost")
	}

	if lp.Position != parser.PosUnknown {
		out.PositionValue = lp.Position.String()
	} else if seat == h.SBSeat {
		out.PositionValue = "SB"
	} else if seat == h.BBSeat {
		out.PositionValue = "BB"
	} else {
		out.PositionValue = lang.X("hand_history.seat", "Seat {{.N}}", map[string]any{"N": seat})
	}

	return out
}

func handSelectionKey(h *parser.Hand) string {
	if h == nil {
		return ""
	}
	if h.HandUID != "" {
		return "uid:" + h.HandUID
	}
	if !h.StartTime.IsZero() {
		return "ts:" + h.StartTime.UTC().Format(time.RFC3339Nano)
	}
	if h.ID > 0 {
		return fmt.Sprintf("id:%d", h.ID)
	}
	return ""
}

type seatAction struct {
	seat int
	act  parser.PlayerAction
}

func originalRaiserSeat(h *parser.Hand) int {
	if h == nil {
		return -1
	}
	var actions []seatAction
	for seat, pi := range h.Players {
		if pi == nil {
			continue
		}
		for _, act := range pi.Actions {
			if act.Street == parser.StreetPreFlop {
				actions = append(actions, seatAction{seat: seat, act: act})
			}
		}
	}
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].act.Timestamp.Equal(actions[j].act.Timestamp) {
			return actions[i].seat < actions[j].seat
		}
		return actions[i].act.Timestamp.Before(actions[j].act.Timestamp)
	})
	for _, a := range actions {
		switch a.act.Action {
		case parser.ActionBet, parser.ActionRaise, parser.ActionAllIn:
			return a.seat
		}
	}
	return -1
}

func streetActions(h *parser.Hand, street parser.Street) []seatAction {
	if h == nil {
		return nil
	}
	out := make([]seatAction, 0)
	for seat, pi := range h.Players {
		if pi == nil {
			continue
		}
		for _, act := range pi.Actions {
			if act.Street == street {
				out = append(out, seatAction{seat: seat, act: act})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].act.Timestamp.Equal(out[j].act.Timestamp) {
			return out[i].seat < out[j].seat
		}
		return out[i].act.Timestamp.Before(out[j].act.Timestamp)
	})
	return out
}

func actionLineColor(sa seatAction, localSeat, openRaiser int) color.Color {
	lineColor := color.NRGBA{R: 0xD0, G: 0xD8, B: 0xE2, A: 0xFF}
	if sa.seat == openRaiser {
		lineColor = color.NRGBA{R: 0xFF, G: 0xB3, B: 0x00, A: 0xFF}
	}
	if sa.seat == localSeat {
		lineColor = color.NRGBA{R: 0x66, G: 0xBB, B: 0x6A, A: 0xFF}
	}
	if sa.seat == localSeat && sa.seat == openRaiser {
		lineColor = color.NRGBA{R: 0x7E, G: 0x57, B: 0xC2, A: 0xFF}
	}
	return lineColor
}

func streetActionSection(h *parser.Hand, street parser.Street, localSeat, openRaiser int, normalSize float32) fyne.CanvasObject {
	actions := streetActions(h, street)
	if len(actions) == 0 {
		return nil
	}

	header := widget.NewLabelWithStyle(strings.ToUpper(street.String()), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	header.Wrapping = fyne.TextWrapWord

	rows := make([]fyne.CanvasObject, 0, len(actions))
	for _, sa := range actions {
		seatLabel := lang.X("hand_history.seat", "Seat {{.N}}", map[string]any{"N": sa.seat})
		if sa.seat == localSeat {
			seatLabel += lang.X("hand_history.you", " (You)")
		}
		if sa.seat == openRaiser {
			seatLabel += lang.X("hand_history.original_raiser", " (OR)")
		}

		amount := "-"
		if sa.act.Amount > 0 {
			amount = fmt.Sprintf("%d", sa.act.Amount)
		}
		timeText := "-"
		if !sa.act.Timestamp.IsZero() {
			timeText = sa.act.Timestamp.Format("15:04:05")
		}

		lineColor := actionLineColor(sa, localSeat, openRaiser)
		headline := canvas.NewText(lang.X("hand_history.timeline.headline", "{{.Seat}} - {{.Action}} ({{.Amount}})", map[string]any{
			"Seat":   seatLabel,
			"Action": sa.act.Action.String(),
			"Amount": amount,
		}), lineColor)
		headline.TextStyle = fyne.TextStyle{Bold: true}
		headline.TextSize = normalSize
		headline.Alignment = fyne.TextAlignLeading

		timeLabel := newSubtleText(lang.X("hand_history.timeline.time", "Time: {{.T}}", map[string]any{"T": timeText}))

		dot := canvas.NewCircle(lineColor)
		dotSlot := canvas.NewRectangle(color.Transparent)
		dotSlot.SetMinSize(fyne.NewSize(18, 18))
		dotWrap := container.NewStack(dotSlot, container.NewCenter(dot))
		rows = append(rows, container.NewHBox(dotWrap, container.NewVBox(headline, timeLabel)))
	}

	return newSectionCard(container.NewVBox(header, container.NewVBox(rows...)))
}

// buildDetailPanel creates the right-side detail view for a selected hand.
func buildDetailPanel(h *parser.Hand, localSeat int) fyne.CanvasObject {
	if h == nil {
		return newCenteredEmptyState(lang.X("hand_history.select_hand", "Select a hand to see details."))
	}

	seat := localSeatForHand(h, localSeat)

	bigSize := theme.TextSize() * 1.8
	normalSize := theme.TextSize()

	// ----- Hole Cards -----
	holeHeader := widget.NewLabel(lang.X("hand_history.hole_cards", "Hole Cards"))
	holeHeader.TextStyle = fyne.TextStyle{Bold: true}

	var holeCards []parser.Card
	if lp, ok := h.Players[seat]; ok {
		holeCards = lp.HoleCards
	}
	holeRow := cardsRow(holeCards, bigSize, lang.X("hand_history.cards_not_recorded", "Cards not recorded"))

	// ----- Community Cards -----
	commHeader := widget.NewLabel(lang.X("hand_history.community_cards", "Community Cards"))
	commHeader.TextStyle = fyne.TextStyle{Bold: true}

	var flopCards, turnCards, riverCards []parser.Card
	cc := h.CommunityCards
	if len(cc) >= 3 {
		flopCards = cc[:3]
	}
	if len(cc) >= 4 {
		turnCards = cc[3:4]
	}
	if len(cc) >= 5 {
		riverCards = cc[4:5]
	}

	var boardRows []fyne.CanvasObject
	if len(flopCards) > 0 {
		flopLabel := widget.NewLabel(lang.X("hand_history.flop", "Flop:"))
		flopLabel.Alignment = fyne.TextAlignLeading
		boardRows = append(boardRows, container.NewGridWithColumns(2, flopLabel, cardsRow(flopCards, normalSize, "")))
	}
	if len(turnCards) > 0 {
		turnLabel := widget.NewLabel(lang.X("hand_history.turn", "Turn:"))
		turnLabel.Alignment = fyne.TextAlignLeading
		boardRows = append(boardRows, container.NewGridWithColumns(2, turnLabel, cardsRow(turnCards, normalSize, "")))
	}
	if len(riverCards) > 0 {
		riverLabel := widget.NewLabel(lang.X("hand_history.river", "River:"))
		riverLabel.Alignment = fyne.TextAlignLeading
		boardRows = append(boardRows, container.NewGridWithColumns(2, riverLabel, cardsRow(riverCards, normalSize, "")))
	}
	if len(boardRows) == 0 {
		boardRows = append(boardRows, widget.NewLabel(lang.X("hand_history.no_community_cards", "No community cards")))
	}

	// ----- Result -----
	resultHeader := widget.NewLabel(lang.X("hand_history.result", "Result"))
	resultHeader.TextStyle = fyne.TextStyle{Bold: true}

	outcome := buildHandOutcomeSummary(h, seat)
	var resultText *canvas.Text
	if outcome.InHand {
		if outcome.Won {
			resultText = canvas.NewText(outcome.Result, color.NRGBA{R: 0x4c, G: 0xaf, B: 0x50, A: 0xff})
		} else {
			resultText = canvas.NewText(outcome.Result, color.NRGBA{R: 0xf4, G: 0x43, B: 0x36, A: 0xff})
		}
	} else {
		resultText = canvas.NewText(lang.X("hand_history.not_in_hand", "Not in this hand"), theme.ForegroundColor())
	}
	resultText.TextStyle = fyne.TextStyle{Bold: true}
	resultText.TextSize = normalSize * 1.2
	resultText.Alignment = fyne.TextAlignLeading

	resultTable := container.NewVBox(
		container.NewGridWithColumns(2,
			widget.NewLabel(lang.X("hand_history.result", "Result")),
			resultText,
		),
		container.NewGridWithColumns(2,
			widget.NewLabel(lang.X("hand_history.net_chips", "Net Chips")),
			widget.NewLabel(outcome.NetValue),
		),
		container.NewGridWithColumns(2,
			widget.NewLabel(lang.X("hand_history.pot", "Pot")),
			widget.NewLabel(outcome.PotValue),
		),
		container.NewGridWithColumns(2,
			widget.NewLabel(lang.X("hand_history.position", "Position")),
			widget.NewLabel(outcome.PositionValue),
		),
		container.NewGridWithColumns(2,
			widget.NewLabel(lang.X("hand_history.players", "Players")),
			widget.NewLabel(outcome.PlayersValue),
		),
	)

	// ----- Actions -----
	actionsHeader := widget.NewLabel(lang.X("hand_history.all_actions", "All Player Actions"))
	actionsHeader.TextStyle = fyne.TextStyle{Bold: true}

	openRaiser := originalRaiserSeat(h)
	streetOrder := []parser.Street{
		parser.StreetPreFlop,
		parser.StreetFlop,
		parser.StreetTurn,
		parser.StreetRiver,
		parser.StreetShowdown,
	}

	actionSections := make([]fyne.CanvasObject, 0)
	for _, st := range streetOrder {
		sec := streetActionSection(h, st, seat, openRaiser, normalSize)
		if sec != nil {
			actionSections = append(actionSections, sec)
		}
	}
	if len(actionSections) == 0 {
		actionSections = append(actionSections, widget.NewLabel(lang.X("hand_history.no_actions", "No actions recorded")))
	}

	boardSection := newSectionCard(container.NewVBox(commHeader, container.NewVBox(boardRows...)))
	holeSection := newSectionCard(container.NewVBox(holeHeader, holeRow))
	resultSection := newSectionCard(container.NewVBox(resultHeader, resultTable))
	actionsSection := newSectionCard(container.NewVBox(actionsHeader, container.NewVBox(actionSections...)))

	sections := []fyne.CanvasObject{holeSection, boardSection, resultSection}
	if h.HasDataAnomaly() {
		warnHeader := widget.NewLabel(lang.X("hand_history.anomaly.title", "Data Quality Warning"))
		warnHeader.TextStyle = fyne.TextStyle{Bold: true}
		lines := make([]string, 0, len(h.Anomalies))
		for _, anom := range h.Anomalies {
			lines = append(lines, anom.Code)
		}
		detail := strings.Join(lines, ", ")
		if detail == "" {
			detail = lang.X("hand_history.anomaly.flagged", "Potentially anomalous hand data detected.")
		}
		warnText := canvas.NewText(detail, uiWarningColor)
		warnText.TextSize = normalSize
		warnText.Alignment = fyne.TextAlignLeading
		sections = append(sections, newSectionCard(container.NewVBox(warnHeader, warnText)))
	}
	sections = append(sections, actionsSection)

	content := container.NewVBox(sections...)

	return container.NewScroll(container.NewPadded(content))
}

// NewHandHistoryTab creates the "Hand History" tab canvas object.
// hands should be in chronological order; this function reverses them for display.
func NewHandHistoryTab(hands []*parser.Hand, localSeat int, state *HandHistoryViewState) fyne.CanvasObject {
	validHands := make([]*parser.Hand, 0, len(hands))
	for _, h := range hands {
		if h != nil {
			validHands = append(validHands, h)
		}
	}

	if len(validHands) == 0 {
		return newCenteredEmptyState(lang.X("hand_history.no_hands", "No hands recorded yet.\nStart playing in the VR Poker world!"))
	}
	if state == nil {
		state = &HandHistoryViewState{}
	}

	// Reverse order: newest first.
	reversed := make([]*parser.Hand, len(validHands))
	for i, h := range validHands {
		reversed[len(validHands)-1-i] = h
	}

	// Detail panel placeholder - replaced on selection.
	detailContent := container.NewStack()
	detailContent.Objects = []fyne.CanvasObject{buildDetailPanel(nil, localSeat)}
	rowRefs := make(map[fyne.CanvasObject]*handListEntryRefs)

	list := widget.NewList(
		func() int { return len(reversed) },
		func() fyne.CanvasObject {
			row, refs := newHandListEntryRow()
			rowRefs[row] = refs
			return row
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			h := reversed[id]
			line1, line2 := handSummaryEntryFields(h, localSeat)
			if refs, ok := rowRefs[obj]; ok {
				refs.setFields(line1, line2)
			}
		},
	)

	list.OnSelected = func(id widget.ListItemID) {
		h := reversed[id]
		state.SelectedHandKey = handSelectionKey(h)
		detail := buildDetailPanel(h, localSeat)
		detailContent.Objects = []fyne.CanvasObject{detail}
		detailContent.Refresh()
	}

	if state.SelectedHandKey != "" {
		for i, h := range reversed {
			if handSelectionKey(h) == state.SelectedHandKey {
				list.Select(i)
				break
			}
		}
	}

	split := container.NewHSplit(list, detailContent)
	split.Offset = 0.48

	title := widget.NewLabelWithStyle(lang.X("hand_history.title", "Recent Hands"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	subtitle := widget.NewLabel(lang.X("hand_history.subtitle", "Select a hand to inspect street-by-street action flow."))
	subtitle.Wrapping = fyne.TextWrapWord

	return container.NewBorder(container.NewVBox(title, subtitle, newSectionDivider()), nil, nil, nil, split)
}
