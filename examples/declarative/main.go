package main

import (
	"log"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	sourceui "github.com/golang-gui/goui-sourceview/ui"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/theme/modern"
	base "github.com/golang-gui/goui/ui"
)

func main() {
	document := source.NewDocument("{\n    \"editor\": \"GOUI SourceView\",\n    \"editable\": true\n}\n")
	_ = document.SetLanguage(language.JSON)
	if err := base.Run("org.golang-gui.SourceViewDeclarative", func(app base.App) base.RootView {
		return base.Root().StyleSheet(modern.FromSettings(app.Settings())).Windows(base.Window("main").Title("Declarative SourceView").Size(900, 600).Content(
			base.Overlay(
				base.ScrollView(sourceui.SourceView().ID("editor").Document(document).AcceptsTab(true)),
				base.OverlayItem(sourceui.FindBar("editor")).OnPosition(func(available, size geometry.Size, p *geometry.Point) { p.X = max(0, available.Width-size.Width-8) }),
			)))
	}); err != nil {
		log.Fatal(err)
	}
}
