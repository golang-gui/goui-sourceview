// Completion panel verification. Requires a desktop/window manager and native
// fonts. Run: go run ./tests/window/editor/completion [-dark] [-case CASE]
// [-auto]. CASE: json, html, xml, long, unicode, scroll, wrapped, edge.
// Linux without an IME service: XMODIFIERS=@im=none. Initial state: an editable
// sample with a completion popup. Inspect aligned name/detail columns, muted
// smaller descriptions, prefix colors, a 1-DIP outline and ellipsis. Check
// light/dark and 1x/2x. Scroll has 32 items and a position indicator; edge puts
// the owner near the workarea corner, where the popup must remain on screen.
// Up/Down, hover, wheel, Enter/Tab and clicking work without stealing editor
// focus. -auto checks semantic row bounds, stable width while scrolling,
// editor hover retention, keyboard/hover/wheel selection, inert padding,
// pointer acceptance and undo via
// normal Window/Popover.DispatchEvent; READY means setup, not pixel acceptance.
// Native IME and physical shortcut interception require separate verification.
// No clipboard, files or settings change; edge moves only this program's window.
// Restart to reset. Close to exit; closing early with -auto is a failure.
package main

import (
	"context"
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
func selected(info gui.WidgetInfo) int {
	for i, row := range info.Children {
		if row.Selected {
			return i
		}
	}
	return -1
}
func main() {
	runtime.LockOSThread()
	dark := flag.Bool("dark", false, "dark code scheme")
	scenario := flag.String("case", "json", "json/html/xml/long/unicode/scroll/wrapped/edge")
	auto := flag.Bool("auto", false, "verify input and close automatically")
	flag.Parse()
	text, prefix, id := "{\n    \"value\": tr", "tr", language.JSON
	switch *scenario {
	case "json", "long", "scroll":
	case "html":
		text, prefix, id = "<di", "di", language.HTML
	case "xml":
		text, prefix, id = "<node xml:", "xml:", language.XML
	case "unicode":
		text, prefix, id = "中", "中", language.Plain
	case "wrapped":
		text = "tr" + strings.Repeat("a", 100)
		prefix = text
	case "edge":
		text = strings.Repeat("\n", 8) + strings.Repeat(" ", 40) + "tr"
	default:
		log.Fatalf("unknown case %q", *scenario)
	}
	app, err := gui.NewApplication("org.golang-gui.SourceViewCompletionPreview")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 700, Height: 340}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("GOUI SourceView completion preview"))
	doc := source.NewDocument(text)
	must(doc.SetLanguage(id))
	v := source.NewSourceView()
	v.SetDocument(doc)
	v.SetAcceptsTab(true)

	if *scenario == "wrapped" {
		v.SetWrapMode(source.WrapChar)
	}
	var items []source.Completion
	switch *scenario {
	case "html":
		items = []source.Completion{{Label: "dialog", Detail: "HTML"}, {Label: "dir", Detail: "HTML"}}
	case "wrapped":
		items = []source.Completion{{Label: prefix + "Value", Detail: "A wrapped identifier"}, {Label: prefix + "Variant", Detail: "Another wrapped identifier"}}
	case "json", "edge":
		items = []source.Completion{{Label: "transform", Detail: "Apply a transformation to the current value"}, {Label: "translate", Detail: "Translate a value"}, {Label: "trim", Detail: "Remove whitespace"}, {Label: "truncate", Detail: "Limit the length of text"}}
	case "long":
		items = []source.Completion{{Label: "tr" + strings.Repeat("ansformation", 20), Detail: strings.Repeat("A long description 中文 e\u0301 ", 30)}, {Label: "trim", Detail: "Remove whitespace"}}
	case "unicode":
		items = []source.Completion{{Label: "中文标识符", Detail: "中文说明 · e\u0301 · 👩‍💻"}, {Label: "中" + strings.Repeat("文标识符", 20), Detail: "שלום · multilingual description"}}
	case "scroll":
		for i := 0; i < 32; i++ {
			items = append(items, source.Completion{Label: fmt.Sprintf("transform%02d", i), Detail: "document word"})
		}
		items[31].Detail = "A considerably longer description outside the first page"
	}
	if items != nil {
		v.SetCompletionProvider(&source.CompletionProvider{Complete: func(context.Context, source.CompletionContext) ([]source.Completion, error) { return items, nil }})
	}
	caret := len(text)
	v.SetSelection(textedit.Selection{Anchor: caret, Caret: caret})
	scroll := gui.NewScrollView()
	scroll.SetChild(v)
	scroll.SetMainWeight(1)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(gui.NewLabel("Completion · " + *scenario + "   Ctrl+Space · ↑/↓ · Enter/Tab · Esc"))
	box.AddChild(scroll)
	probe := &scaleProbe{}
	box.AddChild(probe)
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(v)
	if *scenario == "edge" {
		area, err := win.WorkAreaAt(geometry.Point{})
		must(err)
		must(win.SetPosition(nil, geometry.Point{X: max(0, area.Width-720), Y: max(0, area.Height-360)}))
	}
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	primary, _ := gui.ModPrimary.Resolve()
	var popup gui.Popover
	var initialWidth float32
	complete := false
	opened := app.NewTimer()
	opened.ConnectTimeout(func() {
		v.RevealSelection()
		key(events.KeySpace, events.ModifierControl)
	})
	must(opened.StartOnce(750 * time.Millisecond))
	ready := app.NewTimer()
	ready.ConnectTimeout(func() {
		check(len(win.Popovers()) == 1, "completion did not open")
		popup = win.Popovers()[0]
		check(popup != nil && win.Focused() && v.Focused() && probe.scale > 0, "native popup/focus/painter not ready")
		info := popup.Widget().Snapshot()
		check(info.Role == "completion-list" && len(info.Children) > 0 && selected(info) >= 0, "completion rows not ready")
		initialWidth = info.Bounds.Width
		if *scenario == "scroll" {
			for i := 0; i < 12; i++ {
				key(events.KeyArrowDown, 0)
			}
			info = popup.Widget().Snapshot()
			check(info.VisibleStart > 0 && info.Bounds.Width == initialWidth, "scroll changed width or hid selection")
		}
		log.Printf("READY case=%s dark=%t typography=%s painter=%s scale=%g width=%g rows=%d first=%d", *scenario, *dark, app.Typography().Name(), probe.painter, probe.scale, initialWidth, len(info.Children), info.VisibleStart)
	})
	must(ready.StartOnce(1250 * time.Millisecond))
	if *auto {
		hover := app.NewTimer()
		hover.ConnectTimeout(func() {
			b := v.Snapshot().Bounds
			must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerMove, Position: geometry.Point{X: b.X + 40, Y: b.Y + 40}}))
		})
		must(hover.StartOnce(1600 * time.Millisecond))
		leave := app.NewTimer()
		leave.ConnectTimeout(func() {
			check(len(win.Popovers()) == 1, "editor hover closed completion")
			must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerMove, Position: geometry.Point{X: 10, Y: 5}}))
		})
		must(leave.StartOnce(2000 * time.Millisecond))
		verify := app.NewTimer()
		verify.ConnectTimeout(func() {
			check(len(win.Popovers()) == 1, "editor hover leave closed completion")
			info := popup.Widget().Snapshot()
			check(info.Bounds.Width == initialWidth, "panel width changed after paint")
			// Padding is inside the native surface, but must not activate a candidate.
			for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
				must(popup.DispatchEvent(events.PointerEvent{EventType: kind, Button: events.PointerButtonLeft, Position: geometry.Point{X: info.Bounds.X + 1, Y: info.Children[0].Bounds.Y + info.Children[0].Bounds.Height/2}}))
			}
			check(len(win.Popovers()) == 1 && doc.Model().Text() == text, "padding accepted a candidate")
			key(events.KeyArrowDown, 0)
			key(events.KeyArrowUp, 0)
			before := selected(popup.Widget().Snapshot())
			check(before >= 0, "keyboard selection disappeared")
			first := info.Children[0].Bounds
			point := geometry.Point{X: first.X + first.Width/2, Y: first.Y + first.Height/2}
			must(popup.DispatchEvent(events.PointerEvent{EventType: events.PointerMove, Position: point}))
			info = popup.Widget().Snapshot()
			check(selected(info) == 0, "hover selection failed")
			must(popup.DispatchEvent(events.WheelEvent{Position: point, DeltaY: 1}))
			info = popup.Widget().Snapshot()
			check(selected(info) == 1, "wheel selection failed")
			target := info.Children[1]
			point = geometry.Point{X: target.Bounds.X + target.Bounds.Width/2, Y: target.Bounds.Y + target.Bounds.Height/2}
			check(v.Focused() && win.FocusedWidget() == v, "popup stole editor focus")
			for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
				must(popup.DispatchEvent(events.PointerEvent{EventType: kind, Button: events.PointerButtonLeft, Position: point}))
			}
			expected := text[:caret-len(prefix)] + target.Text
			check(len(win.Popovers()) == 0 && doc.Model().Text() == expected, "pointer completion inserted wrong text")
			key(events.KeyZ, primary)
			check(doc.Model().Text() == text, "completion undo failed")
			log.Print("PASS stable size, editor hover retention, keyboard/hover/wheel, padding, pointer acceptance, focus and undo")
			complete = true
			win.Destroy()
		})
		must(verify.StartOnce(2800 * time.Millisecond))
	}
	app.Run()
	if *auto && !complete {
		log.Fatal("verification aborted")
	}
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
