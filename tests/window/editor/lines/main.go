// Logical line editing verification. Requires a desktop/window manager, native
// typography and fonts. Run: go run ./tests/window/editor/lines. Linux without
// an IME service: XMODIFIERS=@im=none. Runs automatically and exits nonzero on
// failure; early close is a failure. Initially two editors share a plain-text
// document, with separate selections. Checks Alt/Option+Up/Down to move lines,
// Alt/Option+Shift+Up/Down to duplicate (Linux adds Control), Primary+Shift+K to
// delete lines, single Undo/Redo, shared-view anchors, exact modifier matching,
// readonly/edge key consumption, menu state and pointer/keyboard activation,
// completion cancellation and host-owned destruction. All simulated input uses
// Window/Popover.DispatchEvent. IME, physical OS-reserved shortcuts and real
// mixed-monitor DPI require separate checks. No clipboard/files/settings change.
// Record OS, actual painter and scale. Restart to reset; closes on completion.
package main

import (
	"errors"
	"fmt"
	"image"
	"log"
	"runtime"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	source "github.com/golang-gui/goui-sourceview/sourceview"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
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
func item(info gui.WidgetInfo, label string) *gui.WidgetInfo {
	if info.Role == gui.RoleMenuItem && info.Text == label {
		return &info
	}
	for _, child := range info.Children {
		if found := item(child, label); found != nil {
			return found
		}
	}
	return nil
}
func selectedLabel(info gui.WidgetInfo) string {
	if info.Role == gui.RoleMenuItem && info.Selected {
		return info.Text
	}
	for _, child := range info.Children {
		if label := selectedLabel(child); label != "" {
			return label
		}
	}
	return ""
}
func main() {
	runtime.LockOSThread()
	app, err := gui.NewApplication("org.golang-gui.SourceViewLineCommands")
	must(err)
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 700, Height: 430}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("GOUI SourceView line commands"))
	doc := source.NewDocument("one\n中two\nlast")
	a, b := source.NewSourceView(), source.NewSourceView()
	bindings := a.KeyBindings()
	bindings[source.CommandComplete] = []gui.KeyGesture{{Key: gui.KeyF2}}
	must(a.SetKeyBindings(bindings))
	a.SetDocument(doc)
	b.SetDocument(doc)
	columns := gui.NewLinearBox(layout.DirectionHorizontal)
	columns.SetCrossAlign(layout.CrossStretch)
	columns.SetMainWeight(1)
	for _, v := range []*source.SourceView{a, b} {
		scroll := gui.NewScrollView()
		scroll.SetStyleName("source-view")
		scroll.SetMainWeight(1)
		scroll.SetChild(v)
		columns.AddChild(scroll)
	}
	probe := &scaleProbe{}
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(gui.NewLabel("Move / duplicate / delete lines · two views, one document"))
	box.AddChild(columns)
	box.AddChild(probe)
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(a)
	primary, _ := gui.ModPrimary.Resolve()
	alt, _ := gui.ModAlt.Resolve()
	copyMods := gui.ModAlt | gui.ModShift
	if runtime.GOOS == "linux" {
		copyMods |= gui.ModControl
	}
	duplicate, _ := copyMods.Resolve()
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	fallback := 0
	for _, gesture := range []gui.KeyGesture{{Key: gui.KeyArrowUp, Modifiers: gui.ModAlt}, {Key: gui.KeyK, Modifiers: gui.ModPrimary | gui.ModShift}} {
		shortcut := gui.NewShortcut(gesture)
		shortcut.ConnectActivate(func() { fallback++ })
		win.Shortcuts().AddShortcut(shortcut)
	}
	reported := 0
	a.ConnectError(func(error) { reported++ })
	var popup gui.Popover
	complete := false
	steps := []func(){
		func() {
			check(win.Focused() && a.Focused() && probe.scale > 0, "native focus/painter not ready")
			a.SetSelection(source.TextSelection{Anchor: 10, Caret: 7})
			b.SetSelection(source.TextSelection{Anchor: 8, Caret: 8})
			key(events.KeyArrowUp, alt)
			check(doc.Text() == "中two\none\nlast" && a.Selection() == (source.TextSelection{Anchor: 6, Caret: 3}) && b.Selection() == (source.TextSelection{Anchor: 4, Caret: 4}), "move up or shared-view anchors failed")
			key(events.KeyZ, primary)
			check(doc.Text() == "one\n中two\nlast" && a.Selection() == (source.TextSelection{Anchor: 10, Caret: 7}), "move undo failed")
			key(events.KeyZ, primary|events.ModifierShift)
			check(doc.Text() == "中two\none\nlast" && a.Selection() == (source.TextSelection{Anchor: 6, Caret: 3}), "move redo failed")
			key(events.KeyArrowDown, duplicate)
			check(doc.Text() == "中two\n中two\none\nlast" && a.Selection() == (source.TextSelection{Anchor: 13, Caret: 10}), "duplicate below failed")
			key(events.KeyZ, primary)
			key(events.KeyArrowUp, duplicate)
			check(doc.Text() == "中two\n中two\none\nlast" && a.Selection() == (source.TextSelection{Anchor: 6, Caret: 3}), "duplicate above failed")
			key(events.KeyZ, primary)
			key(events.KeyK, primary|events.ModifierShift)
			check(doc.Text() == "one\nlast" && a.Selection() == (source.TextSelection{}), "delete lines failed")
			key(events.KeyZ, primary)
			check(doc.Text() == "中two\none\nlast" && a.Selection() == (source.TextSelection{Anchor: 6, Caret: 3}), "delete undo failed")
			key(events.KeyArrowDown, alt)
			check(doc.Text() == "one\n中two\nlast" && a.Selection() == (source.TextSelection{Anchor: 10, Caret: 7}), "move down failed")
		},
		func() {
			doc.SetText("a\nb")
			a.SetSelection(source.TextSelection{Anchor: 1, Caret: 1})
			must(win.CommitText("X"))
			key(events.KeyArrowDown, alt)
			must(win.CommitText("Y"))
			key(events.KeyZ, primary)
			check(doc.Text() == "b\naX", "typing after line command merged into its history")
			key(events.KeyZ, primary)
			check(doc.Text() == "aX\nb", "line command merged into preceding typing")
			key(events.KeyZ, primary)
			check(doc.Text() == "a\nb", "preceding typing history lost")
			doc.SetText("a\nb")
			a.SetSelection(source.TextSelection{Caret: 2})
			key(events.KeyArrowDown, alt)
			check(doc.Text() == "b\na" && a.Selection() == (source.TextSelection{Anchor: 2, Caret: 3}), "half-open selection or EOF failed")
			doc.SetText("\n")
			a.SetSelection(source.TextSelection{})
			key(events.KeyArrowDown, alt)
			check(a.Selection() == (source.TextSelection{Anchor: 1, Caret: 1}) && doc.Text() == "\n", "empty-line movement stuck")
			key(events.KeyZ, primary)
			check(a.Selection() == (source.TextSelection{}), "empty-line undo failed")
			doc.SetText("a\nb")
			a.SetSelection(source.TextSelection{})
			key(events.KeyArrowUp, alt)
			check(!doc.CanUndo() && fallback == 0, "edge key changed history or leaked")
			a.SetReadOnly(true)
			key(events.KeyArrowDown, duplicate)
			key(events.KeyK, primary|events.ModifierShift)
			check(doc.Text() == "a\nb" && fallback == 0 && reported == 0, "readonly command edited, leaked or reported an error")
			key(events.KeyF10, events.ModifierShift)
			check(len(win.Popovers()) == 1, "readonly menu missing")
			popup = win.Popovers()[0]
		},
		func() {
			for _, label := range []string{"Move Lines Up", "Move Lines Down", "Duplicate Lines Up", "Duplicate Lines Down", "Delete Lines"} {
				row := item(popup.Widget().Snapshot(), label)
				check(row != nil && !row.Enabled, "readonly line menu state incorrect")
			}
			key(events.KeyEscape, 0)
			a.SetReadOnly(false)
			key(events.KeyF10, events.ModifierShift)
			check(len(win.Popovers()) == 1, "line menu missing")
			popup = win.Popovers()[0]
		},
		func() {
			up, down := item(popup.Widget().Snapshot(), "Move Lines Up"), item(popup.Widget().Snapshot(), "Move Lines Down")
			check(up != nil && !up.Enabled && down != nil && down.Enabled, "menu ignored document edge")
			key(events.KeyHome, 0)
			for i := 0; i < 20 && selectedLabel(popup.Widget().Snapshot()) != "Duplicate Lines Down"; i++ {
				key(events.KeyArrowDown, 0)
			}
			check(selectedLabel(popup.Widget().Snapshot()) == "Duplicate Lines Down", "menu keyboard target not reached")
			key(events.KeyEnter, 0)
			check(doc.Text() == "a\na\nb" && len(win.Popovers()) == 0, "menu keyboard duplication failed")
			key(events.KeyZ, primary)
			a.SetSelection(source.TextSelection{Anchor: 2, Caret: 2})
			key(events.KeyF10, events.ModifierShift)
			popup = win.Popovers()[0]
		},
		func() {
			target := item(popup.Widget().Snapshot(), "Move Lines Up")
			check(target != nil && target.Enabled, "menu pointer target disabled")
			point := geometry.Point{X: target.Bounds.X + target.Bounds.Width/2, Y: target.Bounds.Y + target.Bounds.Height/2}
			for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
				must(popup.DispatchEvent(events.PointerEvent{EventType: kind, Button: events.PointerButtonLeft, Position: point}))
			}
			check(doc.Text() == "b\na" && a.Selection() == (source.TextSelection{}) && len(win.Popovers()) == 0, "menu pointer move failed")
			key(events.KeyZ, primary)
			must(doc.SetLanguage(language.JSON))
			doc.SetText("tr\nx")
			a.SetSelection(source.TextSelection{Anchor: 2, Caret: 2})
			key(events.KeyF2, 0)
			check(len(win.Popovers()) == 1, "completion fixture missing")
			key(events.KeyArrowDown, alt)
			check(doc.Text() == "x\ntr" && len(win.Popovers()) == 0, "line move did not cancel completion")
		},
		func() {
			check(reported == 0 && fallback == 0, "unexpected errors or shortcut fallback")
			a.SetReadOnly(true)
			win.Destroy()
			check(a.Destroyed() && b.Destroyed(), "host did not destroy editors")
			saved := doc.Text()
			for _, run := range []func() error{
				b.DeleteLines,
				func() error { return a.ReplaceSelection("unexpected") },
				func() error { return a.MoveLines(false) },
				func() error { return a.DuplicateLines(true) },
			} {
				check(errors.Is(run(), source.ErrDisabled) && doc.Text() == saved, "destroyed editor accepted an edit")
			}
			log.Printf("PASS line keyboard/menu, selection/history, shared views, readonly/edges, completion/lifecycle; typography=%s painter=%s scale=%g", app.Typography().Name(), probe.painter, probe.scale)
			complete = true
		},
	}
	step := 0
	timer := app.NewTimer()
	timer.ConnectTimeout(func() {
		if step < len(steps) {
			f := steps[step]
			step++
			f()
		}
	})
	must(timer.Start(350 * time.Millisecond))
	app.Run()
	check(complete, "line verification aborted")
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
