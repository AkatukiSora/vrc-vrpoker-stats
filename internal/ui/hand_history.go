package ui

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

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
	seat := localSeatForHand(h, localSeat)
	if lp, ok := h.Players[seat]; ok {
		if lp.Won {
			result = lang.X("hand_history.won", "Won {{.Chips}} chips", map[string]any{"Chips": lp.PotWon})
		} else {
			result = lang.X("hand_history.lost", "Lost")
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
	return fmt.Sprintf("Result: %s | Pot: %d | Position: %s | Players: %d",
		result, h.TotalPot, posStr, h.NumPlayers)
}

type HandHistoryViewState struct {
	SelectedHandID int
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

func streetActionSection(h *parser.Hand, street parser.Street, localSeat, openRaiser int, normalSize float32) fyne.CanvasObject {
	actions := streetActions(h, street)
	if len(actions) == 0 {
		return nil
	}

	sep := canvas.NewRectangle(theme.ShadowColor())
	sep.SetMinSize(fyne.NewSize(0, 1))

	header := canvas.NewText(strings.ToUpper(street.String()), color.NRGBA{R: 0xCF, G: 0xD8, B: 0xDC, A: 0xFF})
	header.TextStyle = fyne.TextStyle{Bold: true}
	header.TextSize = normalSize * 1.2

	lines := make([]fyne.CanvasObject, 0, len(actions)+2)
	lines = append(lines, sep, header)

	for _, sa := range actions {
		label := lang.X("hand_history.seat", "Seat {{.N}}", map[string]any{"N": sa.seat})
		if sa.seat == localSeat {
			label += lang.X("hand_history.you", " (You)")
		}
		if sa.seat == openRaiser {
			label += lang.X("hand_history.original_raiser", " (OR)")
		}

		line := fmt.Sprintf("%s: %s", label, sa.act.Action.String())
		if sa.act.Amount > 0 {
			line = fmt.Sprintf("%s %d", line, sa.act.Amount)
		}
		if !sa.act.Timestamp.IsZero() {
			line = fmt.Sprintf("%s  @%s", line, sa.act.Timestamp.Format("15:04:05"))
		}

		lineColor := color.NRGBA{R: 0xE0, G: 0xE0, B: 0xE0, A: 0xFF}
		if sa.seat == openRaiser {
			lineColor = color.NRGBA{R: 0xFF, G: 0xB3, B: 0x00, A: 0xFF}
		}
		if sa.seat == localSeat {
			lineColor = color.NRGBA{R: 0x66, G: 0xBB, B: 0x6A, A: 0xFF}
		}
		if sa.seat == localSeat && sa.seat == openRaiser {
			lineColor = color.NRGBA{R: 0x7E, G: 0x57, B: 0xC2, A: 0xFF}
		}

		txt := canvas.NewText(line, lineColor)
		txt.TextSize = normalSize
		lines = append(lines, txt)
	}

	return container.NewVBox(lines...)
}

// buildDetailPanel creates the right-side detail view for a selected hand.
func buildDetailPanel(h *parser.Hand, localSeat int) fyne.CanvasObject {
	if h == nil {
		msg := widget.NewLabel(lang.X("hand_history.select_hand", "Select a hand to see details."))
		msg.Alignment = fyne.TextAlignCenter
		msg.TextStyle = fyne.TextStyle{Italic: true}
		return container.NewCenter(msg)
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
		boardRows = append(boardRows, container.NewHBox(flopLabel, cardsRow(flopCards, normalSize, "")))
	}
	if len(turnCards) > 0 {
		turnLabel := widget.NewLabel(lang.X("hand_history.turn", "Turn:"))
		boardRows = append(boardRows, container.NewHBox(turnLabel, cardsRow(turnCards, normalSize, "")))
	}
	if len(riverCards) > 0 {
		riverLabel := widget.NewLabel(lang.X("hand_history.river", "River:"))
		boardRows = append(boardRows, container.NewHBox(riverLabel, cardsRow(riverCards, normalSize, "")))
	}
	if len(boardRows) == 0 {
		boardRows = append(boardRows, widget.NewLabel(lang.X("hand_history.no_community_cards", "No community cards")))
	}

	// ----- Result -----
	resultHeader := widget.NewLabel(lang.X("hand_history.result", "Result"))
	resultHeader.TextStyle = fyne.TextStyle{Bold: true}

	var resultText *canvas.Text
	if lp, ok := h.Players[seat]; ok {
		if lp.Won {
			resultText = canvas.NewText(
				lang.X("hand_history.won_detail", "Won  +{{.Chips}} chips (pot: {{.Pot}})", map[string]any{"Chips": lp.PotWon, "Pot": h.TotalPot}),
				color.NRGBA{R: 0x4c, G: 0xaf, B: 0x50, A: 0xff},
			)
		} else {
			invested := 0
			for _, act := range lp.Actions {
				invested += act.Amount
			}
			resultText = canvas.NewText(
				lang.X("hand_history.lost_detail", "Lost  -{{.Chips}} chips (pot: {{.Pot}})", map[string]any{"Chips": invested, "Pot": h.TotalPot}),
				color.NRGBA{R: 0xf4, G: 0x43, B: 0x36, A: 0xff},
			)
		}
	} else {
		resultText = canvas.NewText(lang.X("hand_history.not_in_hand", "Not in this hand"), theme.ForegroundColor())
	}
	resultText.TextStyle = fyne.TextStyle{Bold: true}
	resultText.TextSize = normalSize * 1.2

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

	// ----- Assemble -----
	boardSection := container.NewVBox(boardRows...)
	actionsSection := container.NewVBox(actionSections...)

	sep := func() fyne.CanvasObject {
		r := canvas.NewRectangle(theme.ShadowColor())
		r.SetMinSize(fyne.NewSize(0, 1))
		return r
	}

	content := container.NewVBox(
		holeHeader,
		holeRow,
		sep(),
		commHeader,
		boardSection,
		sep(),
		resultHeader,
		resultText,
		sep(),
		actionsHeader,
		actionsSection,
	)

	return container.NewScroll(container.NewPadded(content))
}

// NewHandHistoryTab creates the "Hand History" tab canvas object.
// hands should be in chronological order; this function reverses them for display.
func NewHandHistoryTab(hands []*parser.Hand, localSeat int, state *HandHistoryViewState) fyne.CanvasObject {
	if len(hands) == 0 {
		msg := widget.NewLabel(lang.X("hand_history.no_hands", "No hands recorded yet.\nStart playing in the VR Poker world!"))
		msg.Alignment = fyne.TextAlignCenter
		msg.Wrapping = fyne.TextWrapWord
		return container.NewCenter(msg)
	}
	if state == nil {
		state = &HandHistoryViewState{SelectedHandID: -1}
	}

	// Reverse order: newest first.
	reversed := make([]*parser.Hand, len(hands))
	for i, h := range hands {
		reversed[len(hands)-1-i] = h
	}

	// Detail panel placeholder – replaced on selection.
	detailContent := container.NewStack()
	detailContent.Objects = []fyne.CanvasObject{buildDetailPanel(nil, localSeat)}

	list := widget.NewList(
		func() int { return len(reversed) },
		func() fyne.CanvasObject {
			line1 := widget.NewLabel("")
			line1.TextStyle = fyne.TextStyle{Bold: true}
			line2 := widget.NewLabel("")
			line2.TextStyle = fyne.TextStyle{Italic: true}
			return container.NewVBox(line1, line2)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			vbox := obj.(*fyne.Container)
			h := reversed[id]
			line1 := vbox.Objects[0].(*widget.Label)
			line2 := vbox.Objects[1].(*widget.Label)
			line1.SetText(handSummaryLine1(h, localSeat))
			line2.SetText(handSummaryLine2(h, localSeat))
		},
	)

	list.OnSelected = func(id widget.ListItemID) {
		h := reversed[id]
		state.SelectedHandID = h.ID
		detail := buildDetailPanel(h, localSeat)
		detailContent.Objects = []fyne.CanvasObject{detail}
		detailContent.Refresh()
	}

	if state.SelectedHandID >= 0 {
		for i, h := range reversed {
			if h.ID == state.SelectedHandID {
				list.Select(i)
				break
			}
		}
	}

	split := container.NewHSplit(list, detailContent)
	split.Offset = 0.40

	return split
}
