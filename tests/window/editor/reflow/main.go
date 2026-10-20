// Deferred resize regression. Requires a desktop and native fonts. Linux Pango
// currently implements the optional deferred capability; other backends report
// unsupported rather than treating a synchronous call as incremental work.
// Run: GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1 go run ./tests/window/editor/reflow
// Add -textview for GOUI TextView, or -file <path> to read existing single-line
// JSON without modifying it. Also run at scale 2 and with opengl.
// Initial state: Modern integrated 760x360 DIP window, a button and wrapped
// 32 KiB paragraph. Line numbers and folding are disabled to keep the test
// text origin explicit (SourceView padding6 / TextView Padding()). Wait for
// READY, then resize the real window repeatedly
// (e.g. xdotool windowsize <id> 680 360, 820 360, 620 360, 900 360 at scale1).
// At scale2 use twice those physical sizes. End at any width and leave it idle.
// The program clicks the button through Window.DispatchEvent while reflow is
// pending, asserts no synchronous SetSize after READY, then checks complete
// native geometry, visible ink, clicking and edit/cancellation. Measurement of
// the final full reference is excluded from heartbeat diagnostics. Logged times
// are process observations, not compositor latency or hardware-independent limits.
// Closes on PASS; early close fails. No clipboard/settings/user-file changes.
// Restart resets all state. Native IME and physical pointer input remain manual.
package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"os"
	"runtime"
	"slices"
	"strings"
	"time"

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
func check(ok bool, msg string) {
	if !ok {
		panic(msg)
	}
}

type observedApp struct {
	gui.Application
	ctx typography.Context
}

func (a observedApp) Typography() typography.Context { return a.ctx }

type context struct {
	typography.Context
	long *tracked
}

func (c *context) NewTextLayout(s string, f typography.TextFormat, w, h float32) (typography.TextLayout, error) {
	l, e := c.Context.NewTextLayout(s, f, w, h)
	if e != nil || len(s) < 8192 {
		return l, e
	}
	r, ok := l.(typography.ReflowLayout)
	check(ok, "backend lacks deferred reflow")
	t := &tracked{ReflowLayout: r}
	c.long = t
	return t, nil
}

type tracked struct {
	typography.ReflowLayout
	pending                          bool
	requests, published, synchronous int
	lastRequest                      time.Time
	longest                          time.Duration
	lines                            []typography.TextLine
	clusters                         []typography.TextCluster
}

