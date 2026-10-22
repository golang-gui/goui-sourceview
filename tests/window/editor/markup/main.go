// Native markup verification; desktop/window manager required. Run:
// XMODIFIERS=@im=none go run ./tests/window/editor/markup [-dark].
// Automatic normal Window.CommitText/key steps create and rename XML/HTML
// tags, undo the linked edit, continue Markdown lists, then enter JSON inside a
// fence and check child-language pair insertion and completion identity.
// Starts in empty XML; no files/clipboard/settings writes. PASS closes window;
// closing early fails, restart resets. Physical IME, RTL and DPI transitions
// require separate observation. Record theme/backend/scale for each run.
package main

import (
	"flag"
	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/theme/modern"
	"log"
	"runtime"
	"time"
)

func must(e error) {
	if e != nil {
		log.Fatal(e)
	}
}
func check(ok bool, message string) {
	if !ok {
		log.Fatal(message)
	}
}
func main() {
	runtime.LockOSThread()
	dark := flag.Bool("dark", false, "Dark Modern")
	flag.Parse()
	app, e := gui.NewApplication("org.golanggui.SourceViewMarkup")
	must(e)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	win, e := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 750, Height: 460}, Chrome: gui.WindowChromeIntegrated})
	must(e)
	defer win.Destroy()
	v := source.NewSourceView()
	doc := source.NewDocument("    word  \n中e\u0301x\n\t  value")
	v.SetDocument(doc)
	v.SetAutomaticCompletion(false)
	must(v.SetWhitespaceMode(source.WhitespaceAll))
	v.SetShowLineEndings(true)
	must(v.SetRulers([]int{40, 80}))
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(v)
	overlay := gui.NewOverlay()
	overlay.SetChild(scroll)
	bar := source.NewGoToBar()
	bar.SetTarget(v)
	overlay.AddOverlay(bar)
	overlay.ConnectOverlayPosition(bar, func(a, s geometry.Size, p *geometry.Point) { p.X = max(0, a.Width-s.Width-8) })
	win.SetWidget(overlay)
	must(win.Show())
	win.SetFocusedWidget(v)
	primary, _ := gui.ModPrimary.Resolve()
	key := func(k events.Key, m events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: m}))
		}
	}

	var steps []func()
	steps = append(steps, func() {
		must(doc.SetLanguage(language.XML))
		doc.Model().SetText("")
		v.SetSelection(textedit.Selection{})
		must(win.CommitText("<"))
		must(win.CommitText("node"))
		must(win.CommitText(">"))
		check(doc.Model().Text() == "<node></node>" && v.Selection().Caret == 6, "tag close")
	})
	steps = append(steps, func() {
		v.SetSelection(textedit.Selection{Anchor: 5, Caret: 5})
		must(win.CommitText("x"))
		check(doc.Model().Text() == "<nodex></nodex>", "linked typing")
		key(events.KeyBackspace, 0)
		check(doc.Model().Text() == "<node></node>", "linked Backspace")
		key(events.KeyZ, primary)
		check(doc.Model().Text() == "<nodex></nodex>", "linked Undo")
	})
	steps = append(steps, func() {
		must(doc.SetLanguage(language.HTML))
		doc.Model().SetText("<BR")
		v.SetSelection(textedit.Selection{Anchor: 3, Caret: 3})
		must(win.CommitText(">"))
		check(doc.Model().Text() == "<BR>", "HTML void closed")
		must(doc.SetLanguage(language.Markdown))
		doc.Model().SetText("- item")
		v.SetSelection(textedit.Selection{Anchor: 6, Caret: 6})
	})
	steps = append(steps, func() {
		key(events.KeyEnter, 0)
		check(doc.Model().Text() == "- item\n- ", "Markdown list continuation")
		doc.Model().SetText("```json\n\n```")
		v.SetSelection(textedit.Selection{Anchor: 8, Caret: 8})
	})
	steps = append(steps, func() {
		must(win.CommitText("{"))
		check(doc.Model().Text() == "```json\n{}\n```", "embedded JSON pair")
		key(events.KeyEnter, 0)
		check(doc.Model().Text() == "```json\n{\n    \n}\n```", "embedded JSON newline")
	})

	index := 0
	complete := false
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		if index < len(steps) {
			steps[index]()
			index++
		} else {
			complete = true
			log.Printf("PASS native markup and embedded editing; dark=%t", *dark)
			win.Destroy()
		}
	})
	must(timer.Start(200 * time.Millisecond))
	app.Run()
	check(complete, "closed early")
}
