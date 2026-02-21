package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

type HintSide int

const (
	HintSideLeft HintSide = iota
	HintSideRight
	HintSideAuto
)

// newHoverHint creates a reusable hover-trigger hint overlay.
// Preferred behavior is to place the hint on the left side of the icon,
// and automatically switch side if there is not enough space.
func newHoverHint(message string, preferred HintSide) (fyne.CanvasObject, *fyne.Container) {
	hintLabel := canvas.NewText(message, color.White)
	hintLabel.TextSize = 12

	hintBg := canvas.NewRectangle(color.NRGBA{R: 0x26, G: 0x2B, B: 0x33, A: 0xF2})
	hintBg.CornerRadius = 6
	hintBg.StrokeColor = color.NRGBA{R: 0xFF, G: 0xC1, B: 0x07, A: 0xC8}
	hintBg.StrokeWidth = 1

	hintContent := container.NewPadded(hintLabel)
	hintMin := hintContent.MinSize()
	hint := container.NewStack(hintBg, hintContent)
	hint.Resize(fyne.NewSize(hintMin.Width+6, hintMin.Height+4))
	hint.Hide()

	overlay := container.NewWithoutLayout(hint)

	var icon *hoverWarnIcon
	icon = newHoverWarnIcon(func(hover bool) {
		if hover {
			overlaySize := overlay.Size()
			hintSize := hint.Size()
			iconSize := icon.MinSize()
			anchorX := overlaySize.Width - iconSize.Width - 2

			leftX := anchorX - hintSize.Width - 2
			rightX := anchorX + iconSize.Width + 2

			x := leftX
			switch preferred {
			case HintSideRight:
				x = rightX
				if x+hintSize.Width > overlaySize.Width-4 {
					x = leftX
				}
			case HintSideAuto:
				spaceLeft := anchorX
				spaceRight := overlaySize.Width - (anchorX + iconSize.Width)
				if spaceRight > spaceLeft {
					x = rightX
				}
				if x+hintSize.Width > overlaySize.Width-4 {
					x = leftX
				}
			default:
				if x < 4 {
					x = rightX
				}
			}

			if x < 4 {
				x = 4
			}
			if x+hintSize.Width > overlaySize.Width-4 {
				x = overlaySize.Width - hintSize.Width - 4
			}

			y := float32(4)
			hint.Move(fyne.NewPos(x, y))
			hint.Show()
		} else {
			hint.Hide()
		}
		overlay.Refresh()
	})

	return icon, overlay
}
