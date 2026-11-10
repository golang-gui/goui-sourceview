// Host-provided inline suffix verification. Requires a native desktop/WM/fonts.
// Run: XMODIFIERS=@im=none go run ./tests/window/editor/inline_suggestion [-dark] [-basic].
// Initially two views share "fmt.Pr"; the left view owns a fixed host suggestion.
// Timers publish results through the public GUI-sequence setter, then dispatch
// normal Window Tab/Escape/undo/redo/text input. Assert default off, view-local
// preview, no source/history/IME/export changes, one-edit accept, ordinary Tab
// fallback, stale/foreign rejection, focus/readonly/disabled/reset cleanup,
// CJK literals, cluster-safe truncation, resizing, detach/reattach and completion
// list priority. A forwarding Painter observes every actual ghost draw, its gray
// color and clip. The footer tags frames for optional independent X11 capture.
// PASS closes; timeout/early close/assertion fails. Restart resets all fixtures.
// No network, files, clipboard or user settings change. IME is covered by unit
// projection tests; physical IME and other native OSes require separate runs.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image/color"
	"log"
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

type ghostPainter struct {
	gui.Painter
	target string
	clip   geometry.Rectangle
	stack  []geometry.Rectangle
	seen   bool
}

func (p *ghostPainter) Save() { p.stack = append(p.stack, p.clip); p.Painter.Save() }
func (p *ghostPainter) Restore() {
	p.clip = p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
	p.Painter.Restore()
}
func (p *ghostPainter) SetClipRect(r geometry.Rectangle) { p.clip = r; p.Painter.SetClipRect(r) }
func (p *ghostPainter) DrawTextLayout(at geometry.Point, l typography.TextLayout) {
	if p.target != "" && l.Text() == p.target {
		check(p.clip.Width > 0 && p.clip.Height > 0 && p.clip.Height < 40, "ghost escaped current display row")
		c := color.NRGBAModel.Convert(l.Format().TextColor).(color.NRGBA)
		check(c.A > 0 && max(c.R, c.G, c.B)-min(c.R, c.G, c.B) < 40, "ghost is not a quiet gray")
		p.seen = true
	}
	p.Painter.DrawTextLayout(at, l)
}

type observedView struct {
	*source.SourceView
	checked int
}

