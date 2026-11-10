// Multiline host-provided inline suggestions. Requires a native desktop/WM/fonts.
// Run: XMODIFIERS=@im=none go run ./tests/window/editor/inline_multiline [-dark] [-basic].
// Two views initially share "func demo() {\nTAIL"; only the left view publishes
// suggestions. Timers publish through the public setter and send normal Window
// Tab/Escape/undo/redo/text/pointer events. Assert view-local measured height,
// unchanged source/export/IME/history, real line numbers, no overlapping tail,
// literal one-edit acceptance, dismissal, native CJK/combining wrap and resize,
// scrolling while the caret is above the viewport, shifted-source hit testing,
// typing/focus/external-edit cleanup, and nowrap horizontal extent restoration.
// A forwarding Painter checks actual ghost draws, gray color and viewport clip;
// after-paint footer tags allow independent X11 screenshot capture.
// PASS closes; timeout/early close/assertion fails. Restart resets fixtures.
// No network, user files, clipboard or settings are changed. Physical IME and
// other native operating systems require separate runs; unit tests cover IME.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"runtime"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
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
func check(ok bool, why string) {
	if !ok {
		panic(why)
	}
}
func closeTo(a, b float32) bool { return math.Abs(float64(a-b)) < .1 }

type footer struct{ gui.WidgetBase }

func (f *footer) Measure(c layout.Constraint) layout.Measurement {
	return layout.Measured(c.Clamp(geometry.Size{Width: 1, Height: 26}))
}
func (f *footer) Paint(p gui.Painter) {
	p.FillRect(geometry.Rect(0, 0, f.Rect().Width, f.Rect().Height), graphics.ColorOf(color.NRGBA{R: 60, G: 60, B: 60, A: 255}))
}
func meta(v *source.SourceView) map[string]any {
	m, _ := v.Snapshot().Attributes["goui.sourceview"].(map[string]any)["inlineSuggestion"].(map[string]any)
	return m
}

type drawRecord struct {
	Text  string
	At    geometry.Point
	Clip  geometry.Rectangle
	Ghost bool
}
type observedView struct {
	*source.SourceView
	draws   []drawRecord
	checked int
}
type observePainter struct {
	gui.Painter
	view        *observedView
	first, rest string
	active      bool
	clip        geometry.Rectangle
	stack       []geometry.Rectangle
}

