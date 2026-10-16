// Editor experience verification. Requires a desktop, window manager and native
// fonts. Run: XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software go run
// ./tests/window/experience. Runs automatically and exits nonzero on failure.
// Initial state: a plain-text editor with line numbers hidden. Checks double
// click including the trailing half of a character, word dragging, triple click,
// partial selection foreground, right-click preservation, Delete and Undo via
// normal window/popup event dispatch. No clipboard, files or settings are changed.
// Record OS, backend and scale. Native IME is outside this program's coverage.
// Reset by restarting; closes automatically, or close the window to abort.
package main

import (
	"bytes"
	"log"
	"runtime"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/typography"
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

type trackedContext struct {
	typography.Context
	line typography.TextLayout
}

func (c *trackedContext) NewTextLayout(s string, f typography.TextFormat, w, h float32) (typography.TextLayout, error) {
	l, err := c.Context.NewTextLayout(s, f, w, h)
	if err == nil && s == "alpha beta" {
		c.line = l
	}
	return l, err
}

type trackedApp struct {
	gui.Application
	context typography.Context
}

func (a trackedApp) Typography() typography.Context { return a.context }

func main() {
	runtime.LockOSThread()
	app, err := gui.NewApplication("org.golang-gui.SourceViewExperience")
	must(err)
	ctx := &trackedContext{Context: app.Typography()}
	gui.App = trackedApp{Application: app, context: ctx}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 700, Height: 400}})
	must(err)
	defer win.Destroy()
	v := source.NewSourceView()
	v.SetShowLineNumbers(false)
	v.SetScheme(scheme.Light())
	v.SetAcceptsTab(true)
	d := v.Document()
	d.Model().SetText("alpha beta\n")
	scroll := gui.NewScrollView()
	scroll.SetChild(v)
	win.SetWidget(scroll)
	must(win.Show())
	win.SetFocusedWidget(v)
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	pointer := func(kind events.EventType, button events.PointerButton, p geometry.Point) {
		must(win.DispatchEvent(events.PointerEvent{EventType: kind, Button: button, Position: p}))
	}
	click := func(button events.PointerButton, p geometry.Point) {
		pointer(events.PointerDown, button, p)
		pointer(events.PointerUp, button, p)
	}
	var point geometry.Point
	var popup gui.Popover
	primary, _ := gui.ModPrimary.Resolve()
	steps := []func(){
		func() {
			check(win.Focused() && ctx.line != nil, "native focus/layout unavailable")
			lines, clusters := ctx.line.MeasureMetrics()
			g := textedit.NewGeometry(lines, clusters, 10, 22)
			r := g.Selection(textedit.Range{Start: 4, End: 5})[0]
			bounds := v.Snapshot().Bounds
			point = geometry.Point{X: bounds.X + 6 + r.X + r.Width*.8, Y: bounds.Y + 6 + r.Y + r.Height/2}
			click(events.PointerButtonLeft, point)
			click(events.PointerButtonLeft, point)
			check(v.Selection().Range() == (textedit.Range{Start: 0, End: 5}), "double-click release lost word")
			v.RevealSelection()
			original := ctx.line
			f := original.Format()
			f.TextColor = scheme.Light().Definition().Foreground
			fresh, err := ctx.Context.NewTextLayout("alpha beta", f, 1<<20, 1<<24)
			must(err)
			fresh.SetTextColor(0, 5, scheme.Light().Definition().SelectionForeground)
			got, err := original.Rasterize(1, nil)
			must(err)
			pixels := bytes.Clone(got.Pixels)
			want, err := fresh.Rasterize(1, nil)
			must(err)
			check(got.Width == want.Width && got.Height == want.Height && got.Stride == want.Stride && bytes.Equal(pixels, want.Pixels), "partial selection foreground mismatch")
			fresh.Destroy()
			click(events.PointerButtonLeft, point)
			check(v.Selection().Range() == (textedit.Range{Start: 0, End: 11}), "triple-click release lost logical line")
			v.SetSelection(textedit.Selection{Caret: 5})
			click(events.PointerButtonRight, point)
			check(v.Selection().Range() == (textedit.Range{Start: 0, End: 5}), "right-click changed selected trailing half")
			check(len(win.Popovers()) == 1, "context menu missing")
			popup = win.Popovers()[0]
		},
		func() {
			var target *gui.WidgetInfo
			var find func(gui.WidgetInfo)
			find = func(info gui.WidgetInfo) {
				if info.Role == gui.RoleMenuItem && info.Text == "Delete" {
					copy := info
					target = &copy
				}
				for _, child := range info.Children {
					find(child)
				}
			}
			find(popup.Widget().Snapshot())
			check(target != nil && target.Enabled, "Delete menu item unavailable")
			p := geometry.Point{X: target.Bounds.X + target.Bounds.Width/2, Y: target.Bounds.Y + target.Bounds.Height/2}
			for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
				must(popup.DispatchEvent(events.PointerEvent{EventType: kind, Button: events.PointerButtonLeft, Position: p}))
			}
			check(d.Model().Text() == " beta\n", "menu Delete failed")
			check(len(win.Popovers()) == 0, "menu failed to close")
			key(events.KeyZ, primary)
			check(d.Model().Text() == "alpha beta\n", "menu Delete undo failed")
			key(events.KeyF10, events.ModifierShift)
			check(len(win.Popovers()) == 1, "keyboard menu missing")
			key(events.KeyEscape, 0)
			check(len(win.Popovers()) == 0, "Escape did not close menu")
			log.Printf("PASS double/triple click, partial selection colors, context menu and history; backend=%s", app.Typography().Name())
			win.Destroy()
		},
	}
	step := 0
	timer := app.NewTimer()
	timer.ConnectTimeout(func() {
		if step < len(steps) {
			f := steps[step]
			step++
			f()
		}
	})
	must(timer.Start(250 * time.Millisecond))
	app.Run()
}
