// 连续输入的着色稳定性，需要桌面、原生排版及正常事件循环。
// 启动：XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software GOUI_PLAT_SCALE=1
// go run ./tests/window/editor/highlight [-dark]；另以 1.5 倍运行。
// 初始为有焦点的 JSON SourceView，光标在首行字符串中，补全/折叠/换行关闭。
// 经 CommitText 连续插入 30 字，再经 DispatchEvent Backspace 恢复原文，检查撤销/重做。
// 每次输入后经 DispatchEvent(PaintEvent{}) 在分析定时器前绘制一帧，确定性覆盖待分析状态；
// 同时检查事件循环自然产生的帧，不直接调用分析器或修改控件私有状态。
// 每个窗口帧在 Paint 外 RenderWidget；首行未编辑前缀和后续行的像素必须与基线完全一致。
// 使用同窗口、字体、后端的预乘 sRGB RGBA 值，零误差；裁剪排除光标、侧栏及当前行变化部分。
// 基线需包含已着色文字，首行固定前缀及后续已缓存行不能清色或消失。
// 输入结束后至少再检查五帧，并断言文本/光标复原；超时、提前关闭或断言失败非零退出。
// 不访问剪贴板/文件/设置，重启复位，自动完成后关闭。RenderWidget 不代表物理显示器合成，
// macOS/Windows 和真实 IME 需分别验证；纯引擎测试另强制覆盖分析未完成的确定性中间状态。
package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"runtime"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
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

func main() {
	runtime.LockOSThread()
	dark := flag.Bool("dark", false, "Modern dark appearance")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewHighlight")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 760, Height: 420}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView — stable colors while typing"))
	v := source.NewSourceView()
	v.SetWrapMode(source.WrapNone)
	v.SetShowLineNumbers(false)
	v.SetFoldingEnabled(false)
	v.SetAutomaticCompletion(false)
	colors := scheme.Light()
	if *dark {
		colors = scheme.Dark()
	}
	v.SetScheme(colors)
	text := "{\"name\": \"hello\",\n" + strings.Repeat("  \"key\": \"value\",\n", 9) + "  \"last\": true\n}"
	m := v.Document().Model()
	m.SetText(text)
	must(v.Document().SetLanguage(language.JSON))
	caret := strings.Index(text, "hello") + 2
	v.SetSelection(textedit.Selection{Anchor: caret, Caret: caret})
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(v)
	win.SetWidget(scroll)
	must(win.Show())
	win.SetFocusedWidget(v)
	primary, _ := gui.ModPrimary.Resolve()
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	var baseline image.Image
	var prefix, suffix image.Rectangle
	checks, edits, settled := 0, 0, 0
	deadline := time.Now().Add(20 * time.Second)
	done := false
	timer := app.NewTimer()
	defer timer.Stop()
	var capture func()
	capture = func() {
		must(win.RequestPaint())
		if v.Rect().Width <= 0 || v.Rect().Height <= 0 {
			check(baseline == nil, "allocated editor disappeared during typing")
			return
		}
		img, err := gui.RenderWidget(v, 0)
		must(err)
		if baseline == nil {
			scale := float64(img.Bounds().Dx()) / float64(v.Rect().Width)
			prefix = image.Rect(int(6*scale), int(6*scale), int(65*scale), int(22*scale))
			suffix = image.Rect(int(6*scale), int(50*scale), int(360*scale), int(200*scale))
			property := colors.Definition().Tokens[language.KindProperty]
			n := 0
			for y := prefix.Min.Y; y < prefix.Max.Y; y++ {
				for x := prefix.Min.X; x < prefix.Max.X; x++ {
					r, g, b, a := img.At(x, y).RGBA()
					if r == uint32(property.R)*257 && g == uint32(property.G)*257 && b == uint32(property.B)*257 && a == 65535 {
						n++
					}
				}
			}
			if n < 5 {
				return
			}
			baseline = img
			log.Printf("baseline ready: dark=%t scale=%g", *dark, scale)
			return
		}
		for _, r := range []image.Rectangle{prefix, suffix} {
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					r1, g1, b1, a1 := baseline.At(x, y).RGBA()
					r2, g2, b2, a2 := img.At(x, y).RGBA()
					check(r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2, fmt.Sprintf("text/color changed at (%d,%d), edits=%d", x, y, edits))
				}
			}
		}
		checks++
		if edits == 62 {
			settled++
		}
	}
	h := win.ConnectFrame(func(time.Time) { capture() })
	defer h.Disconnect()
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), "highlight verification timeout")
		if baseline == nil {
			must(win.RequestPaint())
			return
		}
		switch {
		case edits < 30:
			must(win.CommitText("x"))
		case edits < 60:
			key(events.KeyBackspace, 0)
		case edits == 60:
			key(events.KeyZ, primary)
		case edits == 61:
			key(events.KeyZ, primary|events.ModifierShift)
		default:
			must(win.RequestPaint())
			if settled < 5 {
				return
			}
			check(m.Text() == text && v.Selection().Caret == caret, "input/history did not restore text/caret")
			check(checks >= 10, "too few rendered frames")
			done = true
			log.Printf("PASS stable current/suffix colors across %d frames, 60 typing/deletion events and undo/redo", checks)
			win.Destroy()
			return
		}
		edits++
		must(win.DispatchEvent(events.PaintEvent{}))
	})
	must(timer.Start(8 * time.Millisecond))
	app.Run()
	check(done, "highlight verification aborted")
}
