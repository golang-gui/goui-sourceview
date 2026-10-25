// Run: go run ./example/declarative. A minimal declarative editor with
// built-in find and go-to UI, a stable Document and Integrated window chrome.
package main

import (
	"log"

	"github.com/golang-gui/goui-sourceview/language"
	sourceui "github.com/golang-gui/goui-sourceview/ui"
	"github.com/golang-gui/goui/theme/modern"
	base "github.com/golang-gui/goui/ui"
)

func main() {
	document := sourceui.NewDocument("{\n    \"editor\": \"GOUI SourceView\",\n    \"editable\": true\n}\n")
	if err := document.SetLanguage(language.JSON); err != nil {
		log.Fatal(err)
	}
	bindings := sourceui.DefaultKeyBindings()
	bindings[sourceui.CommandComplete] = []sourceui.KeyGesture{{Key: base.KeyF2}}
	if err := base.Run("org.golang-gui.SourceViewDeclarative", func(app base.App) base.RootView {
		return base.Root().
			StyleSheet(modern.FromSettings(app.Settings())).
			Windows(
				base.Window("main").
					Title("Declarative SourceView").
					Chrome(base.WindowChromeIntegrated).
					Size(900, 600).Center(true).
					Content(
						base.VBox(
							base.HeaderBar(base.Label("Declarative SourceView")).Padding(8),
							base.ScrollView(sourceui.SourceView().ID("editor").Document(document).
								AcceptsTab(true).
								IndentOptions(sourceui.IndentOptions{TabWidth: 4, IndentWidth: 4, InsertSpaces: false}).
								WhitespaceMode(sourceui.WhitespaceBoundary).
								KeyBindings(bindings)).Style("source-view"),
						),
					),
			)
	}); err != nil {
		log.Fatal(err)
	}
}
