package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

const (
	dividerFadeStartFromEdge = 0.30
	dividerFadeEndFromEdge   = 0.10
)

var dividerBaseColor = color.NRGBA{R: 0xAC, G: 0xAF, B: 0xB5, A: 0xFF}

// fadedAlphaFactor computes an alpha factor [0.0, 1.0] for a position pos within total,
// fading from both edges.
func fadedAlphaFactor(pos, total int) float32 {
	if total <= 1 {
		return 1
	}
	t := float32(pos) / float32(total-1)
	edge := t
	if 1-t < edge {
		edge = 1 - t
	}
	if edge <= dividerFadeEndFromEdge {
		return 0
	}
	if edge < dividerFadeStartFromEdge {
		return (edge - dividerFadeEndFromEdge) / (dividerFadeStartFromEdge - dividerFadeEndFromEdge)
	}
	return 1
}

func newSectionDivider() fyne.CanvasObject {
	r := canvas.NewRasterWithPixels(func(x, _x, w, _h int) color.Color {
		c := dividerBaseColor
		c.A = uint8(float32(c.A) * fadedAlphaFactor(x, w))
		return c
	})
	r.SetMinSize(fyne.NewSize(0, 1))
	return r
}
