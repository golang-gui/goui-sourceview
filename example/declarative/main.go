// Run: go run ./example/declarative. A minimal declarative editor with an
// optional FindBar, a stable Document and Integrated window chrome.
package main

import (
	"log"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	sourceui "github.com/golang-gui/goui-sourceview/ui"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/theme/modern"
	base "github.com/golang-gui/goui/ui"
)

func main() {
	document := source.NewDocument("{\n    \"editor\": \"GOUI SourceView\",\n    \"editable\": true\n}\n")
	_ = document.SetLanguage(language.JSON)
	bindings := source.DefaultKeyBindings()
	bindings[source.CommandComplete] = []gui.KeyGesture{{Key: gui.KeyF2}}
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
							base.Overlay(
								base.ScrollView(sourceui.SourceView().ID("editor").Document(document).AcceptsTab(true).KeyBindings(bindings)).Style("source-view"),
								base.OverlayItem(sourceui.FindBar("editor")).OnPosition(func(available, size geometry.Size, p *geometry.Point) {
									p.X = max(0, available.Width-size.Width-8)
								}),
							).MainWeight(1),
						),
					),
			)
	}); err != nil {
		log.Fatal(err)
	}
}
