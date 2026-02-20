package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/parser"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/stats"
)

// positionOrder defines the display order (most aggressive position first).
var positionDisplayOrder = []parser.Position{
	parser.PosBTN,
	parser.PosCO,
	parser.PosMP1,
	parser.PosMP,
	parser.PosUTG1,
	parser.PosUTG,
	parser.PosBB,
	parser.PosSB,
}

// positionTableHeaders are the column headers for the position stats table.
var positionTableHeaders = []string{
	"Position", "Hands", "VPIP%", "PFR%", "3Bet%", "F3Bet%", "W$SD%", "WR%", "Profit",
}

// positionColumnWidths maps each column index to its preferred minimum width.
var positionColumnWidths = []float32{
	60,  // Position
	70,  // Hands
	70,  // VPIP%
	70,  // PFR%
	70,  // 3Bet%
	70,  // F3Bet%
	70,  // W$SD%
	70,  // WR%
	80,  // Profit
}

// NewPositionStatsTab returns the "Position Stats" tab canvas object.
func NewPositionStatsTab(s *stats.Stats) fyne.CanvasObject {
	if s == nil || len(s.ByPosition) == 0 {
		msg := widget.NewLabel("No position data yet.")
		msg.Alignment = fyne.TextAlignCenter
		return container.NewCenter(msg)
	}

	// Build the row data: header row + one row per position.
	// rows[0] = headers, rows[1..] = data rows.
	rows := [][]string{positionTableHeaders}

	for _, pos := range positionDisplayOrder {
		ps, ok := s.ByPosition[pos]
		if !ok || ps.Hands == 0 {
			// Show N/A row only for positions that are in ByPosition with 0 hands,
			// or simply skip entirely. We skip positions with 0 hands.
			continue
		}

		profit := ps.PotWon - ps.Invested
		row := []string{
			pos.String(),
			fmt.Sprintf("%d", ps.Hands),
			fmt.Sprintf("%.1f", ps.VPIPRate()),
			fmt.Sprintf("%.1f", ps.PFRRate()),
			fmt.Sprintf("%.1f", ps.ThreeBetRate()),
			fmt.Sprintf("%.1f", ps.FoldTo3BetRate()),
			fmt.Sprintf("%.1f", ps.WSDRate()),
			fmt.Sprintf("%.1f", ps.WinRate()),
			fmt.Sprintf("%+d", profit),
		}
		rows = append(rows, row)
	}

	numCols := len(positionTableHeaders)
	numRows := len(rows)

	t := widget.NewTable(
		// Dimensions
		func() (int, int) { return numRows, numCols },
		// Create cell template
		func() fyne.CanvasObject {
			lbl := widget.NewLabel("")
			lbl.Alignment = fyne.TextAlignCenter
			return lbl
		},
		// Update cell content
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			lbl := obj.(*widget.Label)
			if id.Row < numRows && id.Col < numCols {
				lbl.SetText(rows[id.Row][id.Col])
			}
			if id.Row == 0 {
				lbl.TextStyle = fyne.TextStyle{Bold: true}
			} else {
				lbl.TextStyle = fyne.TextStyle{}
			}
			lbl.Refresh()
		},
	)

	// Set column widths.
	for i, w := range positionColumnWidths {
		t.SetColumnWidth(i, w)
	}

	return container.NewScroll(t)
}
