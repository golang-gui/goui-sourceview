// 特殊字符窗口验收，需要桌面、原生排版和字体。
// 启动：XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software go run ./tests/window/editor/special_characters
// -basic/-dark 切换主题，GOUI_PLAT_SCALE=1/1.5；初态为同文档两个视图，标记均关闭。
// 正常 Window.DispatchEvent 点击开关、移动鼠标、Delete/Undo；setter 仅准备选区。
// RenderWidget 使用当前窗口 Painter：用独立原生 A+ZWSP+B 的 cluster 度量推导标记位置
// 检查至少 3 个新增红色像素。图片为同后端不透明 sRGB，比较 RGBA 转 8-bit 后的 RGB，
// 红色判据 R>G+30 且 R>B+30；不依赖跨平台字体金图或放大抗锯齿容差。
// 断言第二视图像素不变、关闭后逐像素恢复、tooltip 为安全码点名、正文/修订/保存状态不因开关改变；
// Delete 后标记消失，Undo 恢复原字符和 clean。语义快照仍发布原文及开关状态。
// 不读写文件、剪贴板或系统设置。每步有超时，成功自动退出，提前关闭返回非零；重启复位。
// RenderWidget 不代替桌面合成、真实 IME、物理输入或其他平台原生运行，须另验。
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"os"
	"runtime"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview/sourceview"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/typography"
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
func samePixels(a, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			ar, ag, ab, aa := a.At(x, y).RGBA()
			br, bg, bb, ba := b.At(x, y).RGBA()
			if ar != br || ag != bg || ab != bb || aa != ba {
				return false
			}
		}
	}
	return true
}
func redPixels(img image.Image, logical geometry.Rectangle, width float32) int {
	scale := float64(img.Bounds().Dx()) / float64(width)
	r := image.Rect(int(math.Floor(float64(logical.X)*scale)), int(math.Floor(float64(logical.Y)*scale)), int(math.Ceil(float64(logical.X+logical.Width)*scale)), int(math.Ceil(float64(logical.Y+logical.Height)*scale))).Intersect(img.Bounds())
	count := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			rr, gg, bb := int(r>>8), int(g>>8), int(b>>8)
			if rr > gg+30 && rr > bb+30 {
				count++
			}
		}
	}
	return count
}

