package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// newUITestWindow creates an in-memory Fyne app and window with the production
// theme. Use it for UI interaction and renderer tests; it does not need a
// native display server.
func newUITestWindow(t testing.TB, content fyne.CanvasObject) (fyne.App, fyne.Window) {
	t.Helper()

	app := test.NewTempApp(t)
	app.Settings().SetTheme(newPokerTheme())
	return app, test.NewTempWindow(t, content)
}

// flushUI waits for work queued with fyne.Do to finish before asserting UI
// state. Asynchronous application work still needs an explicit condition wait.
func flushUI() {
	fyne.DoAndWait(func() {})
}
