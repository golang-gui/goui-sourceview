// 色块地图窗口验收：go run ./tests/window/editor/minimap [-theme basic|light|dark]。
// 需要桌面、原生排版及字体；Linux 独立 X11 + 合成器，分别以 1/1.5 缩放运行。
// 初始为 Integrated 窗口及 2000 行 JSON。自动经 Window.DispatchEvent 点击、
// 拖动、滚轮、按键和调整布局的按钮，断言选区/焦点保持、滚动方向、地图边界、
// 正文宽度变化、关闭恢复和单行长文本编辑后任务收敛。RenderWidget 检查地图中
// 与背景不同的内容像素，取内部 NRGBA，不要求字体抗锯齿或跨平台精确金图。
// 日志记录事件分发同步耗时及阶段等待时间；这些不等于首帧上屏延迟。
// 完成自动关闭，失败/提前关闭/30 秒未收敛返回非零；重启复位。
// 不写用户文件、剪贴板或系统设置。其他平台和真实 IME 需另行验收。
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
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewMinimap")
	must(err)
	if *theme == "basic" {
		app.SetStyleSheet(basic.Sheet())
	} else {
		app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *theme == "dark"}))
	}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 900, Height: 520}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("Minimap verification"))
	doc := source.NewDocument(strings.Repeat("{\"name\": \"map\", \"number\": 123},\n", 2000))
	must(doc.SetLanguage(language.JSON))
	v := source.NewSourceView()
	v.SetDocument(doc)
	v.SetShowMinimap(true)
	v.SetAutomaticCompletion(false)
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(v)
	scroll.SetMainWeight(1)
	resize := gui.NewButton()
	resize.SetChild(gui.NewLabel("Resize pane"))
	resize.SetMinSize(geometry.Size{Width: 100})
	resize.SetMaxSize(geometry.Size{Width: 100})
	resize.ConnectClicked(func() { resize.SetMinSize(geometry.Size{Width: 250}); resize.SetMaxSize(geometry.Size{Width: 250}) })
	body := gui.NewLinearBox(layout.DirectionHorizontal)
	body.SetCrossAlign(layout.CrossStretch)
	body.SetMainWeight(1)
	body.AddChild(scroll)
	body.AddChild(resize)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	header := gui.NewHeaderBar()
	header.SetChild(gui.NewLabel("Minimap · normal input and bounded sampling"))
	box.AddChild(header)
	box.AddChild(body)
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(v)
	mapInfo := func() (gui.WidgetInfo, bool) {
		for _, c := range v.Snapshot().Children {
			if c.Role == "minimap" {
				return c, true
			}
		}
		return gui.WidgetInfo{}, false
	}
	ready := func() bool {
		info, ok := mapInfo()
		return ok && !info.Attributes["goui.sourceview.minimap"].(map[string]any)["pending"].(bool)
	}
	dispatch := func(e events.Event) {
		start := time.Now()
		must(win.DispatchEvent(e))
		log.Printf("input %v: %s", e.Type(), time.Since(start))
	}
	pointer := func(kind events.EventType, p geometry.Point) {
		dispatch(events.PointerEvent{EventType: kind, Position: p, Button: events.PointerButtonLeft})
	}
	click := func(p geometry.Point) {
		pointer(events.PointerMove, p)
		pointer(events.PointerDown, p)
		pointer(events.PointerUp, p)
	}
	center := func(r geometry.Rectangle) geometry.Point {
		return geometry.Point{X: r.X + r.Width/2, Y: r.Y + r.Height/2}
	}
	stage := 0
	done := false
	deadline := time.Now().Add(30 * time.Second)
	started := time.Now()
	selected := textedit.Selection{Anchor: 1, Caret: 4}
	var oldWidth, oldHeight, beforeWheel float32
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), fmt.Sprintf("stage %d timed out", stage))
		info, exists := mapInfo()
		if stage != 5 && (!exists || !ready()) {
			return
		}
		switch stage {
		case 0:
			v.SetSelection(selected)
			image, err := gui.RenderWidget(v, 1.5)
			must(err)
			local := info.Bounds.Translate(geometry.Point{X: -v.Snapshot().Bounds.X, Y: -v.Snapshot().Bounds.Y})
			background := color.NRGBAModel.Convert(image.At(int((local.X+local.Width-5)*1.5), int((local.Height-4)*1.5))).(color.NRGBA)
			different := 0
			for y := 20; y < int(local.Height*1.5)-10; y++ {
				for x := int((local.X + 5) * 1.5); x < int((local.X+50)*1.5); x++ {
					if color.NRGBAModel.Convert(image.At(x, y)).(color.NRGBA) != background {
						different++
					}
				}
			}
			check(different > 100, "地图没有内容像素")
			click(geometry.Point{X: info.Bounds.X + 30, Y: info.Bounds.Y + info.Bounds.Height*.7})
		case 1:
			check(scroll.Snapshot().ScrollY > 1000, "地图点击没有滚动")
			check(v.Selection() == selected && win.FocusedWidget() == v, "地图点击改变选区或焦点")
			thumb := info.Attributes["goui.sourceview.minimap"].(map[string]any)["viewport"].(geometry.Rectangle)
			p := geometry.Point{X: info.Bounds.X + 30, Y: info.Bounds.Y + thumb.Y + thumb.Height/2}
			pointer(events.PointerDown, p)
			p.Y = info.Bounds.Y
			pointer(events.PointerMove, p)
			pointer(events.PointerUp, p)
		case 2:
			check(scroll.Snapshot().ScrollY < 2, "拖动地图不能返回顶部")
			beforeWheel = scroll.Snapshot().ScrollY
			dispatch(events.WheelEvent{Position: center(info.Bounds), DeltaY: 4, Mode: events.WheelDeltaLine})
		case 3:
			check(scroll.Snapshot().ScrollY > beforeWheel, "地图上的滚轮未传给 ScrollView")
			check(v.Selection() == selected, "滚轮改变选区")
			oldWidth, oldHeight = v.Rect().Width, v.Rect().Height
			click(center(resize.Snapshot().Bounds))
			header.SetMinSize(geometry.Size{Height: 100})
		case 4:
			check(v.Rect().Width < oldWidth && v.Rect().Height < oldHeight, "调整布局没有改变宽高")
			check(info.Bounds.X+info.Bounds.Width <= v.Snapshot().Bounds.X+v.Rect().Width+.1, "地图超出正文边界")
			v.SetShowMinimap(false)
		case 5:
			_, ok := mapInfo()
			check(!ok, "关闭后仍暴露地图")
			v.SetShowMinimap(true)
			v.SetWrapMode(source.WrapWordChar)
			doc.Model().SetText(strings.Repeat("{\"name\":\"value\",\"n\":1234},", 20000))
			v.SetSelection(textedit.Selection{Anchor: doc.Model().Len() / 2, Caret: doc.Model().Len() / 2})
			v.RevealSelection()
			win.SetFocusedWidget(v)
		case 6:
			// 已确认文本走 IMContext 正常编辑路径，随后等待重新采样结束。
			start := time.Now()
			must(win.CommitText("x"))
			log.Printf("long-line CommitText: %s", time.Since(start))
			selected = v.Selection()
		case 7:
			check(doc.Model().Len() == len(`{"name":"value","n":1234},`)*20000+1 && v.Selection() == selected, "长行编辑或选区异常")
			click(geometry.Point{X: info.Bounds.X + 30, Y: info.Bounds.Y + info.Bounds.Height*.8})
		case 8:
			check(scroll.Snapshot().ScrollY > 0 && v.Selection() == selected, "长折行地图导航改变了选区")
			log.Printf("PASS minimap theme=%s total=%s", *theme, time.Since(started))
			done = true
			win.Destroy()
			return
		}
		log.Printf("stage %d completed after %s", stage, time.Since(started))
		stage++
	})
	must(timer.Start(150 * time.Millisecond))
	app.Run()
	check(done, "窗口提前关闭")
}
