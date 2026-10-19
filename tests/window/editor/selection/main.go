// Syntax-preserving selection verification. Requires a desktop/window manager
// and native fonts. Run: go run ./tests/window/editor/selection [-capture-dir DIR]
// (Linux without IME service: XMODIFIERS=@im=none). DIR must already exist outside
// the repository. Initial state: a read-only editor and a second focus target.
// Automatically checks JSON/XML/HTML under Basic and Modern Light/Dark, using
// Window.DispatchEvent for select-all, reversed/changed selection, focus traversal
// and context menus. Expected syntax spans are literal fixtures, independent of
// the lexer; cached native bitmaps are compared with separately colored layouts.
// Default selection must retain syntax without recoloring/reshaping on movement
// or focus changes. Explicit foreground must apply and clear correctly, including
// menu activity and return to default. Pixel assertions sample a trailing space
// inside the selection/search intersection, away from glyph AA/caret/borders,
// in sRGB with opaque destination and standard premultiplied source-over. One
// byte of error is allowed for premultiplication/8-bit rounding, not font AA.
// Checks run at the configured painter/scale, close automatically and exit
// nonzero on failure or early close. Optional PNGs show the default selection.
// No clipboard/system settings changes; restarting resets. Physical keyboard,
// IME, desktop compositing and mixed-monitor DPI remain manual verification.
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
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
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/modern"
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
func check(ok bool, message string) {
	if !ok {
		log.Fatal(message)
	}
}
func rgba(c color.Color) color.NRGBA { return color.NRGBAModel.Convert(c).(color.NRGBA) }

type part struct {
	text string
	kind language.Kind
}
type sample struct {
	id    language.ID
	parts []part
}

func (s sample) text() string {
	var b strings.Builder
	for _, p := range s.parts {
		b.WriteString(p.text)
	}
	return b.String()
}

//go:embed testdata/syntax.json
var sampleData []byte

var samples = loadSamples()

func loadSamples() []sample {
	var data []struct {
		Language language.ID
		Parts    []struct{ Text, Kind string }
	}
	must(json.Unmarshal(sampleData, &data))
	check(len(data) > 0, "empty syntax fixtures")
	result := make([]sample, 0, len(data))
	for _, entry := range data {
		_, ok := language.Lookup(entry.Language)
		check(ok && len(entry.Parts) > 0, "invalid fixture language or empty parts")
		s := sample{id: entry.Language}
		for _, p := range entry.Parts {
			kind := language.Kind(p.Kind)
			if p.Kind == "plain" {
				kind = language.KindPlain
			}
			check(kind.Valid() && p.Text != "", "invalid fixture part")
			s.parts = append(s.parts, part{text: p.Text, kind: kind})
		}
		result = append(result, s)
	}
	return result
}

type trackedLayout struct {
	typography.TextLayout
	colors []color.NRGBA
	calls  int
	owner  *trackedContext
	dead   bool
}

func (l *trackedLayout) Destroy() {
	check(!l.dead, "document layout destroyed twice")
	l.dead = true
	l.owner.live--
	l.TextLayout.Destroy()
}
func (l *trackedLayout) SetTextColor(start, length int, c color.Color) {
	check(!l.dead, "recolored released document layout")
	l.calls++
	for i := start; i < start+length; i++ {
		l.colors[i] = rgba(c)
	}
	l.TextLayout.SetTextColor(start, length, c)
}

type trackedContext struct {
	typography.Context
	last    map[string]*trackedLayout
	created map[string]int
	live    int
}

func (c *trackedContext) NewTextLayout(s string, f typography.TextFormat, w, h float32) (typography.TextLayout, error) {
	l, err := c.Context.NewTextLayout(s, f, w, h)
	if err != nil {
		return nil, err
	}
	for _, sample := range samples {
		if sample.text() == s {
			colors := make([]color.NRGBA, len(s))
			for i := range colors {
				colors[i] = rgba(f.TextColor)
			}
			tracked := &trackedLayout{TextLayout: l, colors: colors, owner: c}
			c.last[s] = tracked
			c.created[s]++
			c.live++
			return tracked, nil
		}
	}
	return l, nil
}

type trackedApp struct {
	gui.Application
	context typography.Context
}

func (a trackedApp) Typography() typography.Context { return a.context }

