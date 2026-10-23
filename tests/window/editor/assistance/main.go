// Native editing assistance verification. Requires a desktop and window manager.
// Run: XMODIFIERS=@im=none go run ./tests/window/editor/assistance [-dark].
// Starts with mixed indentation and Unicode. Through Window.DispatchEvent and
// CommitText: Home/End toggle, Shift extends, Backspace deletes one indent and
// Undo restores, Control+G opens a floating input, invalid positions stay open,
// Enter jumps using one-based rune columns, Escape restores editor focus, and
// read-only navigation remains possible. Whitespace/guides/rulers are visible.
// Assertions verify source bytes, selections, focus and overlay state. Visual
// quality, physical IME and actual DPI transitions need separate observation.
// No clipboard/files/settings changes; restart resets. Closes after PASS;
// closing early fails. Record backend, scale and theme with the result.
package main

import (
	"flag"
	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/theme/modern"
	"log"
	"runtime"
	"time"
)

func must(e error) {
	if e != nil {
		log.Fatal(e)
	}
}
func check(ok bool, message string) {
	if !ok {
		log.Fatal(message)
	}
}
func main() {
	runtime.LockOSThread()
	dark := flag.Bool("dark", false, "Dark Modern")
	flag.Parse()
	app, e := gui.NewApplication("org.golanggui.SourceViewAssistance")
	must(e)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	win, e := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 750, Height: 460}, Chrome: gui.WindowChromeIntegrated})
	must(e)
	defer win.Destroy()
	v := source.NewSourceView()
	doc := source.NewDocument("    word  \n中e\u0301x\n\t  value")
	v.SetDocument(doc)
	v.SetAutomaticCompletion(false)
	must(v.SetWhitespaceMode(source.WhitespaceAll))
	v.SetShowLineEndings(true)
	must(v.SetRulers([]int{40, 80}))
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(v)
	overlay := gui.NewOverlay()
	overlay.SetChild(scroll)
	win.SetWidget(overlay)
	must(win.Show())
	win.SetFocusedWidget(v)
	primary, _ := gui.ModPrimary.Resolve()
	control, _ := gui.ModControl.Resolve()
	key := func(k events.Key, m events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: m}))
		}
	}
	var steps []func()
	steps = append(steps, func() {
		v.SetSelection(textedit.Selection{Anchor: 6, Caret: 6})
		key(events.KeyHome, 0)
		check(v.Selection().Caret == 4, "smart Home")
		key(events.KeyHome, 0)
		check(v.Selection().Caret == 0, "Home toggle")
		key(events.KeyEnd, events.ModifierShift)
		check(v.Selection().Anchor == 0 && v.Selection().Caret == 8, "smart Shift End")
	})
	steps = append(steps, func() {
		v.SetSelection(textedit.Selection{Anchor: 4, Caret: 4})
		key(events.KeyBackspace, 0)
		check(doc.Model().Text() == "word  \n中e\u0301x\n\t  value", "indent Backspace")
		key(events.KeyZ, primary)
		check(doc.Model().Text() == "    word  \n中e\u0301x\n\t  value", "indent Undo")
	})
	steps = append(steps, func() {
		key(events.KeyG, control)
		check(panelVisible(v, "sourceview-goto-bar"), "Control G")
		must(win.CommitText("999:1"))
		key(events.KeyEnter, 0)
		check(panelVisible(v, "sourceview-goto-bar") && v.Selection().Caret == 4, "invalid position moved/closed")
	})
	steps = append(steps, func() {
		key(events.KeyA, primary)
		must(win.CommitText("2:2"))
		key(events.KeyEnter, 0)
		check(!panelVisible(v, "sourceview-goto-bar") && v.Focused() && v.Selection().Caret == 14, "Unicode goto")
	})
	steps = append(steps, func() {
		v.SetReadOnly(true)
		key(events.KeyG, control)
		check(panelVisible(v, "sourceview-goto-bar"), "readonly goto")
		key(events.KeyEscape, 0)
		check(!panelVisible(v, "sourceview-goto-bar") && v.Focused(), "Escape focus")
	})
	index := 0
	complete := false
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		if index < len(steps) {
			steps[index]()
			index++
		} else {
			complete = true
			log.Printf("PASS native assistance and GoToBar; dark=%t", *dark)
			win.Destroy()
		}
	})
	must(timer.Start(200 * time.Millisecond))
	app.Run()
	check(complete, "closed early")
}

// 只读取普通 Widget 语义和可见性，不访问私有控件类型。
func panel(v *source.SourceView, role string) gui.Widget {
	for _, child := range v.Children() {
		if child.Snapshot().Role == gui.Role(role) {
			return child
		}
	}
	return nil
}
func panelVisible(v *source.SourceView, role string) bool {
	w := panel(v, role)
	return w != nil && w.Visible()
}
