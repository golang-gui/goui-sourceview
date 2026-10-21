// Real desktop input verification; requires a desktop and native typography.
// Build: go build -o /tmp/source-interaction ./tests/window/editor/interaction
// Linux: XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1
//
//	/tmp/source-interaction -file /outside/repo/document.json [-kind text]
//
// Initial: Modern integrated window, the complete UTF-8 file, automatic word/
// character wrapping, normal selection/history/clipboard. SourceView retains
// line numbers and folding; automatic completion is disabled to isolate editing.
// Drive with physical input or XTest (desktop.py), never direct editor actions.
// Exercise insertion, Backspace/Delete, selected replacement/deletion, paste,
// undo/redo at head/middle/tail, then drag the actual window edges/corner.
// The final paint observer displays passive diagnostic bits: input sequence,
// revision, selection, byte length, viewport and SourceView layout-pending flag.
// These describe a completed widget traversal, not text-color completion or
// presentation to a physical monitor. The external observer samples X11 pixels.
// No timer requests painting. Input observation never consumes or edits events.
// Closing checks that the original bytes were restored (undo the test edits);
// failure panics. The input file is never modified. desktop.py temporarily owns
// CLIPBOARD and restores its prior UTF-8 text, if available; rich formats cannot
// be restored. Window focus/geometry and the pointer change. Do not use while
// someone is interacting with the same desktop. Closing/restarting resets.
// Other platforms may run this window manually; desktop.py is X11-only.
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/graphics"
	"github.com/golang-gui/goui/theme/modern"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func emit(v any) { must(json.NewEncoder(os.Stdout).Encode(v)) }

type observer struct {
	gui.EventControllerBase
	sequence uint16
	paint    func()
}

func (o *observer) HandleEvent(ctx gui.EventContext) {
	input := false
	switch e := ctx.Event().(type) {
	case events.KeyEvent:
		input = e.EventType == events.KeyDown && e.Key != events.KeyControl && e.Key != events.KeyShift && e.Key != events.KeyAlt && e.Key != events.KeySuper
	case events.PointerEvent:
		input = e.EventType == events.PointerDown
	}
	if input {
		o.sequence++
		o.paint() // Echo ignored keys too, without handling their default action.
	}
}

type paintObserver struct {
	gui.WidgetBase
	input      *observer
	editor     gui.Widget
	model      *textedit.Model
	last       [24]byte
	frameStart time.Time
}

func (p *paintObserver) Measure(c layout.Constraint) layout.Measurement {
	return layout.Measured(c.Clamp(geometry.Size{Width: 600, Height: 16}))
}

func (p *paintObserver) Paint(painter gui.Painter) {
	info := p.editor.Snapshot()
	var b [24]byte
	b[0] = 0xa7
	binary.LittleEndian.PutUint16(b[1:], p.input.sequence)
	binary.LittleEndian.PutUint32(b[3:], uint32(p.model.Revision()))
	if editing := info.TextEditing; editing != nil {
		binary.LittleEndian.PutUint32(b[7:], uint32(editing.Selection.Caret))
		binary.LittleEndian.PutUint32(b[11:], uint32(editing.Selection.Anchor))
	}
	binary.LittleEndian.PutUint32(b[15:], uint32(p.model.Len()))
	binary.LittleEndian.PutUint16(b[19:], uint16(p.editor.Rect().Width))
	binary.LittleEndian.PutUint16(b[21:], uint16(p.editor.Rect().Height))
	if meta, ok := info.Attributes["goui.sourceview"].(map[string]any); ok && meta["layoutPending"] == true {
		b[23] = 1
	}
	painter.FillRect(geometry.Rect(0, 0, p.Rect().Width, 16), graphics.RGB(255, 255, 255))
	painter.FillRect(geometry.Rect(0, 0, 6, 16), graphics.RGB(255, 0, 255))
	for i, value := range b {
		for bit := range 8 {
			if value&(1<<bit) != 0 {
				painter.FillRect(geometry.Rect(float32(8+(i*8+bit)*3), 0, 3, 16), graphics.RGB(0, 64, 192))
			}
		}
	}
	if b != p.last {
		emit(map[string]any{"event": "paint", "at_ns": time.Now().UnixNano(), "frame_ms": float64(time.Since(p.frameStart)) / 1e6, "status": b, "bounds": p.Snapshot().Bounds})
		p.last = b
	}
}

