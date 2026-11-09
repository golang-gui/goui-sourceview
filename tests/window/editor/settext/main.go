// Full-text replacement without blank transition frames. Requires a native
// desktop/WM and fonts. Run: XMODIFIERS=@im=none go run
// ./tests/window/editor/settext [-basic] [-dark]. Record OS, painter and scale.
// The window starts empty. Normal GUI timers replace short, wrapped, many-row,
// empty, paged and code documents; they also repeat equal text, issue two resets
// before painting, switch Document and detach/reload/reattach the view.
// Every actual Paint must draw current nonempty source at its allocated width,
// never old text, and must not create native layouts. Short/many-row cases must
// draw at least the specified visible rows. Empty text is deliberately blank.
// Equal SetText must preserve revision and native resources. This program
// observes the real painter; RenderWidget is not used. An optional external
// X11 capture can synchronize screenshots using the footer's 16-bit frame tag.
// PASS closes automatically; a failed assertion, timeout or early close fails.
// Restart resets the case. No files, clipboard or system settings are changed.
// Linux software/OpenGL runs do not validate other native OSes, hardware GPUs,
// physical input/IME or desktop compositor behavior.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"os"
	"runtime"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
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

type observedApp struct {
	gui.Application
	context *observedContext
}

func (a observedApp) Typography() typography.Context { return a.context }

type observedContext struct {
	typography.Context
	created, destroyed int
}

func (c *observedContext) NewTextLayout(text string, f typography.TextFormat, w, h float32) (typography.TextLayout, error) {
	l, err := c.Context.NewTextLayout(text, f, w, h)
	if err != nil {
		return nil, err
	}
	c.created++
	return &observedLayout{TextLayout: l, context: c}, nil
}

type observedLayout struct {
	typography.TextLayout
	context *observedContext
	dead    bool
}

func (l *observedLayout) Destroy() {
	check(!l.dead, "native layout destroyed twice")
	l.dead = true
	l.context.destroyed++
	l.TextLayout.Destroy()
}

type expectation struct {
	name, text string
	rows       int
	wrap       bool
}

type observedView struct {
	*source.SourceView
	context *observedContext
	want    expectation
	draws   int
}

type observedPainter struct {
	gui.Painter
	view *observedView
}

func (p *observedPainter) DrawTextLayout(at geometry.Point, l typography.TextLayout) {
	text, want := l.Text(), p.view.want
	if text != "" && want.name != "" {
		check(strings.Contains(want.text, text), fmt.Sprintf("%s painted obsolete text %q", want.name, text))
		p.view.draws++
		if want.wrap {
			// SourceView's documented content padding is 6 DIP on either side;
			// this fixture disables the gutter, folding and minimap.
			width, _ := l.Size()
			check(math.Abs(float64(width-(p.view.Rect().Width-12))) < .01, fmt.Sprintf("%s painted obsolete width %g in %g", want.name, width, p.view.Rect().Width))
		}
	}
	p.Painter.DrawTextLayout(at, l)
}

func (v *observedView) Paint(p gui.Painter) {
	before := v.context.created
	v.draws = 0
	v.SourceView.Paint(&observedPainter{p, v})
	if v.want.name == "" {
		return
	}
	check(v.Document().Model().Text() == v.want.text, "painted revision differs from expected source")
	check(v.draws >= v.want.rows, fmt.Sprintf("%s blank or incomplete paint: got %d text draws, want >=%d", v.want.name, v.draws, v.want.rows))
	check(v.context.created == before, "Paint created native text layouts")
}

type footer struct{ gui.WidgetBase }

func (f *footer) Measure(c layout.Constraint) layout.Measurement {
	return layout.Measured(c.Clamp(geometry.Size{Width: 1, Height: 26}))
}
func (f *footer) Paint(p gui.Painter) {
	p.FillRect(geometry.Rect(0, 0, f.Rect().Width, f.Rect().Height), graphics.ColorOf(color.NRGBA{R: 60, G: 60, B: 60, A: 255}))
}

