// Horizontal scrollbar after typing past the viewport. Requires a native
// desktop/WM and fonts. Run: XMODIFIERS=@im=none go run
// ./tests/window/editor/horizontal_scroll [-basic] [-dark]. Record OS, painter
// and scale. Initial state is an editable no-wrap SourceView in ScrollView.
// Normal CommitText types into an empty row, crosses the 64-KiB paging boundary,
// then edits an already paged row. Keep the caret at its end; use normal pointer
// events to grab the actual horizontal thumb and drag to the left edge, back to
// the right and left again. Later frames must keep ScrollX=0 and expose the row
// start without changing selection, source, revision or undo availability.
// A trough click only prepares the far edge before the first thumb grab.
// No Home, caret movement, SetScrollX, clipboard, files or settings are used to
// return to the start. Undo and a subsequent typed character must still reveal
// the caret. PASS closes automatically; timeout/early close fails. Restart
// resets. This is not physical IME, compositor or other-native-OS validation.
package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"math"
	"runtime"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
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
func check(ok bool, s string) {
	if !ok {
		panic(s)
	}
}
func hbar(w gui.Widget) *gui.ScrollBar {
	if b, ok := w.(*gui.ScrollBar); ok && b.Visible() && b.Orientation() == layout.DirectionHorizontal {
		return b
	}
	for _, c := range w.Children() {
		if b := hbar(c); b != nil {
			return b
		}
	}
	return nil
}

type probe struct {
	gui.WidgetBase
	scale   float32
	painter string
}

func (p *probe) Measure(c layout.Constraint) layout.Measurement {
	return layout.Measured(c.Clamp(geometry.Size{Width: 1, Height: 1}))
}
func (p *probe) Paint(draw gui.Painter) {
	if p.painter != "" {
		return
	}
	p.scale = draw.PixelScale()
	i, err := draw.NewImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	must(err)
	p.painter = fmt.Sprintf("%T", i)
	i.Destroy()
}

