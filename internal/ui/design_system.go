package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var (
	uiMutedTextColor    = color.NRGBA{R: 0xA8, G: 0xAF, B: 0xB8, A: 0xFF}
	uiWarningColor      = color.NRGBA{R: 0xFF, G: 0xC1, B: 0x07, A: 0xE8}
	uiCardBorderColor   = color.NRGBA{R: 0x8A, G: 0x92, B: 0x9C, A: 0x2E}
	uiSurfaceTint       = color.NRGBA{R: 0x72, G: 0x86, B: 0x9A, A: 0x12}
	uiHeroSurfaceTint   = color.NRGBA{R: 0x3F, G: 0x81, B: 0xC6, A: 0x2E}
	uiSuccessAccent     = color.NRGBA{R: 0x4C, G: 0xAF, B: 0x50, A: 0xFF}
	uiDangerAccent      = color.NRGBA{R: 0xF4, G: 0x43, B: 0x36, A: 0xFF}
	uiInfoAccent        = color.NRGBA{R: 0x29, G: 0xB6, B: 0xF6, A: 0xFF}
	uiNeutralChipAccent = color.NRGBA{R: 0x90, G: 0xA4, B: 0xAE, A: 0xFF}
)

func newSectionCard(content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(theme.InputBackgroundColor())
	bg.CornerRadius = 10

	tint := canvas.NewRectangle(uiSurfaceTint)
	tint.CornerRadius = 10

	border := canvas.NewRectangle(color.Transparent)
	border.CornerRadius = 10
	border.StrokeColor = uiCardBorderColor
	border.StrokeWidth = 1

	return container.NewStack(bg, tint, border, container.NewPadded(content))
}

func newHeroCard(content fyne.CanvasObject) fyne.CanvasObject {
	bg := canvas.NewRectangle(theme.InputBackgroundColor())
	bg.CornerRadius = 10

	tint := canvas.NewRectangle(uiHeroSurfaceTint)
	tint.CornerRadius = 10

	border := canvas.NewRectangle(color.Transparent)
	border.CornerRadius = 10
	border.StrokeColor = color.NRGBA{R: 0x6C, G: 0x97, B: 0xC3, A: 0x70}
	border.StrokeWidth = 1

	return container.NewStack(bg, tint, border, container.NewPadded(content))
}

func newMetricChip(text string, accent color.Color) fyne.CanvasObject {
	lbl := widget.NewLabel(text)
	lbl.TextStyle = fyne.TextStyle{Bold: true}

	bg := canvas.NewRectangle(color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x00})
	bg.CornerRadius = 999
	if accent != nil {
		n := toNRGBA(accent)
		bg.FillColor = color.NRGBA{R: n.R, G: n.G, B: n.B, A: 0x2E}
	}

	border := canvas.NewRectangle(color.Transparent)
	border.CornerRadius = 999
	if accent != nil {
		n := toNRGBA(accent)
		border.StrokeColor = color.NRGBA{R: n.R, G: n.G, B: n.B, A: 0x8A}
	} else {
		border.StrokeColor = uiCardBorderColor
	}
	border.StrokeWidth = 1

	return container.NewStack(bg, border, container.NewPadded(lbl))
}

func newSubtleText(content string) *canvas.Text {
	t := canvas.NewText(content, uiMutedTextColor)
	t.TextSize = theme.TextSize() * 0.86
	return t
}

func newWarnMark(show bool) *canvas.Text {
	warn := canvas.NewText("", uiWarningColor)
	warn.TextStyle = fyne.TextStyle{Bold: true}
	warn.Alignment = fyne.TextAlignCenter
	warn.TextSize = theme.TextSize() * 0.9
	if show {
		warn.Text = "!" //i18n:ignore shared warning glyph
	}
	return warn
}

func toNRGBA(c color.Color) color.NRGBA {
	if c == nil {
		return color.NRGBA{}
	}
	r, g, b, a := c.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func newCenteredEmptyState(message string) fyne.CanvasObject {
	label := widget.NewLabel(message)
	label.Alignment = fyne.TextAlignCenter
	label.Wrapping = fyne.TextWrapWord

	card := newSectionCard(container.NewPadded(label))
	widthLock := canvas.NewRectangle(color.Transparent)
	widthLock.SetMinSize(fyne.NewSize(420, 0))

	return container.NewCenter(container.NewStack(widthLock, card))
}

// withMinHeight wraps obj in a Stack with an invisible rectangle enforcing a minimum height.
func withMinHeight(obj fyne.CanvasObject, minHeight float32) fyne.CanvasObject {
	lock := canvas.NewRectangle(color.Transparent)
	lock.SetMinSize(fyne.NewSize(0, minHeight))
	return container.NewStack(lock, obj)
}
