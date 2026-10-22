// Downloaded-corpus verification. Requires a desktop, native typography/fonts.
// Run: GOUI_PLAT_PAINTER=software go run ./tests/window/editor/corpus -file PATH
// Linux without IME: XMODIFIERS=@im=none. Use GOUI_PLAT_SCALE=1 and 2 separately.
// PATH is an existing UTF-8 txt/json/xml/html/markdown document outside the repo;
// the program never modifies it. Sources/hashes and download commands are in
// doc/PlanCorpusLayout.md. -language overrides filename-based language detection.
// Initial: Modern integrated window, empty wrapped SourceView in ScrollView.
// Automatic tests use Window.DispatchEvent/CommitText: load, end/home navigation,
// wheel scroll, vertical scrollbar drag, Unicode insertion/undo/redo, width and
// height changes through separate buttons while the editor prepares its viewport.
// At each settled stage, check original model bytes and visible excerpts;
// displayed content must produce ink in an independent viewport rasterization.
// Layout-pending snapshots are permitted. No input text is truncated or replaced.
// Logs describe native calls, frame-callback-to-final-widget-paint duration and
// timer delivery gaps (including verification overhead). These are observations,
// not compositor latency or hard responsiveness thresholds. Single native
// paragraph calls remain indivisible. -capture-dir optionally writes final PNG
// to an existing directory outside the repository; never writes the input file,
// clipboard or system settings. PASS closes automatically. Early close/timeout
// fails; use an external timeout for a blocked native call. Restart resets.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/typography"
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

type stats struct {
	created, metrics int
	nativeMax        time.Duration
	nativePhase      string
	frameMax, gap    time.Duration
}
type observedContext struct {
	typography.Context
	stats
}

func (c *observedContext) note(start time.Time, phase string) {
	if elapsed := time.Since(start); elapsed > c.nativeMax {
		c.nativeMax, c.nativePhase = elapsed, phase
	}
}
func (c *observedContext) NewTextLayout(s string, f typography.TextFormat, w, h float32) (typography.TextLayout, error) {
	start := time.Now()
	l, err := c.Context.NewTextLayout(s, f, w, h)
	if f.Font.Family != "monospace" || f.WrapMode != typography.WrapWordChar || err != nil {
		return l, err
	}
	c.created++
	c.note(start, "NewTextLayout")
	return &observedLayout{TextLayout: l, context: c}, nil
}

type observedLayout struct {
	typography.TextLayout
	context *observedContext
}

func (l *observedLayout) MeasureSize() (float32, float32) {
	start := time.Now()
	w, h := l.TextLayout.MeasureSize()
	l.context.note(start, "MeasureSize")
	return w, h
}
func (l *observedLayout) MeasureMetrics() ([]typography.TextLine, []typography.TextCluster) {
	start := time.Now()
	lines, clusters := l.TextLayout.MeasureMetrics()
	l.context.metrics++
	l.context.note(start, "MeasureMetrics")
	return lines, clusters
}
func (l *observedLayout) Rasterize(scale float32, region image.Rectangle, buf []byte) (typography.TextBitmap, error) {
	start := time.Now()
	bitmap, err := l.TextLayout.Rasterize(scale, region, buf)
	l.context.note(start, "Rasterize")
	return bitmap, err
}

type observedApp struct {
	gui.Application
	context *observedContext
}

func (a observedApp) Typography() typography.Context { return a.context }

type paintProbe struct {
	gui.WidgetBase
	paint func()
}

func (p *paintProbe) Measure(c layout.Constraint) layout.Measurement {
	return layout.Measured(c.Clamp(geometry.Size{Height: 1}))
}
func (p *paintProbe) Paint(gui.Painter) { p.paint() }

