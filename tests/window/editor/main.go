// SourceView desktop verification. Requires a real desktop and native fonts.
// Run: go run ./tests/window/editor [-auto]. Linux without an input-method
// service: XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software go run ./tests/window/editor -auto.
// Record OS, typography backend, actual display scale and painter preference.
// -auto dispatches keyboard events and CommitText through the normal Window
// path, asserts text/history, focus traversal, pointer selection, reveal,
// shared documents, detach/reattach and color-only native layout reuse, then
// exits nonzero on failure. Recolor checks use borrowed native layouts observed
// through a context wrapper; no widget-private state is exported.
// Manual: type Chinese/emoji with a real IME, replace a selection, inspect the
// candidate position, cancel with Escape, then commit and undo once. Switch
// focus mid-composition and close while preediting. These need native testing;
// CommitText is not an IME simulation. Use examples/editor for three-language
// visual inspection, themes, search, wrap and line numbers at 1x and 2x.
// Initial: a JSON document, editor between two focusable buttons. AcceptsTab
// is enabled; Escape then Tab must reach the following button. Read-only must
// allow selection/copy and reject typing, paste, deletion and history mutation.
// Clipboard is untouched by default. Add -clipboard only with disposable text:
// it writes test text, and X11 may clear it when the owning process exits.
// No files or settings are changed. Reset by restarting. Close the window or
// press Ctrl+C in the launching shell to exit the manual mode.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"os"
	"runtime"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/typography"
)

