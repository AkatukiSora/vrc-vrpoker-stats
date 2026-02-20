package ui

import (
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
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

// handSummaryLine1 formats the first summary line for a hand list item.
func handSummaryLine1(h *parser.Hand, localSeat int) string {
	timeStr := h.StartTime.Format("15:04")

	// Hole cards
	var holeStr string
	if lp, ok := h.Players[localSeat]; ok && len(lp.HoleCards) == 2 {
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
	if lp, ok := h.Players[localSeat]; ok {
		if lp.Won {
			result = fmt.Sprintf("Won %d chips", lp.PotWon)
		} else {
			result = "Lost"
		}
		posStr = lp.Position.String()
	} else {
		result = "N/A"
		posStr = "?"
	}
	return fmt.Sprintf("Result: %s | Pot: %d | Position: %s | Players: %d",
		result, h.TotalPot, posStr, h.NumPlayers)
}

// buildDetailPanel creates the right-side detail view for a selected hand.
func buildDetailPanel(h *parser.Hand, localSeat int) fyne.CanvasObject {
	if h == nil {
		msg := widget.NewLabel("Select a hand to see details.")
		msg.Alignment = fyne.TextAlignCenter
		msg.TextStyle = fyne.TextStyle{Italic: true}
		return container.NewCenter(msg)
	}

	bigSize := theme.TextSize() * 1.8
	normalSize := theme.TextSize()

	// ----- Hole Cards -----
	holeHeader := widget.NewLabel("Hole Cards")
	holeHeader.TextStyle = fyne.TextStyle{Bold: true}

	var holeCards []parser.Card
	if lp, ok := h.Players[localSeat]; ok {
		holeCards = lp.HoleCards
	}
	holeRow := cardsRow(holeCards, bigSize, "Cards not recorded")

	// ----- Community Cards -----
	commHeader := widget.NewLabel("Community Cards")
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
		flopLabel := widget.NewLabel("Flop:")
		boardRows = append(boardRows, container.NewHBox(flopLabel, cardsRow(flopCards, normalSize, "")))
	}
	if len(turnCards) > 0 {
		turnLabel := widget.NewLabel("Turn:")
		boardRows = append(boardRows, container.NewHBox(turnLabel, cardsRow(turnCards, normalSize, "")))
	}
	if len(riverCards) > 0 {
		riverLabel := widget.NewLabel("River:")
		boardRows = append(boardRows, container.NewHBox(riverLabel, cardsRow(riverCards, normalSize, "")))
	}
	if len(boardRows) == 0 {
		boardRows = append(boardRows, widget.NewLabel("No community cards"))
	}

	// ----- Result -----
	resultHeader := widget.NewLabel("Result")
	resultHeader.TextStyle = fyne.TextStyle{Bold: true}

	var resultText *canvas.Text
	if lp, ok := h.Players[localSeat]; ok {
		if lp.Won {
			resultText = canvas.NewText(
				fmt.Sprintf("Won  +%d chips (pot: %d)", lp.PotWon, h.TotalPot),
				color.NRGBA{R: 0x4c, G: 0xaf, B: 0x50, A: 0xff},
			)
		} else {
			invested := 0
			for _, act := range lp.Actions {
				invested += act.Amount
			}
			resultText = canvas.NewText(
				fmt.Sprintf("Lost  -%d chips (pot: %d)", invested, h.TotalPot),
				color.NRGBA{R: 0xf4, G: 0x43, B: 0x36, A: 0xff},
			)
		}
	} else {
		resultText = canvas.NewText("Not in this hand", theme.ForegroundColor())
	}
	resultText.TextStyle = fyne.TextStyle{Bold: true}
	resultText.TextSize = normalSize * 1.2

	// ----- Actions -----
	actionsHeader := widget.NewLabel("Actions")
	actionsHeader.TextStyle = fyne.TextStyle{Bold: true}

	var actionLines []fyne.CanvasObject
	if lp, ok := h.Players[localSeat]; ok && len(lp.Actions) > 0 {
		for _, act := range lp.Actions {
			var line string
			if act.Amount > 0 {
				line = fmt.Sprintf("[%s] %s %d", act.Street.String(), act.Action.String(), act.Amount)
			} else {
				line = fmt.Sprintf("[%s] %s", act.Street.String(), act.Action.String())
			}
			actionLines = append(actionLines, widget.NewLabel(line))
		}
	} else {
		actionLines = append(actionLines, widget.NewLabel("No actions recorded"))
	}

	// ----- Assemble -----
	boardSection := container.NewVBox(boardRows...)
	actionsSection := container.NewVBox(actionLines...)

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
func NewHandHistoryTab(hands []*parser.Hand, localSeat int) fyne.CanvasObject {
	if len(hands) == 0 {
		msg := widget.NewLabel("No hands recorded yet.\nStart playing in the VR Poker world!")
		msg.Alignment = fyne.TextAlignCenter
		msg.Wrapping = fyne.TextWrapWord
		return container.NewCenter(msg)
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
		detail := buildDetailPanel(h, localSeat)
		detailContent.Objects = []fyne.CanvasObject{detail}
		detailContent.Refresh()
	}

	split := container.NewHSplit(list, detailContent)
	split.Offset = 0.40

	return split
}
