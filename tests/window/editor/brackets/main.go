// Bracket decoration visual verification. Requires a desktop/window manager and
// native fonts. Run: go run ./tests/window/editor/brackets [-dark] [-case CASE]
// [-auto]. CASE is nested, adjacent, multiline, wrap, search, unmatched, selected
// or unfocused. Linux without IME service: XMODIFIERS=@im=none.
// Initial: a read-only JSON editor, selected pair at the caret. Use Light/Dark
// buttons to compare themes; refocus the editor after operating other windows.
// Expect a soft background and 1-DIP outline at both matched brackets, original
// syntax foregrounds, no underline or oversized row box. Check 1x/2x, nested and
// wrapped/multiline pairs. Search keeps its background inside the outline.
// Unmatched/selected/unfocused cases have no bracket decoration. -auto checks
// native focus, layout/selection and painter scale, emits READY, then exits after
// three seconds; screen pixels are inspected externally, not asserted by READY.
// No clipboard, file or setting changes. Reset by restarting. Close to exit;
// early close in -auto is a failure. Native IME is outside this test's scope.
package main

import (
	"flag"
	"fmt"
	"image"
	"log"
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
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
func main() {
	runtime.LockOSThread()
	dark := flag.Bool("dark", false, "dark code scheme")
	scenario := flag.String("case", "nested", "nested/adjacent/multiline/wrap/search/unmatched/selected/unfocused")
	auto := flag.Bool("auto", false, "check native setup, emit READY and close after three seconds")
	flag.Parse()
	text := `{"items": [1, {"active": true}]}`
	switch *scenario {
	case "nested", "search", "selected", "unfocused":
	case "adjacent":
		text = `{"items": []}`
	case "multiline":
		text = "{\n    \"items\": [\n        1,\n        {\"active\": true}\n    ]\n}"
	case "wrap":
		text = `{"items": ["` + strings.Repeat("a long line ", 12) + `"]}`
	case "unmatched":
		text = `{"items": [1, 2}`
	default:
		log.Fatalf("unknown case %q", *scenario)
	}
	app, err := gui.NewApplication("org.golang-gui.SourceViewBracketPreview")
	must(err)
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 680, Height: 310}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("GOUI SourceView bracket preview"))
	doc := source.NewDocument(text)
	must(doc.SetLanguage(language.JSON))
	v := source.NewSourceView()
	v.SetDocument(doc)
	v.SetReadOnly(true)
	if *scenario == "wrap" {
		v.SetWrapMode(source.WrapChar)
	}
	at := strings.Index(text, "[")
	selection := textedit.Selection{Anchor: at, Caret: at}
	if *scenario == "selected" {
		selection.Caret = strings.Index(text, "]") + 1
	}
	v.SetSelection(selection)
	if *scenario == "search" {
		v.SetSearch(source.SearchQuery{Text: "["})
	}
	toolbar := gui.NewLinearBox(layout.DirectionHorizontal)
	toolbar.SetSpacing(8)
	apply := func(s *scheme.Scheme) { v.SetScheme(s); win.SetFocusedWidget(v) }
	lightButton, darkButton := gui.NewButton(), gui.NewButton()
	lightButton.SetChild(gui.NewLabel("Light"))
	darkButton.SetChild(gui.NewLabel("Dark"))
	lightButton.ConnectClicked(func() { apply(scheme.Light()) })
	darkButton.ConnectClicked(func() { apply(scheme.Dark()) })
	toolbar.AddChild(lightButton)
	toolbar.AddChild(darkButton)
	toolbar.AddChild(gui.NewLabel("Bracket matching · " + *scenario))
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetMainWeight(1)
	scroll.SetChild(v)
	probe := &scaleProbe{}
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(toolbar)
	box.AddChild(scroll)
	box.AddChild(probe)
	win.SetWidget(box)
	if *dark {
		apply(scheme.Dark())
	} else {
		apply(scheme.Light())
	}
	must(win.Show())
	win.SetFocusedWidget(v)
	if *scenario == "unfocused" {
		win.SetFocusedWidget(lightButton)
	}
	complete := false
	if *auto {
		ready := app.NewTimer()
		ready.ConnectTimeout(func() {
			info := v.Snapshot()
			if !win.Focused() || info.TextEditing == nil || info.TextEditing.Selection != selection || info.Bounds.Width <= 0 || probe.scale <= 0 {
				log.Fatal("native focus, layout or selection was not ready")
			}
			log.Printf("READY case=%s dark=%t typography=%s painter=%s scale=%g", *scenario, *dark, app.Typography().Name(), probe.painter, probe.scale)
		})
		must(ready.StartOnce(750 * time.Millisecond))
		exit := app.NewTimer()
		exit.ConnectTimeout(func() { complete = true; win.Destroy() })
		must(exit.StartOnce(3 * time.Second))
	}
	app.Run()
	if *auto && !complete {
		log.Fatal("preview aborted before completion")
	}
}

type scaleProbe struct {
	gui.WidgetBase
	scale   float32
	painter string
}

func (p *scaleProbe) Measure(c layout.Constraint) layout.Measurement {
	return layout.Measured(c.Clamp(geometry.Size{Width: 1, Height: 1}))
}
func (p *scaleProbe) Paint(draw gui.Painter) {
	if p.scale != 0 {
		return
	}
	p.scale = draw.PixelScale()
	img, err := draw.NewImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	must(err)
	defer img.Destroy()
	p.painter = fmt.Sprintf("%T", img)
}
