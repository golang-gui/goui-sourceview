// Run: go run ./example/markdown [-dark]. Requires native typography and a
// desktop (Linux Integrated chrome needs a compositor). This example uses only
// public SourceView and GOUI APIs. It edits an in-memory Markdown sample; no
// files or clipboard are changed automatically. Try Enter on a list, backticks,
// [ and @ completion, Ctrl+Space templates, bracket matching, Bold, comments,
// Find/Replace and Undo. The syntax is a small illustrative subset. Restart to
// reset; close the Integrated window to exit.
package main

import (
	"flag"
	"fmt"
	"log"
	"runtime"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/example/markdown/markdown"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/modern"
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
func main() {
	runtime.LockOSThread()
	dark := flag.Bool("dark", false, "use dark appearance")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewMarkdown")
	must(err)
	applyTheme := func() {
		app.SetStyleSheet(style.Sheet(append(modern.Rules(modern.Options{Dark: *dark}), markdown.Rules(*dark)...)...))
	}
	applyTheme()
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1040, Height: 760}, Chrome: gui.WindowChromeIntegrated, Center: true})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("Markdown · GOUI SourceView"))
	chrome := win.Chrome().ConnectInfo(func(info gui.ChromeInfo) { log.Printf("window chrome: mode=%d enabled=%t", info.Mode, info.Enabled) })
	defer chrome.Disconnect()
	doc := source.NewDocument(markdown.Sample)
	definition, err := markdown.NewLanguage()
	must(err)
	doc.SetLanguageDefinition(definition)
	editor := source.NewSourceView()
	editor.SetDocument(doc)
	editor.SetAcceptsTab(true)
	editor.SetWrapMode(source.WrapWordChar)
	editor.SetCompletionProvider(markdown.Provider())
	scroll := gui.NewScrollView()
	scroll.SetChild(editor)
	viewport := gui.NewOverlay()
	viewport.SetChild(scroll)
	viewport.SetMainWeight(1)
	find := source.NewFindBar()
	find.SetTarget(editor)
	viewport.AddOverlay(find)
	viewport.ConnectOverlayPosition(find, func(available, size geometry.Size, p *geometry.Point) { p.X = max(0, available.Width-size.Width-8) })
	status := gui.NewLabel("")
	status.SetStyleName(modern.MutedText)
	update := func() {
		m := doc.Model()
		status.SetText(fmt.Sprintf("Markdown · Ln %d · %d lines · %d bytes", m.LineAt(editor.Selection().Caret)+1, m.LineCount(), m.Len()))
	}
	editor.ConnectSelection(func(textedit.Selection) { update() })
	editor.ConnectChange(func(textedit.Change) { update() })
	report := func(err error) {
		if err != nil {
			status.SetText(err.Error())
		}
	}
	editor.ConnectError(report)
	find.ConnectError(report)
	button := func(label string, action func()) *gui.Button {
		b := gui.NewButton()
		b.SetChild(gui.NewLabel(label))
		b.ConnectClicked(func() { win.SetFocusedWidget(editor); action() })
		return b
	}
	tools := gui.NewLinearBox(layout.DirectionHorizontal)
	tools.SetPadding(8)
	tools.SetSpacing(8)
	tools.SetCrossAlign(layout.CrossCenter)
	tools.AddChild(button("Undo", func() { editor.Undo() }))
	tools.AddChild(button("Redo", func() { editor.Redo() }))
	tools.AddChild(button("Bold", func() {
		s := editor.Selection()
		r := s.Range()
		text, err := doc.Model().Slice(r)
		if err != nil {
			report(err)
			return
		}
		report(editor.ApplyEdits(doc, doc.Model().Revision(), []textedit.Edit{{Range: r, Text: "**" + text + "**"}}, textedit.Selection{Anchor: s.Anchor + 2, Caret: s.Caret + 2}))
	}))
	tools.AddChild(button("Comment", func() { report(editor.ToggleBlockComment()) }))
	tools.AddChild(button("Complete", func() { report(editor.ShowCompletion()) }))
	tools.AddChild(button("Find / Replace", func() { report(find.Open(true)) }))
	tools.AddChild(button("Light / Dark", func() { *dark = !*dark; applyTheme() }))
	header := gui.NewHeaderBar()
	title := gui.NewLabel("Markdown  ·  Custom language, editing rules and completion")
	header.SetPadding(12)
	header.SetChild(title)
	content := gui.NewLinearBox(layout.DirectionVertical)
	content.SetCrossAlign(layout.CrossStretch)
	for _, child := range []gui.Widget{header, tools, viewport, status} {
		content.AddChild(child)
	}
	win.SetWidget(content)
	update()
	must(win.Show())
	win.SetFocusedWidget(editor)
	app.Run()
}
