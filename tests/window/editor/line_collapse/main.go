// Manual long-line collapse. Requires a native desktop/WM and fonts. Run:
// XMODIFIERS=@im=none go run ./tests/window/editor/line_collapse [-basic] [-dark].
// Two editors share one document. F6/F7 are test-local bindings for collapse /
// expand. Timers dispatch real key/pointer/menu input, change fixture documents,
// resize an adjacent pane, navigate/search/edit and detach/reattach the editor.
// Assert one-row preview, independent views, an interactive semantic badge,
// source/revision/history preservation, full-source copy in this private desktop,
// read-only operation, resize intent, minified JSON/Go/paged text and stale reset
// cleanup. Input is never replaced with a direct menu Command.Activate call.
// The footer tags actual after-paint frames for optional external X11 capture.
// PASS closes; timeout/early close/assertion fails. Restart resets all fixtures.
// Clipboard ownership lasts only for this test process/display; no user files
// or settings change. Physical IME and other native OSes require separate runs.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"runtime"
	"slices"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/mlang"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/platform/typography"
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

type footer struct{ gui.WidgetBase }

func (f *footer) Measure(c layout.Constraint) layout.Measurement {
	return layout.Measured(c.Clamp(geometry.Size{Width: 1, Height: 26}))
}
func (f *footer) Paint(p gui.Painter) {
	p.FillRect(geometry.Rect(0, 0, f.Rect().Width, f.Rect().Height), graphics.ColorOf(color.NRGBA{R: 60, G: 60, B: 60, A: 255}))
}

func menuItem(w gui.Widget, name string) (gui.WidgetInfo, bool) {
	i := w.Snapshot()
	if i.Role == gui.RoleMenuItem && i.Text == name {
		return i, true
	}
	for _, c := range w.Children() {
		if i, ok := menuItem(c, name); ok {
			return i, true
		}
	}
	return gui.WidgetInfo{}, false
}
func badge(v *source.SourceView) gui.WidgetInfo {
	for _, i := range v.Snapshot().Children {
		if _, ok := i.Attributes["goui.sourceview.collapsedLine"]; ok {
			return i
		}
	}
	panic("missing visible long-line expand badge")
}

// A forwarding observer checks every actual collapsed paint, including frames
// too short for an external screenshot sampler. It performs no layout or UI
// mutation, and never substitutes offscreen rendering for window painting.
type paintedView struct {
	*source.SourceView
	checked int
}
type paintedText struct {
	gui.Painter
	prefixes map[int]string
	seen     map[int]bool
}

func (p *paintedText) DrawTextLayout(at geometry.Point, l typography.TextLayout) {
	for row, prefix := range p.prefixes {
		if strings.HasPrefix(l.Text(), prefix) {
			p.seen[row] = true
		}
	}
	p.Painter.DrawTextLayout(at, l)
}
func (v *paintedView) Paint(p gui.Painter) {
	observe := &paintedText{Painter: p, prefixes: make(map[int]string), seen: make(map[int]bool)}
	for _, row := range v.CollapsedLines() {
		r, _ := v.Document().Model().LineRange(row)
		r.End = min(r.End, r.Start+24)
		for !v.Document().Model().IsRuneBoundary(r.End) {
			r.End--
		}
		prefix, err := v.Document().Model().Slice(r)
		must(err)
		observe.prefixes[row] = prefix
	}
	v.SourceView.Paint(observe)
	for row := range observe.prefixes {
		check(observe.seen[row], fmt.Sprintf("actual collapsed paint lost the current-width paragraph on line %d", row))
		v.checked++
	}
}

