package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"
)

type hoverWarnIcon struct {
	widget.BaseWidget
	onHover func(bool)
}

func newHoverWarnIcon(onHover func(bool)) *hoverWarnIcon {
	w := &hoverWarnIcon{onHover: onHover}
	w.ExtendBaseWidget(w)
	return w
}

func (w *hoverWarnIcon) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewCircle(color.NRGBA{R: 0xFF, G: 0xC1, B: 0x07, A: 0x24})
	bg.StrokeColor = color.NRGBA{R: 0xFF, G: 0xC1, B: 0x07, A: 0xD8}
	bg.StrokeWidth = 1.5

	mark := canvas.NewText(lang.X("warn_icon.mark", "!"), color.NRGBA{R: 0xFF, G: 0xC1, B: 0x07, A: 0xFF})
	mark.TextStyle = fyne.TextStyle{Bold: true}
	mark.Alignment = fyne.TextAlignCenter
	mark.TextSize = 13

	return widget.NewSimpleRenderer(container.NewStack(bg, container.NewCenter(mark)))
}

func (w *hoverWarnIcon) MinSize() fyne.Size {
	// Keep a generous hit area so hover is easy to trigger.
	return fyne.NewSize(24, 24)
}

func (w *hoverWarnIcon) MouseIn(*desktop.MouseEvent) {
	if w.onHover != nil {
		w.onHover(true)
	}
}

func (w *hoverWarnIcon) MouseMoved(*desktop.MouseEvent) {}

func (w *hoverWarnIcon) MouseOut() {
	if w.onHover != nil {
		w.onHover(false)
	}
}
