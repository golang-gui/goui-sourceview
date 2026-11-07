// Menu command/hint integration. Requires a desktop, native fonts and an event
// loop. Run: XMODIFIERS=@im=none go run ./tests/window/editor/menus [-dark]
// [-basic] [-capture-dir DIR]. DIR must exist outside the repository.
// Starts one editor with selected text. Opens menus via dispatched Shift+F10,
// checks public MenuItemInfo hints against actual bindings and natural label
// boxes sharing one right edge (0.01-DIP float tolerance, no font golden).
// Rebinds/clears keys while open, changes readonly/selection/language, rejects
// invalid bindings, clicks Delete and Undo through Popover.DispatchEvent, then
// detaches/reattaches and changes documents. Old menu commands must not survive.
// State setters establish fixtures; input follows normal dispatch. Captures are
// observational. No clipboard, user files or system settings change. Restart
// resets; automatic success closes, failed assertion/timeout/early close exits
// nonzero. Physical IME/keyboard, mixed DPI and other OSes need native testing.
package main

import (
	"flag"
	"fmt"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/mlang"
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

type menuRow struct {
	info   gui.WidgetInfo
	labels []gui.WidgetInfo
}

func rows(p gui.Popover) []menuRow {
	var result []menuRow
	var walk func(gui.Widget)
	walk = func(w gui.Widget) {
		info := w.Snapshot()
		if info.Role == gui.RoleMenuItem {
			r := menuRow{info: info}
			for _, child := range w.Children() {
				if label, ok := child.(*gui.Label); ok && label.Visible() {
					r.labels = append(r.labels, label.Snapshot())
				}
			}
			result = append(result, r)
			return
		}
		for _, child := range w.Children() {
			walk(child)
		}
	}
	walk(p.Widget())
	return result
}

func main() {
	runtime.LockOSThread()
	dark := flag.Bool("dark", false, "Modern dark")
	basicTheme := flag.Bool("basic", false, "Basic theme")
	capture := flag.String("capture-dir", "", "existing external capture directory")
	flag.Parse()
	must(mlang.SetLanguage("en-US"))
	app, err := gui.NewApplication("org.golanggui.SourceViewMenus")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	if *basicTheme {
		app.SetStyleSheet(basic.Sheet())
	}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 850, Height: 700}})
	must(err)
	defer win.Destroy()
	v := source.NewSourceView()
	v.SetAutomaticCompletion(false)
	v.ConnectError(func(err error) { panic(err) })
	const text = "alpha\nbeta\n"
	doc := source.NewDocument(text)
	v.SetDocument(doc)
	v.SetSelection(textedit.Selection{Caret: 5})
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(v)
	win.SetWidget(scroll)
	must(win.Show())
	win.SetFocusedWidget(v)
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	open := func() { key(events.KeyF10, events.ModifierShift) }
	var popup gui.Popover
	var retained gui.Popover
	var observed gui.Popover
	frames, previousFrame := 0, 0
	var previousLayout []geometry.Rectangle
	var longWidth float32
	ready := func() bool {
		if len(win.Popovers()) != 1 {
			return false
		}
		popup = win.Popovers()[0]
		r := rows(popup)
		return len(r) == 16 && r[0].info.Bounds.Width > 0 && len(r[0].labels) > 0 && r[0].labels[0].Bounds.Width > 0
	}
	settled := func() bool {
		if !ready() {
			return false
		}
		if observed != popup {
			observed, frames, previousFrame, previousLayout = popup, 0, 0, nil
			popup.ConnectFrame(func(time.Time) { frames++ })
		}
		must(popup.RequestPaint())
		if frames == previousFrame {
			return false
		}
		previousFrame = frames
		layout := []geometry.Rectangle{popup.Widget().Rect()}
		for _, row := range rows(popup) {
			layout = append(layout, row.info.Bounds)
			for _, label := range row.labels {
				layout = append(layout, label.Bounds)
			}
		}
		stable := slices.Equal(previousLayout, layout)
		previousLayout = layout
		return stable
	}
	verify := func() {
		r := rows(popup)
		check(len(r) == 16, fmt.Sprintf("menu rows=%d bounds=%+v snapshot=%+v", len(r), popup.Widget().Rect(), popup.Widget().Snapshot()))
		bindings := v.KeyBindings()
		var edge, titleX float32
		first := true
		for i, row := range r {
			c := source.Command(i + 1)
			hint := ""
			if len(bindings[c]) > 0 {
				hint = bindings[c][0].DisplayText()
			}
			meta, ok := row.info.Attributes[gui.MenuItemInfoKey].(gui.MenuItemInfo)
			check(ok && meta.Shortcut == hint, fmt.Sprintf("command %d hint=%q want=%q", c, meta.Shortcut, hint))
			check(len(row.labels) > 0 && row.labels[0].Text == row.info.Text && !strings.Contains(row.info.Text, "\t"), "hint was concatenated into title")
			if i == 0 {
				titleX = row.labels[0].Bounds.X
			}
			check(math.Abs(float64(row.labels[0].Bounds.X-titleX)) < .01, "menu title columns differ")
			if hint == "" {
				check(len(row.labels) == 1, "unbound command retains a hint label")
				continue
			}
			check(len(row.labels) == 2 && row.labels[1].Text == hint, "visible hint does not match semantic metadata")
			h := row.labels[1].Bounds
			check(row.labels[0].Bounds.X+row.labels[0].Bounds.Width < h.X, "title overlaps shortcut column")
			if first {
				edge, first = h.X+h.Width, false
			}
			check(math.Abs(float64(h.X+h.Width-edge)) < .01, "shortcut text right edges are not aligned")
		}
	}
	click := func(index int) {
		r := rows(popup)[index].info
		check(r.Enabled, "test tried to activate a disabled command")
		for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
			must(popup.DispatchEvent(events.PointerEvent{EventType: kind, Button: events.PointerButtonLeft, Position: r.Bounds.Center()}))
		}
	}
	shot := func(name string) {
		if *capture == "" {
			return
		}
		im, err := gui.RenderWidget(popup.Widget(), 2)
		must(err)
		f, err := os.Create(filepath.Join(*capture, name+".png"))
		must(err)
		must(png.Encode(f, im))
		must(f.Close())
	}
	steps := []func() bool{
		func() bool {
			if v.Rect().Width <= 0 {
				return false
			}
			open()
			return true
		},
		func() bool {
			if !ready() {
				return false
			}
			verify()
			shot("default")
			retained = popup
			bindings := v.KeyBindings()
			bindings[source.CommandUndo] = []gui.KeyGesture{{Key: gui.KeyF6}, {Key: gui.KeyF7}}
			bindings[source.CommandSelectAll] = []gui.KeyGesture{{Key: gui.KeyF8, Modifiers: gui.ModControl | gui.ModAlt | gui.ModShift}}
			must(v.SetKeyBindings(bindings))
			return true
		},
		func() bool {
			check(len(win.Popovers()) == 1 && win.Popovers()[0] == retained, "rebinding recreated or closed menu")
			verify()
			longWidth = popup.Widget().Rect().Width
			err := v.SetKeyBindings(source.KeyBindings{source.CommandUndo: {{Key: gui.KeyF9}}, source.CommandRedo: {{Key: gui.KeyF9}}})
			check(err != nil, "conflicting bindings accepted")
			verify()
			v.SetReadOnly(true)
			return true
		},
		func() bool {
			r := rows(popup)
			check(!r[5].info.Enabled && r[11].info.Enabled, "readonly menu availability was not updated")
			v.SetReadOnly(false)
			v.SetSelection(textedit.Selection{})
			return true
		},
		func() bool {
			check(!rows(popup)[5].info.Enabled, "collapsed selection left Delete enabled")
			v.SetSelection(textedit.Selection{Caret: 5})
			must(mlang.SetLanguage("zh-CN"))
			return true
		},
		func() bool {
			check(win.Popovers()[0] == retained && rows(popup)[11].info.Text == "全选", "language change replaced menu or missed labels")
			verify()
			shot("rebound-zh")
			must(v.SetKeyBindings(nil))
			return true
		},
		func() bool {
			verify()
			check(popup.Widget().Rect().Width < longWidth, "menu did not shrink after clearing hints")
			click(5)
			return true
		},
		func() bool {
			check(doc.Model().Text() == "\nbeta\n" && len(win.Popovers()) == 0, fmt.Sprintf("unbound Delete: text=%q popovers=%d selection=%+v", doc.Model().Text(), len(win.Popovers()), v.Selection()))
			open()
			return true
		},
		func() bool {
			if !ready() {
				return false
			}
			verify()
			click(0)
			return true
		},
		func() bool {
			check(doc.Model().Text() == text && len(win.Popovers()) == 0, "menu undo failed")
			open()
			return true
		},
		func() bool {
			if !ready() {
				return false
			}
			scroll.SetChild(nil)
			check(len(win.Popovers()) == 0, "unmount left menu open")
			scroll.SetChild(v)
			win.SetFocusedWidget(v)
			must(v.SetKeyBindings(source.DefaultKeyBindings()))
			return true
		},
		func() bool { open(); return true },
		func() bool {
			if !ready() {
				return false
			}
			verify()
			v.SetDocument(source.NewDocument("replacement"))
			check(len(win.Popovers()) == 0 && v.Document().Model().Text() == "replacement", "document replacement retained old menu")
			return true
		},
	}
	step := 0
	done := false
	deadline := time.Now().Add(25 * time.Second)
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), fmt.Sprintf("menu regression timeout at step %d", step))
		if step < len(steps) {
			if len(win.Popovers()) != 0 && !settled() {
				return
			}
			if steps[step]() {
				step++
				previousLayout = nil
			}
			return
		}
		done = true
		log.Printf("PASS menu hint columns, live remapping/clear, aliases, validation, locale, readonly/selection, dispatched actions/undo, detach and document replacement; typography=%s dark=%t basic=%t", app.Typography().Name(), *dark, *basicTheme)
		win.Destroy()
	})
	must(timer.Start(200 * time.Millisecond))
	app.Run()
	check(done, "menu verification closed early")
}
