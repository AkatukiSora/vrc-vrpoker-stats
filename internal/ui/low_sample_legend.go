package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func newLowSampleLegend(text string) fyne.CanvasObject {
	mark := canvas.NewText(lang.X("warn_icon.mark", "!"), color.NRGBA{R: 0xFF, G: 0xC1, B: 0x07, A: 0xFF})
	mark.TextStyle = fyne.TextStyle{Bold: true}
	mark.TextSize = theme.TextSize() * 0.95

	legend := widget.NewLabel(text)
	legend.Wrapping = fyne.TextWrapWord

	return container.NewPadded(container.NewBorder(nil, nil, mark, nil, legend))
}

func withFixedLowSampleLegend(content fyne.CanvasObject) fyne.CanvasObject {
	legend := newLowSampleLegend(lang.X("metric.low_sample_legend", "! indicates a low-sample metric and may be inaccurate."))
	return container.NewBorder(nil, legend, nil, nil, content)
}
