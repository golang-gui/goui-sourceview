// Native search enhancements verification; desktop/window manager required.
// Run: XMODIFIERS=@im=none go run ./tests/window/editor/search [-dark].
// Through normal key/pointer/CommitText paths: preserve pasted multiline text,
// Alt+Enter inserts a query newline, toggle regex and selection scope, replace
// captures once and undo, retain invalid syntax with inline feedback, navigate
// zero-width matches. All assertions concern text, selection, count and enabled
// state. No files/clipboard/settings changes; closing early fails, restart
// resets; exits after PASS. Physical IME and real DPI transitions are separate.
package main

import (
	"flag"
	source "github.com/golang-gui/goui-sourceview"
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
	app, e := gui.NewApplication("org.golanggui.SourceViewSearch")
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
	scroll.SetChild(v)
	overlay := gui.NewOverlay()
	overlay.SetChild(scroll)
	bar := source.NewFindBar()
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

	var named func(gui.Widget, string) gui.Widget
	named = func(w gui.Widget, name string) gui.Widget {
		if w.Name() == name {
			return w
		}
		for _, child := range w.Children() {
			if found := named(child, name); found != nil {
				return found
			}
		}
		return nil
	}
	click := func(name string) {
		w := named(bar, name)
		check(w != nil, "missing control "+name)
		r := w.Snapshot().Bounds
		p := geometry.Point{X: r.X + r.Width/2, Y: r.Y + r.Height/2}
		for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
			must(win.DispatchEvent(events.PointerEvent{EventType: kind, Position: p, Button: events.PointerButtonLeft}))
		}
	}
	var steps []func()
	steps = append(steps, func() {
		doc.Model().SetText("a=1\na=2\na=3")
		v.SetSelection(textedit.Selection{})
		key(events.KeyF, primary)
		must(win.CommitText("a=1\na=2"))
		check(v.Search().Text == "a=1\na=2", "multiline query flattened")
		key(events.KeyEnter, 0)
		check(v.Selection().Range() == (textedit.Range{Start: 0, End: 7}), "multiline result")
	})
	steps = append(steps, func() {
		key(events.KeyA, primary)
		must(win.CommitText("a="))
		key(events.KeyEnter, events.ModifierAlt)
		must(win.CommitText("中"))
		check(v.Search().Text == "a=\n中", "Alt Enter newline")
		bar.Close()
		v.SetSelection(textedit.Selection{Anchor: 4, Caret: 7})
		must(bar.Open(true))
	})
	steps = append(steps, func() {
		click("Find in selection")
		click("Regular expression")
		win.SetFocusedWidget(named(bar, "Find"))
		key(events.KeyA, primary)
		must(win.CommitText(`a=(\d)`))
		key(events.KeyEnter, 0)
		check(v.Selection().Range() == (textedit.Range{Start: 4, End: 7}), "scoped regex result")
		r, ok := v.SearchRange()
		check(ok && r == (textedit.Range{Start: 4, End: 7}), "scope changed with result selection")
		win.SetFocusedWidget(named(bar, "Replacement"))
		must(win.CommitText("<$1>"))
	})
	steps = append(steps, func() {
		state := bar.Snapshot().Attributes["sourceview.find"].(map[string]any)
		check(state["countComplete"] == true && state["count"] == 1, "canonical scoped count")
		click("Replace all")
		check(doc.Model().Text() == "a=1\n<2>\na=3", "capture replacement")
		win.SetFocusedWidget(v)
		key(events.KeyZ, primary)
		check(doc.Model().Text() == "a=1\na=2\na=3", "atomic replace Undo")
		must(v.SetSearchRange(nil))
		v.SetSearch(source.SearchQuery{Text: "[", Regex: true})
	})
	steps = append(steps, func() {
		check(named(bar, "Find").Rect().Width >= 90, "syntax error collapsed input")
		check(v.SearchError() != nil && !gui.IsEnabled(named(bar, "Replace all")), "invalid regex not disabled")
		check(v.Search().Text == "[", "invalid query discarded")
		v.SetSearch(source.SearchQuery{Text: `(?m)^`, Regex: true})
		v.SetSelection(textedit.Selection{})
		win.SetFocusedWidget(v)
		key(events.KeyF3, 0)
		check(v.Selection().Caret == 0, "first zero match")
		key(events.KeyF3, 0)
		check(v.Selection().Caret == 4, "zero match did not progress")
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
			log.Printf("PASS native multiline/regex/scope/capture search; dark=%t", *dark)
			win.Destroy()
		}
	})
	must(timer.Start(200 * time.Millisecond))
	app.Run()
	check(complete, "closed early")
}
