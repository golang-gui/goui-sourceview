// Mixed JSX/TSX/PHP folding verification in a native desktop/window manager.
// Run: XMODIFIERS=@im=none go run ./tests/window/editor/folding_mixed [-basic]
// [-dark]. Record OS, typography, GOUI_PLAT_PAINTER and GOUI_PLAT_SCALE.
// Two views share each fixed source; the mirror is read-only. Automatic checks
// wait for exact asynchronous ranges, fold via Window.DispatchEvent keyboard and
// pointer input, check visible source and view isolation, search hidden text,
// delete at a collapsed boundary and undo, then remove a real JSX closing tag,
// PHP endif or nowdoc terminator. Wait for independently expected invalidation
// ranges, undo, and confirm original candidates in both views. Document switches
// exercise detachment/cancellation. No user files, clipboard or settings change.
// PASS closes automatically; timeout, early close or assertion fails nonzero.
// Restart resets. Physical IME, mixed DPI and other OSes need native testing.
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
	app, err := gui.NewApplication("org.golanggui.SourceViewMixedFolding")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	if *basicTheme {
		app.SetStyleSheet(basic.Sheet())
	}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 880, Height: 800}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView mixed folding verification"))
	v, mirror := source.NewSourceView(), source.NewSourceView()
	v.SetAutomaticCompletion(false)
	mirror.SetReadOnly(true)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	probe := &scaleProbe{}
	box.AddChild(probe)
	label := gui.NewLabel("Mixed folding · JSX/TSX/PHP")
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
	jsx := "function Demo() {\n return (\n  <Panel>\n   <Item>\n    {`needle\n      ${value\n      }\n     `}\n   </Item>\n  </Panel>\n );\n}\ntail"
	ranges := func(pairs ...[2]int) []language.FoldRange {
		var out []language.FoldRange
		for _, p := range pairs {
			out = append(out, language.FoldRange{StartLine: p[0], EndLine: p[1]})
		}
		return out
	}
	fixtures := []fixture{
		{language.JSX, jsx, ranges([2]int{0, 11}, [2]int{1, 10}, [2]int{2, 9}, [2]int{3, 8}, [2]int{4, 7}, [2]int{5, 6}), ranges([2]int{4, 7}, [2]int{5, 6}), 5, 5, 3, 8},
		{language.TSX, jsx, ranges([2]int{0, 11}, [2]int{1, 10}, [2]int{2, 9}, [2]int{3, 8}, [2]int{4, 7}, [2]int{5, 6}), ranges([2]int{4, 7}, [2]int{5, 6}), 5, 5, 3, 8},
		{language.PHP, "<main>\n<?php if ($ok): ?>\n <section>\n <?php foreach ($xs as $x): ?>\n  <span>\n   <?= $x ?>needle\n  </span>\n <?php endforeach; ?>\n </section>\n<?php endif; ?>\n</main>\ntail", ranges([2]int{0, 10}, [2]int{1, 8}, [2]int{2, 8}, [2]int{3, 6}, [2]int{4, 6}), ranges([2]int{0, 10}, [2]int{2, 8}, [2]int{3, 6}, [2]int{4, 6}), 5, 4, 2, 9},
		{language.PHP, "<main>\n<?php\nif ($ok) {\n $x = <<<'TEXT'\n  needle } <fake>\n TEXT;\n}\n?>\n</main>\ntail", ranges([2]int{0, 8}, [2]int{2, 6}, [2]int{3, 5}), nil, 4, 3, 2, 5},
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
			label.SetText("Mixed folding · " + string(sample.id))
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
			check(target.Width > 0, "mixed fold button missing")
			point := geometry.Point{X: target.X + target.Width/2, Y: target.Y + target.Height/2}
			for _, kind := range []events.EventType{events.PointerMove, events.PointerDown, events.PointerUp} {
				must(win.DispatchEvent(events.PointerEvent{EventType: kind, Position: point, Button: events.PointerButtonLeft}))
			}
			check(collapsed(v, sample.pointerRow) && !collapsed(mirror, sample.pointerRow), "pointer fold/view isolation")
		})
		next(func() {
			info := v.Snapshot()
			check(info.TextEditing != nil && !strings.Contains(info.TextEditing.VisibleText, "needle"), "hidden mixed source still visible")
			win.SetFocusedWidget(v)
			v.SetSearch(source.SearchQuery{Text: "needle"})
			key(events.KeyF3, 0)
			text, err := doc.Model().Slice(v.Selection().Range())
			must(err)
			check(text == "needle" && !collapsed(v, sample.pointerRow), "search lost mixed source coordinates")
			caret(0, true)
			key(events.KeyBracketLeft, foldMods)
			check(collapsed(v, 0), "outer mixed fold")
			key(events.KeyDelete, 0)
			check(!collapsed(v, 0) && doc.Model().Text() == sample.text, "first Delete removed hidden mixed text")
			key(events.KeyDelete, 0)
			check(doc.Model().Text() != sample.text, "second Delete did not edit")
			key(events.KeyZ, primary)
			check(doc.Model().Text() == sample.text, "mixed boundary undo")
		})
		steps = append(steps, func() bool { return matches(v, sample.want) && matches(mirror, sample.want) })
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
			check(doc.Model().Text() == strings.Join(lines, "\n"), "closing token edit")
		})
		steps = append(steps, func() bool { return matches(v, sample.after) && matches(mirror, sample.after) })
		next(func() { key(events.KeyZ, primary); check(doc.Model().Text() == sample.text, "closing token undo") })
		steps = append(steps, func() bool { return matches(v, sample.want) && matches(mirror, sample.want) })
		next(func() {
			log.Printf("PASS %s closing row %d: mixed ranges, keyboard/pointer, deletion, undo, search, reanalysis and isolation", sample.id, sample.editRow)
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
			log.Printf("PASS mixed folding; typography=%s painter=%s scale=%v basic=%t dark=%t", app.Typography().Name(), probe.painter, probe.scale, *basicTheme, *dark)
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
