// Optional FindBar and declarative lifecycle verification. Requires a desktop,
// native typography and fonts. Run: go run ./tests/window/editor/find (Linux
// without IME service: XMODIFIERS=@im=none). Initially two independent editors
// and a closed optional bar are declared. Automatically verifies Ctrl/Cmd+F,
// replacement opening (macOS Cmd+Option+F; others Ctrl+H), query seeding,
// Enter/Shift+Enter, checkbox keyboard/pointer input, replacement and undo,
// read-only controls, close/focus, declarative rebuild/retarget/removal and
// chunked count invalidation. All interaction uses Window.DispatchEvent and
// CommitText. Public model/setter calls prepare scenarios, not simulated input.
// No clipboard/files/settings change. Real IME/system shortcuts and mixed DPI
// need separate manual checks. Restart to reset. Exits nonzero on assertion or
// early close; closes automatically on success. Records native typography.
package main

import (
	"fmt"
	"image"
	"log"
	"runtime"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	sourceui "github.com/golang-gui/goui-sourceview/ui"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
	base "github.com/golang-gui/goui/ui"
	"github.com/golang-gui/goui/widgets"
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
func check(ok bool, msg string) {
	if !ok {
		log.Fatal(msg)
	}
}
func named(w gui.Widget, name string) gui.Widget {
	if w.Name() == name {
		return w
	}
	for _, c := range w.Children() {
		if found := named(c, name); found != nil {
			return found
		}
	}
	return nil
}
func labelled(w gui.Widget, role gui.Role, text string) gui.Widget {
	if w.Snapshot().Role == role {
		for _, c := range w.Children() {
			if c.Snapshot().Text == text {
				return w
			}
		}
	}
	for _, c := range w.Children() {
		if found := labelled(c, role, text); found != nil {
			return found
		}
	}
	return nil
}
func main() {
	runtime.LockOSThread()
	doc := source.NewDocument("one ONE stone one")
	other := source.NewDocument("beta beta")
	show, target := true, "a"
	started, complete := false, false
	var originalBar *source.FindBar
	var a, b *source.SourceView
	baseControllers, otherControllers, fallback, opened, errors := 0, 0, 0, 0, 0
	partialReset := false
	step := 0
	must(base.Run("org.golang-gui.SourceViewFindVerification", func(app base.App) base.RootView {
		if !started {
			started = true
			app.TickFunc(350*time.Millisecond, func() {
				win := app.FindWindow("main")
				check(win != nil, "window missing")
				a = app.FindWidget("main", "a").(*source.SourceView)
				b = app.FindWidget("main", "b").(*source.SourceView)
				key := func(k events.Key, mods events.Modifiers) {
					for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
						must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
					}
				}
				primary, _ := gui.ModPrimary.Resolve()
				openReplace := func() {
					if runtime.GOOS == "darwin" {
						alt, _ := gui.ModAlt.Resolve()
						key(events.KeyF, primary|alt)
					} else {
						key(events.KeyH, primary)
					}
				}
				click := func(w gui.Widget) {
					check(w != nil, "click target missing")
					r := w.Snapshot().Bounds
					p := geometry.Point{X: r.X + r.Width/2, Y: r.Y + r.Height/2}
					for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
						must(win.DispatchEvent(events.PointerEvent{EventType: kind, Position: p, Button: events.PointerButtonLeft}))
					}
				}
				bar, _ := app.FindWidget("main", "find").(*source.FindBar)
				count := func(want int) {
					check(bar != nil, "bar missing")
					state := bar.Snapshot().Attributes["sourceview.find"].(map[string]any)
					check(state["countComplete"] == true && state["count"] == want, "incorrect completed match count")
				}
				switch step {
				case 0:
					check(bar != nil && bar.Target() == a && !bar.Opened(), "declarative target not attached")
					originalBar = bar
					baseControllers = len(a.EventControllers()) - 1
					otherControllers = len(b.EventControllers())
					s := gui.NewShortcut(gui.KeyGesture{Key: gui.KeyF, Modifiers: gui.ModPrimary})
					s.ConnectActivate(func() { fallback++ })
					win.Shortcuts().AddShortcut(s)
					win.SetFocusedWidget(a)
					a.SetSelection(textedit.Selection{Anchor: 3, Caret: 0})
					key(events.KeyF, primary)
					check(bar.Opened() && named(bar, "Find").Focused() && a.Search().Text == "one" && !doc.Model().CanUndo(), "query seed, focus or history")
				case 1:
					count(3)
					key(events.KeyEnter, 0)
					check(a.Selection() == (textedit.Selection{Anchor: 10, Caret: 13}), "find enter")
					key(events.KeyEnter, events.ModifierShift)
					check(a.Selection() == (textedit.Selection{Caret: 3}), "find shift-enter")
					click(labelled(bar, widgets.RoleCheckBox, "Match case"))
					check(a.Search().IgnoreCase, "case toggle pointer")
				case 2:
					count(4)
					w := labelled(bar, widgets.RoleCheckBox, "Whole word")
					check(w != nil, "word option missing")
					win.SetFocusedWidget(w)
					key(events.KeySpace, 0)
					check(a.Search().WholeWord, "word toggle keyboard")
					openReplace()
					check(bar.ReplaceMode(), "replace shortcut")
				case 3:
					count(3)
					field := named(bar, "Replacement")
					check(field != nil && field.Snapshot().Enabled, "replacement field unavailable")
					win.SetFocusedWidget(field)
					must(win.CommitText("X"))
					key(events.KeyEnter, 0)
					check(doc.Model().Text() == "X ONE stone one" && a.Selection() == (textedit.Selection{Anchor: 2, Caret: 5}), "replace and advance")
					win.SetFocusedWidget(a)
					key(events.KeyZ, primary)
					check(doc.Model().Text() == "one ONE stone one", "replace undo")
					win.SetFocusedWidget(named(bar, "Find"))
					key(events.KeyEscape, 0)
					check(!bar.Opened() && a.Focused() && a.Search().Text == "one", "close focus/query")
					key(events.KeyF3, 0)
					check(a.Selection() == (textedit.Selection{Anchor: 4, Caret: 7}), "closed-bar search navigation")
					a.SetReadOnly(true)
					openReplace()
				case 4:
					check(!named(bar, "Replacement").Snapshot().Enabled && !labelled(bar, gui.RoleButton, "Replace all").Snapshot().Enabled, "readonly replacement enabled")
					a.SetReadOnly(false)
					app.RequestUpdate()
				case 5:
					check(bar == originalBar && bar.Opened() && a.Search().Text == "ONE" && len(a.EventControllers()) == baseControllers+1, "rebuild cleared state or duplicated controller")
					target = "b"
					app.RequestUpdate()
				case 6:
					check(bar.Target() == b && len(a.EventControllers()) == baseControllers && len(b.EventControllers()) == otherControllers+1, "retarget leaked binding")
					win.SetFocusedWidget(a)
					key(events.KeyF, primary)
					check(fallback == 1, "old editor still intercepts find")
					win.SetFocusedWidget(b)
					key(events.KeyF, primary)
					check(b.Search().Text == "" && named(bar, "Find").Focused(), "retarget reused old query")
					show = false
					app.RequestUpdate()
				case 7:
					check(bar == nil && len(b.EventControllers()) == otherControllers, "removed bar retained binding")
					win.SetFocusedWidget(b)
					key(events.KeyF, primary)
					check(fallback == 2, "removed panel intercepted find")
					show = true
					target = "a"
					app.RequestUpdate()
				case 8:
					check(bar != nil && bar != originalBar && bar.Target() == a, "bar remount")
					a.SetSelection(textedit.Selection{})
					win.SetFocusedWidget(a)
					key(events.KeyF, primary)
					doc.Model().SetText(strings.Repeat("x", 200000))
					a.SetSearch(source.SearchQuery{Text: "x"})
					var handle interface{ Disconnect() }
					handle = win.ConnectFrame(func(time.Time) {
						state := bar.Snapshot().Attributes["sourceview.find"].(map[string]any)
						if state["countComplete"] == false && state["count"].(int) > 0 {
							partialReset = true
							handle.Disconnect()
							doc.Model().SetText("reset reset")
							a.SetSearch(source.SearchQuery{Text: "reset"})
						}
					})
				case 9:
					check(partialReset, "did not exercise partial count invalidation")
					count(2)
					openReplace()
					bar.SetReplacement("R")
					a.SetSelection(textedit.Selection{})
					key(events.KeyEnter, 0)
				case 10:
					click(labelled(bar, gui.RoleButton, "Replace all"))
					check(doc.Model().Text() == "R R", "replace all")
					win.SetFocusedWidget(a)
					key(events.KeyZ, primary)
					check(doc.Model().Text() == "reset reset", "replace-all atomic undo")
					openReplace()
					replacementDoc := source.NewDocument("keep reset")
					h := a.ConnectChange(func(textedit.Change) {
						a.SetDocument(replacementDoc)
						a.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
					})
					win.SetFocusedWidget(named(bar, "Replacement"))
					key(events.KeyEnter, 0)
					h.Disconnect()
					check(a.Document() == replacementDoc && replacementDoc.Model().Text() == "keep reset" && a.Selection() == (textedit.Selection{Anchor: 2, Caret: 2}), "replacement callback state overwritten")
					check(opened >= 3 && errors == 0, "companion signals")
					probe := app.FindWidget("main", "probe").(*scaleProbe)
					check(probe.scale > 0, "painter probe not rendered")
					log.Printf("PASS optional find/replace, keyboard/pointer, counting, history, readonly, declarative rebuild/retarget/removal; typography=%s painter=%s scale=%g", gui.App.Typography().Name(), probe.painter, probe.scale)
					complete = true
					bar.Close()
					h = bar.ConnectOpen(func(open bool) {
						if open {
							win.Destroy()
						}
					})
					key(events.KeyF, primary)
					h.Disconnect()
					check(bar.Destroyed() && a.Destroyed() && b.Destroyed(), "host did not destroy companion/editor")
					check(bar.Open(false) == source.ErrNotMounted, "destroyed companion opened")
				}
				step++
			})
		}
		var find base.View
		if show {
			find = sourceui.FindBar(target).ID("find").OnOpen(func(bool) { opened++ }).OnError(func(error) { errors++ })
		}
		return base.Window("main").Title("Optional SourceView find / replace").Size(760, 500).Content(
			base.VBox(find,
				base.ScrollView(sourceui.SourceView().ID("a").Document(doc)).MainWeight(1),
				base.ScrollView(sourceui.SourceView().ID("b").Document(other)).MainWeight(1),
				newProbe(),
			).CrossAlign(layout.CrossStretch))
	}))
	check(complete, "find verification aborted")
}

type probeView struct{ base.ViewBase[probeView] }

func newProbe() base.View                                  { v := &probeView{}; v.Self = v; v.ID("probe"); return v }
func (v *probeView) Build() base.View                      { return v }
func (v *probeView) Mount(base.BuildContext) gui.Widget    { return &scaleProbe{} }
func (v *probeView) Update(base.BuildContext, gui.Widget)  {}
func (v *probeView) Unmount(base.BuildContext, gui.Widget) {}

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
