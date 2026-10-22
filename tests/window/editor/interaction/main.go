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
// revision, selection, byte length, viewport and each editor's layout-pending flag.
// These describe a completed widget traversal, not text-color completion or
// presentation to a physical monitor. External observers sample desktop pixels.
// No timer requests painting. Input observation never consumes or edits events.
// Closing checks that the original bytes were restored (undo the test edits);
// failure panics. The input file is never modified. desktop.py temporarily owns
// CLIPBOARD and restores its prior UTF-8 text, if available; rich formats cannot
// be restored. Window focus/geometry and the pointer change. Do not use while
// someone is interacting with the same desktop. Closing/restarting resets.
// desktop.py --rtl-insertion additionally checks a known RTL middle insertion
// using visual Right with the same exact byte/caret and whole-text assertions.
// desktop.py is X11-only. For VNC/RDP, start with -remote -run-id N (1..127)
// in the interactive session and use remote.py with the same ID. Inspect the
// screenshot before specifying its client bounds and desktop geometry version;
// unique IDs reject cached frames from earlier windows. remote.py uses normal
// pointer/key input, never the clipboard, and checks middle edits and optional
// native edge/corner dragging. It restores the in-memory text and closes the
// window; additionally require the process exit code 0 and its verified record.
// GUI input capture and model change timestamps precede the final widget Paint
// observer. IME commits may have no KeyDown: change-to-Paint excludes delivery
// and earlier change handlers. Neither metric includes native presentation.
// Remote wall times include input transport and screenshot latency; they are
// not UI freeze measurements. The 10 ms GUI timer separately reports gaps.
// Optional -cpu-profile /outside/repo/run.pprof requires a new output path and
// logs native calls exceeding 3 ms. Profiling runs are diagnostic observations,
// excluded from end-to-end before/after statistics because profiling adds work.
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
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
	at := time.Now().UnixNano()
	input := false
	var detail any
	switch e := ctx.Event().(type) {
	case events.KeyEvent:
		input = e.EventType == events.KeyDown && e.Key != events.KeyControl && e.Key != events.KeyShift && e.Key != events.KeyAlt && e.Key != events.KeySuper
		detail = map[string]any{"key": e.Key, "modifiers": e.Modifiers}
	case events.PointerEvent:
		input = e.EventType == events.PointerDown
		detail = map[string]any{"position": e.Position, "button": e.Button}
	}
	if input {
		o.sequence++
		emit(map[string]any{"event": "input", "at_ns": at, "sequence": o.sequence, "detail": detail})
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
	runID      byte
}

func (p *paintObserver) Measure(c layout.Constraint) layout.Measurement {
	return layout.Measured(c.Clamp(geometry.Size{Width: 600, Height: 16}))
}

func (p *paintObserver) Paint(painter gui.Painter) {
	info := p.editor.Snapshot()
	var b [24]byte
	b[0] = 0xa7
	b[23] = p.runID << 1
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
		b[23] |= 1
	}
	if meta, ok := info.Attributes["goui.text"].(map[string]any); ok && meta["layoutPending"] == true {
		b[23] |= 1
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
	profile := flag.String("cpu-profile", "", "optional CPU profile outside the repository; enables native-call diagnostics")
	remote := flag.Bool("remote", false, "reserve space for native window controls and place the observer below them")
	runID := flag.Uint("run-id", 0, "passive marker instance ID (0..127); rejects stale remote frames")
	flag.Parse()
	if *runID > 127 {
		panic("-run-id must be between 0 and 127")
	}
	if *profile != "" {
		out, err := os.OpenFile(*profile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		must(err)
		defer out.Close()
		must(pprof.StartCPUProfile(out))
		defer pprof.StopCPUProfile()
	}
	data, err := os.ReadFile(*file)
	must(err)
	if !utf8.Valid(data) {
		panic("input is not UTF-8")
	}
	original := textedit.Normalize(string(data))
	app, err := gui.NewApplication("org.golanggui.SourceViewInteraction")
	must(err)
	if *profile != "" {
		gui.App = profileApp{Application: app, context: profileContext{app.Typography()}}
	}
	app.SetStyleSheet(modern.Sheet(modern.Options{}))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1100, Height: 760}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	// The editor application owns its minimum viewport. Deriving a native
	// minimum from unbounded TextView content would produce a giant window.
	win.SetMinSize(geometry.Size{Width: 640, Height: 360})
	if *remote {
		must(win.SetPosition(nil, geometry.Point{X: 80, Y: 60}))
	}
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
	changeObserver := model.ConnectChange(func(change textedit.Change) {
		emit(map[string]any{"event": "change", "at_ns": time.Now().UnixNano(), "revision": change.Revision, "bytes": model.Len()})
	})
	defer changeObserver.Disconnect()
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(editor)
	scroll.SetMainWeight(1)
	input := &observer{EventControllerBase: gui.NewEventControllerBase(gui.PhaseCapture), paint: func() { must(win.RequestPaint()) }}
	probe := &paintObserver{input: input, editor: editor, model: model, runID: byte(*runID)}
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	spacer := gui.NewLinearBox(layout.DirectionVertical)
	spacer.SetMinSize(geometry.Size{Height: 16})
	spacer.SetMaxSize(geometry.Size{Height: 16})
	if *remote {
		spacer.SetMinSize(geometry.Size{Height: 48})
		spacer.SetMaxSize(geometry.Size{Height: 48})
	}
	box.AddChild(spacer)
	box.AddChild(scroll)
	// A top-anchored overlay paints last without moving while the WM resizes
	// the client. A bottom marker can be clipped before the next GUI frame.
	overlay := gui.NewOverlay()
	overlay.SetChild(box)
	overlay.AddOverlay(probe)
	if *remote {
		overlay.ConnectOverlayPosition(probe, func(_, _ geometry.Size, position *geometry.Point) {
			*position = geometry.Point{X: 100, Y: 24}
		})
	}
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
