// Run: go run ./example/debug [-dark]. Requires a desktop, native typography
// and a compositor for Integrated chrome on Linux. This is an in-memory debug
// presentation example, not a debugger connection. Click the mark gutter or
// press F9 to toggle, Shift-click to enable/disable, and right-click for kinds
// and execution placement. Verify simulates adapter confirmation; Step moves
// the execution line without moving the selection. Edit, fold and undo to see
// anchor tracking. Restart to reset; closing the window exits. No files,
// clipboard, processes being debugged or global settings are modified.
package main

import (
	"flag"
	"fmt"
	"log"
	"runtime"
	"strings"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/theme/modern"
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	runtime.LockOSThread()
	dark := flag.Bool("dark", false, "use dark appearance")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewDebug")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1050, Height: 720}, Chrome: gui.WindowChromeIntegrated, Center: true})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("Debug presentation · GOUI SourceView"))
	var text strings.Builder
	text.WriteString("package main\n\nimport \"fmt\"\n\n")
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&text, "func step%d(value int) int {\n    // Try a breakpoint, then edit the lines above it.\n    fmt.Println(value)\n    return value + %d\n}\n\n", i, i)
	}
	doc := source.NewDocument(text.String())
	must(doc.SetLanguage(language.Go))
	editor := source.NewSourceView()
	editor.SetDocument(doc)
	editor.SetShowLineMarks(true)
	bindings := editor.KeyBindings()
	bindings[source.CommandToggleLineMark] = []gui.KeyGesture{{Key: gui.KeyF9}}
	must(editor.SetKeyBindings(bindings))
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(editor)
	scroll.SetMainWeight(1)
	status := gui.NewLabel("")
	update := func() {
		valid := 0
		for _, m := range doc.LineMarks() {
			if m.Valid {
				valid++
			}
		}
		status.SetText(fmt.Sprintf("%d active marks · %d invalid anchors · F9: toggle · Shift-click: enable/disable", valid, len(doc.LineMarks())-valid))
	}
	h := doc.ConnectLineMarksChanged(update)
	defer h.Disconnect()
	editor.ConnectError(func(err error) { status.SetText(err.Error()) })
	popup := gui.NewPopoverMenu(editor)
	set := func(line int, kind source.LineMarkKind) {
		_, err := doc.AddLineMark(source.LineMark{Line: line, Kind: kind, State: source.MarkPending})
		must(err)
	}
	toggle := func(e source.LineMarkEvent) {
		if len(e.Marks) == 0 {
			set(e.Line, source.MarkBreakpoint)
			return
		}
		for _, m := range e.Marks {
			if m.Line == e.Line {
				doc.RemoveLineMark(m.ID)
				return
			}
		}
		status.SetText("Marks are inside this fold; unfold to select the original line.")
	}
	editor.ConnectLineMarkAction(func(e source.LineMarkEvent) {
		switch e.Action {
		case source.LineMarkToggle:
			toggle(e)
		case source.LineMarkToggleEnabled:
			for _, m := range e.Marks {
				mark := m.LineMark
				if mark.State == source.MarkDisabled {
					mark.State = source.MarkPending
				} else {
					mark.State = source.MarkDisabled
				}
				must(doc.UpdateLineMark(m.ID, mark))
			}
		case source.LineMarkContextMenu:
			menu := gui.NewMenu()
			safe := func(fn func()) {
				if doc.Model().Revision() == e.Revision && editor.Document() == doc {
					fn()
				}
			}
			menu.Append("Toggle breakpoint", func() { safe(func() { toggle(e) }) })
			for _, item := range []struct {
				name string
				kind source.LineMarkKind
			}{{"Conditional breakpoint", source.MarkConditionalBreakpoint}, {"Logpoint", source.MarkLogpoint}, {"Bookmark", source.MarkBookmark}} {
				menu.Append(item.name, func() { safe(func() { set(e.Line, item.kind) }) })
			}
			menu.Append("Execute here", func() { safe(func() { must(editor.SetExecutionLine(e.Line)) }) })
			popup.SetMenu(menu)
			must(popup.ShowAt(e.Position))
		}
	})
	tools := gui.NewLinearBox(layout.DirectionHorizontal)
	tools.SetSpacing(8)
	tools.SetPadding(geometry.UniformInsets(8))
	button := func(label string, fn func()) {
		b := gui.NewButton()
		b.SetChild(gui.NewLabel(label))
		b.ConnectClicked(func() { win.SetFocusedWidget(editor); fn() })
		tools.AddChild(b)
	}
	button("Verify pending", func() {
		for _, m := range doc.LineMarks() {
			if m.Valid && m.State == source.MarkPending {
				mark := m.LineMark
				mark.State = source.MarkEnabled
				must(doc.UpdateLineMark(m.ID, mark))
			}
		}
	})
	button("Step", func() {
		line, ok := editor.ExecutionLine()
		if !ok {
			line = 3
		}
		line = (line + 1) % doc.Model().LineCount()
		must(editor.SetExecutionLine(line))
		must(editor.RevealLine(line))
	})
	button("Stop", editor.ClearExecutionLine)
	button("Clear invalid", func() {
		for _, m := range doc.LineMarks() {
			if !m.Valid {
				doc.RemoveLineMark(m.ID)
			}
		}
	})
	button("Light / Dark", func() { *dark = !*dark; app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark})) })
	_, err = doc.AddLineMark(source.LineMark{Line: 6, Kind: source.MarkBreakpoint})
	must(err)
	_, err = doc.AddLineMark(source.LineMark{Line: 12, Kind: source.MarkConditionalBreakpoint, State: source.MarkDisabled})
	must(err)
	_, err = doc.AddLineMark(source.LineMark{Line: 18, Kind: source.MarkLogpoint, State: source.MarkPending})
	must(err)
	must(editor.SetExecutionLine(6))
	header := gui.NewHeaderBar()
	header.SetChild(gui.NewLabel("Debug presentation — application-owned breakpoints"))
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	for _, w := range []gui.Widget{header, tools, scroll, status} {
		box.AddChild(w)
	}
	win.SetWidget(box)
	update()
	must(win.Show())
	win.SetFocusedWidget(editor)
	app.Run()
}
