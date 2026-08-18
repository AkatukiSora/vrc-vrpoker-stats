package ui

import (
	"context"
	"io"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/lang"

	"github.com/AkatukiSora/vrc-vrpoker-ststs/internal/application"
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