func main() {
	file := flag.String("file", "", "existing UTF-8 corpus document")
	idFlag := flag.String("language", "", "optional builtin language ID")
	capture := flag.String("capture-dir", "", "existing directory for a final PNG")
	flag.Parse()
	check(*file != "", "-file is required")
	data, err := os.ReadFile(*file)
	must(err)
	check(utf8.Valid(data), "corpus must be valid UTF-8")
	original := textedit.Normalize(string(data))
	digest := sha256.Sum256(data)
	checksum := hex.EncodeToString(digest[:])
	longest := 0
	for _, line := range strings.Split(original, "\n") {
		longest = max(longest, len(line))
	}
	id := language.Guess(*file, data[:min(len(data), 4096)])
	if *idFlag != "" {
		id = language.ID(*idFlag)
	}
	app, err := gui.NewApplication("org.golanggui.SourceViewCorpus")
	must(err)
	ctx := &observedContext{Context: app.Typography()}
	gui.App = observedApp{app, ctx}
	app.SetStyleSheet(modern.Sheet(modern.Options{}))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 960, Height: 640}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView corpus — " + filepath.Base(*file)))
	var frameStart time.Time
	frames := 0
	win.ConnectFrame(func(time.Time) { frameStart = time.Now(); frames++ })
	v := source.NewSourceView()
	v.SetWrapMode(source.WrapWordChar)
	v.SetShowLineNumbers(false)
	v.SetFoldingEnabled(false) // Isolate text layout from full-document fold parsing.
	v.SetAutomaticCompletion(false)
	v.ConnectError(func(err error) { panic(err) })
	must(v.Document().SetLanguage(id))
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(v)
	widthButton := gui.NewButton()
	widthButton.SetChild(gui.NewLabel("Resize width"))
	heightButton := gui.NewButton()
	heightButton.SetChild(gui.NewLabel("Resize height"))
	setWidth := func(width float32) {
		widthButton.SetMinSize(geometry.Size{Width: width})
		widthButton.SetMaxSize(geometry.Size{Width: width})
	}
	setHeight := func(height float32) {
		heightButton.SetMinSize(geometry.Size{Height: height})
		heightButton.SetMaxSize(geometry.Size{Height: height})
	}
	setWidth(120)
	setHeight(36)
	narrow, short := false, false
	widthButton.ConnectClicked(func() {
		narrow = !narrow
		width := float32(120)
		if narrow {
			width = 260
		}
		setWidth(width)
	})
	heightButton.ConnectClicked(func() {
		short = !short
		height := float32(36)
		if short {
			height = 180
		}
		setHeight(height)
	})
	row := gui.NewLinearBox(layout.DirectionHorizontal)
	row.SetCrossAlign(layout.CrossStretch)
	row.SetMainWeight(1)
	row.AddChild(scroll)
	row.AddChild(widthButton)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(row)
	box.AddChild(heightButton)
	box.AddChild(&paintProbe{paint: func() {
		ctx.frameMax = max(ctx.frameMax, time.Since(frameStart))
	}})
	win.SetWidget(box)
	must(win.Show())
	primary, _ := gui.ModPrimary.Resolve()
	key := func(k events.Key, modifiers events.Modifiers) {
		win.SetFocusedWidget(v)
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: modifiers}))
		}
	}
	edge := func(end bool) {
		k := events.KeyHome
		if end {
			k = events.KeyEnd
		}
		if runtime.GOOS == "darwin" {
			k = events.KeyArrowUp
			if end {
				k = events.KeyArrowDown
			}
		}
		key(k, primary)
	}
	click := func(w gui.Widget) {
		r := w.Snapshot().Bounds
		point := geometry.Point{X: r.X + r.Width/2, Y: r.Y + r.Height/2}
		for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
			must(win.DispatchEvent(events.PointerEvent{EventType: kind, Position: point, Button: events.PointerButtonLeft}))
		}
	}
	const insertion = "\nSourceView corpus edit ✓\n"
	const middleInsertion = "编辑✓"
	want := original
	oldWidth, oldHeight, wheelStart := float32(0), float32(0), float32(0)
	type action struct {
		name  string
		run   func()
		check func()
	}
	actions := []action{
		{"load", func() { v.Document().Model().SetText(original) }, func() { check(v.Selection().Caret == 0, "load moved caret") }},
		{"end", func() { edge(true) }, func() { check(v.Selection().Caret == len(want), "end key missed document end") }},
		{"home", func() { edge(false) }, func() { check(v.Selection().Caret == 0, "home key missed document start") }},
		{"wheel", func() {
			wheelStart = scroll.ScrollY()
			r := v.Snapshot().Bounds
			must(win.DispatchEvent(events.WheelEvent{Position: geometry.Point{X: r.X + r.Width/2, Y: r.Y + r.Height/2}, DeltaY: 45, Mode: events.WheelDeltaLine}))
		}, func() {
			check(scroll.ScrollY() > wheelStart && v.Selection().Caret == 0, "wheel did not scroll independently of caret")
		}},
		{"scrollbar-drag", func() {
			var bar *gui.ScrollBar
			for _, child := range scroll.Children() {
				if b, ok := child.(*gui.ScrollBar); ok && b.Orientation() == layout.DirectionVertical {
					bar = b
				}
			}
			check(bar != nil && bar.Visible(), "missing vertical scrollbar")
			r := bar.Snapshot().Bounds
			for _, event := range []events.PointerEvent{
				{EventType: events.PointerDown, Position: geometry.Point{X: r.X + r.Width/2, Y: r.Y + r.Height/2}, Button: events.PointerButtonLeft},
				{EventType: events.PointerMove, Position: geometry.Point{X: r.X + r.Width/2, Y: r.Y + r.Height*.8}, Buttons: events.PointerButtonLeftDown},
				{EventType: events.PointerUp, Position: geometry.Point{X: r.X + r.Width/2, Y: r.Y + r.Height*.8}, Button: events.PointerButtonLeft},
			} {
				must(win.DispatchEvent(event))
			}
		}, func() {
			check(scroll.ScrollY() > scroll.Snapshot().MaxScrollY*.5, "scrollbar drag did not reach the distant viewport")
		}},
		{"middle-insert", func() {
			click(v)
			at := v.Selection().Caret
			check(at > 0 && at < len(original), "click did not position the caret inside the distant document")
			must(win.CommitText(middleInsertion))
			want = original[:at] + middleInsertion + original[at:]
		}, func() {}},
		{"middle-undo", func() { key(events.KeyZ, primary); want = original }, func() {}},
		{"insert", func() { edge(true); must(win.CommitText(insertion)); want = original + insertion }, func() { check(v.Selection().Caret == len(want), "insert caret") }},
		{"undo", func() { key(events.KeyZ, primary); want = original }, func() {}},
		{"redo", func() { key(events.KeyZ, primary|events.ModifierShift); want = original + insertion }, func() {}},
		{"restore", func() { key(events.KeyZ, primary); want = original }, func() {}},
		{"narrow", func() { oldWidth = v.Rect().Width; click(widthButton) }, func() { check(v.Rect().Width < oldWidth, "width did not shrink") }},
		{"widen", func() { click(widthButton) }, func() { check(v.Rect().Width == oldWidth, "width did not restore") }},
		{"shorten", func() { oldHeight = v.Rect().Height; click(heightButton) }, func() { check(v.Rect().Height < oldHeight, "height did not shrink") }},
		{"height-restore", func() { click(heightButton) }, func() { check(v.Rect().Height == oldHeight, "height did not restore") }},
	}
	stage, stable, actionFrame := 0, 0, 0
	var started, previousTick time.Time
	var actionTime time.Duration
	complete, active := false, false
	lastMetrics := 0
	deadline := time.Now().Add(90 * time.Second)
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		now := time.Now()
		if !now.Before(deadline) {
			panic(fmt.Sprintf("corpus timeout at %s: stats=%+v scroll=%g selection=%+v metadata=%v", actions[stage].name, ctx.stats, scroll.ScrollY(), v.Selection(), v.Snapshot().Attributes))
		}
		if !previousTick.IsZero() {
			ctx.gap = max(ctx.gap, now.Sub(previousTick))
		}
		previousTick = now
		must(win.RequestPaint())
		if frames < 2 {
			return
		}
		if !active {
			ctx.stats = stats{}
			started, actionFrame = time.Now(), frames
			actions[stage].run()
			actionTime = time.Since(started)
			active, stable = true, 0
			lastMetrics = ctx.metrics
			return
		}
		info := v.Snapshot()
		meta, _ := info.Attributes["goui.sourceview"].(map[string]any)
		pending, _ := meta["layoutPending"].(bool)
		if !pending && frames > actionFrame+1 && ctx.metrics == lastMetrics {
			stable++
		} else {
			stable = 0
		}
		lastMetrics = ctx.metrics
		if stable < 3 {
			return
		}
		actions[stage].check()
		model := v.Document().Model()
		check(model.EqualText(want), "model bytes differ at "+actions[stage].name)
		editing := info.TextEditing
		check(editing != nil, "missing text snapshot")
		visible, err := model.Slice(textedit.Range{Start: editing.VisibleRange.Start, End: editing.VisibleRange.End})
		must(err)
		check(visible == editing.VisibleText, "visible excerpt differs from original model bytes")
		img, err := gui.RenderWidget(v, 0)
		must(err)
		ink := 0
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
			for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
				r, g, b, a := img.At(x, y).RGBA()
				if a != 0 && r+g+b < 3*0xd000 {
					ink++
				}
			}
		}
		check(ink > 100, "settled viewport contains no text ink beyond a caret")
		must(json.NewEncoder(os.Stdout).Encode(map[string]any{
			"file": filepath.Base(*file), "bytes": len(data), "lines": model.LineCount(), "longest_line": longest,
			"sha256": checksum, "normalized_bytes": len(original),
			"language": id, "stage": actions[stage].name, "action_ms": float64(actionTime) / 1e6,
			"settle_ms": float64(time.Since(started)) / 1e6, "frame_max_ms": float64(ctx.frameMax) / 1e6,
			"timer_gap_ms": float64(ctx.gap) / 1e6, "native_max_ms": float64(ctx.nativeMax) / 1e6, "native_phase": ctx.nativePhase,
			"native_created": ctx.created, "metric_exports": ctx.metrics, "scroll_y": scroll.ScrollY(), "viewport": v.Rect().Size,
		}))
		stage++
		if stage == len(actions) {
			if *capture != "" {
				out, err := os.Create(filepath.Join(*capture, filepath.Base(*file)+".png"))
				must(err)
				must(png.Encode(out, img))
				must(out.Close())
			}
			complete = true
			win.Destroy()
			return
		}
		active = false
	})
	must(timer.Start(20 * time.Millisecond))
	app.Run()
	check(complete, "corpus verification aborted")
	log.Printf("PASS corpus %s (%s): %d bytes", filepath.Base(*file), ctx.Name(), len(data))
}
