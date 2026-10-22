// Editor experience verification. Requires a desktop, window manager and native
// fonts. Run: XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software go run
// ./tests/window/experience. Runs automatically and exits nonzero on failure.
// Initial state: a plain-text editor with line numbers hidden. Checks double
// click including the trailing half of a character, word dragging, triple click,
// partial selection foreground, right-click preservation, menu keyboard/pointer
// activation, completion keyboard/pointer/cancellation/stale asynchronous provider results, bracket
// insertion/overtype/surround/deletion/newline and atomic undo via normal
// window/popup event dispatch. No clipboard, files or settings are changed.
// Record OS, backend and scale. Native IME is outside this program's coverage.
// Reset by restarting; closes automatically, or close the window to abort.
package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"log"
	"math"
	"runtime"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
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

func completionInfo(info gui.WidgetInfo) gui.WidgetInfo {
	if info.Role == "completion-list" {
		return info
	}
	for _, child := range info.Children {
		if found := completionInfo(child); found.Role != "" {
			return found
		}
	}
	return gui.WidgetInfo{}
}

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
	bindings := v.KeyBindings()
	bindings[source.CommandComplete] = []gui.KeyGesture{{Key: gui.KeyF2}}
	must(v.SetKeyBindings(bindings))
	v.SetShowLineNumbers(false)
	// Preserve the existing explicit-foreground regression. Default syntax
	// preservation is exercised by editor/selection with tokenized samples.
	colors := scheme.Light().Definition()
	colors.SelectionForeground = color.NRGBA{R: 0x11, G: 0x11, B: 0x11, A: 255}
	explicit, err := scheme.New(colors)
	must(err)
	v.SetScheme(explicit)
	v.SetAcceptsTab(true)
	d := v.Document()
	d.Model().SetText("alpha beta\n")
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
	completed := false
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
			f.TextColor = colors.Foreground
			fresh, err := ctx.Context.NewTextLayout("alpha beta", f, 1<<20, 1<<24)
			must(err)
			fresh.SetTextColor(0, 5, colors.SelectionForeground)
			got, err := original.Rasterize(1, rasterRegion(original, 1), nil)
			must(err)
			pixels := bytes.Clone(got.Pixels)
			want, err := fresh.Rasterize(1, rasterRegion(fresh, 1), nil)
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
			key(events.KeyEnd, 0)
		},
		func() {
			// Newly opened menu rows are realized by the next native frame.
			var selected func(gui.WidgetInfo) bool
			selected = func(i gui.WidgetInfo) bool {
				if i.Role == gui.RoleMenuItem && i.Selected && i.Text == "Select All" {
					return true
				}
				for _, c := range i.Children {
					if selected(c) {
						return true
					}
				}
				return false
			}
			check(selected(win.Popovers()[0].Widget().Snapshot()), "menu keyboard selection missing")
			key(events.KeyEnter, 0)
			check(v.Selection().Range() == (textedit.Range{Start: 0, End: 11}) && len(win.Popovers()) == 0, "menu keyboard activation failed")
			key(events.KeyF10, events.ModifierShift)
			key(events.KeyEscape, 0)
			check(len(win.Popovers()) == 0, "Escape did not close menu")
			key(events.KeyF2, 0)
			check(len(win.Popovers()) == 0, "completion opened for a nonempty selection")
			log.Printf("PASS double/triple click, partial selection colors, context menu and history; backend=%s", app.Typography().Name())
			must(d.SetLanguage(language.JSON))
			d.Model().SetText("tr")
			v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
			key(events.KeyF2, 0)
			check(len(win.Popovers()) == 1, "JSON completion popup missing")
		},
		func() {
			popup = win.Popovers()[0]
			info := completionInfo(popup.Widget().Snapshot())
			check(info.Role == gui.Role("completion-list") && len(info.Children) > 0 && info.Children[0].Text == "true", "JSON completion proposal missing")
			key(events.KeyEnter, 0)
			check(d.Model().Text() == "true" && v.Selection().Caret == 4 && len(win.Popovers()) == 0, "completion acceptance failed")
			key(events.KeyZ, primary)
			check(d.Model().Text() == "tr" && v.Selection().Caret == 2, "completion undo failed")
			d.Model().SetText("")
			v.SetSelection(textedit.Selection{})
			must(win.CommitText("["))
			check(d.Model().Text() == "[]" && v.Selection().Caret == 1, "auto pair insertion failed")
			key(events.KeyEnter, 0)
			check(d.Model().Text() == "[\n    \n]" && v.Selection().Caret == 6, "pair newline indentation failed")
			key(events.KeyZ, primary)
			check(d.Model().Text() == "[]" && v.Selection().Caret == 1, "pair newline undo failed")
			must(win.CommitText("{"))
			must(win.CommitText("}"))
			must(win.CommitText("]"))
			check(d.Model().Text() == "[{}]" && v.Selection().Caret == 4, "nested pair overtype failed")
			key(events.KeyZ, primary)
			check(d.Model().Text() == "[]", "nested pair undo failed")
			d.Model().SetText("")
			v.SetSelection(textedit.Selection{})
			must(win.CommitText("["))
			key(events.KeyBackspace, 0)
			check(d.Model().Text() == "", "pair backspace failed")
			key(events.KeyZ, primary)
			check(d.Model().Text() == "[]", "pair delete undo failed")
			d.Model().SetText("word")
			v.SetSelection(textedit.Selection{Caret: 4})
			must(win.CommitText(`"`))
			check(d.Model().Text() == `"word"` && v.Selection() == (textedit.Selection{Anchor: 1, Caret: 5}), "selection surround failed")
			key(events.KeyZ, primary)
			check(d.Model().Text() == "word", "selection surround undo failed")
			d.Model().SetText("")
			v.SetSelection(textedit.Selection{})
			must(win.CommitText("t"))
			check(len(win.Popovers()) == 1, "automatic JSON completion missing")
			key(events.KeyEscape, 0)
			check(d.Model().Text() == "t" && len(win.Popovers()) == 0, "completion Escape modified text")
			must(d.SetLanguage(language.HTML))
			d.Model().SetText("<di")
			v.SetSelection(textedit.Selection{Anchor: 3, Caret: 3})
			key(events.KeyF2, 0)
			check(len(win.Popovers()) == 1, "HTML completion missing")
		},
		func() {
			popup = win.Popovers()[0]
			info := completionInfo(popup.Widget().Snapshot())
			check(len(info.Children) > 0 && info.Children[0].Text == "div", "HTML proposal missing")
			r := info.Children[0].Bounds
			p := geometry.Point{X: r.X + r.Width/2, Y: r.Y + r.Height/2}
			for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
				must(popup.DispatchEvent(events.PointerEvent{EventType: kind, Button: events.PointerButtonLeft, Position: p}))
			}
			check(d.Model().Text() == "<div" && len(win.Popovers()) == 0, "completion pointer activation failed")
			d.Model().SetText("<di")
			v.SetSelection(textedit.Selection{Anchor: 3, Caret: 3})
			key(events.KeyF2, 0)
			v.SetReadOnly(true)
			check(len(win.Popovers()) == 0, "read-only retained completion")
			v.SetReadOnly(false)
			v.SetCompletionProvider(&source.CompletionProvider{Complete: func(ctx context.Context, c source.CompletionContext) ([]source.Completion, error) {
				// Application mutation belongs on GUI, even from a provider.
				app.Post(func() { d.Model().SetText("changed") })
				return []source.Completion{{Label: "div"}}, nil
			}})
			key(events.KeyF2, 0)
		},
		func() {
			check(len(win.Popovers()) == 0 && d.Model().Text() == "changed", "late completion escaped revision guard")
			v.SetCompletionProvider(nil)
			must(d.SetLanguage(language.JSON))
			d.Model().SetText("tr")
			v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
			key(events.KeyF2, 0)
			scroll.SetChild(nil)
			check(len(win.Popovers()) == 0, "unmount retained completion")
			scroll.SetChild(v)
			log.Print("PASS JSON/HTML completion keyboard/pointer, cancellation, stale asynchronous provider results, pair insertion/overtype/surround/deletion and atomic history")
			completed = true
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
	check(completed, "experience verification aborted before completing all checks")
}

func rasterRegion(layout typography.TextLayout, scale float32) image.Rectangle {
	w, h := layout.MeasureSize()
	return image.Rect(0, 0, int(math.Ceil(float64(w*scale))), int(math.Ceil(float64(h*scale))))
}
