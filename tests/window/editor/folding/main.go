// Folding integration verification.
// Environment: native desktop and window manager; record OS, typography,
// GOUI_PLAT_PAINTER and GOUI_PLAT_SCALE. Linux: XMODIFIERS=@im=none.
// Run: go run ./tests/window/editor/folding [-dark]. Starts with two views of
// one nested JSON document; the lower view is read-only. Automatic steps:
//  1. Wait for background folding, fold the array by keyboard; model/history and
//     the other view remain unchanged. Hidden caret moves to its header.
//  2. Delete at the boundary first unfolds, then deletes normally; undo restores.
//  3. Hide line numbers and click the semantic fold button through normal window
//     pointer input; only that view toggles. Inspect arrow, quiet dots and gutter.
//  4. Fold the outer object, navigate to a hidden search match with F3, verify
//     ancestors expand and the exact source bytes are selected.
//  5. Switch to HTML with quoted >, void/raw-text elements, and recheck folding.
//
// No files, clipboard or settings change. Closing early fails; restart resets.
// Exits automatically on PASS. Physical IME, real DPI changes and visual quality
// need separate desktop observation; this program does not claim those passed.
package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"runtime"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/theme/modern"
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
	dark := flag.Bool("dark", false, "Modern dark appearance")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewFolding")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 800, Height: 640}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView folding verification"))
	original := "{\n  \"items\": [\n    1,\n    2\n  ]\n}\ntail"
	doc := source.NewDocument(original)
	must(doc.SetLanguage(language.JSON))
	v, other := source.NewSourceView(), source.NewSourceView()
	v.SetDocument(doc)
	other.SetDocument(doc)
	other.SetReadOnly(true)
	v.SetID("editor")
	other.SetID("mirror")
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	probe := &scaleProbe{}
	box.AddChild(probe)
	box.AddChild(gui.NewLabel("Folding · JSON/XML/HTML · shared document, independent views"))
	for _, editor := range []*source.SourceView{v, other} {
		scroll := gui.NewScrollView()
		scroll.SetChild(editor)
		scroll.SetMainWeight(1)
		box.AddChild(scroll)
	}
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(v)
	v.ConnectError(func(err error) { log.Fatal(err) })
	primary, _ := gui.ModPrimary.Resolve()
	foldMods, _ := (gui.ModPrimary | gui.ModShift).Resolve()
	if runtime.GOOS == "darwin" {
		foldMods, _ = (gui.ModPrimary | gui.ModAlt).Resolve()
	}
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	caret := func(line int, end bool) {
		r, err := doc.Model().LineRange(line)
		must(err)
		at := r.Start
		if end {
			at = r.End
		}
		v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
	}
	var steps []func() bool
	next := func(f func()) { steps = append(steps, func() bool { f(); return true }) }
	steps = append(steps, func() bool { return len(v.FoldingRegions()) == 2 && len(other.FoldingRegions()) == 2 })
	next(func() {
		caret(3, false)
		key(events.KeyBracketLeft, foldMods)
		check(v.FoldingRegions()[1].Collapsed && !other.FoldingRegions()[1].Collapsed, "keyboard folding/view isolation")
		check(doc.Model().Text() == original && !doc.Model().CanUndo(), "fold changed model/history")
	})
	next(func() {
		caret(1, true)
		key(events.KeyDelete, 0)
		check(!v.FoldingRegions()[1].Collapsed && doc.Model().Text() == original, "boundary delete changed hidden text")
		key(events.KeyDelete, 0)
		check(doc.Model().Text() != original, "second Delete did not edit")
		key(events.KeyZ, primary)
		check(doc.Model().Text() == original, "undo did not restore")
	})
	steps = append(steps, func() bool { return len(v.FoldingRegions()) == 2 })
	next(func() { v.SetShowLineNumbers(false); caret(1, true) })
	next(func() {
		var target *gui.WidgetInfo
		info := v.Snapshot()
		for i := range info.Children {
			child := &info.Children[i]
			if data, ok := child.Attributes["goui.sourceview.fold"].(map[string]any); ok && data["startLine"] == 1 {
				target = child
				break
			}
		}
		check(target != nil, "fold button disappeared with line numbers")
		point := geometry.Point{X: target.Bounds.X + target.Bounds.Width/2, Y: target.Bounds.Y + target.Bounds.Height/2}
		for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
			must(win.DispatchEvent(events.PointerEvent{EventType: kind, Position: point, Button: events.PointerButtonLeft}))
		}
		check(v.FoldingRegions()[1].Collapsed && !other.FoldingRegions()[1].Collapsed, "pointer fold did not toggle exact view")
	})
	next(func() {
		caret(0, true)
		key(events.KeyBracketLeft, foldMods)
		check(v.FoldingRegions()[0].Collapsed, "outer fold")
		v.SetSearch(source.SearchQuery{Text: "2"})
		key(events.KeyF3, 0)
		check(!v.FoldingRegions()[0].Collapsed && !v.FoldingRegions()[1].Collapsed, "search target remained hidden")
		text, err := doc.Model().Slice(v.Selection().Range())
		must(err)
		check(text == "2", "search lost source coordinates")
	})
	next(func() {
		doc.Model().SetText("<DIV title=\">\">\n<br>\n<script>\nconst x = '<fake>';\n</script>\n</div>")
		must(doc.SetLanguage(language.HTML))
	})
	steps = append(steps, func() bool { return len(v.FoldingRegions()) == 2 })
	next(func() {
		caret(0, true)
		key(events.KeyBracketLeft, foldMods)
		info := v.Snapshot()
		check(info.TextEditing != nil && strings.HasPrefix(info.TextEditing.VisibleText, "<DIV"), "fold snapshot missing header")
		log.Printf("PASS folding keyboard/pointer/hidden deletion/search/history/shared views; typography=%s painter=%s scale=%s dark=%t", app.Typography().Name(), probe.painter, fmt.Sprint(probe.scale), *dark)
	})
	index := 0
	deadline := time.Now().Add(15 * time.Second)
	complete := false
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		if time.Now().After(deadline) {
			log.Fatalf("folding step %d timed out; regions=%v", index, v.FoldingRegions())
		}
		if index < len(steps) {
			if steps[index]() {
				index++
				deadline = time.Now().Add(8 * time.Second)
			}
		} else {
			complete = true
			win.Destroy()
		}
	})
	must(timer.Start(100 * time.Millisecond))
	app.Run()
	check(complete, "folding verification closed early")
}

type scaleProbe struct {
	gui.WidgetBase
	painter string
	scale   float32
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
