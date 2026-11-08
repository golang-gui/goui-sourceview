// Built-in code folding verification. Requires a native desktop/window manager.
// Run: XMODIFIERS=@im=none go run ./tests/window/editor/folding_languages
// [-basic] [-dark]. Record OS, typography, GOUI_PLAT_PAINTER and GOUI_PLAT_SCALE.
// Two views share each C/C++/Java/C#/Rust fixture; the mirror is read-only.
// Automatic checks wait for exact asynchronous ranges, use normal Window key
// and pointer dispatch to fold code/literals, check view isolation and snapshots,
// delete at a hidden boundary, undo, search/reveal, disable/re-enable folding,
// edit away a closing delimiter, and wait for the revised ranges. Each language
// starts a fresh Document, exercising cancellation/detachment of the prior one.
// All expected line ranges are fixed fixtures, independent of the provider.
// No user files, clipboard or settings change. Restart resets. PASS closes the
// window; timeout, early close or any failed assertion exits nonzero. Physical
// IME, mixed DPI and other native platforms need separate verification.
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
	"github.com/golang-gui/goui/theme/basic"
	"github.com/golang-gui/goui/theme/modern"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func check(ok bool, message string) {
	if !ok {
		panic(message)
	}
}

type fixture struct {
	id   language.ID
	text string
	want []language.FoldRange
}