func main() {
	runtime.LockOSThread()
	basicTheme, dark := flag.Bool("basic", false, "Basic theme"), flag.Bool("dark", false, "Modern dark theme")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewHorizontalScroll")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	if *basicTheme {
		app.SetStyleSheet(basic.Sheet())
	}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 763, Height: 360}})
	must(err)
	defer win.Destroy()
	v := source.NewSourceView()
	v.SetWrapMode(source.WrapNone)
	v.SetAutomaticCompletion(false)
	v.SetFoldingEnabled(false)
	v.ConnectError(func(err error) { panic(err) })
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(v)
	scroll.SetMainWeight(1)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(scroll)
	p := &probe{}
	box.AddChild(p)
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(v)
	primary, _ := gui.ModPrimary.Resolve()
	key := func(k events.Key, m events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: m}))
		}
	}
	pointer := func(kind events.EventType, point geometry.Point) {
		e := events.PointerEvent{EventType: kind, Position: point}
		if kind == events.PointerMove {
			e.Buttons = events.PointerButtonLeftDown
		} else {
			e.Button = events.PointerButtonLeft
		}
		must(win.DispatchEvent(e))
	}
	var steps []func() bool
	next := func(f func()) { steps = append(steps, func() bool { f(); return true }) }
	settle := func(checkEnd func()) {
		ticks := 0
		steps = append(steps, func() bool {
			ticks++
			if ticks < 6 {
				return false
			}
			checkEnd()
			return true
		})
	}
	steps = append(steps, func() bool { return p.painter != "" && v.Focused() && v.Rect().Width > 0 })
	for _, sample := range []struct {
		name        string
		seed, typed int
		numbers     bool
	}{{"ordinary", 0, 128, true}, {"paging boundary", 65530, 12, false}, {"paged", 80000, 16, true}} {
		var doc *source.Document
		var before string
		var selection textedit.Selection
		var revision uint64
		var undo bool
		unchanged := func() {
			check(v.Selection() == selection && doc.Model().Revision() == revision && doc.Model().Text() == before && doc.Model().CanUndo() == undo, "scroll changed source, selection or history")
		}
		origin := func() {
			unchanged()
			check(scroll.ScrollX() == 0, fmt.Sprintf("horizontal thumb could not return to origin after %s typing: x=%g max=%g", sample.name, scroll.ScrollX(), scroll.Snapshot().MaxScrollX))
			info := v.Snapshot().TextEditing
			check(info != nil && info.VisibleRange.Start == 0 && strings.HasPrefix(info.VisibleText, "x"), "row start not visible")
		}
		next(func() {
			doc = source.NewDocument(strings.Repeat("x", sample.seed))
			v.SetDocument(doc)
			v.SetShowLineNumbers(sample.numbers)
			v.SetSelection(textedit.Selection{Anchor: sample.seed, Caret: sample.seed})
		})
		// Every character is a separate normal text commit, with real frame
		// work between commits. The first fixture crosses the viewport edge.
		for range sample.typed {
			next(func() { must(win.CommitText("x")) })
		}
		settle(func() {
			before = doc.Model().Text()
			selection = v.Selection()
			revision = doc.Model().Revision()
			undo = doc.Model().CanUndo()
			check(len(before) == sample.seed+sample.typed && selection.Caret == len(before), "typing did not reach line end")
			check(hbar(scroll) != nil && scroll.ScrollX() > 0, "horizontal bar did not appear")
		})
		// Prepare the far edge through a real trough click, then grab the
		// thumb there. The grab itself must not jump to a different value.
		next(func() {
			b := hbar(scroll).Snapshot().Bounds
			point := geometry.Point{X: b.X + b.Width - 1, Y: b.Y + b.Height/2}
			pointer(events.PointerDown, point)
			pointer(events.PointerUp, point)
		})
		settle(func() {
			unchanged()
			check(scroll.ScrollX() == float32(math.Floor(float64(scroll.Snapshot().MaxScrollX))), "trough setup did not reach far edge")
		})
		drag := func(left bool) {
			next(func() {
				b := hbar(scroll).Snapshot().Bounds
				x := b.X + 2
				if left {
					x = b.X + b.Width - 2
				}
				grab := geometry.Point{X: x, Y: b.Y + b.Height/2}
				old := scroll.ScrollX()
				pointer(events.PointerDown, grab)
				check(scroll.ScrollX() == old, "thumb grab jumped on the trough")
				target := geometry.Point{X: b.X + b.Width + 32, Y: grab.Y}
				if left {
					target.X = b.X - 32
				}
				pointer(events.PointerMove, target)
				pointer(events.PointerUp, target)
			})
		}
		drag(true)
		settle(origin)
		drag(false)
		settle(func() { unchanged(); check(scroll.ScrollX() > 0, "thumb did not scroll right") })
		drag(true)
		settle(origin)
		next(func() { doc.Model().BreakUndoGroup(); must(win.CommitText("y")) })
		settle(func() {
			check(scroll.ScrollX() > 0 && v.Selection().Caret == len(before)+1 && doc.Model().Text() == before+"y", "typing after manual scroll did not reveal current caret")
			key(events.KeyZ, primary)
		})
		settle(func() {
			check(doc.Model().Text() == before && v.Selection() == selection, "typed character undo changed selection/history")
			log.Printf("PASS horizontal %s: typing, real thumb drag left/right/left, stable origin, source/history and renewed reveal", sample.name)
		})
	}
	index, done := 0, false
	deadline := time.Now().Add(30 * time.Second)
	timer := app.NewTimer()
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), fmt.Sprintf("horizontal step %d timed out", index))
		if index < len(steps) {
			if steps[index]() {
				index++
			}
			return
		}
		timer.Stop()
		done = true
		log.Printf("PASS horizontal scrolling after typing; typography=%s painter=%s scale=%g basic=%t dark=%t", app.Typography().Name(), p.painter, p.scale, *basicTheme, *dark)
		win.RequestClose()
	})
	must(timer.Start(16 * time.Millisecond))
	app.Run()
	check(done, "horizontal verification aborted")
}
