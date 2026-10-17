// Editor shortcut verification. Requires a desktop/window manager, native
// typography and fonts. Run: go run ./tests/window/editor/shortcuts (Linux without
// an IME service: XMODIFIERS=@im=none). Two editors initially share plain text.
// Automatically checks insert above/below, indentation, repeated line selection,
// code-word navigation/deletion, bracket jump, find navigation, undo/redo,
// read-only and extra-modifier propagation. macOS uses Option for words and
// Command+arrows for line/document edges. All keys use Window.DispatchEvent;
// text uses CommitText. Physical system/IME interception still needs manual
// checks. No clipboard/files/settings change. Records typography, actual painter
// and scale; closes on success, exits nonzero on failed assertion/early close.
package main

import (
	"fmt"
	"image"
	"log"
	"runtime"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
func check(ok bool, message string) {
	if !ok {
		log.Fatal(message)
	}
}
func main() {
	runtime.LockOSThread()
	app, err := gui.NewApplication("org.golang-gui.SourceViewShortcuts")
	must(err)
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 720, Height: 420}})
	must(err)
	defer win.Destroy()
	a, b := source.NewSourceView(), source.NewSourceView()
	doc := source.NewDocument("  foo.bar\nlast")
	a.SetDocument(doc)
	b.SetDocument(doc)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	for _, v := range []*source.SourceView{a, b} {
		scroll := gui.NewScrollView()
		scroll.SetChild(v)
		scroll.SetMainWeight(1)
		box.AddChild(scroll)
	}
	probe := &scaleProbe{}
	box.AddChild(probe)
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(a)
	primary, _ := gui.ModPrimary.Resolve()
	word := primary
	if runtime.GOOS == "darwin" {
		word, _ = gui.ModAlt.Resolve()
	}
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	caret := func(at int) { a.SetSelection(textedit.Selection{Anchor: at, Caret: at}) }
	fallback := 0
	for _, g := range []gui.KeyGesture{{Key: gui.KeyF, Modifiers: gui.ModPrimary}, {Key: gui.KeyA, Modifiers: gui.ModPrimary | gui.ModShift}, {Key: gui.KeyEnter, Modifiers: gui.ModPrimary}} {
		s := gui.NewShortcut(g)
		s.ConnectActivate(func() { fallback++ })
		win.Shortcuts().AddShortcut(s)
	}
	complete := false
	steps := []func(){
		func() {
			check(a.Focused() && probe.scale > 0, "window not ready")
			caret(5)
			b.SetSelection(textedit.Selection{Anchor: 12, Caret: 12})
			key(events.KeyEnter, primary)
			check(doc.Model().Text() == "  foo.bar\n  \nlast" && a.Selection().Caret == 12 && b.Selection().Caret == 15, "insert below or shared mapping")
			key(events.KeyZ, primary)
			check(a.Selection().Caret == 5 && doc.Model().Text() == "  foo.bar\nlast", "insert undo")
			key(events.KeyEnter, primary|events.ModifierShift)
			check(doc.Model().Text() == "  \n  foo.bar\nlast" && a.Selection().Caret == 2, "insert above")
			key(events.KeyZ, primary)
			a.SetSelection(textedit.Selection{Anchor: 9, Caret: 2})
			key(events.KeyBracketRight, primary)
			check(doc.Model().Text() == "      foo.bar\nlast" && a.Selection() == (textedit.Selection{Anchor: 13, Caret: 6}), "indent selection direction")
			key(events.KeyBracketLeft, primary)
			check(doc.Model().Text() == "  foo.bar\nlast" && a.Selection() == (textedit.Selection{Anchor: 9, Caret: 2}), "outdent")
			caret(4)
			key(events.KeyL, primary)
			check(a.Selection() == (textedit.Selection{Caret: 10}), "select line")
			key(events.KeyL, primary)
			check(a.Selection() == (textedit.Selection{Caret: 14}), "expand line selection")
		},
		func() {
			caret(0)
			key(events.KeyArrowRight, word)
			check(a.Selection().Caret == 5, "word right")
			key(events.KeyArrowRight, word|events.ModifierShift)
			check(a.Selection() == (textedit.Selection{Anchor: 5, Caret: 9}), "fast word expansion")
			key(events.KeyBackspace, word)
			check(doc.Model().Text() == "  foo\nlast", "delete selected word")
			key(events.KeyZ, primary)
			check(a.Selection() == (textedit.Selection{Anchor: 5, Caret: 9}), "delete undo selection")
			caret(6)
			key(events.KeyArrowLeft, word)
			check(a.Selection().Caret == 2, "fast word left")
			caret(5)
			key(events.KeyDelete, word)
			check(doc.Model().Text() == "  foobar\nlast", "word delete separator")
			key(events.KeyZ, primary)
			doc.Model().SetText("中 e\u0301")
			caret(0)
			key(events.KeyArrowRight, word)
			key(events.KeyArrowRight, word|events.ModifierShift)
			check(a.Selection() == (textedit.Selection{Anchor: 3, Caret: 7}), "Unicode word selection")
			key(events.KeyBackspace, word)
			check(doc.Model().Text() == "中", "Unicode word delete")
			key(events.KeyZ, primary)
			check(doc.Model().Text() == "中 e\u0301", "Unicode undo")
			doc.Model().SetText("a\n  beta")
			caret(1)
			key(events.KeyDelete, word)
			check(doc.Model().Text() == "abeta", "join and delete indentation")
		},
		func() {
			doc.Model().SetText("({[]})")
			caret(0)
			key(events.KeyBackslash, primary|events.ModifierShift)
			check(a.Selection().Caret == 5, "jump to bracket")
			key(events.KeyBackslash, primary|events.ModifierShift)
			check(a.Selection().Caret == 0, "jump back")
			doc.Model().SetText("(")
			caret(0)
			key(events.KeyBackslash, primary|events.ModifierShift)
			check(a.Selection().Caret == 0, "unmatched jump moved caret")
			doc.Model().SetText("one two one")
			a.SetSearch(source.SearchQuery{Text: "one"})
			caret(0)
			key(events.KeyF3, 0)
			check(a.Selection() == (textedit.Selection{Caret: 3}), "find first")
			key(events.KeyF3, 0)
			check(a.Selection() == (textedit.Selection{Anchor: 8, Caret: 11}), "find next")
			key(events.KeyF3, events.ModifierShift)
			check(a.Selection() == (textedit.Selection{Caret: 3}), "find previous")
			if runtime.GOOS == "darwin" {
				key(events.KeyG, primary)
				check(a.Selection().Caret == 11, "mac find next")
				key(events.KeyArrowLeft, primary)
				check(a.Selection().Caret == 0, "mac line start")
				key(events.KeyArrowDown, primary|events.ModifierShift)
				check(a.Selection() == (textedit.Selection{Caret: 11}), "mac document selection")
			}
		},
		func() {
			caret(2)
			before := a.Selection()
			key(events.KeyA, primary|events.ModifierShift)
			check(a.Selection() == before && fallback == 1, "unexpected Shift+A binding")
			key(events.KeyF, primary)
			check(fallback == 2, "bare editor intercepted application find")
			a.SetReadOnly(true)
			key(events.KeyEnter, primary)
			check(fallback == 2 && doc.Model().Text() == "one two one", "readonly key leaked or edited")
			a.SetReadOnly(false)
			doc.Model().SetText("a")
			caret(1)
			must(win.CommitText("X"))
			key(events.KeyEnter, primary)
			must(win.CommitText("Y"))
			key(events.KeyZ, primary)
			check(doc.Model().Text() == "aX\n", "typing merged after line insert")
			key(events.KeyZ, primary)
			check(doc.Model().Text() == "aX", "line insert merged before typing")
			key(events.KeyZ, primary)
			check(doc.Model().Text() == "a", "typing history missing")
			log.Printf("PASS editor shortcuts, Unicode, selection/history, shared views and key propagation; typography=%s painter=%s scale=%g", app.Typography().Name(), probe.painter, probe.scale)
			complete = true
			win.Destroy()
		},
	}
	index := 0
	timer := app.NewTimer()
	timer.ConnectTimeout(func() {
		if index < len(steps) {
			fn := steps[index]
			index++
			fn()
		}
	})
	must(timer.Start(350 * time.Millisecond))
	app.Run()
	check(complete, "shortcut verification aborted")
}

type scaleProbe struct {
	gui.WidgetBase
	scale   float32
	painter string
}

func (p *scaleProbe) Measure(c layout.Constraint) layout.Measurement {
	return layout.Measured(c.Clamp(geometry.Size{Width: 1, Height: 1}))
}
func (p *scaleProbe) Paint(draw gui.Painter) {
	if p.scale != 0 {
		return
	}
	p.scale = draw.PixelScale()
	img, err := draw.NewImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	must(err)
	defer img.Destroy()
	p.painter = fmt.Sprintf("%T", img)
}
