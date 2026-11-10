// Unified gutter folding. Requires a native desktop, WM and fonts. Run:
// XMODIFIERS=@im=none go run ./tests/window/editor/fold_preview [-basic] [-dark]
// Optionally add -file /path/to/single-line.json to reproduce real input. The
// file must contain a quoted iVBOR PNG payload; it is read and never saved.
// Initially an expanded minified JSON has one logical line and a long-line
// arrow. Normal Window pointer/key/text events collapse/expand it, insert two
// newlines before the payload, then operate the structural arrow and badge.
// Assert one-row structural preview, no competing arrows, correct logical
// ranges, caret relocation, search/navigation reveal, resize/wrap retention,
// source/revision/history preservation and plain-text gutter support. A Painter
// observer checks every collapsed window paint for real text with a one-row
// clip; the footer tags frames for optional independent X11 pixel capture.
// PASS closes; timeout, assertion or early close fails. Restart resets fixtures.
// No clipboard, user settings or files are changed. Other OSes and physical
// IME require separate verification; this program uses the normal event loop.
package main

import (
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

type previewPaint struct {
	gui.Painter
	clip   geometry.Rectangle
	stack  []geometry.Rectangle
	prefix string
	seen   bool
}

func (p *previewPaint) Save() { p.stack = append(p.stack, p.clip); p.Painter.Save() }
func (p *previewPaint) Restore() {
	p.clip = p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
	p.Painter.Restore()
}
func (p *previewPaint) SetClipRect(r geometry.Rectangle) { p.clip = r; p.Painter.SetClipRect(r) }
func (p *previewPaint) DrawTextLayout(at geometry.Point, l typography.TextLayout) {
	if strings.HasPrefix(l.Text(), p.prefix) {
		check(p.clip.Width > 0 && p.clip.Height > 0 && p.clip.Height < 40, "preview body escaped its one-row clip")
		p.seen = true
	}
	p.Painter.DrawTextLayout(at, l)
}

type paintedView struct {
	*source.SourceView
	checked int
}

func (v *paintedView) Paint(p gui.Painter) {
	collapsed := len(v.CollapsedLines()) > 0
	for _, f := range v.FoldingRegions() {
		collapsed = collapsed || f.Collapsed
	}
	if !collapsed || v.WrapMode() == source.WrapNone {
		v.SourceView.Paint(p)
		return
	}
	r, _ := v.Document().Model().LineRange(0)
	r.End = min(r.End, r.Start+24)
	for !v.Document().Model().IsRuneBoundary(r.End) {
		r.End--
	}
	prefix, err := v.Document().Model().Slice(r)
	must(err)
	draw := &previewPaint{Painter: p, prefix: prefix}
	v.SourceView.Paint(draw)
	check(draw.seen, "collapsed window frame has no current-width text")
	check(len(draw.stack) == 0, "preview painter left saved state")
	v.checked++
}

func gutter(v *source.SourceView) gui.WidgetInfo {
	var result gui.WidgetInfo
	count := 0
	for _, c := range v.Snapshot().Children {
		if meta, ok := c.Attributes["goui.sourceview.fold"].(map[string]any); ok && meta["startLine"] == 0 {
			result = c
			count++
		}
	}
	check(count == 1, fmt.Sprintf("expected one gutter entry, got %d", count))
	return result
}
func badge(v *source.SourceView) gui.WidgetInfo {
	for _, c := range v.Snapshot().Children {
		if _, ok := c.Attributes["goui.sourceview.collapsedLine"]; ok {
			return c
		}
	}
	panic("missing preview badge")
}

func main() {
	runtime.LockOSThread()
	dark, basicTheme := flag.Bool("dark", false, "Modern dark"), flag.Bool("basic", false, "Basic")
	file := flag.String("file", "", "optional read-only single-line JSON fixture")
	flag.Parse()
	must(mlang.SetLanguage("en"))
	original := `{"message":{"code":0},"meta":"` + strings.Repeat("wrapped header ", 70) + `","content":"iVBOR` + strings.Repeat("ABCDE12345", 16000) + `"}`
	if *file != "" {
		data, err := os.ReadFile(*file)
		must(err)
		original = string(data)
	}
	split := strings.Index(original, `"iVBOR`)
	check(split > 0 && !strings.Contains(original, "\n") && json.Valid([]byte(original)), "fixture must be valid single-line JSON with a quoted PNG payload")
	doc := source.NewDocument(original)
	must(doc.SetLanguage(language.JSON))
	app, err := gui.NewApplication("org.golanggui.SourceViewFoldPreview")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	if *basicTheme {
		app.SetStyleSheet(basic.Sheet())
	}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1000, Height: 560}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView unified folding"))
	v := source.NewSourceView()
	v.SetDocument(doc)
	v.SetWrapMode(source.WrapWordChar)
	v.SetAutomaticCompletion(false)
	v.ConnectError(func(err error) { panic(err) })
	observed := &paintedView{SourceView: v}
	host := gui.NewScrollView()
	host.SetStyleName("source-view")
	host.SetChild(observed)
	host.SetMainWeight(1)
	pane := gui.NewLinearBox(layout.DirectionVertical)
	setPane := func(width float32) {
		pane.SetMinSize(geometry.Size{Width: width})
		pane.SetMaxSize(geometry.Size{Width: width})
	}
	setPane(0)
	row := gui.NewLinearBox(layout.DirectionHorizontal)
	row.SetCrossAlign(layout.CrossStretch)
	row.SetMainWeight(1)
	row.AddChild(host)
	row.AddChild(pane)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(row)
	box.AddChild(&footer{})
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(v)
	frame, phase := 0, "original"
	enc := json.NewEncoder(os.Stdout)
	win.ConnectAfterPaint(func(p gui.Painter, b geometry.Rectangle) {
		frame++
		info := v.Snapshot()
		var mark geometry.Rectangle
		for _, c := range info.Children {
			if _, ok := c.Attributes["goui.sourceview.collapsedLine"]; ok {
				mark = c.Bounds
				break
			}
		}
		must(enc.Encode(map[string]any{"event": "paint", "frame": frame, "phase": phase, "folds": v.FoldingRegions(), "manual": v.CollapsedLines(), "content": v.ContentSize(), "editor": info.Bounds, "badge": mark, "scale": p.PixelScale()}))
		p.FillRect(geometry.Rect(6, b.Height-22, 3, 16), graphics.ColorOf(color.NRGBA{R: 255, B: 255, A: 255}))
		for bit := range 16 {
			c := color.NRGBA{A: 255}
			if frame&(1<<bit) != 0 {
				c.R, c.G, c.B = 255, 255, 255
			}
			p.FillRect(geometry.Rect(12+float32(bit)*4, b.Height-22, 4, 16), graphics.ColorOf(c))
		}
	})
	click := func(point geometry.Point) {
		for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
			must(win.DispatchEvent(events.PointerEvent{EventType: kind, Button: events.PointerButtonLeft, Position: point}))
		}
	}
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	preview := func() {
		b := badge(v)
		check(v.ContentSize().Height <= b.Bounds.Height+14, "folded JSON still has a tall wrapped header")
		check(v.Snapshot().TextEditing.VisibleRange.End < split, "snapshot exposed the hidden header or payload")
		check(gutter(v).Attributes["goui.sourceview.fold"].(map[string]any)["collapsed"] == true, "gutter does not reflect preview state")
	}
	var steps []func() bool
	settle := func(f func()) {
		start := -1
		steps = append(steps, func() bool {
			if start < 0 {
				start = frame
			}
			meta := v.Snapshot().Attributes["goui.sourceview"].(map[string]any)
			if frame < start+3 || meta["layoutPending"] == true || meta["foldingPending"] == true {
				return false
			}
			f()
			return true
		})
	}
	revision, undo := doc.Model().Revision(), doc.Model().CanUndo()
	settle(func() {
		check(doc.Model().LineCount() == 1 && len(v.FoldingRegions()) == 0, "single-line JSON should have no cross-line fold")
		check(gutter(v).Attributes["goui.sourceview.fold"].(map[string]any)["kind"] == "line", "single-line JSON missing long-line gutter entry")
		click(gutter(v).Bounds.Center())
		phase = "line-collapsed"
	})
	settle(func() {
		preview()
		check(len(v.CollapsedLines()) == 1 && doc.Model().Revision() == revision && doc.Model().CanUndo() == undo && doc.Model().Text() == original, "line collapse changed source or history")
		click(gutter(v).Bounds.Center())
		phase = "line-expanded"
	})
	settle(func() {
		check(len(v.CollapsedLines()) == 0, "gutter failed to expand line")
		v.SetSelection(textedit.Selection{Anchor: split, Caret: split})
		must(win.CommitText("\n\n"))
		phase = "after-newlines"
	})
	settle(func() {
		ranges := v.FoldingRegions()
		check(doc.Model().LineCount() == 3 && len(ranges) == 1 && ranges[0].StartLine == 0 && ranges[0].EndLine == 2, "newline insertion produced the wrong logical fold range")
		v.SetSelection(textedit.Selection{})
		click(gutter(v).Bounds.Center())
		phase = "structure-collapsed"
	})
	settle(func() {
		preview()
		check(v.FoldingRegions()[0].Collapsed && len(v.CollapsedLines()) == 0, "structure preview was stored as a manual line collapse")
		click(badge(v).Bounds.Center())
		phase = "badge-expanded"
	})
	settle(func() {
		check(!v.FoldingRegions()[0].Collapsed, "badge failed to unfold structure")
		v.SetSelection(textedit.Selection{Anchor: split - 2, Caret: split - 2})
		click(gutter(v).Bounds.Center())
		phase = "caret-relocated"
	})
	settle(func() {
		preview()
		check(v.Selection().Caret < v.Snapshot().TextEditing.VisibleRange.End, "fold left the caret in its hidden header")
		key(events.KeyArrowDown, 0)
		phase = "navigation-expanded"
	})
	settle(func() {
		check(!v.FoldingRegions()[0].Collapsed, "Down did not reveal hidden header")
		v.SetSelection(textedit.Selection{})
		click(gutter(v).Bounds.Center())
		phase = "before-resize"
	})
	settle(func() { preview(); setPane(350); phase = "narrow-collapsed" })
	settle(func() { preview(); setPane(0); phase = "wide-collapsed" })
	settle(func() { preview(); v.SetWrapMode(source.WrapNone); phase = "wrap-off" })
	settle(func() {
		check(v.FoldingRegions()[0].Collapsed, "disabling wrap lost structural state")
		v.SetWrapMode(source.WrapWordChar)
		phase = "wrap-on-collapsed"
	})
	settle(func() {
		preview()
		v.SetSearch(source.SearchQuery{Text: "iVBOR"})
		key(events.KeyF3, 0)
		phase = "search-expanded"
	})
	settle(func() {
		check(!v.FoldingRegions()[0].Collapsed && v.Selection().Caret > split, "search failed to reveal hidden payload")
		check(doc.Model().Text() == original[:split]+"\n\n"+original[split:] && json.Valid([]byte(doc.Model().Text())), "folding changed source bytes")
		v.SetSearch(source.SearchQuery{})
		key(events.KeyZ, events.ModifierControl)
		phase = "undo-newlines"
	})
	settle(func() {
		check(doc.Model().Text() == original && doc.Model().LineCount() == 1 && len(v.FoldingRegions()) == 0, "undo failed to restore single-line JSON")
		plain := source.NewDocument(strings.Repeat("plain wrapped text ", 400))
		v.SetDocument(plain)
		phase = "plain"
	})
	settle(func() {
		check(gutter(v).Attributes["goui.sourceview.fold"].(map[string]any)["kind"] == "line", "plain text has no gutter entry")
		click(gutter(v).Bounds.Center())
		phase = "plain-collapsed"
	})
	settle(func() {
		check(len(v.CollapsedLines()) == 1, "plain text gutter failed to collapse")
		click(gutter(v).Bounds.Center())
		phase = "plain-expanded"
	})
	settle(func() { check(len(v.CollapsedLines()) == 0, "plain text gutter failed to expand") })
	index, done := 0, false
	deadline := time.Now().Add(40 * time.Second)
	timer := app.NewTimer()
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), fmt.Sprintf("timeout at step %d phase %s", index, phase))
		must(win.RequestPaint())
		if index < len(steps) {
			if steps[index]() {
				index++
			}
			return
		}
		timer.Stop()
		done = true
		log.Printf("PASS unified gutter: %d actual one-row paints; JSON bytes=%d file=%t; long line, structure, caret, badge, resize, wrap, search, undo, plain text; typography=%s basic=%t dark=%t", observed.checked, len(original), *file != "", app.Typography().Name(), *basicTheme, *dark)
		win.RequestClose()
	})
	must(timer.Start(100 * time.Millisecond))
	app.Run()
	check(done, "probe interrupted")
}
