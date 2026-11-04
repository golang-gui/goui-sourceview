// Run: go run ./example/semantic [-dark]. Requires a desktop and native fonts;
// Linux Integrated chrome needs a compositor. Edit the in-memory Go sample:
// change "hello" to 42, add declarations, type person. or press F2 for checked
// scope/member completion. Semantic colors arrive after external analysis.
// Slow analysis makes stale-result handling and the current API's popup-close
// limitation visible. Imports and project loading are deliberately unsupported.
// No source is executed and no file, clipboard or system setting is modified.
// Reset restores the sample; close the window to cancel analysis and exit.
package main

import (
	"flag"
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/example/semantic/analysis"
	"github.com/golang-gui/goui-sourceview/language"
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
	app, err := gui.NewApplication("org.golanggui.SourceViewSemantic")
	must(err)
	theme := func() {
		app.SetStyleSheet(style.Sheet(append(modern.Rules(modern.Options{Dark: *dark}), analysis.Rules(*dark)...)...))
	}
	theme()
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1100, Height: 780}, Chrome: gui.WindowChromeIntegrated, Center: true})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("External Go analysis · SourceView API exercise"))
	doc := source.NewDocument(analysis.Sample)
	must(doc.SetLanguage(language.Go))
	editor := source.NewSourceView()
	editor.SetDocument(doc)
	editor.SetAcceptsTab(true)
	keys := editor.KeyBindings()
	keys[source.CommandComplete] = []gui.KeyGesture{{Key: gui.KeyF2}}
	must(editor.SetKeyBindings(keys))
	status := gui.NewLabel("Waiting for external analysis…")
	status.SetStyleName(modern.MutedText)
	diagnostics := gui.NewLabel("")
	diagnostics.SetStyleName(modern.MutedText)
	alive := true
	service := analysis.NewService(app.Post, func(result analysis.Result, err error) {
		if !alive || doc.Model().Len() > analysis.MaxBytes {
			return
		}
		if err != nil {
			status.SetText(err.Error())
			return
		}
		if result.Snapshot.DocumentID != doc.ID() || result.Snapshot.Revision != doc.Model().Revision() {
			return
		}
		definition, err := analysis.Definition(result)
		if err != nil {
			status.SetText(err.Error())
			return
		}
		doc.SetLanguageDefinition(definition)
		status.SetText(fmt.Sprintf("Semantic revision %d · %d identifiers · %d diagnostics", result.Snapshot.Revision, len(result.Spans), len(result.Diagnostics)))
		message := "No syntax/type errors · single file, no imports"
		if len(result.Diagnostics) > 0 {
			message = strings.Join(result.Diagnostics[:min(3, len(result.Diagnostics))], "  |  ")
		}
		diagnostics.SetText(message)
	})
	defer service.Close()
	editor.SetCompletionProvider(service.Provider())
	slow := false
	submit := func() {
		m := doc.Model()
		text := ""
		if m.Len() <= analysis.MaxBytes {
			text = m.Text()
			status.SetText(fmt.Sprintf("Checking revision %d…", m.Revision()))
		} else {
			status.SetText("Analysis paused: example limit is 256 KiB")
		}
		delay := 150 * time.Millisecond
		if slow {
			delay = 1200 * time.Millisecond
		}
		service.Submit(analysis.Snapshot{DocumentID: doc.ID(), Revision: m.Revision(), Text: text}, delay)
	}
	changes := editor.ConnectChange(func(textedit.Change) { submit() })
	defer changes.Disconnect()
	editor.ConnectError(func(err error) {
		if alive {
			status.SetText(err.Error())
		}
	})
	destroy := win.ConnectDestroy(func() { alive = false; changes.Disconnect(); service.Close() })
	defer destroy.Disconnect()
	button := func(label string, fn func()) *gui.Button {
		b := gui.NewButton()
		b.SetChild(gui.NewLabel(label))
		b.ConnectClicked(func() { win.SetFocusedWidget(editor); fn() })
		return b
	}
	tools := gui.NewLinearBox(layout.DirectionHorizontal)
	tools.SetSpacing(8)
	tools.SetPadding(geometry.UniformInsets(8))
	tools.AddChild(button("Complete (F2)", func() {
		if err := editor.ShowCompletion(); err != nil {
			status.SetText(err.Error())
		}
	}))
	tools.AddChild(button("Slow analysis", func() { slow = !slow; submit() }))
	tools.AddChild(button("Reset", func() { doc.Model().SetText(analysis.Sample); submit() }))
	tools.AddChild(button("Light / Dark", func() { *dark = !*dark; theme() }))
	header := gui.NewHeaderBar()
	header.SetChild(gui.NewLabel("Go editor · external parser and type checker"))
	hint := gui.NewLabel("String variables: pink · numeric variables: blue · F2 / person. for semantic completion")
	limitation := gui.NewLabel("API limitation: publishing new semantic colors currently closes completion.")
	limitation.SetStyleName(modern.MutedText)
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(editor)
	scroll.SetMainWeight(1)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	for _, child := range []gui.Widget{header, tools, hint, limitation, scroll, status, diagnostics} {
		box.AddChild(child)
	}
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(editor)
	submit()
	app.Run()
	service.Close()
	service.Wait()
}
