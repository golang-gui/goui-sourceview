// Native editing assistance verification. Requires a desktop and window manager.
// Run: XMODIFIERS=@im=none go run ./tests/window/editor/assistance [-basic | -dark].
// Starts with mixed indentation and Unicode. Through Window.DispatchEvent and
// CommitText: Home/End toggle, Shift extends, Backspace deletes one indent and
// Undo restores, Control+G opens a floating input, invalid positions stay open,
// Enter jumps using one-based rune columns, Escape restores editor focus, and
// read-only navigation remains possible. Whitespace/guides/rulers are visible.
// Assertions verify source bytes, selections, focus and overlay state. Visual
// quality, physical IME and actual DPI transitions need separate observation.
// Also checks real native whitespace cells (space/Tab/NBSP/NNBSP/ideographic
// space), modes and selected marks through Window.DispatchEvent + RenderWidget.
// Each measured cell must gain pixels compared with None. Pixels are compared
// in the same opaque sRGB image, so there is no cross-platform font golden or
// widened AA tolerance. Scale is read from the actual capture; repeat with
// GOUI_PLAT_SCALE=1/1.5/2 and -basic/-dark. Native metrics stay authoritative.
// No clipboard/files/settings changes; restart resets. Closes after PASS;
// closing early fails. Record backend, scale and theme with the result.
package main

import (
	"bytes"
	"flag"
	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/theme/modern"
	"image"
	"log"
	"math"
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
	basic := flag.Bool("basic", false, "Basic theme")
	dark := flag.Bool("dark", false, "Dark Modern")
	flag.Parse()
	app, e := gui.NewApplication("org.golanggui.SourceViewAssistance")
	must(e)
	if !*basic {
		app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	}
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
	steps = append(steps, whitespaceChecks(app, win, v)...)
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

// 通过实际窗口及正常键盘路径验证标记；不公开私有几何或添加测试控制协议。
func whitespaceChecks(app gui.Application, win gui.Window, v *source.SourceView) []func() {
	const text = " \t\u00a0\u202f\u3000"
	var plain, marked image.Image
	var scale float32
	var cells []geometry.Rectangle
	capture := func() image.Image { img, err := gui.RenderWidget(v, 0); must(err); return img }
	changed := func(a, b image.Image, r geometry.Rectangle) int {
		x0, y0 := int(math.Floor(float64(r.X*scale))), int(math.Floor(float64(r.Y*scale)))
		x1, y1 := int(math.Ceil(float64((r.X+r.Width)*scale))), int(math.Ceil(float64((r.Y+r.Height)*scale)))
		count := 0
		for y := max(0, y0); y < min(a.Bounds().Dy(), y1); y++ {
			for x := max(0, x0); x < min(a.Bounds().Dx(), x1); x++ {
				ar, ag, ab, aa := a.At(x, y).RGBA()
				br, bg, bb, ba := b.At(x, y).RGBA()
				if ar != br || ag != bg || ab != bb || aa != ba {
					count++
				}
			}
		}
		return count
	}
	return []func(){func() {
		v.SetReadOnly(false)
		v.SetDocument(source.NewDocument(text + "\r\n"))
		v.SetShowLineNumbers(false)
		v.SetFoldingEnabled(false)
		v.SetShowIndentGuides(false)
		v.SetWrapMode(source.WrapNone)
		v.SetShowLineEndings(false)
		must(v.SetWhitespaceMode(source.WhitespaceAll))
		win.SetFocusedWidget(v)
	}, func() {
		// 参考只调用公开排版 API，字体及 Tab 格式与示例显式默认一致。
		format := typography.TextFormat{Font: typography.FontInfo{Family: "monospace", Size: 12}}
		space, err := app.Typography().NewTextLayout(" ", format, 1<<20, 1<<20)
		must(err)
		sw, _ := space.MeasureSize()
		space.Destroy()
		format.TabWidth = 4 * sw
		native, err := app.Typography().NewTextLayout(text, format, 1<<20, 1<<24)
		must(err)
		lines, clusters := native.MeasureMetrics()
		for _, c := range clusters {
			line := lines[c.LineIndex]
			cells = append(cells, geometry.Rect(6+c.X, 6+line.Y, c.Width, line.Height))
		}
		native.Destroy()
		check(len(cells) == 5, "native whitespace sample did not provide five measured cells")
		must(v.SetWhitespaceMode(source.WhitespaceNone))
		plain = capture()
		scale = float32(plain.Bounds().Dx()) / v.Rect().Width
		must(v.SetWhitespaceMode(source.WhitespaceAll))
		marked = capture()
		for i, r := range cells {
			check(changed(plain, marked, r) > 0, "missing native whitespace marker in cell "+string(rune('0'+i)))
		}
		must(v.SetWhitespaceMode(source.WhitespaceBoundary))
	}, func() {
		boundary := capture()
		check(changed(marked, boundary, geometry.Rect(8, 6, v.Rect().Width-8, cells[0].Height)) == 0, "all-blank Boundary differs from All")
		primary, _ := gui.ModPrimary.Resolve()
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: events.KeyA, Modifiers: primary}))
		}
	}, func() {
		must(v.SetWhitespaceMode(source.WhitespaceNone))
		base := capture()
		must(v.SetWhitespaceMode(source.WhitespaceSelection))
		selected := capture()
		for _, r := range cells {
			check(changed(base, selected, r) > 0, "missing selected whitespace marker")
		}
		must(v.SetWhitespaceMode(source.WhitespaceNone))
		v.SetShowLineEndings(false)
		base = capture()
		v.SetShowLineEndings(true)
		end := capture()
		check(changed(base, end, geometry.Rect(0, 0, v.Rect().Width, v.Rect().Height)) > 0, "missing CRLF marker")
		check(changed(base, end, geometry.Rect(0, cells[0].Y+cells[0].Height, v.Rect().Width, v.Rect().Height)) == 0, "phantom EOF marker")
		var out bytes.Buffer
		_, err := v.Document().WriteTo(&out)
		must(err)
		check(out.String() == text+"\r\n" && !v.Document().Model().CanUndo(), "display changed output or undo")
		log.Printf("PASS native whitespace cells, modes, selection and CRLF/EOF; scale=%g backend=%s", scale, app.Typography().Name())
	}}
}