func main() {
	runtime.LockOSThread()
	basicTheme, dark := flag.Bool("basic", false, "Basic theme"), flag.Bool("dark", false, "Modern dark theme")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewSetText")
	must(err)
	ctx := &observedContext{Context: app.Typography()}
	gui.App = observedApp{app, ctx}
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	if *basicTheme {
		app.SetStyleSheet(basic.Sheet())
	}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 780, Height: 480}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView SetText frame probe"))
	raw := source.NewSourceView()
	raw.SetShowLineNumbers(false)
	raw.SetFoldingEnabled(false)
	raw.SetShowIndentGuides(false)
	raw.SetAutomaticCompletion(false)
	raw.ConnectError(func(err error) { panic(err) })
	v := &observedView{SourceView: raw, context: ctx}
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(v)
	scroll.SetMainWeight(1)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(scroll)
	box.AddChild(&footer{})
	win.SetWidget(box)
	must(win.Show())
	frame, checked, stable, previous := 0, 0, 0, 0
	backend := ""
	enc := json.NewEncoder(os.Stdout)
	win.ConnectAfterPaint(func(p gui.Painter, bounds geometry.Rectangle) {
		frame++
		if backend == "" {
			i, err := p.NewImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
			must(err)
			backend = fmt.Sprintf("%T", i)
			i.Destroy()
		}
		if v.want.name != "" {
			checked++
			must(enc.Encode(map[string]any{"event": "after_paint", "frame": frame, "phase": v.want.name, "revision": raw.Document().Model().Revision(), "draws": v.draws, "scale": p.PixelScale(), "viewport": raw.Rect()}))
		}
		p.FillRect(geometry.Rect(6, bounds.Height-22, 3, 16), graphics.ColorOf(color.NRGBA{R: 255, B: 255, A: 255}))
		for bit := range 16 {
			c := color.NRGBA{A: 255}
			if frame&(1<<bit) != 0 {
				c.R, c.G, c.B = 255, 255, 255
			}
			p.FillRect(geometry.Rect(12+float32(bit)*4, bounds.Height-22, 4, 16), graphics.ColorOf(c))
		}
	})
	samples := []expectation{
		{"short-alpha", strings.Repeat("ALPHA text line\n", 12), 12, false},
		{"short-beta", strings.Repeat("BETA text line\n", 12), 12, false},
		{"equal-content", strings.Repeat("BETA text line\n", 12), 12, false},
		{"wrap-gamma", strings.Repeat("GAMMA wrapped words ", 400), 1, true},
		{"wrap-delta", strings.Repeat("DELTA wrapped words ", 400), 1, true},
		{"long-to-short", "EPSILON short text", 1, true},
		{"many-lines", strings.Repeat("ZETA text line\n", 500), 12, true},
		{"empty", "", 0, true},
		{"after-empty", strings.Repeat("ETA text line\n", 12), 12, false},
		{"paged", "THETA " + strings.Repeat("abcdefghij ", 8000), 1, false},
		{"paged-wrap", "PAGED " + strings.Repeat("wrapped x ", 9000), 1, true},
		{"code", "package main\n\nfunc iota() string {\n  return \"IOTA\"\n}\n", 3, false},
		{"latest-reset", "LAMBDA current text", 1, true},
		{"document-switch", "MU new document", 1, true},
		{"reattach", strings.Repeat("NU reloaded line\n", 12), 12, true},
	}
	index, done := 0, false
	deadline := time.Now().Add(30 * time.Second)
	timer := app.NewTimer()
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), fmt.Sprintf("SetText scenario %d timed out", index))
		must(win.RequestPaint())
		if frame == previous {
			return
		}
		previous = frame
		stable++
		if stable < 3 {
			return
		}
		stable = 0
		if index == len(samples) {
			timer.Stop()
			done = true
			log.Printf("PASS %d SetText scenarios, %d checked native paints; typography=%s painter=%s basic=%t dark=%t", index, checked, app.Typography().Name(), backend, *basicTheme, *dark)
			win.RequestClose()
			return
		}
		want := samples[index]
		v.want = want
		mode := source.WrapNone
		if want.wrap {
			mode = source.WrapWordChar
		}
		raw.SetWrapMode(mode)
		if want.name == "code" {
			must(raw.Document().SetLanguage(language.Go))
		} else {
			must(raw.Document().SetLanguage(language.Plain))
		}
		model := raw.Document().Model()
		revision, created := model.Revision(), ctx.created
		switch want.name {
		case "latest-reset":
			model.SetText("KAPPA superseded before painting")
			model.SetText(want.text)
		case "document-switch":
			raw.SetDocument(source.NewDocument(want.text))
			model.SetText("obsolete detached document")
		case "reattach":
			scroll.SetChild(nil)
			model.SetText(want.text)
			scroll.SetChild(v)
		default:
			model.SetText(want.text)
		}
		if want.name == "equal-content" {
			check(model.Revision() == revision && ctx.created == created, "equal SetText invalidated text or native layouts")
		}
		index++
	})
	must(timer.Start(60 * time.Millisecond))
	app.Run()
	check(done, "SetText verification aborted")
	win.Destroy()
	check(ctx.created == ctx.destroyed, fmt.Sprintf("native layouts leaked: %d created, %d destroyed", ctx.created, ctx.destroyed))
}
