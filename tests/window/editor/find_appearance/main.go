// Find overlay appearance and isolation verification. Requires a real desktop,
// native typography and fonts. Run: go run ./tests/window/editor/find_appearance
// (Linux without IME service: XMODIFIERS=@im=none). Optional -capture-dir writes
// PNGs to that existing directory; use a temporary directory outside the repo.
// Initially two editors use Light/Dark in one ordinary window. Automatically
// opens both through dispatched Ctrl/Cmd+F, expands replacement with the normal
// shortcut, checks geometry/colors/focus/hit testing, switches one theme and
// dispatches smaller/restored logical window size events. This exercises layout,
// not native window-manager resizing. Assertions use independent palette constants and
// allocated semantic bounds. Text entry uses CommitText. Closes automatically;
// early close or failed assertion exits nonzero. No clipboard or system settings
// change. Restart to reset. -inspect keeps the restored light/dark window open
// after checks for manual resize/hover/keyboard inspection; close normally.
// Native IME and remote desktop compositing are manual.
package main

import (
	"flag"
	"fmt"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/widgets"
)

func check(ok bool, msg string) {
	if !ok {
		log.Fatal(msg)
	}
}
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
func named(w gui.Widget, name string) gui.Widget {
	if w.Name() == name {
		return w
	}
	for _, c := range w.Children() {
		if r := named(c, name); r != nil {
			return r
		}
	}
	return nil
}
func main() {
	runtime.LockOSThread()
	inspect := flag.Bool("inspect", false, "keep the final window open for manual inspection")
	out := flag.String("capture-dir", "", "existing temporary directory for screenshots")
	flag.Parse()
	app, err := gui.NewApplication("org.golang-gui.SourceViewFindAppearance")
	must(err)
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1040, Height: 400}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("Find overlay — Light / Dark"))
	row := gui.NewLinearBox(layout.DirectionHorizontal)
	row.SetCrossAlign(layout.CrossStretch)
	var editors [2]*source.SourceView
	var bars [2]*source.FindBar
	var overlays [2]*gui.Overlay
	for i := range editors {
		v := source.NewSourceView()
		v.SetDocument(source.NewDocument("one one\n\neditor content\n"))
		if i == 0 {
			v.SetScheme(scheme.Light())
		} else {
			v.SetScheme(scheme.Dark())
		}
		scroll := gui.NewScrollView()
		scroll.SetChild(v)
		overlay := gui.NewOverlay()
		overlay.SetChild(scroll)
		overlay.SetMainWeight(1)
		bar := source.NewFindBar()
		bar.SetTarget(v)
		overlay.AddOverlay(bar)
		overlay.ConnectOverlayPosition(bar, func(available, size geometry.Size, p *geometry.Point) { p.X = max(0, available.Width-size.Width-8) })
		row.AddChild(overlay)
		editors[i], bars[i], overlays[i] = v, bar, overlay
	}
	win.SetWidget(row)
	must(win.Show())
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	primary, _ := gui.ModPrimary.Resolve()
	replace := func() {
		if runtime.GOOS == "darwin" {
			alt, _ := gui.ModAlt.Resolve()
			key(events.KeyF, primary|alt)
		} else {
			key(events.KeyH, primary)
		}
	}
	var before [2]geometry.Rectangle
	sample := func(i int, dark bool, tag string) {
		overlay, bar := overlays[i], bars[i]
		img, err := gui.RenderWidget(overlay, 0)
		must(err)
		bounds, bb := overlay.Snapshot().Bounds, bar.Snapshot().Bounds
		scale := float32(img.Bounds().Dx()) / overlay.Rect().Width
		get := func(x, y float32) color.NRGBA {
			return color.NRGBAModel.Convert(img.At(int((bb.X-bounds.X+x)*scale), int((bb.Y-bounds.Y+y)*scale))).(color.NRGBA)
		}
		expected := uint8(243)
		if dark {
			expected = 37
		}
		expectedBlue := expected
		if dark {
			expectedBlue++
		}
		// Panel's empty bottom-right interior, away from glyphs/border/AA.
		actual := get(bar.Rect().Width-20, bar.Rect().Height-12)
		check(actual.R == expected && actual.G == expected && actual.B == expectedBlue, fmt.Sprintf("panel palette %s: %+v", tag, actual))
		input := named(bar, "Replacement")
		if !input.Visible() {
			input = named(bar, "Find")
		}
		ib := input.Snapshot().Bounds
		c := color.NRGBAModel.Convert(img.At(int((ib.X-bounds.X+1)*scale), int((ib.Y-bounds.Y+1)*scale))).(color.NRGBA)
		expected = 255
		if dark {
			expected = 60
		}
		check(c.R == expected && c.G == expected && c.B == expected, fmt.Sprintf("native input palette %s: %+v", tag, c))
		if *out != "" {
			f, err := os.Create(filepath.Join(*out, tag+".png"))
			must(err)
			must(png.Encode(f, img))
			must(f.Close())
		}
	}
	scale := float32(1)
	stage, done := 0, false
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		switch stage {
		case 0:
			img, err := gui.RenderWidget(overlays[0], 0)
			must(err)
			scale = float32(img.Bounds().Dx()) / overlays[0].Rect().Width
			for i := range editors {
				before[i] = editors[i].Rect()
				win.SetFocusedWidget(editors[i])
				key(events.KeyF, primary)
				must(win.CommitText("one"))
			}
		case 1:
			for i, bar := range bars {
				check(bar.Opened() && !bar.ReplaceMode(), "collapsed open state")
				check(editors[i].Rect() == before[i], "overlay changed editor allocation")
				check(bar.Rect().Width == 448 && bar.Rect().Height < 56, "compact natural bounds")
				check(named(bar, "Match case").Snapshot().Attributes[widgets.CheckInfoKey].(widgets.CheckInfo).State == widgets.CheckChecked, "selected option semantics")
				check(named(bar, "Find").Rect().Width > 120, "input too narrow")
				// Outside the floating rectangle normal hit testing still reaches content.
				picked := gui.Pick(overlays[i], geometry.Point{X: 30, Y: 160})
				check(picked == editors[i], "overlay blocked editor content")
				sample(i, i == 1, fmt.Sprintf("%d-collapsed", i))
				win.SetFocusedWidget(editors[i])
				replace()
			}
		case 2:
			for i, bar := range bars {
				check(bar.ReplaceMode() && bar.Rect().Height < 90, "expanded compact bounds")
				sample(i, i == 1, fmt.Sprintf("%d-expanded", i))
			}
			// Change only the right editor. Left controls must retain their own sheet.
			editors[1].SetScheme(scheme.Light())
		case 3:
			sample(0, false, "light-isolated")
			sample(1, false, "dark-to-light")
			must(win.DispatchEvent(events.SizeEvent{Width: 480, Height: 400, PixelWidth: 480 * scale, PixelHeight: 400 * scale}))
		case 4:
			for i, bar := range bars {
				check(bar.Rect().Width <= 240, "narrow overlay escaped viewport")
				check(!named(bar, "Previous").Visible() && !named(bar, "Match case").Visible(), "narrow secondary controls")
				input := named(bar, "Find")
				check(input.Rect().Width >= 90, "narrow input unusable")
				sample(i, false, fmt.Sprintf("%d-narrow", i))
				// Query remains usable after controls are hidden; Escape restores editor.
				win.SetFocusedWidget(input)
				key(events.KeyEnter, 0)
				check(editors[i].Selection().Range().End > 0, "narrow Enter search")
				key(events.KeyEscape, 0)
				check(!bar.Opened() && editors[i].Focused(), "narrow close focus")
			}
			must(win.DispatchEvent(events.SizeEvent{Width: 1040, Height: 400, PixelWidth: 1040 * scale, PixelHeight: 400 * scale}))
		case 5:
			for i := range bars {
				win.SetFocusedWidget(editors[i])
				key(events.KeyF, primary)
			}
		case 6:
			for _, bar := range bars {
				check(named(bar, "Previous").Visible() && named(bar, "Match case").Visible(), "wide controls not restored")
			}
			done = true
			log.Print("PASS floating find geometry, light/dark native-input colors, independent schemes, collapsed/expanded, narrow/restore, focus and hit testing")
			if *inspect {
				timer.Stop()
				editors[1].SetScheme(scheme.Dark())
				for _, v := range editors {
					win.SetFocusedWidget(v)
					replace()
				}
				return
			}
			win.Destroy()
		}
		stage++
	})
	must(timer.Start(200 * time.Millisecond))
	app.Run()
	check(done, "appearance verification aborted")
}
