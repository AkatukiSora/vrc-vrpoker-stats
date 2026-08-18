package ui

import (
	"context"
	"io"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/application"
	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/persistence"
)

// handExportDialog isolates the native Fyne save picker from export behavior.
// A nil writer means that the user cancelled the picker.
type handExportDialog interface {
	Save(defaultName string, done func(writer io.WriteCloser, name string, err error))
}

type nativeHandExportDialog struct{ window fyne.Window }

func (d nativeHandExportDialog) Save(defaultName string, done func(io.WriteCloser, string, error)) {
	save := dialog.NewFileSave(func(writer fyne.URIWriteCloser, err error) {
		if writer == nil {
			done(nil, "", err)
			return
		}
		done(writer, writer.URI().Name(), err)
	}, d.window)
	save.SetFileName(defaultName)
	save.Show()
}

type handExportFlow struct {
	ctx     context.Context
	service application.AppService
	dialog  handExportDialog
	report  func(string)
}

func (f handExportFlow) begin(uid string) {
	if uid == "" || f.service == nil || f.dialog == nil {
		return
	}
	f.dialog.Save("vrpoker-hand-"+uid+".phh", func(writer io.WriteCloser, name string, err error) {
		if err != nil {
			f.reportError(err)
			return
		}
		if writer == nil {
			f.reportMessage(lang.X("hand_history.export.cancelled", "Hand export cancelled."))
			return
		}
		go func() {
			if err := f.write(uid, writer); err != nil {
				f.reportError(err)
				return
			}
			f.reportMessage(lang.X("hand_history.export.success", "Hand exported: {{.Path}}", map[string]any{"Path": name}))
		}()
	})
}

func (f handExportFlow) write(uid string, writer io.WriteCloser) error {
	data, err := f.service.ExportHandPHH(f.ctx, uid)
	if err == nil {
		_, err = writer.Write(data)
	}
	closeErr := writer.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func (f handExportFlow) beginRange(filter persistence.HandFilter) {
	if f.service == nil || f.dialog == nil {
		return
	}
	f.dialog.Save("vrpoker-hands.phh.zip", func(writer io.WriteCloser, name string, err error) {
		if err != nil {
			f.reportError(err)
			return
		}
		if writer == nil {
			f.reportMessage(lang.X("hand_history.export.cancelled", "Hand export cancelled."))
			return
		}
		go func() {
			data, count, exportErr := f.service.ExportHandsPHH(f.ctx, filter)
			if exportErr == nil {
				_, exportErr = writer.Write(data)
			}
			closeErr := writer.Close()
			if exportErr == nil {
				exportErr = closeErr
			}
			if exportErr != nil {
				f.reportError(exportErr)
				return
			}
			f.reportMessage(lang.X("hand_history.export.range.success", "Exported {{.Count}} hands: {{.Path}}", map[string]any{"Count": count, "Path": name}))
		}()
	})
}

type exportRangeMode int

const (
	exportRangeAll exportRangeMode = iota
	exportRangeHands
	exportRangeDays
	exportRangeMonths
)

func exportRangeFilter(mode exportRangeMode, amount int, now time.Time) (persistence.HandFilter, error) {
	filter := persistence.HandFilter{}
	switch mode {
	case exportRangeAll:
		return filter, nil
	case exportRangeHands:
		filter.LastN = amount
	case exportRangeDays:
		from := now.AddDate(0, 0, -amount)
		filter.FromTime = &from
	case exportRangeMonths:
		from := now.AddDate(0, -amount, 0)
		filter.FromTime = &from
	default:
		return filter, strconv.ErrSyntax
	}
	if amount <= 0 {
		return filter, strconv.ErrSyntax
	}
	return filter, nil
}

func showHandExportRangeDialog(window fyne.Window, onExport func(persistence.HandFilter), onInvalid func(string)) {
	if window == nil || onExport == nil {
		return
	}
	labels := []string{
		lang.X("hand_history.export.range.all", "All hands"),
		lang.X("hand_history.export.range.hands", "Last N hands"),
		lang.X("hand_history.export.range.days", "Last N days"),
		lang.X("hand_history.export.range.months", "Last N months"),
	}
	mode := exportRangeAll
	amount := newPositiveIntCommitEntry()
	amount.SetText("100") //i18n:ignore default numeric value
	amountLabel := widget.NewLabel(lang.X("hand_history.export.range.amount", "Amount:"))
	amount.Hide()
	amountLabel.Hide()
	selectMode := widget.NewSelect(labels, func(value string) {
		for i, label := range labels {
			if label == value {
				mode = exportRangeMode(i)
			}
		}
		if mode == exportRangeAll {
			amount.Hide()
			amountLabel.Hide()
		} else {
			amount.Show()
			amountLabel.Show()
		}
	})
	selectMode.SetSelected(labels[0])
	content := container.NewVBox(
		widget.NewLabel(lang.X("hand_history.export.range.description", "Choose the hands to include in a ZIP of PHH files.")),
		container.NewHBox(widget.NewLabel(lang.X("hand_history.export.range.mode", "Range:")), selectMode),
		container.NewHBox(amountLabel, amount),
	)
	dialog.NewCustomConfirm(
		lang.X("hand_history.export.range.title", "Export hand range"),
		lang.X("hand_history.export.range.confirm", "Export ZIP"),
		lang.X("hand_history.export.range.cancel", "Cancel"),
		content,
		func(ok bool) {
			if !ok {
				return
			}
			value, err := strconv.Atoi(amount.Text)
			if mode != exportRangeAll && (err != nil || value <= 0) {
				if onInvalid != nil {
					onInvalid(lang.X("hand_history.export.range.invalid", "Enter a positive amount."))
				}
				return
			}
			filter, err := exportRangeFilter(mode, value, time.Now())
			if err != nil {
				if onInvalid != nil {
					onInvalid(lang.X("hand_history.export.range.invalid", "Enter a positive amount."))
				}
				return
			}
			onExport(filter)
		}, window,
	).Show()
}

func (f handExportFlow) reportError(err error) {
	f.reportMessage(lang.X("hand_history.export.error", "Could not export hand: {{.Error}}", map[string]any{"Error": err}))
}

func (f handExportFlow) reportMessage(message string) {
	if f.report != nil {
		f.report(message)
	}
}

func newHandExportFlow(ctx context.Context, service application.AppService, dialog handExportDialog, report func(string)) handExportFlow {
	if ctx == nil {
		ctx = context.Background()
	}
	return handExportFlow{ctx: ctx, service: service, dialog: dialog, report: report}
}