func (v *observedView) Paint(p gui.Painter) {
	target := ""
	if meta, ok := v.Snapshot().Attributes["goui.sourceview"].(map[string]any)["inlineSuggestion"].(map[string]any); ok && meta["visible"] == true {
		target = meta["text"].(string)
	}
	draw := &ghostPainter{Painter: p, target: target}
	v.SourceView.Paint(draw)
	if target != "" {
		check(draw.seen, "visible inline suggestion was not painted")
		v.checked++
	}
	check(len(draw.stack) == 0, "ghost painter left saved state")
}
func meta(v *source.SourceView) map[string]any {
	m, _ := v.Snapshot().Attributes["goui.sourceview"].(map[string]any)["inlineSuggestion"].(map[string]any)
	return m
}
func main() {
	runtime.LockOSThread()
	dark, basicTheme := flag.Bool("dark", false, "Modern dark"), flag.Bool("basic", false, "Basic")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewInlineSuggestion")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	if *basicTheme {
		app.SetStyleSheet(basic.Sheet())
	}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1000, Height: 560}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView inline suggestion"))
	doc := source.NewDocument("fmt.Pr")
	v, other := source.NewSourceView(), source.NewSourceView()
	observed := &observedView{SourceView: v}
	v.SetDocument(doc)
	other.SetDocument(doc)
	v.SetAutomaticCompletion(false)
	other.SetAutomaticCompletion(false)
	v.SetAcceptsTab(true)
	v.SetWrapMode(source.WrapWordChar)
	v.ConnectError(func(err error) { panic(err) })
	hosts := []*gui.ScrollView{gui.NewScrollView(), gui.NewScrollView()}
	hosts[0].SetChild(observed)
	hosts[1].SetChild(other)
	for _, h := range hosts {
		h.SetStyleName("source-view")
	}
	hosts[0].SetMainWeight(1)
	setPane := func(width float32) {
		hosts[1].SetMinSize(geometry.Size{Width: width})
		hosts[1].SetMaxSize(geometry.Size{Width: width})
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
	caretEnd := func() { at := v.Document().Model().Len(); v.SetSelection(textedit.Selection{Anchor: at, Caret: at}) }
	caretEnd()
	suffix := `intln("hello")`
	publish := func(text string) {
		ok, err := v.SetInlineSuggestion(v.Document(), v.Document().Model().Revision(), source.InlineSuggestion{Position: v.Selection().Caret, Text: text})
		must(err)
		check(ok, "host result was rejected")
	}
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	frame, phase := 0, "initial"
	enc := json.NewEncoder(os.Stdout)
	win.ConnectAfterPaint(func(p gui.Painter, b geometry.Rectangle) {
		frame++
		must(enc.Encode(map[string]any{"event": "paint", "frame": frame, "phase": phase, "editor": v.Snapshot().Bounds, "preview": meta(v), "scale": p.PixelScale()}))
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
	visible := func() { check(meta(v) != nil && meta(v)["visible"] == true, "ghost preview not visible") }
	unchanged := func() {
		check(doc.Model().Text() == "fmt.Pr" && !doc.Model().CanUndo(), "preview changed source/history")
		check(other.Snapshot().Attributes["goui.sourceview"].(map[string]any)["inlineSuggestion"] == nil, "ghost leaked to shared view")
	}
	settle(func() {
		check(!v.InlineSuggestionsEnabled(), "inline feature should default off")
		ok, err := v.SetInlineSuggestion(doc, doc.Model().Revision(), source.InlineSuggestion{Position: 6, Text: suffix})
		must(err)
		check(!ok, "default-off feature accepted text")
		v.SetInlineSuggestionsEnabled(true)
		publish(suffix)
		phase = "ghost"
	})
	settle(func() {
		visible()
		unchanged()
		var saved bytes.Buffer
		_, err := doc.WriteTo(&saved)
		must(err)
		check(saved.String() == "fmt.Pr" && v.Snapshot().TextEditing.Preedit == nil, "ghost leaked into export or IME")
		key(events.KeyTab, 0)
		phase = "accepted"
	})
	settle(func() {
		check(doc.Model().Text() == `fmt.Println("hello")` && meta(v) == nil, "Tab failed exact accept")
		key(events.KeyZ, events.ModifierControl)
		phase = "undo"
	})
	settle(func() { unchanged(); key(events.KeyZ, events.ModifierControl|events.ModifierShift); phase = "redo" })
	settle(func() {
		check(doc.Model().Text() == `fmt.Println("hello")`, "redo failed")
		key(events.KeyZ, events.ModifierControl)
		phase = "restore"
	})
	settle(func() { publish(suffix); phase = "before-escape" })
	settle(func() { visible(); key(events.KeyEscape, 0); phase = "dismissed" })
	settle(func() {
		unchanged()
		check(meta(v) == nil, "Escape failed dismissal")
		key(events.KeyTab, 0)
		phase = "ordinary-tab"
	})
	settle(func() {
		check(doc.Model().Text() == "fmt.Pr  ", fmt.Sprintf("ordinary Tab did not reach the next tab stop: %q", doc.Model().Text()))
		key(events.KeyZ, events.ModifierControl)
		phase = "before-input"
	})
	settle(func() { publish(suffix); phase = "before-typing" })
	settle(func() { must(win.CommitText("i")); phase = "typed" })
	settle(func() {
		check(doc.Model().Text() == "fmt.Pri" && meta(v) == nil, "typed input retained stale ghost")
		old := doc.Model().Revision()
		doc.Model().SetText("fmt.Pr")
		caretEnd()
		ok, err := v.SetInlineSuggestion(doc, old, source.InlineSuggestion{Position: 6, Text: suffix})
		must(err)
		check(!ok, "stale result accepted")
		publish(suffix)
		phase = "before-readonly"
	})
	settle(func() { visible(); v.SetReadOnly(true); phase = "readonly" })
	settle(func() {
		check(meta(v) == nil, "readonly retained ghost")
		v.SetReadOnly(false)
		publish(suffix)
		phase = "before-disable"
	})
	settle(func() { visible(); v.SetEnabled(false); phase = "disabled" })
	settle(func() {
		check(meta(v) == nil, "disabled retained ghost")
		v.SetEnabled(true)
		win.SetFocusedWidget(v)
		publish(suffix)
		phase = "before-blur"
	})
	settle(func() { visible(); win.SetFocusedWidget(other); phase = "blurred" })
	settle(func() {
		check(meta(v) == nil, "blur retained ghost")
		win.SetFocusedWidget(v)
		doc.Model().SetText("变量")
		caretEnd()
		publish("名称")
		phase = "cjk"
	})
	settle(func() {
		visible()
		check(doc.Model().Text() == "变量", "CJK preview changed source")
		key(events.KeyTab, 0)
		phase = "cjk-accepted"
	})
	settle(func() {
		check(doc.Model().Text() == "变量名称" && v.Selection().Caret == len("变量名称"), "CJK literal accept failed")
		doc.Model().SetText("fmt.Pr")
		caretEnd()
		publish(strings.Repeat("中文e\u0301", 120))
		phase = "truncated"
	})
	settle(func() {
		visible()
		check(meta(v)["truncated"] == true, "long suggestion not truncated")
		setPane(450)
		phase = "narrow"
	})
	settle(func() {
		visible()
		check(meta(v)["truncated"] == true, "narrow preview lost truncation")
		setPane(250)
		phase = "wide"
	})
	settle(func() {
		visible()
		v.ClearInlineSuggestion()
		v.SetCompletionProvider(&source.CompletionProvider{Complete: func(context.Context, source.CompletionContext) ([]source.Completion, error) {
			return []source.Completion{{Label: "Println", Text: "Println"}}, nil
		}})
		publish(suffix)
		must(v.ShowCompletion())
		phase = "completion"
	})
	steps = append(steps, func() bool {
		if len(win.Popovers()) == 0 {
			return false
		}
		check(meta(v) == nil, "completion list left ghost visible")
		key(events.KeyTab, 0)
		phase = "list-accepted"
		return true
	})
	settle(func() {
		check(doc.Model().Text() == "fmt.Println", "Tab accepted ghost instead of list item")
		v.SetCompletionProvider(nil)
		doc.Model().SetText("fmt.Pr")
		caretEnd()
		publish(suffix)
		phase = "before-detach"
	})
	settle(func() {
		hosts[0].SetChild(nil)
		check(meta(v) == nil, "detach retained ghost")
		hosts[0].SetChild(observed)
		win.SetFocusedWidget(v)
		phase = "reattached"
	})
	settle(func() { publish(suffix); phase = "final-ghost" })
	settle(func() {
		visible()
		old := doc
		next := source.NewDocument("fmt.Pr")
		v.SetDocument(next)
		caretEnd()
		ok, err := v.SetInlineSuggestion(old, old.Model().Revision(), source.InlineSuggestion{Position: 6, Text: suffix})
		must(err)
		check(!ok && meta(v) == nil, "foreign document result accepted")
		phase = "foreign-rejected"
	})
	index, done := 0, false
	deadline := time.Now().Add(45 * time.Second)
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
		log.Printf("PASS inline suggestion: %d actual ghost paints; default off, view-local preview, Tab/Escape/undo/redo/input, stale/foreign results, readonly/disabled/focus, CJK, clipping/resize, completion priority, reattach; typography=%s basic=%t dark=%t", observed.checked, app.Typography().Name(), *basicTheme, *dark)
		win.RequestClose()
	})
	must(timer.Start(100 * time.Millisecond))
	app.Run()
	check(done, "probe interrupted")
}
