// 行标记窗口验收，需要桌面、窗口管理器、原生排版与字体。
// 启动：go run ./tests/window/editor/line_marks [-theme basic|light|dark] [-minimap]。
// Linux 独立 X11 设置 XMODIFIERS=@im=none，分别 GOUI_PLAT_SCALE=1/1.5 验证。
// 初始：Integrated 窗口、可滚动 JSON 文档、一个断点及同位置执行箭头。
// 自动通过 Window.DispatchEvent 点击、Shift-click、右键、F9 和行号；断言
// 事件目标、选区保持、折叠汇总保留原始行、独立显露、文本编辑和重新绑定。
// RenderWidget 在标记栏内部查找不透明红/黄色实心像素，NRGBA 精确匹配；
// 采样不包含文字/边框，不要求跨平台字体抗锯齿相同，不代替桌面截图检查。
// 所有断言通过后自动关闭；超时/提前关闭/失败返回非零。重启复位。
// 不读写用户文件、剪贴板或设置；真实 IME 和其他平台需另行验收。
package main

import (
	"flag"
	"fmt"
	"image/color"
	"log"
	"runtime"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/theme/basic"
	"github.com/golang-gui/goui/theme/modern"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func check(ok bool, msg string) {
	if !ok {
		panic(msg)
	}
}

func main() {
	runtime.LockOSThread()
	theme := flag.String("theme", "light", "basic, light or dark")
	minimap := flag.Bool("minimap", false, "also verify with the minimap visible")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewLineMarks")
	must(err)
	if *theme == "basic" {
		app.SetStyleSheet(basic.Sheet())
	} else {
		app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *theme == "dark"}))
	}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 760, Height: 460}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("Line marks verification"))
	doc := source.NewDocument("{\n  \"a\": 1,\n  \"b\": 2,\n  \"c\": 3\n}\n" + strings.Repeat("\n", 150))
	must(doc.SetLanguage(language.JSON))
	v := source.NewSourceView()
	v.SetDocument(doc)
	v.SetShowLineMarks(true)
	v.SetShowMinimap(*minimap)
	v.SetAutomaticCompletion(false)
	id, err := doc.AddLineMark(source.LineMark{Line: 2})
	must(err)
	must(v.SetExecutionLine(2))
	keys := v.KeyBindings()
	keys[source.CommandToggleLineMark] = []gui.KeyGesture{{Key: gui.KeyF9}}
	must(v.SetKeyBindings(keys))
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(v)
	scroll.SetMainWeight(1)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	header := gui.NewHeaderBar()
	header.SetChild(gui.NewLabel("Line marks · normal event dispatch"))
	box.AddChild(header)
	box.AddChild(scroll)
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(v)
	var received []source.LineMarkEvent
	v.ConnectLineMarkAction(func(e source.LineMarkEvent) { received = append(received, e) })
	markInfo := func(line int) (gui.WidgetInfo, bool) {
		for _, c := range v.Snapshot().Children {
			if a, ok := c.Attributes["goui.sourceview.lineMark"].(map[string]any); ok && a["line"] == line {
				return c, true
			}
		}
		return gui.WidgetInfo{}, false
	}
	click := func(point geometry.Point, button events.PointerButton, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.PointerMove, events.PointerDown, events.PointerUp} {
			must(win.DispatchEvent(events.PointerEvent{EventType: kind, Position: point, Button: button, Modifiers: mods}))
		}
	}
	stage := 0
	done := false
	deadline := time.Now().Add(20 * time.Second)
	var selected textedit.Selection
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), fmt.Sprintf("stage %d timed out", stage))
		info, ok := markInfo(2)
		if stage < 4 && !ok {
			return
		}
		switch stage {
		case 0:
			if len(v.FoldingRegions()) == 0 {
				return
			}
			v.SetSelection(textedit.Selection{Anchor: 1, Caret: 3})
			selected = v.Selection()
			img, err := gui.RenderWidget(v, 2)
			must(err)
			bounds := v.Snapshot().Bounds
			rect := info.Bounds.Translate(geometry.Point{X: -bounds.X, Y: -bounds.Y})
			red, yellow := 0, 0
			for y := int(rect.Y*2) + 2; y < int((rect.Y+rect.Height)*2)-2; y++ {
				for x := 2; x < int(rect.Width*2)-2; x++ {
					c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
					if c == (color.NRGBA{R: 0xd8, G: 0x3b, B: 0x36, A: 255}) {
						red++
					}
					if c == (color.NRGBA{R: 0xc3, G: 0x94, A: 255}) {
						yellow++
					}
				}
			}
			check(red > 10 && yellow > 5, fmt.Sprintf("missing independent glyphs: red=%d yellow=%d", red, yellow))
			click(geometry.Point{X: info.Bounds.X + 8, Y: info.Bounds.Y + info.Bounds.Height/2}, events.PointerButtonLeft, 0)
			check(len(received) == 1 && received[0].Action == source.LineMarkToggle && received[0].Marks[0].ID == id, "left click request")
		case 1:
			click(geometry.Point{X: info.Bounds.X + 8, Y: info.Bounds.Y + info.Bounds.Height/2}, events.PointerButtonLeft, events.ModifierShift)
			check(len(received) == 2 && received[1].Action == source.LineMarkToggleEnabled, "shift click request")
		case 2:
			click(geometry.Point{X: info.Bounds.X + 8, Y: info.Bounds.Y + info.Bounds.Height/2}, events.PointerButtonRight, 0)
			check(len(received) == 3 && received[2].Action == source.LineMarkContextMenu, "context request")
			check(v.Selection() == selected, "gutter changed selection")
		case 3:
			for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
				must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: events.KeyF9}))
			}
			check(len(received) == 4 && received[3].Line == doc.Model().LineAt(selected.Caret), "F9 target")
			check(v.Selection() == selected && len(doc.LineMarks()) == 1, "request changed model")
			click(geometry.Point{X: info.Bounds.X + info.Bounds.Width + 8, Y: info.Bounds.Y + info.Bounds.Height/2}, events.PointerButtonLeft, 0)
			check(v.Selection().Anchor != v.Selection().Caret, "line numbers no longer select")
			v.SetSelection(textedit.Selection{})
			check(v.Fold(0), "cannot fold")
		case 4:
			fold, ok := markInfo(0)
			check(ok, "missing folded mark")
			click(geometry.Point{X: fold.Bounds.X + 8, Y: fold.Bounds.Y + fold.Bounds.Height/2}, events.PointerButtonLeft, 0)
			e := received[len(received)-1]
			check(e.Line == 0 && len(e.Marks) == 1 && e.Marks[0].Line == 2, "lost hidden mark target")
			selected = v.Selection()
			must(v.RevealLine(120))
		case 5:
			check(scroll.Snapshot().ScrollY > 0 && v.Selection() == selected, "reveal moved selection or failed to scroll")
			must(v.RevealLine(2))
		case 6:
			check(!v.FoldingRegions()[0].Collapsed, "reveal did not expand containing fold")
			must(doc.Model().Apply(textedit.Edit{Range: textedit.Range{}, Text: "\n"}, textedit.EditOptions{Atomic: true}))
			check(doc.LineMarks()[0].Line == 3, "mark did not track mounted edit")
			line, ok := v.ExecutionLine()
			check(ok && line == 3, "execution did not track")
			done = true
			log.Printf("PASS marks: input, fold, reveal, glyphs, tracking; typography=%s theme=%s", app.Typography().Name(), *theme)
			win.Destroy()
		}
		stage++
	})
	must(timer.Start(150 * time.Millisecond))
	app.Run()
	check(done, "closed before verification")
}
