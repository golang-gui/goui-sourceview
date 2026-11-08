// Layout folding verification in an independent native desktop/WM.
// Run: XMODIFIERS=@im=none go run ./tests/window/editor/folding_layout
// [-basic] [-dark]. Record OS, typography, GOUI_PLAT_PAINTER and GOUI_PLAT_SCALE.
// Two views share F#/Haskell fixtures; the mirror is read-only.
// Wait for exact asynchronous ranges, fold nested code by normal Window
// keyboard events and a literal/comment by pointer events; verify visible
// source, view isolation, hidden search/reveal, boundary Delete and Undo. Remove
// each outer declaration/terminator, check the expected inner ranges in both views, then
// undo and await original candidates. Document switches exercise cancellation.
// Includes F# aligned branches/interpolation, Haskell implicit/explicit layout,
// string gaps, nested comments, and changes to indentation. Removing the outer
// declaration or layout introducer must invalidate its candidate in both views.
// No user files, clipboard or settings change. PASS closes automatically;
// timeout, early close or assertion fails nonzero. Restart resets. Physical IME,
// mixed DPI and other native OSes require separate verification.
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
	id                            language.ID
	text                          string
	want, after                   []language.FoldRange
	keyRow, keyHeader, pointerRow int
	editRow                       int
}

