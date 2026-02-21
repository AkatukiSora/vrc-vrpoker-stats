package ui

import (
	"embed"
	"log"

	"fyne.io/fyne/v2/lang"
)

//go:embed translations
var translationsFS embed.FS

func init() {
	if err := lang.AddTranslationsFS(translationsFS, "translations"); err != nil {
		log.Printf("failed to load translations: %v", err)
	}
}