func main() {
	runtime.LockOSThread()
	auto := flag.Bool("auto", false, "assert normal event-path integration and exit")
	clipboardTest := flag.Bool("clipboard", false, "exercise native cut/paste; overwrites the clipboard with disposable test text")
	flag.Parse()
	app, err := gui.NewApplication("org.golang-gui.SourceViewWindowTest")
	must(err)
	observed := &observedTypography{Context: app.Typography(), created: make(map[string]int), last: make(map[string]typography.TextLayout)}
	gui.App = observedApplication{Application: app, typography: observed}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 850, Height: 550}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("GOUI SourceView window verification"))
	document := source.NewDocument("{\n    \"hello\": \"中文 é 👩‍💻\",\n    \"ready\": true\n}\n")
	must(document.SetLanguage(language.JSON))
	editor := source.NewSourceView()
	editor.SetDocument(document)
	editor.SetAcceptsTab(true)
	editor.SetID("source")
	second := source.NewSourceView()
	second.SetDocument(document)
	second.SetReadOnly(true)
	scroll := gui.NewScrollView()
	scroll.SetChild(editor)
	scroll.SetMainWeight(1)
	before, after := gui.NewButton(), gui.NewButton()
	before.SetChild(gui.NewLabel("Before"))
	after.SetChild(gui.NewLabel("After"))
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(&paintProbe{})
	box.AddChild(before)
	box.AddChild(scroll)
	box.AddChild(after)
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(editor)
	log.Printf("typography=%s DISPLAY=%s painter preference=%s scale preference=%s", app.Typography().Name(), os.Getenv("DISPLAY"), os.Getenv("GOUI_PLAT_PAINTER"), os.Getenv("GOUI_PLAT_SCALE"))
	if *auto {
		timer := app.NewTimer()
		timer.ConnectTimeout(func() {
			// The native event loop has completed initial layout and painting.
			check := func(ok bool, format string, args ...any) {
				if !ok {
					log.Fatalf(format, args...)
				}
			}
			key := func(k events.Key, mods events.Modifiers) {
				must(win.DispatchEvent(events.KeyEvent{EventType: events.KeyDown, Key: k, Modifiers: mods}))
				must(win.DispatchEvent(events.KeyEvent{EventType: events.KeyUp, Key: k, Modifiers: mods}))
			}
			primary, _ := gui.ModPrimary.Resolve()
			// Recolor the already displayed line without changing its text/font.
			probe, _ := document.Model().LineRange(2)
			probeText, err := document.Model().Slice(probe)
			must(err)
			original := observed.last[probeText]
			check(original != nil && win.Focused(), "recolor probe has no active native window/layout")
			created := observed.created[probeText]
			for i := 0; i < 20; i++ {
				base := []*scheme.Scheme{scheme.Light(), scheme.Dark()}[i%2]
				def := base.Definition()
				def.SelectionForeground = color.NRGBA{R: 0x11, G: 0x11, B: 0x11, A: 255}
				if i%2 == 1 {
					def.SelectionForeground = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
				}
				s, err := scheme.New(def)
				must(err)
				editor.SetScheme(s)
				editor.SetSelection(textedit.Selection{Anchor: probe.Start, Caret: probe.End})
				editor.RevealSelection()
				check(observed.last[probeText] == original && observed.created[probeText] == created, "color/selection rebuilt native line")
				format := original.Format()
				format.TextColor = s.Definition().SelectionForeground
				fresh, err := observed.Context.NewTextLayout(probeText, format, 1<<20, 1<<24)
				must(err)
				got, err := original.Rasterize(1, nil)
				must(err)
				pixels := bytes.Clone(got.Pixels)
				want, err := fresh.Rasterize(1, nil)
				must(err)
				check(got.Width == want.Width && got.Height == want.Height && got.Stride == want.Stride && bytes.Equal(pixels, want.Pixels), "active selection retained obsolete foreground")
				fresh.Destroy()
			}
			editor.SetScheme(nil)
			log.Print("PASS native recolor reuse and active selection foreground")
			document.Model().SetText("a\nb\n")
			editor.SetSelection(textedit.Selection{})
			must(win.CommitText("x"))
			check(document.Model().Text() == "xa\nb\n", "ordinary commit: %q", document.Model().Text())
			key(events.KeyZ, primary)
			check(document.Model().Text() == "a\nb\n", "undo")
			key(events.KeyZ, primary|events.ModifierShift)
			check(document.Model().Text() == "xa\nb\n", "redo")
			editor.SetReadOnly(true)
			check(win.CommitText("bad") != nil, "read-only exposed text input")
			key(events.KeyBackspace, 0)
			check(document.Model().Text() == "xa\nb\n", "read only changed")
			editor.SetReadOnly(false)
			key(events.KeyA, primary)
			key(events.KeyTab, 0)
			check(document.Model().Text() == "    xa\n    b\n", "multiline indent %q", document.Model().Text())
			key(events.KeyZ, primary)
			check(document.Model().Text() == "xa\nb\n", "atomic indent undo")
			key(events.KeyEscape, 0)
			key(events.KeyTab, 0)
			check(win.FocusedWidget() == after, "Escape/Tab did not leave editor")
			win.SetFocusedWidget(editor)
			editor.SetAcceptsTab(false)
			key(events.KeyTab, 0)
			check(win.FocusedWidget() == after, "default Tab traversal")
			editor.SetAcceptsTab(true)
			win.SetFocusedWidget(editor)
			for _, id := range []language.ID{language.JSON, language.XML, language.HTML} {
				must(document.SetLanguage(id))
			}
			editor.SetSearch(source.SearchQuery{Text: "b"})
			r, ok := editor.FindNext(false)
			check(ok && r == (textedit.Range{Start: 3, End: 4}), "find %v %v", r, ok)
			replaced, err := editor.ReplaceCurrent("β")
			must(err)
			check(replaced && document.Model().Text() == "xa\nβ\n", "replace")
			// Snapshot and view lifetime use the same public document.
			info := editor.Snapshot()
			check(info.TextEditing != nil && info.TextEditing.VisibleText == "xa\nβ\n", "snapshot %+v", info)
			scroll.SetChild(nil)
			var numbered strings.Builder
			for row := 0; row < 2000; row++ {
				fmt.Fprintf(&numbered, "<p>line %04d</p>\n", row)
			}
			document.Model().SetText(numbered.String())
			scroll.SetChild(editor)
			// Bind a second native view briefly; unloading it must not stop the first.
			otherScroll := gui.NewScrollView()
			otherScroll.SetChild(second)
			box.AddChild(otherScroll)
			box.RemoveChild(otherScroll)
			end := document.Model().Len()
			editor.SetSelection(textedit.Selection{Anchor: end, Caret: end})
			editor.RevealSelection()
			timer2 := app.NewTimer()
			timer2.ConnectTimeout(func() {
				check(scroll.ScrollY() > 0, "end caret was not revealed; scroll=%g", scroll.ScrollY())
				info := editor.Snapshot()
				check(info.TextEditing != nil && info.TextEditing.Selection.Caret == end, "end selection")
				// A real pointer press must move selection through gesture dispatch.
				bounds := info.Bounds
				for _, eventType := range []events.EventType{events.PointerDown, events.PointerUp} {
					must(win.DispatchEvent(events.PointerEvent{EventType: eventType, Position: geometry.Point{X: bounds.X + 80, Y: bounds.Y + 15}, Button: events.PointerButtonLeft}))
				}
				check(editor.Selection().Caret < end, "pointer did not select")
				scroll.SetScrollY(12000)
				timer3 := app.NewTimer()
				timer3.ConnectTimeout(func() {
					old := editor.Snapshot().TextEditing
					inserted := "<!-- inserted -->\n"
					must(document.Model().Replace(textedit.Range{}, inserted))
					timer4 := app.NewTimer()
					timer4.ConnectTimeout(func() {
						current := editor.Snapshot().TextEditing
						check(current.VisibleRange.Start == old.VisibleRange.Start+len(inserted), "viewport anchor moved after insert: %v -> %v", old.VisibleRange, current.VisibleRange)
						check(strings.HasPrefix(current.VisibleText, strings.Split(old.VisibleText, "\n")[0]), "anchor text changed")

						if *clipboardTest {
							document.Model().SetText("sourceview clipboard")
							win.SetFocusedWidget(editor)
							key(events.KeyA, primary)
							key(events.KeyX, primary)
							cutOK := document.Model().Text() == ""
							key(events.KeyV, primary)
							pasteOK := document.Model().Text() == "sourceview clipboard"
							check(cutOK && pasteOK, "native clipboard cut=%v paste=%v", cutOK, pasteOK)
							log.Print("PASS native clipboard cut/paste with disposable test text")
						}
						log.Print("PASS normal input, history, indent, readonly, Tab, search, snapshot, shared document, remount, reveal, pointer, viewport anchor")
						win.Destroy()
					})
					must(timer4.StartOnce(150 * time.Millisecond))
				})
				must(timer3.StartOnce(150 * time.Millisecond))
			})
			must(timer2.StartOnce(300 * time.Millisecond))
		})
		must(timer.StartOnce(300 * time.Millisecond))
	}
	app.Run()
}