func main() {
	runtime.LockOSThread()
	dark := flag.Bool("dark", false, "Modern dark")
	basicTheme := flag.Bool("basic", false, "Basic")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewLayoutFolding")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	if *basicTheme {
		app.SetStyleSheet(basic.Sheet())
	}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 880, Height: 800}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView layout folding verification"))
	v, mirror := source.NewSourceView(), source.NewSourceView()
	v.SetAutomaticCompletion(false)
	mirror.SetReadOnly(true)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	probe := &scaleProbe{}
	box.AddChild(probe)
	label := gui.NewLabel("Layout folding · F#/Haskell")
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
	ranges := func(pairs ...[2]int) []language.FoldRange {
		var out []language.FoldRange
		for _, p := range pairs {
			out = append(out, language.FoldRange{StartLine: p[0], EndLine: p[1]})
		}
		return out
	}
	fixtures := []fixture{
		{language.FSharp, "begin\nlet run x =\n    first x\n    \"needle\"\nend\ntail", ranges([2]int{0, 4}, [2]int{1, 3}), ranges([2]int{1, 3}), 2, 1, 1, 4},
		{language.FSharp, "let run() =\n    if ready then\n        first()\n        second()\n    let text = @\"first\n        needle end }\n    last\"\n    finish()\n    finish()\nlet next = 1", ranges([2]int{0, 8}, [2]int{1, 3}, [2]int{4, 6}), ranges([2]int{1, 3}, [2]int{4, 6}), 2, 1, 4, 0},
		{language.FSharp, "let ``match with`` x =\n    match x with\n    | Some value ->\n        work value\n    | None ->\n        other()\n    let text = \"first\n        needle end }\n    last\"\nlet next = 1", ranges([2]int{0, 8}, [2]int{1, 5}, [2]int{2, 3}, [2]int{4, 5}, [2]int{6, 8}), ranges([2]int{1, 5}, [2]int{2, 3}, [2]int{4, 5}, [2]int{6, 8}), 3, 2, 6, 0},
		{language.FSharp, "begin\nlet text = $$\"\"\"first {{\n    if ready then\n        \"needle\"\n    else\n        \"other\"\n}} last\"\"\"\nwork()\nend\ntail", ranges([2]int{0, 8}, [2]int{1, 6}, [2]int{2, 5}, [2]int{4, 5}), ranges([2]int{1, 6}, [2]int{2, 5}, [2]int{4, 5}), 3, 2, 1, 8},
		{language.FSharp, "let run() =\n    try\n        work()\n    finally\n        cleanup()\n    (* outer\n      (* needle end }\n      *)\n    *)\n    finish()\nlet next = 1", ranges([2]int{0, 9}, [2]int{1, 4}, [2]int{3, 4}, [2]int{5, 8}, [2]int{6, 7}), ranges([2]int{1, 4}, [2]int{3, 4}, [2]int{5, 8}, [2]int{6, 7}), 4, 3, 5, 0},
		{language.Haskell, "run = do\n  case value of\n    A -> first\n    _ -> second\n  let text = \"first\\\n    \\needle\\\n    \\last\"\n  finish\n  finish\nnext = 1", ranges([2]int{0, 8}, [2]int{1, 3}, [2]int{4, 6}), ranges([2]int{1, 3}, [2]int{4, 6}), 2, 1, 4, 0},
		{language.Haskell, "module Demo where\nrun = do\n  first\n  second\ntext = \"first\\\n  \\needle\\\n  \\last\"\nother = 1", ranges([2]int{0, 7}, [2]int{1, 3}, [2]int{4, 6}), ranges([2]int{1, 3}, [2]int{4, 6}), 2, 1, 4, 0},
		{language.Haskell, "run = do {\nwork (do\n  first\n  second\n );\n{- outer\n {- needle let do\n -}\n-}\n}\nnext = 1", ranges([2]int{0, 9}, [2]int{1, 4}, [2]int{5, 8}, [2]int{6, 7}), ranges([2]int{1, 4}, [2]int{5, 8}, [2]int{6, 7}), 2, 1, 5, 9},
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
	collapsed := func(editor *source.SourceView, row int) bool {
		for _, r := range editor.FoldingRegions() {
			if r.StartLine == row {
				return r.Collapsed
			}
		}
		panic("missing fold header")
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
			label.SetText("Layout folding · " + string(sample.id))
		})
		steps = append(steps, func() bool { return matches(v, sample.want) && matches(mirror, sample.want) })
		next(func() {
			caret(sample.keyRow, false)
			key(events.KeyBracketLeft, foldMods)
			check(collapsed(v, sample.keyHeader) && !collapsed(mirror, sample.keyHeader), "keyboard fold/view isolation")
			check(doc.Model().Text() == sample.text && !doc.Model().CanUndo(), "fold changed source/history")
			key(events.KeyBracketRight, foldMods)
			check(!collapsed(v, sample.keyHeader), "keyboard unfold")
			caret(0, false)
		})
		next(func() {
			var target geometry.Rectangle
			for _, child := range v.Snapshot().Children {
				if data, ok := child.Attributes["goui.sourceview.fold"].(map[string]any); ok && data["startLine"] == sample.pointerRow {
					target = child.Bounds
				}
			}
			check(target.Width > 0, "layout fold button missing")
			point := geometry.Point{X: target.X + target.Width/2, Y: target.Y + target.Height/2}
			for _, kind := range []events.EventType{events.PointerMove, events.PointerDown, events.PointerUp} {
				must(win.DispatchEvent(events.PointerEvent{EventType: kind, Position: point, Button: events.PointerButtonLeft}))
			}
			check(collapsed(v, sample.pointerRow) && !collapsed(mirror, sample.pointerRow), "pointer fold/view isolation")
		})
		next(func() {
			info := v.Snapshot()
			check(info.TextEditing != nil && !strings.Contains(info.TextEditing.VisibleText, "needle"), "hidden layout source still visible")
			// Folded snapshots expose separate source fragments; VisibleText
			// intentionally describes only the first continuous fragment.
			spans := info.Attributes["goui.sourceview"].(map[string]any)["visibleSpans"].([]map[string]any)
			endVisible := false
			for _, span := range spans {
				text := span["text"].(string)
				check(!strings.Contains(text, "needle"), "hidden source leaked into visible spans")
				endVisible = endVisible || strings.TrimSpace(text) == "end"
			}
			if strings.HasPrefix(sample.text, "begin\nlet run x") {
				check(endVisible, "inner fold hid outer end")
			}
			win.SetFocusedWidget(v)
			v.SetSearch(source.SearchQuery{Text: "needle"})
			key(events.KeyF3, 0)
			text, err := doc.Model().Slice(v.Selection().Range())
			must(err)
			check(text == "needle" && !collapsed(v, sample.pointerRow), "search lost layout source coordinates")
			caret(0, true)
			key(events.KeyBracketLeft, foldMods)
			check(collapsed(v, 0), "outer layout fold")
			key(events.KeyDelete, 0)
			check(!collapsed(v, 0) && doc.Model().Text() == sample.text, "first Delete removed hidden layout text")
			key(events.KeyDelete, 0)
			check(doc.Model().Text() != sample.text, "second Delete did not edit")
			key(events.KeyZ, primary)
			check(doc.Model().Text() == sample.text, "layout boundary undo")
		})
		steps = append(steps, func() bool { return matches(v, sample.want) && matches(mirror, sample.want) })
		if strings.HasPrefix(sample.text, "let run() =\n    if") || strings.HasPrefix(sample.text, "run = do\n  case") {
			next(func() {
				r, err := doc.Model().LineRange(3)
				must(err)
				line, err := doc.Model().Slice(r)
				must(err)
				leading := len(line) - len(strings.TrimLeft(line, " "))
				v.SetSelection(textedit.Selection{Anchor: r.Start, Caret: r.Start + leading})
				key(events.KeyDelete, 0)
				lines := strings.Split(sample.text, "\n")
				lines[3] = strings.TrimLeft(lines[3], " ")
				check(doc.Model().Text() == strings.Join(lines, "\n"), "indentation edit")
			})
			changed := ranges([2]int{0, 2}, [2]int{1, 2}, [2]int{4, 6})
			steps = append(steps, func() bool { return matches(v, changed) && matches(mirror, changed) })
			next(func() { key(events.KeyZ, primary); check(doc.Model().Text() == sample.text, "indentation undo") })
			steps = append(steps, func() bool { return matches(v, sample.want) && matches(mirror, sample.want) })
		}

		next(func() {
			mirror.FoldAll()
			check(collapsed(mirror, 0), "read-only folding")
		})
		next(func() {
			r, err := doc.Model().LineRange(sample.editRow)
			must(err)
			v.SetSelection(textedit.Selection{Anchor: r.Start, Caret: r.End})
			key(events.KeyDelete, 0)
			lines := strings.Split(sample.text, "\n")
			lines[sample.editRow] = ""
			check(doc.Model().Text() == strings.Join(lines, "\n"), "boundary line edit")
		})
		steps = append(steps, func() bool { return matches(v, sample.after) && matches(mirror, sample.after) })
		next(func() { key(events.KeyZ, primary); check(doc.Model().Text() == sample.text, "boundary line undo") })
		steps = append(steps, func() bool { return matches(v, sample.want) && matches(mirror, sample.want) })
		next(func() {
			log.Printf("PASS %s boundary row %d: layout ranges, keyboard/pointer, deletion, undo, search, reanalysis and isolation", sample.id, sample.editRow)
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
			log.Printf("PASS layout folding; typography=%s painter=%s scale=%v basic=%t dark=%t", app.Typography().Name(), probe.painter, probe.scale, *basicTheme, *dark)
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
