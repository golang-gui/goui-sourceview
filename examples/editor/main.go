// Run with go run ./examples/editor. An optional filename loads local UTF-8
// text. Toolbar changes affect memory only; this example does not save files.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/theme/modern"
)

var samples = map[language.ID]string{
	language.JSON: "{\n    \"name\": \"GOUI SourceView\",\n    \"languages\": [\"JSON\", \"XML\", \"HTML\"],\n    \"editable\": true,\n    \"unicode\": \"中文 é 👩‍💻\",\n    \"count\": 42\n}\n",
	language.XML:  "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!-- A comment spanning\n     two logical lines. -->\n<editor name=\"GOUI &amp; SourceView\">\n    <![CDATA[<unparsed> text]]>\n    <language id=\"xml\">中文</language>\n</editor>\n",
	language.HTML: "<!doctype html>\n<html lang=\"en\">\n<head>\n    <title>SourceView &amp; GOUI</title>\n    <style>body { color: #336699; }</style>\n</head>\n<body>\n    <!-- HTML has its own raw-text states. -->\n    <p class=\"intro\">Edit this document.</p>\n    <script>if (a < b) { console.log(\"<tag>\"); }</script>\n</body>\n</html>\n",
}

func main() {
	flag.Parse()
	app, err := gui.NewApplication("org.golang-gui.SourceViewExample")
	if err != nil {
		log.Fatal(err)
	}
	options := modern.Options{Dark: app.Settings().ColorScheme() == gui.ColorSchemeDark, AccentColor: app.Settings().AccentColor(), FontFamily: app.Settings().FontFamily(), FontSize: app.Settings().FontSize()}
	app.SetStyleSheet(modern.Sheet(options))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1050, Height: 720}})
	if err != nil {
		log.Fatal(err)
	}
	defer win.Destroy()
	_ = win.SetTitle("GOUI SourceView — JSON · XML · HTML")
	doc := source.NewDocument(samples[language.JSON])
	_ = doc.SetLanguage(language.JSON)
	if flag.NArg() > 0 {
		data, err := os.ReadFile(flag.Arg(0))
		if err != nil {
			log.Fatal(err)
		}
		doc.Model().SetText(string(data))
		_ = doc.SetLanguage(language.Guess(flag.Arg(0), data))
	}
	editor := source.NewSourceView()
	editor.SetID("editor")
	editor.SetDocument(doc)
	editor.SetAcceptsTab(true)
	scroll := gui.NewScrollView()
	scroll.SetChild(editor)
	viewport := gui.NewOverlay()
	viewport.SetChild(scroll)
	viewport.SetMainWeight(1)
	toolbar := gui.NewLinearBox(layout.DirectionHorizontal)
	toolbar.SetSpacing(6)
	status := gui.NewLabel("")
	update := func() {
		s := editor.Selection()
		status.SetText(fmt.Sprintf("%s   %d bytes   anchor %d / caret %d", doc.Language(), doc.Model().Len(), s.Anchor, s.Caret))
	}
	button := func(text string, fn func()) {
		b := gui.NewButton()
		b.SetChild(gui.NewLabel(text))
		b.ConnectClicked(fn)
		toolbar.AddChild(b)
	}
	for _, id := range []language.ID{language.JSON, language.XML, language.HTML} {
		id := id
		button(string(id), func() { _ = doc.SetLanguage(id); doc.Model().SetText(samples[id]); update() })
	}
	button("Undo", func() { editor.Undo() })
	button("Redo", func() { editor.Redo() })
	button("Read only", func() { editor.SetReadOnly(!editor.ReadOnly()); update() })
	button("Wrap", func() {
		w := source.WrapWordChar
		if editor.WrapMode() != source.WrapNone {
			w = source.WrapNone
		}
		editor.SetWrapMode(w)
	})
	button("Light / dark", func() {
		options.Dark = !options.Dark
		app.SetStyleSheet(modern.Sheet(options))
	})
	button("Line numbers", func() { editor.SetShowLineNumbers(!editor.ShowLineNumbers()) })
	searchbar := source.NewFindBar()
	searchbar.SetTarget(editor)
	viewport.AddOverlay(searchbar)
	viewport.ConnectOverlayPosition(searchbar, func(available, size geometry.Size, p *geometry.Point) {
		p.X = max(0, available.Width-size.Width-8)
		p.Y = 0
	})
	button("Find / Replace", func() {
		if err := searchbar.Open(true); err != nil {
			status.SetText(err.Error())
		}
	})
	editor.ConnectSelection(func(_ gui.TextSelection) { update() })
	editor.ConnectChange(func(_ gui.TextChange) { update() })
	editor.ConnectError(func(err error) { status.SetText(err.Error()) })
	content := gui.NewLinearBox(layout.DirectionVertical)
	content.SetCrossAlign(layout.CrossStretch)
	content.SetSpacing(6)
	content.AddChild(toolbar)
	content.AddChild(gui.NewLabel("Ctrl/Cmd+F: find   F3 / Shift+F3: next / previous   Ctrl+Space: complete"))
	content.AddChild(viewport)
	content.AddChild(status)
	win.SetWidget(content)
	update()
	if err := win.Show(); err != nil {
		log.Fatal(err)
	}
	win.SetFocusedWidget(editor)
	app.Run()
}