func main() {
	runtime.LockOSThread()
	dark, basicTheme := flag.Bool("dark", false, "Modern dark"), flag.Bool("basic", false, "Basic")
	flag.Parse()
	must(mlang.SetLanguage("en"))
	app, err := gui.NewApplication("org.golanggui.SourceViewLineCollapse")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	if *basicTheme {
		app.SetStyleSheet(basic.Sheet())
	}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1000, Height: 560}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView long-line collapse"))
	text := "BEFORE\n" + strings.Repeat("wrapped 中文 words ", 500) + "NEEDLE\nAFTER"
	doc := source.NewDocument(text)
	v, other := source.NewSourceView(), source.NewSourceView()
	observed := &paintedView{SourceView: v}
	hosts := []*gui.ScrollView{gui.NewScrollView(), gui.NewScrollView()}
	for i, view := range []*source.SourceView{v, other} {
		view.SetDocument(doc)
		view.SetWrapMode(source.WrapWordChar)
		view.SetAutomaticCompletion(false)
		view.ConnectError(func(err error) { panic(err) })
		hosts[i].SetStyleName("source-view")
		if i == 0 {
			hosts[i].SetChild(observed)
		} else {
			hosts[i].SetChild(view)
		}
	}
	hosts[0].SetMainWeight(1)
	setPane := func(width float32) {
		hosts[1].SetMinSize(geometry.Size{Width: width})
		hosts[1].SetMaxSize(geometry.Size{Width: width})
	}
	setPane(250)
	row := gui.NewLinearBox(layout.DirectionHorizontal)
	row.SetCrossAlign(layout.CrossStretch)
	row.SetSpacing(8)
	row.SetMainWeight(1)
	row.AddChild(hosts[0])
	row.AddChild(hosts[1])
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(row)
	box.AddChild(&footer{})
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(v)
	keys := v.KeyBindings()
	keys[source.CommandCollapseLine] = []gui.KeyGesture{{Key: gui.KeyF6}}
	keys[source.CommandExpandLine] = []gui.KeyGesture{{Key: gui.KeyF7}}
	must(v.SetKeyBindings(keys))
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	click := func(target interface{ DispatchEvent(events.Event) error }, point geometry.Point) {
		for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
			must(target.DispatchEvent(events.PointerEvent{EventType: kind, Button: events.PointerButtonLeft, Position: point}))
		}
	}
	frame, phase, backend := 0, "initial", ""
	enc := json.NewEncoder(os.Stdout)
	win.ConnectAfterPaint(func(p gui.Painter, b geometry.Rectangle) {
		frame++
		if backend == "" {
			i, err := p.NewImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
			must(err)
			backend = fmt.Sprintf("%T", i)
			i.Destroy()
		}
		info := v.Snapshot()
		var badgeBounds geometry.Rectangle
		for _, child := range info.Children {
			if _, ok := child.Attributes["goui.sourceview.collapsedLine"]; ok {
				badgeBounds = child.Bounds
				break
			}
		}
		must(enc.Encode(map[string]any{"event": "paint", "frame": frame, "phase": phase, "collapsed": v.CollapsedLines(), "maxY": hosts[0].Snapshot().MaxScrollY, "scale": p.PixelScale(), "editor": info.Bounds, "badge": badgeBounds}))
		p.FillRect(geometry.Rect(6, b.Height-22, 3, 16), graphics.ColorOf(color.NRGBA{R: 255, B: 255, A: 255}))
		for bit := range 16 {
			c := color.NRGBA{A: 255}
			if frame&(1<<bit) != 0 {
				c.R, c.G, c.B = 255, 255, 255
			}
			p.FillRect(geometry.Rect(12+float32(bit)*4, b.Height-22, 4, 16), graphics.ColorOf(c))
		}
	})
	primary, _ := gui.ModPrimary.Resolve()
	line := 1
	selectStart := func() {
		r, _ := doc.Model().LineRange(line)
		v.SetSelection(textedit.Selection{Anchor: r.Start, Caret: r.Start})
	}
	revision, undo := doc.Model().Revision(), doc.Model().CanUndo()
	unchanged := func() {
		check(doc.Model().Text() == text && doc.Model().Revision() == revision && doc.Model().CanUndo() == undo, "collapse changed source/revision/history")
	}
	var steps []func() bool
	next := func(f func()) { steps = append(steps, func() bool { f(); return true }) }
	settle := func(f func()) {
		start := -1
		steps = append(steps, func() bool {
			if start < 0 {
				start = frame
			}
			if frame < start+3 || v.Snapshot().Attributes["goui.sourceview"].(map[string]any)["layoutPending"] == true {
				return false
			}
			f()
			return true
		})
	}
	settle(func() {
		check(hosts[0].Snapshot().MaxScrollY > 500, "fixture is not vertically long")
		selectStart()
		key(events.KeyF6, 0)
		phase = "collapsed"
	})
	settle(func() {
		unchanged()
		check(slices.Equal(v.CollapsedLines(), []int{1}) && len(other.CollapsedLines()) == 0, "shared views did not remain independent")
		b := badge(v)
		check(b.Role == gui.RoleButton && len(b.Actions) == 1 && b.Bounds.Height < 40, "preview badge is missing or not one display row")
		check(hosts[0].Snapshot().MaxScrollY == 0 && hosts[1].Snapshot().MaxScrollY > 500, "collapse retained full paragraph height or changed another view")
		key(events.KeyA, primary)
		key(events.KeyC, primary)
	})
	copied := false
	next(func() {
		app.Clipboard().RequestText(func(got string, err error) {
			must(err)
			check(got == text, "copy omitted hidden source")
			copied = true
		})
	})
	steps = append(steps, func() bool { return copied })
	next(func() {
		unchanged()
		check(slices.Equal(v.CollapsedLines(), []int{1}), "copy expanded hidden source")
		click(win, badge(v).Bounds.Center())
		phase = "badge-expanded"
	})
	settle(func() {
		unchanged()
		check(len(v.CollapsedLines()) == 0 && hosts[0].Snapshot().MaxScrollY > 500, "badge did not expand")
		selectStart()
		key(events.KeyF10, events.ModifierShift)
	})
	steps = append(steps, func() bool {
		if len(win.Popovers()) != 1 {
			return false
		}
		pop := win.Popovers()[0]
		item, ok := menuItem(pop.Widget(), "Collapse long line")
		if !ok || item.Bounds.Width <= 0 {
			return false
		}
		check(item.Enabled, "long-line menu entry is disabled")
		meta, ok := item.Attributes[gui.MenuItemInfoKey].(gui.MenuItemInfo)
		check(ok && meta.Shortcut == keys[source.CommandCollapseLine][0].DisplayText(), "long-line menu shortcut association is wrong")
		click(pop, item.Bounds.Center())
		phase = "menu-collapsed"
		return true
	})
	settle(func() {
		unchanged()
		check(len(v.CollapsedLines()) == 1, "menu did not collapse")
		key(events.KeyF7, 0)
	})
	settle(func() {
		check(len(v.CollapsedLines()) == 0, "expand shortcut failed")
		must(mlang.SetLanguage("zh-CN"))
		selectStart()
		key(events.KeyF10, events.ModifierShift)
	})
	steps = append(steps, func() bool {
		if len(win.Popovers()) != 1 {
			return false
		}
		pop := win.Popovers()[0]
		item, ok := menuItem(pop.Widget(), "收起长行")
		if !ok || item.Bounds.Width <= 0 {
			return false
		}
		check(item.Enabled, "localized collapse menu is disabled")
		click(pop, item.Bounds.Center())
		phase = "localized-collapsed"
		return true
	})
	settle(func() {
		check(len(v.CollapsedLines()) == 1, "localized menu did not collapse")
		key(events.KeyF10, events.ModifierShift)
	})
	steps = append(steps, func() bool {
		if len(win.Popovers()) != 1 {
			return false
		}
		pop := win.Popovers()[0]
		item, ok := menuItem(pop.Widget(), "展开长行")
		if !ok || item.Bounds.Width <= 0 {
			return false
		}
		check(item.Enabled, "localized expand menu is disabled")
		click(pop, item.Bounds.Center())
		must(mlang.SetLanguage("en"))
		return true
	})
	settle(func() {
		check(len(v.CollapsedLines()) == 0, "expand shortcut failed")
		selectStart()
		key(events.KeyF6, 0)
		v.SetSearch(source.SearchQuery{Text: "NEEDLE"})
		key(events.KeyF3, 0)
		phase = "search-expanded"
	})
	settle(func() {
		check(len(v.CollapsedLines()) == 0 && v.Selection().Range().Start == strings.Index(text, "NEEDLE"), "search did not reveal hidden content")
		selectStart()
		key(events.KeyF6, 0)
		key(events.KeyArrowDown, 0)
		phase = "navigation-expanded"
	})
	settle(func() {
		check(len(v.CollapsedLines()) == 0, "Down did not expand hidden display lines")
		selectStart()
		key(events.KeyF6, 0)
		must(win.CommitText("!"))
		phase = "edit-expanded"
	})
	settle(func() {
		check(len(v.CollapsedLines()) == 0 && strings.Contains(doc.Model().Text(), "!wrapped"), "typing did not expand the affected line")
		key(events.KeyZ, primary)
	})
	settle(func() {
		check(doc.Model().Text() == text, "editing undo changed source")
		revision, undo = doc.Model().Revision(), doc.Model().CanUndo()
		selectStart()
		v.SetReadOnly(true)
		key(events.KeyF6, 0)
		phase = "readonly-collapsed"
	})
	settle(func() {
		unchanged()
		check(len(v.CollapsedLines()) == 1, "readonly line cannot collapse")
		click(win, badge(v).Bounds.Center())
		v.SetReadOnly(false)
		selectStart()
		key(events.KeyF6, 0)
		setPane(450)
		phase = "narrow-collapsed"
	})
	settle(func() {
		unchanged()
		check(len(v.CollapsedLines()) == 1 && hosts[0].Snapshot().MaxScrollY == 0, "resize lost preview or collapse intent")
		setPane(250)
		phase = "restored-collapsed"
	})
	settle(func() {
		unchanged()
		check(len(v.CollapsedLines()) == 1, "restoring width lost intent")
		hosts[0].SetChild(nil)
		hosts[0].SetChild(observed)
		win.SetFocusedWidget(v)
		phase = "reattach-collapsed"
	})
	settle(func() {
		check(len(v.CollapsedLines()) == 1, "unchanged reattach lost intent")
		hosts[0].SetChild(nil)
		doc.Model().SetText("fresh text")
		hosts[0].SetChild(observed)
		win.SetFocusedWidget(v)
		phase = "reloaded-expanded"
	})
	settle(func() { check(len(v.CollapsedLines()) == 0, "unmounted edit reused stale collapse") })
	for _, sample := range []struct {
		name, text string
		id         language.ID
		line       int
	}{
		{"json", `{"data":"` + strings.Repeat("JSON words ", 600) + `"}`, language.JSON, 0},
		{"go", "package main\nvar data = \"" + strings.Repeat("Go words ", 700) + "\"\n", language.Go, 1},
		{"paged", "PAGED " + strings.Repeat("wrapped words ", 9000) + "\nTAIL", language.Plain, 0},
	} {
		next(func() {
			text, line = sample.text, sample.line
			must(doc.SetLanguage(sample.id))
			doc.Model().SetText(text)
			phase = sample.name + "-expanded"
		})
		settle(func() { selectStart(); key(events.KeyF6, 0); phase = sample.name + "-collapsed" })
		settle(func() {
			check(slices.Equal(v.CollapsedLines(), []int{line}) && hosts[0].Snapshot().MaxScrollY == 0, "language/paged line could not collapse: "+sample.name)
			check(badge(v).Bounds.Height < 40, "language/paged preview is too tall")
			check(doc.Model().Text() == text, "language collapse changed source")
			if sample.name == "paged" {
				at := len(text) - 1
				v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
				key(events.KeyArrowUp, 0)
				check(len(v.CollapsedLines()) == 0 && v.Selection().Caret > len(text)-256, "Up missed the last display line of the collapsed paged paragraph")
			} else {
				key(events.KeyF7, 0)
			}
		})
	}
	next(func() { doc.Model().SetText("short"); phase = "short" })
	settle(func() { key(events.KeyF6, 0); check(len(v.CollapsedLines()) == 0, "short line was collapsed") })
	index, done := 0, false
	deadline := time.Now().Add(40 * time.Second)
	timer := app.NewTimer()
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), fmt.Sprintf("line collapse timed out at step %d phase %s", index, phase))
		must(win.RequestPaint())
		if index < len(steps) {
			if steps[index]() {
				index++
			}
			return
		}
		timer.Stop()
		done = true
		check(observed.checked > 20, "too few actual collapsed paints were checked")
		log.Printf("PASS long-line collapse: %d actual collapsed paints; shortcuts, menu, badge, shared views, copy, readonly, navigation/search/edit/undo, resize, reattach, JSON/Go/paged source; typography=%s painter=%s basic=%t dark=%t", observed.checked, app.Typography().Name(), backend, *basicTheme, *dark)
		win.RequestClose()
	})
	must(timer.Start(30 * time.Millisecond))
	app.Run()
	check(done, "long-line collapse verification aborted")
}
