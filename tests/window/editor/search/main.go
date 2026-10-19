// Native search enhancements verification; desktop/window manager required.
// Run: XMODIFIERS=@im=none go run ./tests/window/editor/search [-dark].
// Through normal key/pointer/CommitText paths: preserve pasted multiline text,
// Alt+Enter inserts a query newline, toggle regex and selection scope, replace
// captures once and undo, retain invalid syntax with inline feedback, navigate
// zero-width matches. All assertions concern text, selection, count and enabled
// state. Also checks disabled empty-selection scope, immediate results/counts
// on toggling literal/regex scopes, closing clears scope/highlights but keeps
// query/replacement/F3, and scope navigation yields to reentrant callbacks.
// No files/clipboard/settings changes; closing early fails, restart
// resets; exits after PASS. Physical IME and real DPI transitions are separate.
package main

import (
	"flag"
	"log"
	"runtime"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/theme/modern"
	"github.com/golang-gui/goui/widgets"
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

	count := func(want int, complete bool) {
		state := bar.Snapshot().Attributes["sourceview.find"].(map[string]any)
		check(state["count"] == want && state["countComplete"] == complete, "scope result count was stale")
	}
	checked := func() bool {
		return named(bar, "Find in selection").Snapshot().Attributes[widgets.CheckInfoKey].(widgets.CheckInfo).State == widgets.CheckChecked
	}
	for _, regex := range []bool{false, true} {
		steps = append(steps, func() {
			bar.Close()
			doc.Model().SetText("foo bar foo bar foo")
			v.SetSelection(textedit.Selection{})
			v.SetSearch(source.SearchQuery{Text: "foo", Regex: regex})
			win.SetFocusedWidget(v)
			key(events.KeyF, primary)
			check(!gui.IsEnabled(named(bar, "Find in selection")), "empty-selection scope enabled")
			click("Find in selection")
			_, limited := v.SearchRange()
			check(!limited && !checked(), "disabled scope button changed search range")
		})
		steps = append(steps, func() {
			v.SetSelection(textedit.Selection{Anchor: 4, Caret: 19})
			check(gui.IsEnabled(named(bar, "Find in selection")), "selection did not enable scope synchronously")
			v.SetReadOnly(true)
			click("Find in selection")
			r, limited := v.SearchRange()
			check(limited && checked() && r == (textedit.Range{Start: 4, End: 19}), "captured scope lost")
			check(v.Selection().Range() == (textedit.Range{Start: 8, End: 11}), "scope activation did not select its first match")
			count(2, true)
			v.SetReadOnly(false)
		})
		steps = append(steps, func() {
			v.SetSelection(textedit.Selection{})
			r, limited := v.SearchRange()
			check(limited && r == (textedit.Range{Start: 4, End: 19}), "result selection replaced captured scope")
			check(gui.IsEnabled(named(bar, "Find in selection")), "active scope cannot be turned off without a selection")
			click("Find in selection")
			_, limited = v.SearchRange()
			check(!limited && !checked() && v.Selection().Range() == (textedit.Range{Start: 0, End: 3}), "scope deactivation did not update result")
			count(3, true)
		})
		steps = append(steps, func() {
			v.SetSelection(textedit.Selection{Anchor: 4, Caret: 7})
			click("Find in selection")
			check(v.Selection().Range() == (textedit.Range{Start: 4, End: 7}), "empty result replaced user selection")
			count(0, true)
			click("Find in selection")
			check(v.Selection().Range() == (textedit.Range{Start: 8, End: 11}), "removing empty scope kept stale result")
			count(3, true)
		})
		steps = append(steps, func() {
			click("Find in selection")
			count(1, true)
			selection, query, replacement := v.Selection(), v.Search(), bar.Replacement()
			win.SetFocusedWidget(named(bar, "Find"))
			key(events.KeyEscape, 0)
			_, limited := v.SearchRange()
			check(!bar.Opened() && !limited && !v.HighlightSearchMatches(), "close left search scope/decorations")
			check(v.Focused() && v.Selection() == selection && v.Search() == query && bar.Replacement() == replacement, "close lost focus/selection/query/replacement")
			key(events.KeyF3, 0)
			check(v.Selection().Range() == (textedit.Range{Start: 16, End: 19}) && !v.HighlightSearchMatches(), "closed F3 search restored decorations or kept scope")
			v.SetSelection(textedit.Selection{})
			key(events.KeyF, primary)
			check(bar.Opened() && v.HighlightSearchMatches() && v.Search() == query && !checked(), "reopening failed to restore whole-document search")
		})
	}
	steps = append(steps, func() {
		// Keep this a chunked-count scenario. A single enormous paragraph with
		// whole-word/whitespace decorations exercises a separate rendering cost.
		doc.Model().SetText(strings.Repeat("foo\n", 20000))
		v.SetSearch(source.SearchQuery{Text: "foo"})
		v.SetSelection(textedit.Selection{Anchor: 40000, Caret: 40007})
		click("Find in selection")
		count(2, false)
	})
	steps = append(steps, func() {
		count(2, true)
		bar.Close()
		doc.Model().SetText("foo bar foo bar foo")
		v.SetSelection(textedit.Selection{})
		must(bar.Open(false))
		v.SetSelection(textedit.Selection{Anchor: 4, Caret: 19})
		replacement := source.NewDocument("keep")
		h := v.ConnectSelection(func(s textedit.Selection) {
			if s.Range() == (textedit.Range{Start: 8, End: 11}) {
				v.SetDocument(replacement)
				bar.Close()
			}
		})
		click("Find in selection")
		h.Disconnect()
		check(v.Document() == replacement && v.Selection() == (textedit.Selection{}) && !bar.Opened() && !v.HighlightSearchMatches(), "scope navigation overwrote reentrant document/close")
	})
	steps = append(steps, func() {
		must(bar.Open(false))
		foreign := source.NewSourceView()
		foreign.Document().Model().SetText("foo")
		foreign.SetSearch(source.SearchQuery{Text: "foo"})
		scope := textedit.Range{Start: 0, End: 3}
		must(foreign.SetSearchRange(&scope))
		bar.SetTarget(foreign) // Not mounted: this bar has never managed its search.
		bar.Close()
		actual, limited := foreign.SearchRange()
		check(foreign.HighlightSearchMatches() && limited && actual == scope, "bar cleared an unattached target's independent search")
		bar.SetTarget(v)
	})

	index := 0
	complete := false
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		if index < len(steps) {
			log.Printf("search scenario %d/%d", index+1, len(steps))
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