func (p *observePainter) Save() { p.stack = append(p.stack, p.clip); p.Painter.Save() }
func (p *observePainter) Restore() {
	p.clip = p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
	p.Painter.Restore()
}
func (p *observePainter) SetClipRect(r geometry.Rectangle) { p.clip = r; p.Painter.SetClipRect(r) }
func (p *observePainter) DrawTextLayout(at geometry.Point, l typography.TextLayout) {
	text := l.Text()
	ghost := p.active && (p.first != "" && text == p.first || strings.HasSuffix(text, p.rest) && p.rest != "")
	if ghost {
		check(p.clip.Width > 0 && p.clip.Height > 0 && p.clip.Y >= 0 && p.clip.Y+p.clip.Height <= p.view.Rect().Height+.1, "ghost escaped viewport")
		c := color.NRGBAModel.Convert(l.Format().TextColor).(color.NRGBA)
		check(c.A > 0 && max(c.R, c.G, c.B)-min(c.R, c.G, c.B) < 40, "ghost is not a quiet gray")
		p.view.checked++
	}
	p.view.draws = append(p.view.draws, drawRecord{text, at, p.clip, ghost})
	p.Painter.DrawTextLayout(at, l)
}
func (v *observedView) Paint(p gui.Painter) {
	v.draws = nil
	first, rest := "", ""
	m := meta(v.SourceView)
	active := m != nil && m["visible"] == true
	if active {
		first, rest, _ = strings.Cut(m["text"].(string), "\n")
	}
	draw := &observePainter{Painter: p, view: v, first: first, rest: rest, active: active}
	v.SourceView.Paint(draw)
	check(len(draw.stack) == 0, "painter state not restored")
	if active {
		seen := false
		tailY := float32(-1)
		for _, r := range v.draws {
			if r.Text == "TAIL" {
				tailY = r.At.Y
			}
			seen = seen || r.Ghost
		}
		check(seen, "visible suggestion has no actual ghost draw")
		if tailY >= 0 {
			for _, r := range v.draws {
				if r.Ghost {
					check(r.Clip.Y+r.Clip.Height <= tailY+.1, "preview overlaps following source")
				}
			}
		}
	}
}
func (v *observedView) at(text string) geometry.Point {
	for _, r := range v.draws {
		if r.Text == text {
			return r.At
		}
	}
	panic("missing actual draw: " + text)
}
func main() {
	runtime.LockOSThread()
	dark, basicTheme := flag.Bool("dark", false, "Modern dark"), flag.Bool("basic", false, "Basic")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewInlineMultiline")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	if *basicTheme {
		app.SetStyleSheet(basic.Sheet())
	}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1000, Height: 560}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView inline multiline"))
	const original = "func demo() {\nTAIL"
	const position = len("func demo() {")
	doc := source.NewDocument(original)
	v, other := source.NewSourceView(), source.NewSourceView()
	observed := &observedView{SourceView: v}
	hosts := []*gui.ScrollView{gui.NewScrollView(), gui.NewScrollView()}
	for i, view := range []*source.SourceView{v, other} {
		view.SetDocument(doc)
		view.SetAutomaticCompletion(false)
		view.SetWrapMode(source.WrapWordChar)
		view.ConnectError(func(err error) { panic(err) })
		hosts[i].SetStyleName("source-view")
	}
	hosts[0].SetChild(observed)
	hosts[1].SetChild(other)
	hosts[0].SetMainWeight(1)
	setPane := func(w float32) {
		hosts[1].SetMinSize(geometry.Size{Width: w})
		hosts[1].SetMaxSize(geometry.Size{Width: w})
	}
	setPane(250)
	row := gui.NewLinearBox(layout.DirectionHorizontal)
	row.SetCrossAlign(layout.CrossStretch)
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
	v.SetInlineSuggestionsEnabled(true)
	v.SetSelection(textedit.Selection{Anchor: position, Caret: position})
	publish := func(text string) {
		ok, err := v.SetInlineSuggestion(doc, doc.Model().Revision(), source.InlineSuggestion{Position: v.Selection().Caret, Text: text})
		must(err)
		check(ok, "host suggestion rejected")
	}
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	click := func(point geometry.Point) {
		for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
			must(win.DispatchEvent(events.PointerEvent{EventType: kind, Button: events.PointerButtonLeft, Position: point}))
		}
	}
	reset := func() {
		v.ClearInlineSuggestion()
		doc.Model().SetText(original)
		hosts[0].SetScrollY(0)
		hosts[0].SetScrollX(0)
		v.SetSelection(textedit.Selection{Anchor: position, Caret: position})
		win.SetFocusedWidget(v)
	}
	frame, phase := 0, "initial"
	enc := json.NewEncoder(os.Stdout)
	win.ConnectAfterPaint(func(p gui.Painter, b geometry.Rectangle) {
		frame++
		var draws []drawRecord
		if hosts[0].Child() == observed {
			draws = observed.draws
		}
		must(enc.Encode(map[string]any{"event": "paint", "frame": frame, "phase": phase, "editor": v.Snapshot().Bounds, "preview": meta(v), "content": v.ContentSize(), "scrollY": hosts[0].ScrollY(), "maxY": hosts[0].Snapshot().MaxScrollY, "draws": draws, "scale": p.PixelScale()}))
		p.FillRect(geometry.Rect(6, b.Height-22, 3, 16), graphics.ColorOf(color.NRGBA{R: 255, B: 255, A: 255}))
		for bit := range 16 {
			c := color.NRGBA{A: 255}
			if frame&(1<<bit) != 0 {
				c.R, c.G, c.B = 255, 255, 255
			}
			p.FillRect(geometry.Rect(12+float32(bit)*4, b.Height-22, 4, 16), graphics.ColorOf(c))
		}
	})
	var steps []func() bool
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
	visible := func() { check(meta(v) != nil && meta(v)["visible"] == true, "multiline preview not visible") }
	clean := func() {
		check(doc.Model().Text() == original && doc.Model().LineCount() == 2 && !doc.Model().CanUndo(), "preview changed source/history")
		check(meta(other) == nil, "preview leaked into shared view")
	}
	var baseY, baseHeight, wideHeight float32
	const suggestion = "\n\treturn value\n}"
	settle(func() {
		baseY = observed.at("TAIL").Y
		baseHeight = v.ContentSize().Height
		publish(suggestion)
		phase = "multiline"
	})
	settle(func() {
		visible()
		clean()
		check(meta(v)["lineCount"] == 3, "logical suggestion line count lost")
		check(observed.at("TAIL").Y > baseY && closeTo(observed.at("2").Y, observed.at("TAIL").Y), "real line number did not follow shifted source")
		check(v.ContentSize().Height > other.ContentSize().Height, "preview extent leaked or did not reserve height")
		var saved bytes.Buffer
		_, err := doc.WriteTo(&saved)
		must(err)
		check(saved.String() == original && v.Snapshot().TextEditing.Preedit == nil && !strings.Contains(v.Snapshot().TextEditing.VisibleText, "return value"), "preview entered source or IME")
		key(events.KeyEscape, 0)
		phase = "dismissed"
	})
	settle(func() {
		clean()
		check(meta(v) == nil && closeTo(observed.at("TAIL").Y, baseY) && v.ContentSize().Height == baseHeight, "cancel retained virtual extent")
		publish(suggestion)
		phase = "before-accept"
	})
	settle(func() { visible(); key(events.KeyTab, 0); phase = "accepted" })
	settle(func() {
		check(doc.Model().Text() == "func demo() {\n\treturn value\n}\nTAIL" && doc.Model().LineCount() == 4 && meta(v) == nil, "Tab failed exact multiline insertion")
		key(events.KeyZ, events.ModifierControl)
		phase = "undo"
	})
	settle(func() { clean(); key(events.KeyZ, events.ModifierControl|events.ModifierShift); phase = "redo" })
	settle(func() {
		check(doc.Model().LineCount() == 4, "redo failed")
		reset()
		publish(strings.Repeat("中文e\u0301 ", 100) + suggestion)
		phase = "wrapped"
	})
	settle(func() { visible(); clean(); wideHeight = v.ContentSize().Height; setPane(500); phase = "narrow" })
	settle(func() {
		visible()
		check(v.ContentSize().Height >= wideHeight, "narrower native wrap reduced preview height")
		setPane(250)
		phase = "wide"
	})
	settle(func() {
		visible()
		check(closeTo(v.ContentSize().Height, wideHeight), "restoring width did not restore measured height")
		v.ClearInlineSuggestion()
		publish("\n" + strings.Repeat("virtual line\n", 40) + "end")
		phase = "long"
	})
	settle(func() {
		visible()
		check(hosts[0].Snapshot().MaxScrollY > 0, "multiline extent did not create scroll range")
		hosts[0].SetScrollY(180)
		phase = "scrolled"
	})
	settle(func() {
		visible()
		check(hosts[0].ScrollY() > 100, "virtual continuation cannot scroll")
		key(events.KeyTab, 0)
		phase = "scrolled-accepted"
	})
	settle(func() {
		check(doc.Model().LineCount() == 43 && meta(v) == nil, "visible continuation could not be accepted with caret above viewport")
		key(events.KeyZ, events.ModifierControl)
		phase = "scrolled-undo"
	})
	settle(func() {
		clean()
		check(hosts[0].ScrollY() == 0 && hosts[0].Snapshot().MaxScrollY == 0, "undo retained virtual scrolling")
		// Use the actual widget for pointer tests: embedding it in a forwarding
		// Paint observer changes pointer-focus identity in GOUI's event tree.
		hosts[0].SetChild(v)
		reset()
		publish(suggestion)
		phase = "before-hit"
	})
	settle(func() {
		visible()
		bounds := v.Snapshot().Bounds
		tailY := baseY + v.ContentSize().Height - baseHeight
		click(geometry.Point{X: bounds.X + 60, Y: bounds.Y + tailY + 8})
		phase = "hit-tail"
	})
	settle(func() {
		clean()
		check(meta(v) == nil && v.Selection().Caret >= position+1 && v.Selection().Caret <= len(original), "shifted source click mapped into ghost")
		reset()
		publish(suggestion)
		phase = "before-virtual-hit"
	})
	var heldPoint geometry.Point
	settle(func() {
		visible()
		bounds := v.Snapshot().Bounds
		r := meta(v)["bounds"].(geometry.Rectangle)
		heldPoint = geometry.Point{X: bounds.X + 80, Y: r.Y + 5}
		must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerDown, Button: events.PointerButtonLeft, Position: heldPoint}))
		check(v.Selection().Caret == position, "virtual press did not hit insertion point")
		phase = "virtual-held"
	})
	settle(func() {
		must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerMove, Position: heldPoint}))
		must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerUp, Button: events.PointerButtonLeft, Position: heldPoint}))
		phase = "virtual-hit"
	})
	settle(func() {
		clean()
		check(meta(v) == nil && v.Selection().Caret == position, fmt.Sprintf("virtual-only hit produced a ghost model position: %+v", v.Selection()))
		hosts[0].SetChild(observed)
		reset()
		publish(suggestion)
		phase = "before-type"
	})
	settle(func() { visible(); must(win.CommitText("x")); phase = "typed" })
	settle(func() {
		check(doc.Model().Text() == "func demo() {x\nTAIL" && meta(v) == nil, "typing retained preview")
		reset()
		publish(suggestion)
		phase = "before-external"
	})
	settle(func() { visible(); must(doc.Model().Replace(textedit.Range{}, "X")); phase = "external" })
	settle(func() {
		check(meta(v) == nil && closeTo(observed.at("TAIL").Y, baseY), "external edit retained preview height")
		reset()
		publish(suggestion)
		phase = "before-blur"
	})
	settle(func() { visible(); win.SetFocusedWidget(other); phase = "blurred" })
	settle(func() {
		check(meta(v) == nil && v.ContentSize().Height == baseHeight, "blur retained virtual height")
		win.SetFocusedWidget(v)
		v.SetWrapMode(source.WrapNone)
		phase = "nowrap-base"
	})
	var baseWidth float32
	settle(func() {
		baseWidth = v.ContentSize().Width
		publish("suffix\n" + strings.Repeat("X", 300))
		phase = "nowrap"
	})
	settle(func() {
		visible()
		check(v.ContentSize().Width > baseWidth, "nowrap omitted preview horizontal extent")
		key(events.KeyEscape, 0)
		phase = "nowrap-dismissed"
	})
	settle(func() {
		check(v.ContentSize().Width == baseWidth && closeTo(observed.at("TAIL").Y, baseY), "nowrap dismissal retained extent")
	})
	index, done := 0, false
	deadline := time.Now().Add(50 * time.Second)
	timer := app.NewTimer()
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), fmt.Sprintf("timeout at %d phase %s", index, phase))
		must(win.RequestPaint())
		if index < len(steps) {
			if steps[index]() {
				index++
			}
			return
		}
		timer.Stop()
		done = true
		log.Printf("PASS multiline inline: %d actual ghost draws; extent, shared view, source/history/line numbers, Tab/Esc/undo/redo, native wrap/resize, scrolling, hits, input/edit/focus, nowrap; typography=%s basic=%t dark=%t", observed.checked, app.Typography().Name(), *basicTheme, *dark)
		win.RequestClose()
	})
	must(timer.Start(100 * time.Millisecond))
	app.Run()
	check(done, "probe interrupted")
}
