// Long-line rasterization regression. Requires a desktop, native typography and
// fonts. Run: GOUI_PLAT_PAINTER=software go run ./tests/window/editor/longlines
// (also opengl; Windows also direct2d), with GOUI_PLAT_SCALE=1, 1.25 or 2.
// Optional -file reads an existing UTF-8 single-line JSON file without modifying
// it. Default data is an original 160 KiB single-line JSON string.
// Initial state: Modern, integrated window, empty editor inside ScrollView.
// The first allocated frame loads the JSON through Document.Model().SetText.
// Normal Window.DispatchEvent/CommitText checks document start/end, insertion,
// undo, horizontal scrolling, repeated paints and gutter changes. Optional -wrap
// also probes wrapping the same very long word; native shaping may take much
// longer than viewport rasterization. Public
// setters prepare documents only. Each viewport must contain syntax-colored
// pixels; visible excerpts must include the actual caret and match model bytes.
// Software/OpenGL raster requests are bounded; native failures are fatal. Native
// Direct2D may bypass Rasterize. This is functional validation, not a performance
// benchmark or a promise of bounded shaping costs. No clipboard/settings writes.
// Closing early fails. Restart resets; PASS closes the window automatically.
package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
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

type observedApp struct {
	gui.Application
	context typography.Context
}

func (a observedApp) Typography() typography.Context { return a.context }

type observedContext struct {
	typography.Context
	observe bool
	calls   int
	failure error
	last    image.Rectangle
	ink     int
}

func (c *observedContext) NewTextLayout(s string, f typography.TextFormat, w, h float32) (typography.TextLayout, error) {
	l, err := c.Context.NewTextLayout(s, f, w, h)
	if err != nil || len(s) < 1024 || !c.observe {
		return l, err
	}
	return &observedLayout{TextLayout: l, context: c}, nil
}

type observedLayout struct {
	typography.TextLayout
	context *observedContext
}

func (l *observedLayout) Rasterize(scale float32, r image.Rectangle, buf []byte) (typography.TextBitmap, error) {
	l.context.calls++
	l.context.last = r
	if r.Dx() > 1024 || r.Dy() > 1024 {
		l.context.failure = fmt.Errorf("unbounded native request %v", r)
	}
	b, err := l.TextLayout.Rasterize(scale, r, buf)
	if err != nil {
		l.context.failure = err
	}
	l.context.ink = 0
	for i := 3; i < len(b.Pixels); i += 4 {
		if b.Pixels[i] != 0 {
			l.context.ink++
		}
	}
	return b, err
}

func main() {
	runtime.LockOSThread()
	file := flag.String("file", "", "existing single-line JSON to read")
	wrap := flag.Bool("wrap", false, "also probe wrapping the long paragraph (native shaping can be slow)")
	flag.Parse()
	text := `{"text":"` + strings.Repeat("abcdefghij", 16000) + `"}`
	if *file != "" {
		data, err := os.ReadFile(*file)
		must(err)
		text = string(data)
	}
	check(utf8.ValidString(text) && !strings.ContainsAny(text, "\r\n") && len(text) > 32768, "expected long single-line UTF-8 text")
	app, err := gui.NewApplication("org.golanggui.SourceViewLongLines")
	must(err)
	ctx := &observedContext{Context: app.Typography(), observe: runtime.GOOS != "windows" || os.Getenv("GOUI_PLAT_PAINTER") == "software" || os.Getenv("GOUI_PLAT_PAINTER") == "opengl"}
	gui.App = observedApp{app, ctx}
	app.SetStyleSheet(modern.Sheet(modern.Options{}))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 760, Height: 360}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	v := source.NewSourceView()
	v.SetDocument(source.NewDocument(""))
	must(v.Document().SetLanguage(language.JSON))
	v.SetAutomaticCompletion(false)
	scroll := gui.NewScrollView()
	scroll.SetChild(v)
	win.SetWidget(scroll)
	must(win.Show())
	win.SetFocusedWidget(v)
	primary, _ := gui.ModPrimary.Resolve()
	key := func(k events.Key, m events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: m}))
		}
	}
	end := func() {
		k := events.KeyEnd
		if runtime.GOOS == "darwin" {
			k = events.KeyArrowDown
		}
		key(k, primary)
	}
	start := func() {
		k := events.KeyHome
		if runtime.GOOS == "darwin" {
			k = events.KeyArrowUp
		}
		key(k, primary)
	}
	pending := "initial layout"
	assertView := func() bool {
		info := v.Snapshot().TextEditing
		check(info != nil, "missing text snapshot")
		caret := v.Selection().Caret
		check(info.VisibleRange.Start <= caret && info.VisibleRange.End >= caret, "caret outside visible excerpt")
		actual, err := v.Document().Model().Slice(textedit.Range{Start: info.VisibleRange.Start, End: info.VisibleRange.End})
		must(err)
		check(actual == info.VisibleText, "visible excerpt differs from model")
		img, err := gui.RenderWidget(v, 0)
		if err != nil {
			panic(fmt.Errorf("render viewport=%v scroll=%v window=%+v: %w", v.Rect(), scroll.Rect(), win.Snapshot().Bounds, err))
		}
		colored := false
		for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y && !colored; y++ {
			for x := img.Bounds().Min.X + 80; x < img.Bounds().Max.X-8; x++ {
				r, g, b, a := img.At(x, y).RGBA()
				if a > 0 && max(r, g, b)-min(r, g, b) > 5000 {
					colored = true
					break
				}
			}
		}
		must(ctx.failure)
		pending = fmt.Sprintf("syntax color not ready: caret=%d offset=%g,%g region=%v ink=%d", caret, scroll.ScrollX(), scroll.ScrollY(), ctx.last, ctx.ink)
		return colored
	}
	complete := false
	loaded := false
	step := 0
	deadline := time.Now().Add(30 * time.Second)
	timer := app.NewTimer()
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), "verification timeout: "+pending)
		if v.Rect().Width <= 0 || v.Rect().Height <= 0 || !win.Focused() {
			return
		}
		if !loaded {
			v.Document().Model().SetText(text)
			loaded = true
			deadline = time.Now().Add(30 * time.Second)
			return
		}
		switch step {
		case 0:
			if !assertView() {
				return
			}
			end()
		case 1:
			check(v.Selection().Caret == len(text), "document end navigation failed")
			if !assertView() {
				return
			}
			must(win.CommitText(" "))
		case 2:
			check(v.Document().Model().Text() == text+" ", "tail insertion lost text")
			if !assertView() {
				return
			}
			key(events.KeyZ, primary)
		case 3:
			check(v.Document().Model().Text() == text, "undo lost text")
			if !assertView() {
				return
			}
			start()
		case 4:
			check(v.Selection().Caret == 0, "document start navigation failed")
			if !assertView() {
				return
			}
			if *wrap {
				v.SetWrapMode(source.WrapWordChar)
			} else {
				v.SetShowLineNumbers(false)
			}
		case 5:
			if !assertView() {
				return
			}
			end()
		case 6:
			if !assertView() {
				return
			}
			must(win.RequestPaint())
		case 7:
			if !assertView() {
				return
			}
			check(!ctx.observe || ctx.calls > 0, "no raster requests observed")
			complete = true
			log.Printf("PASS long-lines bytes=%d typography=%s native-regions=%d", len(text), ctx.Name(), ctx.calls)
			win.Destroy()
		}
		log.Printf("PASS long-lines step=%d", step)
		deadline = time.Now().Add(30 * time.Second)
		step++
	})
	must(timer.Start(350 * time.Millisecond))
	app.Run()
	check(complete, "verification aborted")
}
