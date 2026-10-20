// TOML/YAML/Shell integration verification. Requires a desktop/window manager,
// native typography and fonts. Run: go run ./tests/window/editor/languages
// [-dark]. Linux without IME service: XMODIFIERS=@im=none. Starts one editable
// view, switches six documents, requests builtin completion, accepts it by
// Enter, checks undo and types quotes/brackets through the normal input path.
// Checks readonly and semantic text/selection, then closes automatically.
// Color correctness has independent native bitmap tests; this program does not
// infer pixel acceptance from window startup. Physical IME/system shortcuts
// remain manual. Logs actual typography/painter/scale. No clipboard, file or
// settings changes; restart resets. Early close or failed assertion exits nonzero.
// Each completion waits up to 8 seconds for an allocated viewport before
// inspecting virtual rows; Show does not imply the first layout has completed.
package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"runtime"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
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
func completionInfo(info gui.WidgetInfo) gui.WidgetInfo {
	if info.Role == "completion-list" {
		return info
	}
	for _, child := range info.Children {
		if found := completionInfo(child); found.Role != "" {
			return found
		}
	}
	return gui.WidgetInfo{}
}

func main() {
	runtime.LockOSThread()
	dark := flag.Bool("dark", false, "Modern dark appearance")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewLanguages")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 720, Height: 420}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView language integration"))
	v := source.NewSourceView()
	bindings := v.KeyBindings()
	bindings[source.CommandComplete] = []gui.KeyGesture{{Key: gui.KeyF2}}
	must(v.SetKeyBindings(bindings))
	v.SetAutomaticCompletion(false)
	scroll := gui.NewScrollView()
	scroll.SetChild(v)
	scroll.SetMainWeight(1)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(gui.NewLabel("TOML · YAML · Shell — completion, quotes and history"))
	box.AddChild(scroll)
	probe := &scaleProbe{}
	box.AddChild(probe)
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(v)
	primary, _ := gui.ModPrimary.Resolve()
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	fixtures := []struct {
		id         language.ID
		text, want string
	}{
		{language.TOML, "enabled = tr", "enabled = true"},
		{language.YAML, "enabled: nu", "enabled: null"},
		{language.Shell, "pri", "printf"},
		{language.JSON, "tr", "true"}, {language.XML, "<x xml:l", "<x xml:lang"}, {language.HTML, "<di", "<div"},
	}
	var steps []func()
	var pendingViewport func() bool
	var viewportDeadline time.Time
	for _, tc := range fixtures {
		steps = append(steps, func() {
			d := source.NewDocument(tc.text)
			must(d.SetLanguage(tc.id))
			v.SetDocument(d)
			v.SetSelection(textedit.Selection{Anchor: len(tc.text), Caret: len(tc.text)})
			win.SetFocusedWidget(v)
		})
		steps = append(steps, func() {
			key(events.KeyF2, 0)
			check(len(win.Popovers()) == 1, "missing completion for "+string(tc.id))
			viewportDeadline = time.Now().Add(8 * time.Second)
			pendingViewport = func() bool {
				check(len(win.Popovers()) == 1, "completion closed before layout for "+string(tc.id))
				info := completionInfo(win.Popovers()[0].Widget().Snapshot())
				return info.Bounds.Width > 0 && info.Bounds.Height > 0
			}
		})
		steps = append(steps, func() {
			info := completionInfo(win.Popovers()[0].Widget().Snapshot())
			found := -1
			for i, row := range info.Children {
				if row.Text == tc.want[len(tc.text)-len(completionPrefix(tc.id)):] {
					found = i
					break
				}
			}
			check(found >= 0, "missing expected proposal for "+string(tc.id))
			for i := 0; i < found; i++ {
				key(events.KeyArrowDown, 0)
			}
			key(events.KeyEnter, 0)
			check(v.Document().Model().Text() == tc.want, "wrong completion for "+string(tc.id))
			key(events.KeyZ, primary)
			check(v.Document().Model().Text() == tc.text, "completion undo failed")
			log.Printf("PASS %s completion and undo", tc.id)
		})
	}
	for _, tc := range []struct {
		id                  language.ID
		prefix, typed, want string
	}{
		{language.TOML, "text = ", `"""`, `text = """`},
		{language.YAML, "text: ", "'it''s'", "text: 'it''s'"},
		{language.Shell, "echo ", "'\\'", "echo '\\'"},
	} {
		steps = append(steps, func() {
			d := source.NewDocument(tc.prefix)
			must(d.SetLanguage(tc.id))
			v.SetDocument(d)
			v.SetSelection(textedit.Selection{Anchor: len(tc.prefix), Caret: len(tc.prefix)})
		})
		for _, r := range tc.typed {
			steps = append(steps, func() { must(win.CommitText(string(r))) })
		}
		steps = append(steps, func() {
			check(v.Document().Model().Text() == tc.want && v.Selection().Caret == len(tc.want), "quote typing failed: "+string(tc.id))
			log.Printf("PASS %s quote typing", tc.id)
		})
	}
	steps = append(steps, func() {
		d := source.NewDocument("")
		must(d.SetLanguage(language.Shell))
		v.SetDocument(d)
		v.SetSelection(textedit.Selection{})
	})
	steps = append(steps, func() {
		must(win.CommitText("{"))
		check(v.Document().Model().Text() == "{}" && v.Selection().Caret == 1, "bracket insertion failed")
		v.SetReadOnly(true)
		check(win.CommitText("x") != nil, "readonly accepted text commit")
		check(v.Document().Model().Text() == "{}", "readonly edited text")
		v.SetReadOnly(false)
		check(v.Snapshot().TextEditing != nil, "missing editing semantics")
	})
	complete := false
	at := 0
	timer := app.NewTimer()
	timer.ConnectTimeout(func() {
		if pendingViewport != nil {
			if !pendingViewport() {
				check(time.Now().Before(viewportDeadline), "completion viewport was not allocated within 8 seconds")
				return
			}
			pendingViewport = nil
		}
		if at < len(steps) {
			steps[at]()
			at++
			return
		}
		check(probe.scale > 0, "painter not ready")
		log.Printf("PASS language integration; typography=%s painter=%s scale=%g dark=%t", app.Typography().Name(), probe.painter, probe.scale, *dark)
		complete = true
		win.Destroy()
	})
	must(timer.Start(160 * time.Millisecond))
	app.Run()
	check(complete, "language integration aborted")
}
func completionPrefix(id language.ID) string {
	switch id {
	case language.TOML, language.JSON:
		return "tr"
	case language.YAML:
		return "nu"
	case language.Shell:
		return "pri"
	case language.XML:
		return "xml:l"
	default:
		return "di"
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
