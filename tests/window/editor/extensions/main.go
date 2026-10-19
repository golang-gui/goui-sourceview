// Public extension verification. Run: go run ./tests/window/editor/extensions
// [-dark]. Requires a desktop, window manager and native typography; Linux:
// XMODIFIERS=@im=none. It starts with an application-owned Markdown definition.
// Automatic assertions use normal Window.DispatchEvent/CommitText to check word
// navigation, multi-character pair insertion/newline/history, empty-prefix
// triggers, provider ordering, per-item ranges/carets, additional edits, undo,
// cancellation after same-ID definition changes and provider errors. Delayed
// providers use copied request data and context cancellation. Setup may directly
// replace the document/selection; tested edits use normal input. PASS means all
// assertions completed, not visual or physical-IME acceptance. No files,
// clipboard or settings change. Restart to reset; closes after completion, and
// closing early fails. Record OS, painter, typography and scale separately.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"log"
	"runtime"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/example/markdown/markdown"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
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
func main() {
	runtime.LockOSThread()
	dark := flag.Bool("dark", false, "Modern dark appearance")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewExtensions")
	must(err)
	app.SetStyleSheet(style.Sheet(append(modern.Rules(modern.Options{Dark: *dark}), markdown.Rules(*dark)...)...))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 780, Height: 460}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView public extensions"))
	definition, err := markdown.NewLanguage()
	must(err)
	doc := source.NewDocument("")
	doc.SetLanguageDefinition(definition)
	v := source.NewSourceView()
	bindings := v.KeyBindings()
	bindings[source.CommandComplete] = []gui.KeyGesture{{Key: gui.KeyF2}}
	must(v.SetKeyBindings(bindings))
	v.SetDocument(doc)
	v.SetAcceptsTab(true)
	scroll := gui.NewScrollView()
	scroll.SetChild(v)
	scroll.SetMainWeight(1)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(gui.NewLabel("Application-owned Markdown · public extension verification"))
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
	reset := func(text string) {
		v.SetAutomaticCompletion(false)
		v.SetCompletionProvider(markdown.Provider())
		doc.Model().SetText(text)
		at := len(text)
		v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
		win.SetFocusedWidget(v)
	}
	var steps []func() bool
	next := func(f func()) { steps = append(steps, func() bool { f(); return true }) }
	waitPopup := func(label string, accept events.Key) {
		steps = append(steps, func() bool {
			if len(win.Popovers()) == 0 {
				return false
			}
			rows := win.Popovers()[0].Widget().Snapshot().Children
			if len(rows) == 0 {
				return false
			}
			check(rows[0].Text == label, "wrong provider label/order: "+rows[0].Text)
			key(accept, 0)
			return true
		})
	}
	next(func() {
		reset("- item")
		key(events.KeyEnter, 0)
		check(doc.Model().Text() == "- item\n- ", "list continuation")
		key(events.KeyZ, primary)
		check(doc.Model().Text() == "- item", "list undo")
	})
	next(func() {
		reset("hello-world rest")
		v.SetSelection(textedit.Selection{})
		key(events.KeyArrowRight, events.ModifierControl)
		check(v.Selection().Caret == 11, "custom word navigation")
	})
	next(func() {
		reset("")
		must(win.CommitText("`"))
		check(doc.Model().Text() == "``" && v.Selection().Caret == 1, "inline pair")
		must(win.CommitText("`"))
		must(win.CommitText("`"))
		check(doc.Model().Text() == "``````" && v.Selection().Caret == 3, "triple delimiter")
		key(events.KeyEnter, 0)
		check(doc.Model().Text() == "```\n\n```" && v.Selection().Caret == 4, "fence newline")
		key(events.KeyZ, primary)
		check(doc.Model().Text() == "``````" && v.Selection().Caret == 3, "fence undo")
	})
	next(func() {
		reset("")
		v.SetAutomaticCompletion(true)
		must(win.CommitText("["))
		check(doc.Model().Text() == "[]", "link pair")
	})
	waitPopup("Link", events.KeyEnter)
	next(func() {
		check(doc.Model().Text() == "[label](https://example.com)" && v.Selection().Caret == 8, "per-item range/caret")
		key(events.KeyZ, primary)
		check(doc.Model().Text() == "[]", "link completion undo")
	})
	next(func() { reset(""); v.SetAutomaticCompletion(true); must(win.CommitText("@")) })
	waitPopup("GOUI reference", events.KeyTab)
	next(func() {
		check(doc.Model().Text() == "[GOUI][goui]\n\n[goui]: https://github.com/golang-gui/goui\n" && v.Selection().Caret == 12, "additional edits/caret")
		key(events.KeyZ, primary)
		check(doc.Model().Text() == "@" && v.Selection().Caret == 1, "atomic additional-edit undo")
	})
	next(func() {
		reset("abc")
		v.SetCompletionProvider(&source.CompletionProvider{Complete: func(context.Context, source.CompletionContext) ([]source.Completion, error) {
			return []source.Completion{{Label: "zebra"}, {Label: "alpha"}}, nil
		}})
		key(events.KeyF2, 0)
	})
	waitPopup("zebra", events.KeyEnter)
	next(func() { check(doc.Model().Text() == "zebra", "provider candidate was prefix-filtered or reordered") })
	started, cancelled := make(chan struct{}, 1), make(chan struct{}, 1)
	next(func() {
		reset("abc")
		v.SetCompletionProvider(&source.CompletionProvider{Complete: func(ctx context.Context, c source.CompletionContext) ([]source.Completion, error) {
			started <- struct{}{}
			<-ctx.Done()
			cancelled <- struct{}{}
			return []source.Completion{{Label: "stale"}}, nil
		}})
		key(events.KeyF2, 0)
	})
	steps = append(steps, func() bool {
		select {
		case <-started:
			l, err := markdown.NewLanguage()
			must(err)
			doc.SetLanguageDefinition(l)
			return true
		default:
			return false
		}
	})
	steps = append(steps, func() bool {
		select {
		case <-cancelled:
			check(len(win.Popovers()) == 0 && doc.Model().Text() == "abc", "same-ID definition retained request")
			return true
		default:
			return false
		}
	})
	var reported int
	failure := errors.New("provider verification error")
	v.ConnectError(func(err error) { check(errors.Is(err, failure), "unexpected error: "+err.Error()); reported++ })
	next(func() {
		v.SetCompletionProvider(&source.CompletionProvider{Complete: func(context.Context, source.CompletionContext) ([]source.Completion, error) { return nil, failure }})
		key(events.KeyF2, 0)
	})
	steps = append(steps, func() bool {
		if reported == 0 {
			return false
		}
		check(reported == 1 && len(win.Popovers()) == 0, "provider error repeated")
		return true
	})
	next(func() {
		reset(markdown.Sample)
		check(v.Snapshot().TextEditing != nil && v.Snapshot().TextEditing.Length == len(markdown.Sample) && v.Snapshot().TextEditing.VisibleText != "", "semantic editor text missing")
		check(probe.scale > 0, "painter not observed")
		log.Printf("PASS public Markdown extensions, completion ranges/additional edits/order/cancellation/history; typography=%s painter=%s scale=%g dark=%t", app.Typography().Name(), probe.painter, probe.scale, *dark)
	})
	step := 0
	deadline := time.Now().Add(15 * time.Second)
	complete := false
	timer := app.NewTimer()
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), "extension verification timeout")
		if step < len(steps) {
			if steps[step]() {
				step++
				deadline = time.Now().Add(8 * time.Second)
			}
		} else {
			complete = true
			win.Destroy()
		}
	})
	must(timer.Start(150 * time.Millisecond))
	app.Run()
	check(complete, "verification aborted")
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