func (l *tracked) Reflow(w, h float32) bool {
	ok := l.ReflowLayout.Reflow(w, h)
	if ok {
		l.requests++
		l.pending = true
		l.lastRequest = time.Now()
	}
	return ok
}
func (l *tracked) CancelReflow() { l.pending = false; l.ReflowLayout.CancelReflow() }
func (l *tracked) AdvanceReflow(d time.Time) (bool, error) {
	s := time.Now()
	done, e := l.ReflowLayout.AdvanceReflow(d)
	l.longest = max(l.longest, time.Since(s))
	if done && l.pending {
		l.published++
		l.pending = false
	}
	return done, e
}
func (l *tracked) SetSize(w, h float32) {
	a, b := l.Size()
	if a != w || b != h {
		l.synchronous++
	}
	l.ReflowLayout.SetSize(w, h)
}
func (l *tracked) MeasureMetrics() ([]typography.TextLine, []typography.TextCluster) {
	l.lines, l.clusters = l.ReflowLayout.MeasureMetrics()
	return l.lines, l.clusters
}
func sameLine(a, b typography.TextLine) bool {
	return a.Start == b.Start && a.Length == b.Length && a.X == b.X && a.Y == b.Y && a.Width == b.Width && a.Height == b.Height && a.Baseline == b.Baseline && slices.Equal(a.Clusters, b.Clusters)
}
func main() {
	runtime.LockOSThread()
	plain := flag.Bool("textview", false, "test TextView instead of SourceView")
	file := flag.String("file", "", "existing single-line JSON to read without modification")
	flag.Parse()
	text := strings.Repeat("a", 32768)
	if *file != "" {
		data, e := os.ReadFile(*file)
		must(e)
		text = string(data)
		check(len(text) >= 8192 && !strings.ContainsAny(text, "\r\n"), "expected one long paragraph")
	}
	app, e := gui.NewApplication("org.golanggui.DeferredTextReflow")
	must(e)
	ctx := &context{Context: app.Typography()}
	gui.App = observedApp{app, ctx}
	app.SetStyleSheet(modern.Sheet(modern.Options{}))
	win, e := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 760, Height: 360}, Chrome: gui.WindowChromeIntegrated})
	must(e)
	defer win.Destroy()
	must(win.SetTitle("Deferred text reflow regression"))
	var view gui.Widget
	var model *textedit.Model
	var selection func() textedit.Selection
	inset := float32(6)
	if *plain {
		v := gui.NewTextView()
		view = v
		model = v.Model()
		selection = v.Selection
		inset = v.Padding()
	} else {
		v := source.NewSourceView()
		v.SetWrapMode(source.WrapWordChar)
		v.SetShowLineNumbers(false)
		v.SetFoldingEnabled(false)
		v.SetAutomaticCompletion(false)
		if *file != "" {
			must(v.Document().SetLanguage(language.JSON))
		}
		view = v
		model = v.Document().Model()
		selection = v.Selection
	}
	scroll := gui.NewScrollView()
	scroll.SetChild(view)
	scroll.SetMainWeight(1)
	button := gui.NewButton()
	button.SetMinSize(geometry.Size{Height: 30})
	clicks := 0
	button.ConnectClicked(func() { clicks++ })
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(button)
	box.AddChild(scroll)
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(view)
	click := func(p geometry.Point) {
		for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
			must(win.DispatchEvent(events.PointerEvent{EventType: kind, Position: p, Button: events.PointerButtonLeft}))
		}
	}
	loaded, ready, complete := false, false, false
	deadline := time.Now().Add(2 * time.Minute)
	lastTick := time.Now()
	maxGap := time.Duration(0)
	pendingTicks := 0
	diagnostic := time.Now()
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		now := time.Now()
		check(now.Before(deadline), "resize validation timed out")
		if !loaded {
			if view.Rect().Width <= 0 {
				return
			}
			loaded = true
			model.SetText(text)
			return
		}
		l := ctx.long
		if l == nil || len(l.lines) == 0 {
			return
		}
		if !ready {
			if time.Since(diagnostic) > time.Second {
				diagnostic = time.Now()
				log.Printf("initial layout requests=%d publications=%d pending=%v box=%v lines=%d", l.requests, l.published, l.pending, view.Rect(), len(l.lines))
			}
			if l.pending {
				return
			}
			ready = true
			l.requests, l.published, l.synchronous = 0, 0, 0
			l.longest = 0
			lastTick = now
			log.Printf("READY controlTextView=%v: resize window several times", *plain)
			return
		}
		maxGap = max(maxGap, now.Sub(lastTick))
		lastTick = now
		if l.pending {
			pendingTicks++
			before := clicks
			click(button.Snapshot().Bounds.Center())
			check(clicks == before+1, "button unresponsive while reflow pending")
			win.SetFocusedWidget(view)
		}
		if l.requests < 2 || l.pending || time.Since(l.lastRequest) < 500*time.Millisecond {
			return
		}
		check(l.synchronous == 0, fmt.Sprintf("resize used synchronous layout %d times", l.synchronous))
		check(pendingTicks >= 2, "no event-loop intervals observed between reflow steps")
		check(l.published >= 1, "no deferred layout published")
		log.Printf("resize requests=%d publications=%d pendingTimerTicks=%d maxTimerGap=%s longestAdvance=%s", l.requests, l.published, pendingTicks, maxGap, l.longest)
		w, h := l.Size()
		reference, e := ctx.Context.NewTextLayout(text, l.Format(), w, h)
		must(e)
		lines, clusters := reference.MeasureMetrics()
		check(slices.EqualFunc(lines, l.lines, sameLine) && slices.Equal(clusters, l.clusters), "published geometry differs from full native reference")
		reference.Destroy()
		r := image.Rect(0, 0, int(w), 180)
		bitmap, e := l.Rasterize(1, r, nil)
		must(e)
		ink := false
		for i := 3; i < len(bitmap.Pixels); i += 4 {
			ink = ink || bitmap.Pixels[i] > 0
		}
		check(ink, "new layout has no visible ink")
		var target typography.TextCluster
		for _, c := range l.clusters {
			if c.Start == 3 {
				target = c
				break
			}
		}
		b := view.Snapshot().Bounds
		click(geometry.Point{X: b.X + inset + target.X + target.Width/4, Y: b.Y + inset + target.Y + target.Height/2})
		check(selection().Caret == 3, fmt.Sprintf("caret after reflow=%d want3", selection().Caret))
		// A subsequent edit drops the long paragraph and any work tied to it.
		model.SetText("edited")
		check(!l.pending, "edited paragraph retained pending reflow")
		complete = true
		log.Printf("PASS deferred resize, interaction and geometry")
		must(win.RequestClose())
	})
	must(timer.Start(20 * time.Millisecond))
	app.Run()
	check(complete, "window closed before validation")
}