func main() {
	runtime.LockOSThread()
	out := flag.String("capture-dir", "", "existing temporary directory for PNGs")
	flag.Parse()
	app, err := gui.NewApplication("org.golang-gui.SourceViewSelection")
	must(err)
	ctx := &trackedContext{Context: app.Typography(), last: make(map[string]*trackedLayout), created: make(map[string]int)}
	gui.App = trackedApp{Application: app, context: ctx}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 900, Height: 300}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView — syntax-preserving selection"))
	v := source.NewSourceView()
	v.SetShowLineNumbers(false)
	v.SetFoldingEnabled(false) // Isolate selection pixels at the documented 6-DIP content origin.
	v.SetReadOnly(true)
	scroll := gui.NewScrollView()
	scroll.SetChild(v)
	scroll.SetMainWeight(1)
	other := gui.NewButton()
	other.SetChild(gui.NewLabel("Tab: focus here; Shift+Tab: return to editor"))
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(scroll)
	box.AddChild(other)
	win.SetWidget(box)
	must(win.Show())
	primary, _ := gui.ModPrimary.Resolve()
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	caseIndex, stage := 0, 0
	var def scheme.Definition
	var cached *trackedLayout
	var created, calls int
	var scale float32
	done := false
	current := func() sample { return samples[caseIndex%len(samples)] }
	assertText := func(override bool, retain bool) {
		s := current()
		text := s.text()
		l := ctx.last[text]
		check(l != nil && l == cached && ctx.created[text] == created, "selection reshaped native text")
		if retain {
			check(l.calls == calls, "default selection/focus recolored native text")
		}
		f := l.Format()
		f.TextColor = def.Foreground
		fresh, err := ctx.Context.NewTextLayout(text, f, 1<<20, 1<<24)
		must(err)
		defer fresh.Destroy()
		wanted := make([]color.NRGBA, len(text))
		at := 0
		for _, p := range s.parts {
			c := def.Foreground
			if p.kind != language.KindPlain {
				c = def.Tokens[p.kind]
			}
			for i := at; i < at+len(p.text); i++ {
				wanted[i] = c
			}
			fresh.SetTextColor(at, len(p.text), c)
			at += len(p.text)
		}
		if override {
			r := v.Selection().Range()
			for i := r.Start; i < r.End; i++ {
				wanted[i] = def.SelectionForeground
			}
			fresh.SetTextColor(r.Start, r.End-r.Start, def.SelectionForeground)
		}
		for i, c := range wanted {
			check(l.colors[i] == c, fmt.Sprintf("%s theme=%d byte=%d color=%v want=%v", s.id, caseIndex/3, i, l.colors[i], c))
		}
		got, err := l.Rasterize(1, nil)
		must(err)
		pixels := bytes.Clone(got.Pixels)
		want, err := fresh.Rasterize(1, nil)
		must(err)
		check(got.Width == want.Width && got.Height == want.Height && got.Stride == want.Stride && bytes.Equal(pixels, want.Pixels), "native bitmap differs from independent syntax/selection spans")
	}
	assertBackground := func(active, search bool) {
		text := current().text()
		lines, clusters := cached.MeasureMetrics()
		g := textedit.NewGeometry(lines, clusters, len(text), 22)
		r := g.Selection(textedit.Range{Start: len(text) - 3, End: len(text) - 2})[0]
		img, err := gui.RenderWidget(v, 0)
		must(err)
		scale = float32(img.Bounds().Dx()) / v.Rect().Width
		got := rgba(img.At(int((6+r.X+r.Width/2)*scale), int((6+r.Y+r.Height/2)*scale)))
		base := def.Background
		if active {
			base = def.CurrentLine
		}
		if search {
			base = def.SearchBackground
		}
		tint := def.InactiveSelectionBackground
		if active {
			tint = def.SelectionBackground
		}
		blend := func(bg, fg uint8) uint8 { return uint8((int(bg)*(255-int(tint.A)) + int(fg)*int(tint.A) + 127) / 255) }
		want := color.NRGBA{R: blend(base.R, tint.R), G: blend(base.G, tint.G), B: blend(base.B, tint.B), A: 255}
		check(math.Abs(float64(got.R)-float64(want.R)) <= 1 && math.Abs(float64(got.G)-float64(want.G)) <= 1 && math.Abs(float64(got.B)-float64(want.B)) <= 1 && got.A == 255, fmt.Sprintf("selection alpha active=%t search=%t got=%v want=%v", active, search, got, want))
		if *out != "" && active && search {
			f, err := os.Create(filepath.Join(*out, fmt.Sprintf("%d-%s.png", caseIndex/3, current().id)))
			must(err)
			must(png.Encode(f, img))
			must(f.Close())
		}
	}
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		s := current()
		text := s.text()
		switch stage {
		case 0:
			theme := caseIndex / 3
			if theme == 0 {
				app.SetStyleSheet(nil)
			} else {
				app.SetStyleSheet(modern.Sheet(modern.Options{Dark: theme == 2}))
			}
			v.SetScheme(nil)
			v.SetSearch(source.SearchQuery{})
			v.Document().Model().SetText(text)
			must(v.Document().SetLanguage(s.id))
			win.SetFocusedWidget(v)
			key(events.KeyA, primary)
			base := scheme.Light()
			if theme == 2 {
				base = scheme.Dark()
			}
			def = base.Definition()
			root := gui.ResolveWidgetStyle(v, "source-view", "", style.Normal)
			c, _ := root.BackgroundColor()
			def.Background = rgba(c)
			c, _ = root.ForegroundColor()
			def.Foreground = rgba(c)
			def.SelectionBackground = backgroundOf(v, "selection")
			def.InactiveSelectionBackground = backgroundOf(v, "inactive-selection")
		case 1:
			check(win.Focused() && v.Focused(), "editor not actively focused")
			cached = ctx.last[text]
			created = ctx.created[text]
			check(cached != nil, "native line unavailable")
			calls = cached.calls
			assertText(false, true)
			assertBackground(true, false)
			v.SetSearch(source.SearchQuery{Text: "    "})
		case 2:
			assertText(false, true)
			assertBackground(true, true)
			key(events.KeyArrowLeft, events.ModifierShift)
		case 3:
			assertText(false, true)
			key(events.KeyEnd, primary)
			key(events.KeyHome, primary|events.ModifierShift)
		case 4:
			check(v.Selection().Anchor == len(text) && v.Selection().Caret == 0, "reverse selection not created")
			assertText(false, true)
			key(events.KeyTab, 0)
		case 5:
			check(!v.Focused() && other.Focused(), "Tab did not change focus")
			assertText(false, true)
			assertBackground(false, true)
			key(events.KeyTab, events.ModifierShift)
		case 6:
			check(v.Focused(), "Shift+Tab did not restore focus")
			assertText(false, true)
			def.SelectionForeground = color.NRGBA{R: 255, G: 60, B: 180, A: 255}
			explicit, err := scheme.New(def)
			must(err)
			v.SetScheme(explicit)
		case 7:
			assertText(true, false)
			key(events.KeyArrowRight, 0)
		case 8:
			check(v.Selection().Range().Start == v.Selection().Range().End, "selection did not collapse")
			assertText(false, false)
			calls = cached.calls
			key(events.KeyArrowLeft, 0)
		case 9:
			assertText(false, true)
			key(events.KeyA, primary)
		case 10:
			assertText(true, false)
			key(events.KeyTab, 0)
		case 11:
			check(other.Focused(), "explicit selection did not lose focus")
			assertText(false, false)
			key(events.KeyTab, events.ModifierShift)
		case 12:
			assertText(true, false)
			key(events.KeyF10, events.ModifierShift)
		case 13:
			check(len(win.Popovers()) == 1, "context menu unavailable")
			assertText(true, false)
			key(events.KeyEscape, 0)
			v.SetScheme(nil)
		case 14:
			assertText(false, false)
			log.Printf("PASS selection %s theme=%d scale=%g backend=%s", s.id, caseIndex/3, scale, app.Typography().Name())
			caseIndex++
			stage = -1
			if caseIndex == 3*len(samples) {
				done = true
				win.Destroy()
				return
			}
		}
		stage++
	})
	must(timer.Start(140 * time.Millisecond))
	app.Run()
	check(done, "selection verification aborted")
	check(ctx.live == 0, "document layouts leaked after window destruction")
}

func backgroundOf(v gui.Widget, part string) color.NRGBA {
	c, _ := gui.ResolveWidgetStyle(v, "source-view", part, style.Normal).BackgroundColor()
	return rgba(c)
}