func main() {
	file := flag.String("file", "", "UTF-8 file; never overwritten")
	kind := flag.String("kind", "source", "source or text")
	flag.Parse()
	data, err := os.ReadFile(*file)
	must(err)
	if !utf8.Valid(data) {
		panic("input is not UTF-8")
	}
	original := textedit.Normalize(string(data))
	app, err := gui.NewApplication("org.golanggui.SourceViewInteraction")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{}))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1100, Height: 760}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	// The editor application owns its minimum viewport. Deriving a native
	// minimum from unbounded TextView content would produce a giant window.
	win.SetMinSize(geometry.Size{Width: 640, Height: 360})
	must(win.SetTitle(fmt.Sprintf("SourceView interaction %s %s", *kind, filepath.Base(*file))))
	var editor gui.Widget
	var model *textedit.Model
	operationErrors := 0
	switch *kind {
	case "source":
		v := source.NewSourceView()
		v.SetWrapMode(source.WrapWordChar)
		v.SetAutomaticCompletion(false)
		v.ConnectError(func(err error) {
			operationErrors++
			emit(map[string]any{"event": "error", "message": err.Error()})
		})
		must(v.Document().SetLanguage(language.Guess(*file, data[:min(len(data), 4096)])))
		v.Document().Model().SetText(original)
		editor, model = v, v.Document().Model()
	case "text":
		v := gui.NewTextView()
		v.SetWrapMode(gui.WrapWordChar)
		v.Model().SetText(original)
		editor, model = v, v.Model()
	default:
		panic("-kind must be source or text")
	}
	scroll := gui.NewScrollView()
	scroll.SetChild(editor)
	scroll.SetMainWeight(1)
	input := &observer{EventControllerBase: gui.NewEventControllerBase(gui.PhaseCapture), paint: func() { must(win.RequestPaint()) }}
	probe := &paintObserver{input: input, editor: editor, model: model}
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	spacer := gui.NewLinearBox(layout.DirectionVertical)
	spacer.SetMinSize(geometry.Size{Height: 16})
	spacer.SetMaxSize(geometry.Size{Height: 16})
	box.AddChild(spacer)
	box.AddChild(scroll)
	// A top-anchored overlay paints last without moving while the WM resizes
	// the client. A bottom marker can be clipped before the next GUI frame.
	overlay := gui.NewOverlay()
	overlay.SetChild(box)
	overlay.AddOverlay(probe)
	overlay.AddEventController(input)
	win.ConnectFrame(func(time.Time) { probe.frameStart = time.Now() })
	win.ConnectCloseRequest(func(allow *bool) {
		if !model.EqualText(original) {
			panic("original document was not restored before close")
		}
		if operationErrors != 0 {
			panic("editing reported operation errors")
		}
		emit(map[string]any{"event": "verified", "bytes": model.Len(), "revision": model.Revision()})
	})
	win.SetWidget(overlay)
	must(win.Show())
	win.SetFocusedWidget(editor)
	// Passive delivery-gap observation. It does not request layout/paint or
	// perform assertions; full-document verification happens only on close.
	timer := app.NewTimer()
	defer timer.Stop()
	lastTick := time.Now()
	timer.ConnectTimeout(func() {
		now := time.Now()
		if gap := now.Sub(lastTick); gap > 20*time.Millisecond {
			emit(map[string]any{"event": "gap", "from_ns": lastTick.UnixNano(), "at_ns": now.UnixNano(), "gap_ms": float64(gap) / 1e6})
		}
		lastTick = now
	})
	must(timer.Start(10 * time.Millisecond))
	emit(map[string]any{"event": "loaded", "file": *file, "kind": *kind, "bytes": len(original), "sha256": fmt.Sprintf("%x", sha256.Sum256(data)), "backend": app.Typography().Name()})
	app.Run()
}
