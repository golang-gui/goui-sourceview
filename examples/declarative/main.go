package main

import (
	"log"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	sourceui "github.com/golang-gui/goui-sourceview/ui"
	"github.com/golang-gui/goui/layout"
	base "github.com/golang-gui/goui/ui"
)

func main() {
	document := source.NewDocument("{\n    \"editor\": \"GOUI SourceView\",\n    \"editable\": true\n}\n")
	_ = document.SetLanguage(language.JSON)
	if err := base.Run("org.golang-gui.SourceViewDeclarative", func(base.App) base.RootView {
		return base.Window("main").Title("Declarative SourceView").Size(900, 600).Content(
			base.VBox(
				sourceui.FindBar("editor"),
				base.ScrollView(sourceui.SourceView().ID("editor").Document(document).AcceptsTab(true)).MainWeight(1),
			).CrossAlign(layout.CrossStretch))
	}); err != nil {
		log.Fatal(err)
	}
}
