package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// SettingsTab holds settings UI state
type SettingsTab struct {
	LogPath     string
	OnPathChange func(string)
	win         fyne.Window
}

// NewSettingsTab creates the settings tab
func NewSettingsTab(currentPath string, win fyne.Window, onPathChange func(string)) fyne.CanvasObject {
	st := &SettingsTab{
		LogPath:     currentPath,
		OnPathChange: onPathChange,
		win:         win,
	}
	return st.build()
}

func (st *SettingsTab) build() fyne.CanvasObject {
	title := widget.NewLabelWithStyle("Settings", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})

	// Log file path
	pathLabel := widget.NewLabel("VRChat Log File Path:")
	pathEntry := widget.NewEntry()
	pathEntry.SetPlaceHolder("Path to VRChat output_log_*.txt")
	pathEntry.SetText(st.LogPath)

	browseBtn := widget.NewButton("Browse...", func() {
		dialog.ShowFileOpen(func(f fyne.URIReadCloser, err error) {
			if err != nil || f == nil {
				return
			}
			f.Close()
			path := f.URI().Path()
			pathEntry.SetText(path)
			st.LogPath = path
			if st.OnPathChange != nil {
				st.OnPathChange(path)
			}
		}, st.win)
	})

	applyBtn := widget.NewButton("Apply", func() {
		path := pathEntry.Text
		st.LogPath = path
		if st.OnPathChange != nil {
			st.OnPathChange(path)
		}
	})
	applyBtn.Importance = widget.HighImportance

	pathRow := container.NewBorder(nil, nil, nil, container.NewHBox(browseBtn, applyBtn), pathEntry)

	// Info box
	infoLabel := widget.NewLabel(
		"The application monitors your VRChat log file in real-time.\n" +
		"Log files are typically found at:\n\n" +
		"  Linux (Steam Proton):\n" +
		"  ~/.local/share/Steam/steamapps/compatdata/438100/pfx/\n" +
		"  drive_c/users/steamuser/AppData/LocalLow/VRChat/VRChat/\n\n" +
		"  Windows:\n" +
		"  %APPDATA%\\..\\LocalLow\\VRChat\\VRChat\\\n\n" +
		"Statistics are calculated for VR Poker world sessions only.\n" +
		"Historical logs (from before the app was started) are also analyzed.",
	)
	infoLabel.Wrapping = fyne.TextWrapBreak

	// About section
	aboutTitle := widget.NewLabelWithStyle("About", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	aboutText := widget.NewLabel(
		"VRC VRPoker Stats v1.0\n" +
		"Tracks your poker statistics in the VRChat VR Poker world.\n\n" +
		"Metrics tracked:\n" +
		"  • VPIP (Voluntarily Put Money In Pot)\n" +
		"  • PFR (Pre-Flop Raise %)\n" +
		"  • 3Bet% (3-Bet frequency)\n" +
		"  • Fold to 3Bet%\n" +
		"  • W$SD (Won Money at Showdown %)\n" +
		"  • Hand Range Analysis (13x13 grid)\n" +
		"  • Position-based statistics",
	)
	aboutText.Wrapping = fyne.TextWrapBreak

	form := container.NewVBox(
		title,
		widget.NewSeparator(),
		pathLabel,
		pathRow,
		widget.NewSeparator(),
		infoLabel,
		widget.NewSeparator(),
		aboutTitle,
		aboutText,
	)

	return container.NewScroll(container.NewPadded(form))
}