func main() {
	runtime.LockOSThread()
	basicTheme := flag.Bool("basic", false, "Basic theme")
	dark := flag.Bool("dark", false, "Modern dark")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewSpecialCharacters")
	must(err)
	rules := modern.Rules(modern.Options{Dark: *dark})
	if *basicTheme {
		rules = basic.Rules()
	}
	rules = append(rules, style.Name("source-view").FontFamily("monospace").FontSize(12), style.Name("source-view").Part("special-character").ForegroundColor(color.NRGBA{R: 255, A: 255}))
	app.SetStyleSheet(style.Sheet(rules...))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 900, Height: 650}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView special characters"))
	text := "A\u200bB\nC\u2060D\n👩\u200d💻\nabc \u2067אבג\u2069 def\nX\x00Y\n"
	d := source.NewDocument(text)
	a, b := source.NewSourceView(), source.NewSourceView()
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	toggle := gui.NewButton()
	toggle.SetChild(gui.NewLabel("Special characters"))
	toggle.ConnectClicked(func() { a.SetShowSpecialCharacters(!a.ShowSpecialCharacters()) })
	box.AddChild(toggle)
	for _, v := range []*source.SourceView{a, b} {
		v.SetDocument(d)
		v.SetShowLineNumbers(false)
		v.SetFoldingEnabled(false)
		v.SetWrapMode(source.WrapNone)
		v.SetAutomaticCompletion(false)
		v.ConnectError(func(err error) { panic(err) })
		scroll := gui.NewScrollView()
		scroll.SetStyleName("source-view")
		scroll.SetChild(v)
		scroll.SetMainWeight(1)
		box.AddChild(scroll)
	}
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(toggle)
	ref, err := app.Typography().NewTextLayout("A\u200bB", typography.TextFormat{Font: typography.FontInfo{Family: "monospace", Size: 12}}, 1000, 1000)
	must(err)
	lines, clusters := ref.MeasureMetrics()
	ref.Destroy()
	check(len(lines) == 1, "native reference must be one display line")
	var marked source.TextRange
	var markerX float32
	for _, c := range clusters {
		if c.Start <= 1 && 1 < c.Start+c.Length {
			check(c.Direction == typography.TextLeftToRight, "reference cluster must be LTR")
			marked = source.TextRange{Start: c.Start, End: c.Start + c.Length}
			markerX = c.X
			break
		}
	}
	check(marked.Start <= 1 && marked.End >= 4, "native reference omitted ZWSP cluster")
	height := lines[0].Height
	cell := geometry.Rect(6+markerX-3, 6+height-6, 8, 7)
	deleted := text[:marked.Start] + text[marked.End:]
	capture := func(v *source.SourceView) image.Image { img, err := gui.RenderWidget(v, 0); must(err); return img }
	clickToggle := func() {
		r := toggle.Snapshot().Bounds
		p := geometry.Point{X: r.X + r.Width/2, Y: r.Y + r.Height/2}
		must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerDown, Button: events.PointerButtonLeft, Position: p}))
		must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerUp, Button: events.PointerButtonLeft, Position: p}))
	}
	primary, _ := gui.ModPrimary.Resolve()
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	var plain, other image.Image
	revision := d.Revision()
	type scene struct {
		name  string
		start func()
		ready func() bool
	}
	scenes := []scene{
		{"baseline", func() {}, func() bool {
			if a.Rect().Width <= 0 || b.Rect().Width <= 0 {
				return false
			}
			plain, other = capture(a), capture(b)
			check(redPixels(plain, cell, a.Rect().Width) == 0, "baseline already contains a red marker")
			check(!a.ShowSpecialCharacters() && !b.ShowSpecialCharacters() && !d.Modified(), "initial options/save state")
			return true
		}},
		{"normal toggle and visible marker", clickToggle, func() bool {
			check(a.ShowSpecialCharacters() && !b.ShowSpecialCharacters(), "toggle affected wrong view")
			if redPixels(capture(a), cell, a.Rect().Width) < 3 {
				return false
			}
			check(samePixels(other, capture(b)), "shared document polluted other view pixels")
			check(d.Text() == text && d.Revision() == revision && !d.Modified(), "display changed document/save point")
			check(a.Snapshot().Attributes["goui.sourceview"].(map[string]any)["specialCharacters"] == true, "snapshot option missing")
			return true
		}},
		{"normal pointer tooltip", func() {
			r := a.Snapshot().Bounds
			must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerMove, Position: geometry.Point{X: r.X + 6 + markerX, Y: r.Y + 6 + height/2}}))
		}, func() bool { return strings.Contains(a.Tooltip(), "U+200B ZERO WIDTH SPACE") }},
		{"disable restores pixels", clickToggle, func() bool { return !a.ShowSpecialCharacters() && samePixels(plain, capture(a)) && !d.Modified() }},
		{"enable for edit", clickToggle, func() bool { return a.ShowSpecialCharacters() && redPixels(capture(a), cell, a.Rect().Width) >= 3 }},
		{"delete original marked cluster", func() {
			win.SetFocusedWidget(a)
			a.SetSelection(source.TextSelection{Anchor: marked.Start, Caret: marked.End})
			check(a.Selection().Range() == marked, "selection disagreed with native reference cluster")
			key(events.KeyDelete, 0)
		}, func() bool {
			return d.Text() == deleted && d.Modified() && redPixels(capture(a), cell, a.Rect().Width) == 0
		}},
		{"undo restores marker and saved identity", func() { key(events.KeyZ, primary) }, func() bool {
			return d.Text() == text && !d.Modified() && redPixels(capture(a), cell, a.Rect().Width) >= 3
		}},
	}
	timer := app.NewTimer()
	defer timer.Stop()
	stage := -1
	started := time.Now()
	complete := false
	timer.ConnectTimeout(func() {
		if stage < 0 {
			stage = 0
			started = time.Now()
			scenes[stage].start()
			return
		}
		if time.Since(started) > 20*time.Second {
			panic(fmt.Sprintf("timeout %s: special=%t modified=%t text=%q", scenes[stage].name, a.ShowSpecialCharacters(), d.Modified(), d.Text()))
		}
		if !scenes[stage].ready() {
			return
		}
		log.Printf("PASS %s", scenes[stage].name)
		stage++
		if stage == len(scenes) {
			complete = true
			log.Printf("PASS specialcharacters OS=%s typography=%s scale=%s basic=%t dark=%t", runtime.GOOS, app.Typography().Name(), os.Getenv("GOUI_PLAT_SCALE"), *basicTheme, *dark)
			win.Destroy()
			return
		}
		started = time.Now()
		scenes[stage].start()
	})
	must(timer.Start(70 * time.Millisecond))
	app.Run()
	check(complete, "closed before special character verification completed")
}