func main() {
	runtime.LockOSThread()
	dark := flag.Bool("dark", false, "Modern dark")
	basicTheme := flag.Bool("basic", false, "Basic theme")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewFoldingLanguages")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	if *basicTheme {
		app.SetStyleSheet(basic.Sheet())
	}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 880, Height: 760}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView code folding verification"))
	v, mirror := source.NewSourceView(), source.NewSourceView()
	mirror.SetReadOnly(true)
	v.SetAutomaticCompletion(false)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	probe := &scaleProbe{}
	box.AddChild(probe)
	label := gui.NewLabel("C/C++/Java/C#/Rust · independent folds, shared document")
	box.AddChild(label)
	for _, editor := range []*source.SourceView{v, mirror} {
		editor.ConnectError(func(err error) { panic(err) })
		scroll := gui.NewScrollView()
		scroll.SetStyleName("source-view")
		scroll.SetChild(editor)
		scroll.SetMainWeight(1)
		box.AddChild(scroll)
	}
	win.SetWidget(box)
	must(win.Show())
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
	common := "void f() {\n if (x) {\n  hidden();\n }\n"
	fixtures := []fixture{
		{language.C, common + " /* note\n  } [\n */\n}\ntail", []language.FoldRange{{StartLine: 0, EndLine: 7}, {StartLine: 1, EndLine: 3}, {StartLine: 4, EndLine: 6}}},
		{language.Cpp, common + " auto s = R\"tag(\n  } [\n )tag\";\n}\ntail", []language.FoldRange{{StartLine: 0, EndLine: 7}, {StartLine: 1, EndLine: 3}, {StartLine: 4, EndLine: 6}}},
		{language.Java, common + " String s = \"\"\"\n  } [\n \"\"\";\n}\ntail", []language.FoldRange{{StartLine: 0, EndLine: 7}, {StartLine: 1, EndLine: 3}, {StartLine: 4, EndLine: 6}}},
		{language.CSharp, common + " var s = $$\"\"\"\n  } [\n \"\"\";\n}\ntail", []language.FoldRange{{StartLine: 0, EndLine: 7}, {StartLine: 1, EndLine: 3}, {StartLine: 4, EndLine: 6}}},
		{language.Rust, "fn f() {\n if x {\n  hidden();\n }\n /* outer\n  /* inner\n   } [\n  */\n */\n}\ntail", []language.FoldRange{{StartLine: 0, EndLine: 9}, {StartLine: 1, EndLine: 3}, {StartLine: 4, EndLine: 8}, {StartLine: 5, EndLine: 7}}},
	}
	var doc *source.Document
	caret := func(row int, end bool) {
		r, err := doc.Model().LineRange(row)
		must(err)
		at := r.Start
		if end {
			at = r.End
		}
		v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
	}
	matches := func(editor *source.SourceView, want []language.FoldRange) bool {
		got := editor.FoldingRegions()
		if len(got) != len(want) {
			return false
		}
		for i, r := range want {
			if got[i].StartLine != r.StartLine || got[i].EndLine != r.EndLine {
				return false
			}
		}
		return true
	}
	var steps []func() bool
	next := func(f func()) { steps = append(steps, func() bool { f(); return true }) }
	for _, sample := range fixtures {
		next(func() {
			doc = source.NewDocument(sample.text)
			must(doc.SetLanguage(sample.id))
			v.SetDocument(doc)
			mirror.SetDocument(doc)
			win.SetFocusedWidget(v)
			label.SetText("Folding · " + string(sample.id))
		})
		steps = append(steps, func() bool { return matches(v, sample.want) && matches(mirror, sample.want) })
		next(func() {
			caret(2, false)
			key(events.KeyBracketLeft, foldMods)
			check(v.FoldingRegions()[1].Collapsed && !mirror.FoldingRegions()[1].Collapsed, "keyboard fold/view isolation")
			check(doc.Model().Text() == sample.text && !doc.Model().CanUndo(), "fold changed source/history")
		})
		next(func() {
			info := v.Snapshot()
			check(info.TextEditing != nil && !strings.Contains(info.TextEditing.VisibleText, "hidden();"), "hidden body still in visible snapshot")
			caret(1, true)
			key(events.KeyDelete, 0)
			check(!v.FoldingRegions()[1].Collapsed && doc.Model().Text() == sample.text, "first Delete removed hidden text")
			key(events.KeyDelete, 0)
			check(doc.Model().Text() != sample.text, "second Delete did not edit")
			key(events.KeyZ, primary)
			check(doc.Model().Text() == sample.text, "undo did not restore source")
		})
		steps = append(steps, func() bool { return matches(v, sample.want) && matches(mirror, sample.want) })
		next(func() { caret(0, false) })
		next(func() {
			var target geometry.Rectangle
			for _, child := range v.Snapshot().Children {
				if data, ok := child.Attributes["goui.sourceview.fold"].(map[string]any); ok && data["startLine"] == 4 {
					target = child.Bounds
				}
			}
			check(target.Width > 0, "literal/comment fold button missing")
			point := geometry.Point{X: target.X + target.Width/2, Y: target.Y + target.Height/2}
			must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerMove, Position: point}))
			check(v.Cursor() == gui.CursorPointing, "fold button cursor")
			for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
				must(win.DispatchEvent(events.PointerEvent{EventType: kind, Position: point, Button: events.PointerButtonLeft}))
			}
			check(v.FoldingRegions()[2].Collapsed && !mirror.FoldingRegions()[2].Collapsed, "pointer literal fold/view isolation")
		})
		next(func() {
			win.SetFocusedWidget(v)
			caret(0, true)
			key(events.KeyBracketLeft, foldMods)
			check(v.FoldingRegions()[0].Collapsed, "outer fold")
			v.SetSearch(source.SearchQuery{Text: "hidden"})
			key(events.KeyF3, 0)
			check(!v.FoldingRegions()[0].Collapsed, "search did not reveal hidden match")
			text, err := doc.Model().Slice(v.Selection().Range())
			must(err)
			check(text == "hidden", "search selected wrong source bytes")
			mirror.FoldAll()
			check(mirror.FoldingRegions()[0].Collapsed, "read-only mirror cannot fold")
			v.SetFoldingEnabled(false)
			for _, r := range v.FoldingRegions() {
				check(!r.Collapsed, "disabled folding retained hidden lines")
			}
			check(mirror.FoldingRegions()[0].Collapsed, "disabling one view affected mirror")
			v.SetFoldingEnabled(true)
		})
		next(func() {
			caret(sample.want[0].EndLine, true)
			key(events.KeyBackspace, 0)
			check(doc.Model().Text() == strings.Replace(sample.text, "\n}\ntail", "\n\ntail", 1), "closing delimiter edit")
		})
		steps = append(steps, func() bool { return matches(v, sample.want[1:]) && matches(mirror, sample.want[1:]) })
		next(func() { key(events.KeyZ, primary); check(doc.Model().Text() == sample.text, "closing delimiter undo") })
		steps = append(steps, func() bool { return matches(v, sample.want) && matches(mirror, sample.want) })
		next(func() {
			log.Printf("PASS %s exact ranges, keyboard/pointer, history, search, view isolation and reanalysis", sample.id)
		})
	}
	index, complete := 0, false
	deadline := time.Now().Add(15 * time.Second)
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		if time.Now().After(deadline) {
			panic(fmt.Sprintf("step %d timeout; ranges=%v", index, v.FoldingRegions()))
		}
		if index == len(steps) {
			log.Printf("PASS code folding; typography=%s painter=%s scale=%v basic=%t dark=%t", app.Typography().Name(), probe.painter, probe.scale, *basicTheme, *dark)
			complete = true
			win.Destroy()
		} else if steps[index]() {
			index++
			deadline = time.Now().Add(8 * time.Second)
		}
	})
	must(timer.Start(100 * time.Millisecond))
	app.Run()
	check(complete, "verification closed early")
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
