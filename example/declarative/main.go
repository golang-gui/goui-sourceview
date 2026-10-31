// Run: go run ./example/declarative. A minimal declarative editor with
// built-in find and go-to UI, a stable Document, local selection binding and
// Integrated window chrome.
package main

import (
	"fmt"
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
	selection := base.MakeState(sourceui.TextSelection{})
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
								BindSelection(&selection).
								AcceptsTab(true).
								IndentOptions(sourceui.IndentOptions{TabWidth: 4, IndentWidth: 4, InsertSpaces: false}).
								WhitespaceMode(sourceui.WhitespaceBoundary).
								KeyBindings(bindings)).Style("source-view"),
							base.Build(func() base.View {
								value := selection.Get()
								return base.HBox(
									base.Label(fmt.Sprintf("Selection: %d–%d (UTF-8 bytes)", value.Anchor, value.Caret)).MainWeight(1),
									base.Button("Go to start").OnClick(func() { selection.Set(sourceui.TextSelection{}) }),
								).Padding(8)
							}),
						),
					),
			)
	}); err != nil {
		log.Fatal(err)
	}
}
