package ui

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

// statCard builds a single stat card with a name label and a large colored value label.
func statCard(name, value string, valueColor color.Color) fyne.CanvasObject {
	nameLabel := widget.NewLabel(name)
	nameLabel.TextStyle = fyne.TextStyle{Bold: true}
	nameLabel.Alignment = fyne.TextAlignCenter

	valueText := canvas.NewText(value, valueColor)
	valueText.TextStyle = fyne.TextStyle{Bold: true}
	valueText.TextSize = theme.TextSize() * 1.6
	valueText.Alignment = fyne.TextAlignCenter

	separator := canvas.NewRectangle(theme.ShadowColor())
	separator.SetMinSize(fyne.NewSize(0, 1))

	card := container.NewVBox(
		nameLabel,
		separator,
		container.NewCenter(valueText),
	)

	bg := canvas.NewRectangle(theme.OverlayBackgroundColor())
	bg.CornerRadius = 6

	return container.NewStack(bg, container.NewPadded(card))
}

// colorForWinRate picks green/red/default based on win percentage.
func colorForWinRate(rate float64) color.Color {
	if rate > 50 {
		return color.NRGBA{R: 0x4c, G: 0xaf, B: 0x50, A: 0xff} // green
	}
	if rate < 40 {
		return color.NRGBA{R: 0xf4, G: 0x43, B: 0x36, A: 0xff} // red
	}
	return theme.ForegroundColor()
}

// colorForVPIP picks yellow/orange/default based on VPIP percentage.
func colorForVPIP(rate float64) color.Color {
	if rate > 35 {
		return color.NRGBA{R: 0xff, G: 0x98, B: 0x00, A: 0xff} // orange
	}
	if rate >= 20 {
		return color.NRGBA{R: 0xff, G: 0xd6, B: 0x00, A: 0xff} // yellow
	}
	return theme.ForegroundColor()
}

// colorForProfit picks green for positive, red for negative profit.
func colorForProfit(profit int) color.Color {
	if profit > 0 {
		return color.NRGBA{R: 0x4c, G: 0xaf, B: 0x50, A: 0xff}
	}
	if profit < 0 {
		return color.NRGBA{R: 0xf4, G: 0x43, B: 0x36, A: 0xff}
	}
	return theme.ForegroundColor()
}

// NewOverviewTab returns the "Overview" tab canvas object.
func NewOverviewTab(s *stats.Stats) fyne.CanvasObject {
	// Empty state
	if s == nil || s.TotalHands == 0 {
		msg := widget.NewLabel("No hands recorded yet.\nStart playing in the VR Poker world!")
		msg.Alignment = fyne.TextAlignCenter
		msg.Wrapping = fyne.TextWrapWord
		return container.NewCenter(msg)
	}

	// Title
	title := widget.NewLabel("Overall Statistics")
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter

	titleSep := canvas.NewRectangle(theme.PrimaryColor())
	titleSep.SetMinSize(fyne.NewSize(0, 2))

	// Build stat cards
	profit := s.TotalPotWon - s.TotalInvested

	cards := []fyne.CanvasObject{
		statCard("Hands Played",
			fmt.Sprintf("%d", s.TotalHands),
			theme.ForegroundColor()),

		statCard("Win Rate",
			fmt.Sprintf("%.1f%%", s.WinRate()),
			colorForWinRate(s.WinRate())),

		statCard("VPIP",
			fmt.Sprintf("%.1f%%", s.VPIPRate()),
			colorForVPIP(s.VPIPRate())),

		statCard("PFR",
			fmt.Sprintf("%.1f%%", s.PFRRate()),
			theme.ForegroundColor()),

		statCard("3Bet%",
			fmt.Sprintf("%.1f%%", s.ThreeBetRate()),
			theme.ForegroundColor()),

		statCard("Fold to 3Bet%",
			fmt.Sprintf("%.1f%%", s.FoldTo3BetRate()),
			theme.ForegroundColor()),

		statCard("W$SD",
			fmt.Sprintf("%.1f%%", s.WSDRate()),
			theme.ForegroundColor()),

		statCard("Total Profit",
			fmt.Sprintf("%+d chips", profit),
			colorForProfit(profit)),

		statCard("Showdowns",
			fmt.Sprintf("%d / %d", s.WonShowdowns, s.ShowdownHands),
			theme.ForegroundColor()),

		statCard("Won Hands",
			fmt.Sprintf("%d / %d", s.WonHands, s.TotalHands),
			theme.ForegroundColor()),
	}

	grid := container.NewGridWithColumns(2, cards...)

	content := container.NewVBox(
		container.NewPadded(title),
		titleSep,
		container.NewPadded(grid),
	)

	return container.NewScroll(content)
}