type observedApplication struct {
	gui.Application
	typography typography.Context
}

func (a observedApplication) Typography() typography.Context { return a.typography }

type observedTypography struct {
	typography.Context
	created map[string]int
	last    map[string]typography.TextLayout // borrowed until ConnectDestroy fires
}

func (c *observedTypography) NewTextLayout(text string, format typography.TextFormat, width, height float32) (typography.TextLayout, error) {
	l, err := c.Context.NewTextLayout(text, format, width, height)
	if err != nil {
		return nil, err
	}
	c.created[text]++
	c.last[text] = l
	l.ConnectDestroy(func() {
		if c.last[text] == l {
			delete(c.last, text)
		}
	})
	return l, nil
}
func must(err error) {
	if err != nil {
		panic(fmt.Sprintf("window verification: %v", err))
	}
}

// The test records the painter actually used, rather than inferring it from an environment preference.
type paintProbe struct {
	gui.WidgetBase
	reported bool
}

func (p *paintProbe) Measure(c layout.Constraint) layout.Measurement {
	return layout.Measured(c.Clamp(geometry.Size{Width: 1, Height: 1}))
}
func (p *paintProbe) Paint(painter gui.Painter) {
	if p.reported {
		return
	}
	p.reported = true
	img, err := painter.NewImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	must(err)
	log.Printf("actual image=%T PixelScale=%g", img, painter.PixelScale())
	img.Destroy()
}
