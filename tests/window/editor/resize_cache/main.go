// Recent-width layout cache verification. Requires a desktop and native fonts.
// Run: XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 go run ./tests/window/editor/resize_cache
// Also run at scale 2; other painters/platforms require their own native run.
// Initial state: Modern integrated window, an empty wrapped SourceView inside
// ScrollView, and a button that changes its adjacent pane's allocated width.
// After the first frame, load an original 8 KiB single-word paragraph. Normal
// Window.DispatchEvent clicks narrow the editor, restore its measured width,
// and position the caret at byte 3. The restored width and click must create no
// native layouts and call no cluster measurement, resize or color setter.
// Painter's per-frame occupied-size queries are recorded separately; those are
// normal reads of a measured layout, not invalidation or new shaping requests.
// Both native widths must remain separate; close releases each exactly once.
// This checks reuse across normal layout/paint/input, not bounded first layout
// or continuous resizing responsiveness (both still require finite shaping).
// Automatic assertions fail on early close or timeout; PASS closes the window.
// No clipboard, files or system settings are modified. Restart resets the case.
package main

import (
	"fmt"
	"image/color"
	"log"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
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

type observedApp struct {
	gui.Application
	context *observedContext
}

func (a observedApp) Typography() typography.Context { return a.context }

type counters struct{ created, sizes, metrics, resizes, colors, destroyed int }

func (c counters) sameLayoutWork(other counters) bool {
	return c.created == other.created && c.metrics == other.metrics && c.resizes == other.resizes && c.colors == other.colors && c.destroyed == other.destroyed
}

type observedContext struct {
	typography.Context
	counters
	format typography.TextFormat
}

func (c *observedContext) NewTextLayout(text string, format typography.TextFormat, width, height float32) (typography.TextLayout, error) {
	l, err := c.Context.NewTextLayout(text, format, width, height)
	if err != nil || len(text) < 8192 {
		return l, err
	}
	c.created++
	c.format = format
	return &observedLayout{TextLayout: l, context: c}, nil
}

type observedLayout struct {
	typography.TextLayout
	context *observedContext
	dead    bool
}

func (l *observedLayout) MeasureSize() (float32, float32) {
	l.context.sizes++
	return l.TextLayout.MeasureSize()
}
func (l *observedLayout) MeasureMetrics() ([]typography.TextLine, []typography.TextCluster) {
	l.context.metrics++
	return l.TextLayout.MeasureMetrics()
}
func (l *observedLayout) SetSize(width, height float32) {
	l.context.resizes++
	l.TextLayout.SetSize(width, height)
}
func (l *observedLayout) SetTextColor(start, length int, c color.Color) {
	l.context.colors++
	l.TextLayout.SetTextColor(start, length, c)
}
func (l *observedLayout) Destroy() {
	check(!l.dead, "native layout destroyed twice")
	l.dead = true
	l.context.destroyed++
	l.TextLayout.Destroy()
}

func main() {
	app, err := gui.NewApplication("org.golanggui.SourceViewResizeCache")
	must(err)
	ctx := &observedContext{Context: app.Typography()}
	gui.App = observedApp{app, ctx}
	app.SetStyleSheet(modern.Sheet(modern.Options{}))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 760, Height: 360}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	v := source.NewSourceView()
	v.SetShowLineNumbers(false)
	v.SetFoldingEnabled(false)
	v.SetAutomaticCompletion(false)
	v.SetWrapMode(source.WrapWordChar)
	scroll := gui.NewScrollView()
	scroll.SetChild(v)
	scroll.SetMainWeight(1)
	button := gui.NewButton()
	button.SetChild(gui.NewLabel("Resize pane"))
	wide := false
	setPane := func(width float32) {
		button.SetMinSize(geometry.Size{Width: width})
		button.SetMaxSize(geometry.Size{Width: width})
	}
	setPane(120)
	button.ConnectClicked(func() {
		wide = !wide
		width := float32(120)
		if wide {
			width = 240
		}
		setPane(width)
	})
	box := gui.NewLinearBox(layout.DirectionHorizontal)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(scroll)
	box.AddChild(button)
	win.SetWidget(box)
	must(win.Show())
	click := func(point geometry.Point) {
		for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
			must(win.DispatchEvent(events.PointerEvent{EventType: kind, Position: point, Button: events.PointerButtonLeft}))
		}
	}
	clickButton := func() {
		r := button.Snapshot().Bounds
		click(geometry.Point{X: r.X + r.Width/2, Y: r.Y + r.Height/2})
	}
	frame, previousFrame, stable, stage := 0, 0, 0, 0
	loaded, complete := false, false
	var previous, baseline counters
	var originalWidth float32
	win.ConnectFrame(func(time.Time) { frame++ })
	timer := app.NewTimer()
	defer timer.Stop()
	deadline := time.Now().Add(20 * time.Second)
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), fmt.Sprintf("timeout at stage %d: %+v", stage, ctx.counters))
		must(win.RequestPaint())
		if frame == 0 {
			return
		}
		if !loaded {
			loaded = true
			v.Document().Model().SetText(strings.Repeat("a", 8192))
			return
		}
		if frame > previousFrame && ctx.created > 0 && previous.sameLayoutWork(ctx.counters) {
			stable++
		} else {
			stable = 0
		}
		previous, previousFrame = ctx.counters, frame
		if stable < 4 {
			return
		}
		stable = 0
		log.Printf("stage=%d width=%g counters=%+v", stage, v.Rect().Width, ctx.counters)
		switch stage {
		case 0:
			originalWidth, baseline = v.Rect().Width, ctx.counters
			clickButton()
		case 1:
			check(v.Rect().Width < originalWidth && ctx.created > baseline.created, "narrowing did not produce an independent layout")
			baseline = ctx.counters
			clickButton()
		case 2:
			check(v.Rect().Width == originalWidth && ctx.counters.sameLayoutWork(baseline), "restoring width repeated native layout work")
			glyph, err := ctx.Context.NewTextLayout("a", ctx.format, 100, 100)
			must(err)
			advance, height := glyph.MeasureSize()
			glyph.Destroy()
			r := v.Snapshot().Bounds
			click(geometry.Point{X: r.X + 6 + 3*advance, Y: r.Y + 6 + height/2})
		case 3:
			check(v.Selection() == (textedit.Selection{Anchor: 3, Caret: 3}), "cached geometry did not place caret at byte 3")
			check(ctx.counters.sameLayoutWork(baseline), "caret click repeated native layout work")
			check(ctx.created-ctx.destroyed <= 2, "more than two measured widths retained")
			complete = true
			win.Destroy()
			check(ctx.created == ctx.destroyed, "window destruction leaked native layouts")
			log.Printf("PASS recent width cache: typography=%s", ctx.Name())
			return
		}
		stage++
	})
	must(timer.Start(25 * time.Millisecond))
	app.Run()
	check(complete, "width-cache verification aborted")
}
