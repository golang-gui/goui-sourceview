// 编辑器外框与滚动条组合验证。需要桌面、原生字体和事件循环。
// 启动：XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1
// go run ./tests/window/editor/frame；分别以 1、1.5、2 倍运行。
// -theme basic/light/dark 选择样式；-inspect 在自动断言通过后保留窗口。
// 初始为 Integrated 窗口中的 TextView、SourceView，各有双向滚动条。
// 正常 DispatchEvent 输入依次验证点击聚焦、滚轮、两轴拖动、焦点转移；
// 再验证禁用、自动换行隐藏横条、短文本隐藏双条及主题切换后重新布局。
// 每步断言视口、两条滚动条都在同一外框内，聚焦/禁用不改变正文宽高，
// 语义树仅暴露真实正文，滚动范围和拖动后偏移符合预期。
// 自动验证通过后关闭；超时、提前关闭或断言失败返回非零。手动检查时滚动、
// 缩放窗口并在两编辑器间切换焦点，边框及圆角不得被正文或滚动条覆盖。
// 各平台/绘制后端需各自原生运行；本程序不代表真实 IME 或混合 DPI 验收。
// 不访问剪贴板、文件或系统设置；重启复位，关闭窗口退出。
package main

import (
	"flag"
	"fmt"
	"log"
	"math"
	"runtime"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/basic"
	"github.com/golang-gui/goui/theme/modern"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func check(ok bool, message string) {
	if !ok {
		panic(message)
	}
}
func near(a, b float32) bool { return math.Abs(float64(a-b)) < .001 }

func main() {
	runtime.LockOSThread()
	mode := flag.String("theme", "light", "basic, light or dark")
	inspect := flag.Bool("inspect", false, "keep the verified window open")
	flag.Parse()
	app, err := gui.NewApplication("org.golang-gui.SourceViewFrame")
	must(err)
	sheet := func() style.StyleSheet {
		if *mode == "basic" {
			return basic.Sheet()
		}
		return modern.Sheet(modern.Options{Dark: *mode == "dark"})
	}
	app.SetStyleSheet(sheet())
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1000, Height: 460}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	win.SetMinSize(geometry.Size{Width: 600, Height: 240})
	must(win.SetTitle("Editor frames — TextView / SourceView"))
	plain, code := gui.NewTextView(), source.NewSourceView()
	plain.SetWrapMode(gui.WrapNone)
	code.SetWrapMode(source.WrapNone)
	code.SetAutomaticCompletion(false)
	text := strings.Repeat("A line with enough text to overflow: "+strings.Repeat("word ", 240)+"\n", 80)
	plain.Model().SetText(text)
	code.Document().Model().SetText(text)
	editors := []gui.Widget{plain, code}
	hosts := []*gui.ScrollView{gui.NewScrollView(), gui.NewScrollView()}
	row := gui.NewLinearBox(layout.DirectionHorizontal)
	row.SetCrossAlign(layout.CrossStretch)
	row.SetSpacing(16)
	row.SetPadding(12)
	row.SetMainWeight(1)
	for i, host := range hosts {
		host.SetStyleName([]string{"text-view", "source-view"}[i])
		host.SetChild(editors[i])
		row.AddChild(host)
	}
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	bar := gui.NewHeaderBar()
	bar.SetChild(gui.NewLabel("TextView / SourceView — shared editor frames"))
	box.AddChild(bar)
	box.AddChild(row)
	win.SetWidget(box)
	must(win.Show())
	click := func(point geometry.Point) {
		for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
			must(win.DispatchEvent(events.PointerEvent{EventType: kind, Button: events.PointerButtonLeft, Position: point}))
		}
	}
	viewport := func(i int) geometry.Rectangle { return hosts[i].Children()[0].Rect() }
	barOf := func(i int, direction layout.Direction) *gui.ScrollBar {
		for _, w := range hosts[i].Children() {
			if b, ok := w.(*gui.ScrollBar); ok && b.Orientation() == direction {
				return b
			}
		}
		panic("missing structural scrollbar")
	}
	verify := func() {
		for i, host := range hosts {
			inset := float32(1)
			if *mode != "basic" {
				inset = 3 + 4*(1-1/math.Sqrt2)
			}
			v := viewport(i)
			check(near(v.X, inset) && near(v.Y, inset), fmt.Sprintf("wrong viewport origin: %v, inset %g", v, inset))
			h, b := host.Rect(), host.Snapshot()
			check(len(b.Children) == 1, "frame changed semantic children")
			for _, w := range host.Children() {
				if !w.Visible() {
					continue
				}
				r := w.Rect()
				check(r.X >= inset-.001 && r.Y >= inset-.001 && r.X+r.Width <= h.Width-inset+.001 && r.Y+r.Height <= h.Height-inset+.001, "viewport/bar outside editor frame")
			}
		}
	}
	drag := func(i int, direction layout.Direction) {
		b := barOf(i, direction).Snapshot().Bounds
		p := geometry.Point{X: b.X + b.Width/2, Y: b.Y + 8}
		if direction == layout.DirectionHorizontal {
			p = geometry.Point{X: b.X + 8, Y: b.Y + b.Height/2}
		}
		must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerDown, Button: events.PointerButtonLeft, Position: p}))
		if direction == layout.DirectionVertical {
			p.Y += 80
		} else {
			p.X += 80
		}
		must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerMove, Buttons: events.PointerButtonLeftDown, Position: p}))
		must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerUp, Button: events.PointerButtonLeft, Position: p}))
	}
	timer := app.NewTimer()
	defer timer.Stop()
	stage, frames, previous := 0, 0, 0
	done := false
	var stable [2]geometry.Rectangle
	win.ConnectFrame(func(time.Time) { frames++ })
	deadline := time.Now().Add(30 * time.Second)
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), fmt.Sprintf("timeout at stage %d", stage))
		must(win.RequestPaint())
		if frames == previous {
			return
		}
		previous = frames
		verify()
		switch stage {
		case 0:
			for _, host := range hosts {
				if info := host.Snapshot(); info.MaxScrollX == 0 || info.MaxScrollY == 0 {
					return // 等待正文的事件循环分步测量完成，仍受总超时约束。
				}
			}
			for i, host := range hosts {
				check(host.Snapshot().MaxScrollX > 0 && host.Snapshot().MaxScrollY > 0, "initial text lacks two-axis overflow")
				stable[i] = viewport(i)
			}
			r := plain.Snapshot().Bounds
			click(geometry.Point{X: r.X + 100, Y: r.Y + 14})
		case 1:
			check(plain.Focused() && hosts[0].ContainsFocus(), "click did not focus editor/frame")
			check(viewport(0) == stable[0], "focus reflowed content")
			r := plain.Snapshot().Bounds
			must(win.DispatchEvent(events.WheelEvent{Position: geometry.Point{X: r.X + 100, Y: r.Y + 100}, DeltaY: 50}))
		case 2:
			check(hosts[0].ScrollY() > 0, "framed viewport did not receive wheel")
			for i, host := range hosts {
				host.SetScrollY(0)
				drag(i, layout.DirectionVertical)
			}
		case 3:
			for i, host := range hosts {
				check(host.ScrollY() > 0, "framed vertical thumb did not drag")
				host.SetScrollX(0)
				drag(i, layout.DirectionHorizontal)
			}
		case 4:
			for _, host := range hosts {
				check(host.ScrollX() > 0, "framed horizontal thumb did not drag")
			}
			r := code.Snapshot().Bounds
			click(geometry.Point{X: r.X + 100, Y: r.Y + 14})
		case 5:
			check(code.Focused() && !hosts[0].ContainsFocus() && hosts[1].ContainsFocus(), "frame retained stale focus")
			check(viewport(0) == stable[0] && viewport(1) == stable[1], "focus transfer changed viewport")
			hosts[0].SetEnabled(false)
		case 6:
			check(viewport(0) == stable[0], "disabled frame changed content size")
			hosts[0].SetEnabled(true)
			plain.SetWrapMode(gui.WrapWordChar)
			code.SetWrapMode(source.WrapWordChar)
		case 7:
			for i := range hosts {
				check(!barOf(i, layout.DirectionHorizontal).Visible() && barOf(i, layout.DirectionVertical).Visible(), "wrapping did not hide only the horizontal bar")
			}
			plain.Model().SetText("short")
			code.Document().Model().SetText("short")
		case 8:
			for i, host := range hosts {
				check(!barOf(i, layout.DirectionHorizontal).Visible() && !barOf(i, layout.DirectionVertical).Visible(), "short text retained a scrollbar")
				check(host.ScrollX() == 0 && host.ScrollY() == 0, "short text retained an offset")
			}
			if *mode == "basic" {
				*mode = "light"
			} else {
				*mode = "basic"
			}
			app.SetStyleSheet(sheet())
		case 9:
			done = true
			log.Print("PASS editor frame geometry, focus stability, wheel, both thumb drags, disabled, wrapping, content shrink and theme invalidation")
			if *inspect {
				plain.SetWrapMode(gui.WrapNone)
				code.SetWrapMode(source.WrapNone)
				plain.Model().SetText(text)
				code.Document().Model().SetText(text)
				*mode = "light"
				app.SetStyleSheet(sheet())
				timer.Stop()
				return
			}
			win.Destroy()
		}
		stage++
	})
	must(timer.Start(200 * time.Millisecond))
	app.Run()
	check(done, "frame verification aborted")
}
