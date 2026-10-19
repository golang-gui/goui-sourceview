// Command binding integration verification. Requires a desktop/window manager,
// native typography/fonts. Run: go run ./tests/window/editor/keybindings [-dark]
// (Linux without IME service: XMODIFIERS=@im=none). Starts two editors sharing
// one document, a FindBar and a GoToBar. Uses normal Window.DispatchEvent and
// CommitText to check default Ctrl+Space propagation, explicit completion and
// popup keys, remapping/removal, readonly commands, independent views, companion
// open/navigation bindings, clearing/resetting, and reentrant replacement.
// No clipboard/files/settings changes. Logs actual painter/scale; closes on
// PASS, exits nonzero on assertion/early close. Restart resets. Physical system
// interception, native IME and mixed-monitor DPI require separate verification.
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"log"
	"reflect"
	"runtime"
	"time"

	source "github.com/golang-gui/goui-sourceview"
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

func main() {
	runtime.LockOSThread()
	dark := flag.Bool("dark", false, "Modern dark appearance")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewKeyBindings")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 900, Height: 650}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	v, other := source.NewSourceView(), source.NewSourceView()
	doc := source.NewDocument("tr\nx")
	for _, editor := range []*source.SourceView{v, other} {
		editor.SetDocument(doc)
		editor.SetAutomaticCompletion(false)
		editor.ConnectError(func(err error) { log.Fatal(err) })
	}
	v.SetCompletionProvider(&source.CompletionProvider{Complete: func(context.Context, source.CompletionContext) ([]source.Completion, error) {
		return []source.Completion{{Label: "true"}}, nil
	}})
	scroll := gui.NewScrollView()
	scroll.SetChild(v)
	overlay := gui.NewOverlay()
	overlay.SetChild(scroll)
	overlay.SetMainWeight(1)
	find, goTo := source.NewFindBar(), source.NewGoToBar()
	find.SetTarget(v)
	goTo.SetTarget(v)
	for _, bar := range []gui.Widget{find, goTo} {
		overlay.AddOverlay(bar)
		overlay.ConnectOverlayPosition(bar, func(available, size geometry.Size, p *geometry.Point) { p.X = max(0, available.Width-size.Width-8) })
	}
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(overlay)
	mirror := gui.NewScrollView()
	mirror.SetChild(other)
	mirror.SetMainWeight(1)
	box.AddChild(mirror)
	probe := &scaleProbe{}
	box.AddChild(probe)
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(v)
	primary, _ := gui.ModPrimary.Resolve()
	alt, _ := gui.ModAlt.Resolve()
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	space, oldMove, move, oldFind, customFind, oldGoTo, completion, oldNext := 0, 0, 0, 0, 0, 0, 0, 0
	for _, fallback := range []struct {
		gesture gui.KeyGesture
		count   *int
	}{
		{gui.KeyGesture{Key: gui.KeySpace, Modifiers: gui.ModControl}, &space},
		{gui.KeyGesture{Key: gui.KeyArrowDown, Modifiers: gui.ModAlt}, &oldMove},
		{gui.KeyGesture{Key: gui.KeyF6}, &move},
		{gui.KeyGesture{Key: gui.KeyF, Modifiers: gui.ModPrimary}, &oldFind},
		{gui.KeyGesture{Key: gui.KeyF4}, &customFind},
		{gui.KeyGesture{Key: gui.KeyG, Modifiers: gui.ModControl}, &oldGoTo},
		{gui.KeyGesture{Key: gui.KeyF2}, &completion},
		{gui.KeyGesture{Key: gui.KeyF3}, &oldNext},
	} {
		s := gui.NewShortcut(fallback.gesture)
		s.ConnectActivate(func() { *fallback.count++ })
		win.Shortcuts().AddShortcut(s)
	}
	steps := []func(){
		func() {
			check(v.Focused() && probe.scale > 0, "window not ready")
			v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
			key(events.KeySpace, events.ModifierControl)
			check(space == 1 && len(win.Popovers()) == 0, "Ctrl+Space was still a default completion binding")
			bindings := v.KeyBindings()
			for command, gesture := range map[source.Command]gui.KeyGesture{
				source.CommandComplete: {Key: gui.KeyF2}, source.CommandMoveLinesDown: {Key: gui.KeyF6},
				source.CommandUndo: {Key: gui.KeyF7}, source.CommandFind: {Key: gui.KeyF4},
				source.CommandReplace: {Key: gui.KeyF4, Modifiers: gui.ModShift}, source.CommandGoTo: {Key: gui.KeyF5},
				source.CommandFindNext: {Key: gui.KeyF8}, source.CommandFindPrevious: {Key: gui.KeyF8, Modifiers: gui.ModShift},
			} {
				bindings[command] = []gui.KeyGesture{gesture}
			}
			must(v.SetKeyBindings(bindings))
			bindings[source.CommandComplete][0].Key = gui.KeyF12
			win.SetFocusedWidget(other)
			key(events.KeyF2, 0)
			check(completion == 1 && len(win.Popovers()) == 0, "binding leaked to a shared-document view")
			win.SetFocusedWidget(v)
			key(events.KeyF2, 0)
		},
		func() {
			check(len(win.Popovers()) == 1 && v.Focused(), "explicit completion did not open")
			key(events.KeyArrowDown, 0)
			key(events.KeyEnter, 0)
			check(doc.Model().Text() == "true\nx", "completion popup input changed")
			key(events.KeyF7, 0)
			check(doc.Model().Text() == "tr\nx", "remapped undo failed")
			key(events.KeyArrowDown, alt)
			check(oldMove == 1 && doc.Model().Text() == "tr\nx", "old move shortcut survived replacement")
			key(events.KeyF6, 0)
			check(doc.Model().Text() == "x\ntr", "remapped line move failed")
			v.SetReadOnly(true)
			key(events.KeyF6, 0)
			check(move == 0 && doc.Model().Text() == "x\ntr", "readonly remapped command leaked or edited")
			v.SetReadOnly(false)
			key(events.KeyF7, 0)
			check(doc.Model().Text() == "tr\nx", "remapped history failed")
			bindings := v.KeyBindings()
			delete(bindings, source.CommandMoveLinesDown)
			must(v.SetKeyBindings(bindings))
			key(events.KeyF6, 0)
			check(move == 1 && doc.Model().Text() == "tr\nx", "removed key did not propagate")
		},
		func() {
			doc.Model().SetText("foo foo\nlast")
			v.SetSelection(textedit.Selection{})
			v.SetSearch(source.SearchQuery{Text: "foo"})
			key(events.KeyF, primary)
			check(oldFind == 1 && !find.Opened(), "old Find binding remained active")
			key(events.KeyF4, 0)
			check(find.Opened() && find.ContainsFocus(), "remapped Find did not open")
			key(events.KeyF8, 0)
			check(v.Selection().Range() == (textedit.Range{Start: 0, End: 3}), "bar did not use remapped FindNext")
			key(events.KeyF8, events.ModifierShift)
			check(v.Selection().Range() == (textedit.Range{Start: 4, End: 7}), "bar did not use remapped FindPrevious")
			key(events.KeyEscape, 0)
			key(events.KeyF3, 0)
			check(oldNext == 1 && v.Selection().Range() == (textedit.Range{Start: 4, End: 7}), "old FindNext survived replacement")
			key(events.KeyG, events.ModifierControl)
			check(oldGoTo == 1 && !goTo.Opened(), "old GoTo binding remained active")
			key(events.KeyF5, 0)
			check(goTo.Opened(), "remapped GoTo did not open")
			goTo.SetQuery("2")
			key(events.KeyEnter, 0)
			check(!goTo.Opened() && v.Selection().Caret == 8, "GoTo field submit changed")
			key(events.KeyF4, events.ModifierShift)
			check(find.Opened() && find.ReplaceMode(), "remapped Replace did not open")
			key(events.KeyEscape, 0)
		},
		func() {
			before := v.KeyBindings()
			conflict := v.KeyBindings()
			conflict[source.CommandComplete] = conflict[source.CommandFind]
			check(v.SetKeyBindings(conflict) != nil && reflect.DeepEqual(v.KeyBindings(), before), "conflict changed live bindings")
			must(v.SetKeyBindings(nil))
			key(events.KeyF4, 0)
			key(events.KeyF, primary)
			key(events.KeyG, events.ModifierControl)
			key(events.KeyF2, 0)
			check(customFind == 1 && oldFind == 2 && oldGoTo == 2 && completion == 2 && !find.Opened() && !goTo.Opened(), "cleared table still consumed companion/core keys")
			doc.Model().SetText("")
			must(win.CommitText("a"))
			key(events.KeyEnter, 0)
			key(events.KeyBackspace, 0)
			check(doc.Model().Text() == "a", "clearing command bindings disabled ordinary input")
			must(v.InsertLine(true))
			check(doc.Model().Text() == "a\n" && v.Undo() && doc.Model().Text() == "a", "direct methods depended on key bindings")
		},
		func() {
			must(v.SetKeyBindings(source.DefaultKeyBindings()))
			key(events.KeyF, primary)
			check(find.Opened(), "reset did not restore Find binding")
			key(events.KeyEscape, 0)
			key(events.KeyG, events.ModifierControl)
			check(goTo.Opened(), "reset did not restore GoTo binding")
			key(events.KeyEscape, 0)
			key(events.KeySpace, events.ModifierControl)
			check(space == 2, "reset restored an implicit completion binding")
			doc.Model().SetText("a\nb")
			v.SetSelection(textedit.Selection{})
			must(v.SetKeyBindings(source.KeyBindings{source.CommandMoveLinesDown: {{Key: gui.KeyF6}}}))
			h := v.ConnectChange(func(textedit.Change) { must(v.SetKeyBindings(nil)) })
			key(events.KeyF6, 0)
			h.Disconnect()
			check(doc.Model().Text() == "b\na" && len(v.KeyBindings()) == 0 && move == 1, "reentrant replacement reused or leaked the same event")
			key(events.KeyF6, 0)
			check(move == 2, "replacement did not apply to the next event")
		},
		func() {
			doc.Model().SetText("tr\nx")
			v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
			must(v.SetKeyBindings(source.KeyBindings{
				source.CommandComplete: {{Key: gui.KeyF2}}, source.CommandMoveLinesDown: {{Key: gui.KeyF6}},
			}))
			key(events.KeyF2, 0)
		},
		func() {
			check(len(win.Popovers()) == 1, "completion close reentry fixture missing")
			h := win.Popovers()[0].ConnectClosed(func() {
				must(v.SetKeyBindings(source.KeyBindings{source.CommandInsertLineBelow: {{Key: gui.KeyF6}}}))
			})
			key(events.KeyF6, 0)
			h.Disconnect()
			check(doc.Model().Text() == "tr\nx" && move == 2, "popup close reentry executed the same event on a new binding table")
			key(events.KeyF6, 0)
			check(doc.Model().Text() == "tr\n\nx", "replacement binding was not active on the following event")
		},
	}
	index, done := 0, false
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		if index < len(steps) {
			steps[index]()
			index++
			return
		}
		done = true
		log.Printf("PASS command bindings: replace/remove/reset, completion, companion bars, reentry; typography=%s painter=%s scale=%g dark=%t", app.Typography().Name(), probe.painter, probe.scale, *dark)
		win.Destroy()
	})
	must(timer.Start(250 * time.Millisecond))
	app.Run()
	check(done, "verification aborted")
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
