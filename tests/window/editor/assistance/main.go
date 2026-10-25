// 编辑辅助和空白外观的原生窗口验证，需要桌面及窗口管理器。
// 启动：XMODIFIERS=@im=none go run ./tests/window/editor/assistance [-basic | -dark]。
// 初始为混合缩进与 Unicode 文本；通过普通 Window.DispatchEvent/CommitText 检查
// Home/End 切换、Shift 选区、缩进退格/撤销、Ctrl+G 定位、非法位置保留输入、
// Unicode 列定位、Escape 焦点返回和只读导航。断言原文、选区、焦点与浮层状态。
// 后续检查普通空格、Tab、NBSP、窄 NBSP、全角空格的标记、显示模式、全选和 CRLF/EOF。
// RenderWidget 使用当前窗口 Painter，标记须可见、圆形且居中；同后端不透明 sRGB
// 前后像素比较，不使用跨平台字体金图。采样位置、量化阈值及误差在 checkDotShape 说明。
// 分别用 GOUI_PLAT_PAINTER=software/opengl、GOUI_PLAT_SCALE=1/1.5/2 运行；
// -code-size 9/12/18 检查字号变化；-capture-dir DIR 将 PNG 写入仓库外现有目录。
// Linux 的 XMODIFIERS 设置用于未启动输入法服务的桌面，其他系统无需该环境变量。
// RenderWidget 不证明桌面合成、物理 IME 或实际跨屏 DPI 正确，这些需要单独观察。
// 不改变剪贴板、文件或系统设置（显式 capture-dir 的图片除外）；重启复位。
// PASS 后自动关闭，提前关闭失败；记录实际后端、缩放、主题与平台。
package main

import (
	"bytes"
	"flag"
	"fmt"
	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/basic"
	"github.com/golang-gui/goui/theme/modern"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
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
	useBasic := flag.Bool("basic", false, "Basic theme")
	dark := flag.Bool("dark", false, "Dark Modern")
	codeSize := flag.Float64("code-size", 12, "code font size in points")
	captureDir := flag.String("capture-dir", "", "existing external directory for whitespace captures")
	flag.Parse()
	check(*codeSize >= 8 && *codeSize <= 48, "code-size must be between 8 and 48")
	if *captureDir != "" {
		info, err := os.Stat(*captureDir)
		must(err)
		check(info.IsDir(), "capture-dir is not a directory")
	}
	app, e := gui.NewApplication("org.golanggui.SourceViewAssistance")
	must(e)
	rules := basic.Rules()
	if !*useBasic {
		rules = modern.Rules(modern.Options{Dark: *dark})
	}
	rules = append(rules, style.Name("source-view").FontSize(float32(*codeSize)))
	app.SetStyleSheet(style.Sheet(rules...))
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
	steps = append(steps, whitespaceChecks(app, win, v, float32(*codeSize), *captureDir)...)
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
func whitespaceChecks(app gui.Application, win gui.Window, v *source.SourceView, codeSize float32, captureDir string) []func() {
	const text = "      \t\u00a0\u202f\u3000"
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
		format := typography.TextFormat{Font: typography.FontInfo{Family: "monospace", Size: codeSize}}
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
		check(len(cells) == 10, "native whitespace sample did not provide ten measured cells")
		must(v.SetWhitespaceMode(source.WhitespaceNone))
		plain = capture()
		scale = float32(plain.Bounds().Dx()) / v.Rect().Width
		must(v.SetWhitespaceMode(source.WhitespaceAll))
		marked = capture()
		for i, r := range cells {
			check(changed(plain, marked, r) > 0, "missing native whitespace marker in cell "+string(rune('0'+i)))
		}
		if captureDir != "" {
			saveCapture(plain, filepath.Join(captureDir, "whitespace-none.png"))
			saveCapture(marked, filepath.Join(captureDir, "whitespace-all.png"))
		}
		for i := 1; i < 6; i++ {
			checkDotShape(plain, marked, cells[i], sw, float64(scale))
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
		for i := 1; i < 6; i++ {
			checkDotShape(base, selected, cells[i], cells[i].Width, float64(scale))
		}
		if captureDir != "" {
			saveCapture(selected, filepath.Join(captureDir, "whitespace-selection.png"))
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

// 采样只包含实际空格单元，避开光标、字形、边框。比较同后端、不透明 sRGB 的
// 前后 RGB 覆盖差（忽略 <=1 byte 的尾部量化），不依赖字体金图。圆点必须二维，
// 包围盒宽高差最多 1 物理像素，中心距测量单元中心最多 1 像素；4×1 扁条必然失败。
// 水平方向只取单元内侧完整像素，再留 1 像素，避免 ceil 扩张混入相邻 Tab 的边缘。
func checkDotShape(before, after image.Image, cell geometry.Rectangle, spaceWidth float32, scale float64) {
	minX, minY, maxX, maxY := after.Bounds().Max.X, after.Bounds().Max.Y, -1, -1
	for y := int(math.Floor(float64(cell.Y) * scale)); y < int(math.Ceil(float64(cell.Y+cell.Height)*scale)); y++ {
		for x := int(math.Ceil(float64(cell.X)*scale)) + 1; x < int(math.Floor(float64(cell.X+cell.Width)*scale))-1; x++ {
			ar, ag, ab, _ := before.At(x, y).RGBA()
			br, bg, bb, _ := after.At(x, y).RGBA()
			delta := max(math.Abs(float64(ar>>8)-float64(br>>8)), math.Abs(float64(ag>>8)-float64(bg>>8)), math.Abs(float64(ab>>8)-float64(bb>>8)))
			if delta <= 1 {
				continue
			}
			minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
		}
	}
	width, height := maxX-minX+1, maxY-minY+1
	check(width >= 2 && height >= 2 && abs(width-height) <= 1, fmt.Sprintf("whitespace is not round: %dx%d cell=%v scale=%g", width, height, cell, scale))
	expectedDiameter := float64(spaceWidth) * 2 / 7 * scale
	check(float64(max(width, height)) <= math.Ceil(expectedDiameter)+2 && float64(min(width, height)) >= math.Max(2, math.Floor(expectedDiameter)-1), fmt.Sprintf("whitespace diameter %dx%d, expected %g", width, height, expectedDiameter))
	cx, cy := float64(minX+maxX+1)/2, float64(minY+maxY+1)/2
	check(math.Abs(cx-float64(cell.X+cell.Width/2)*scale) <= 1 && math.Abs(cy-float64(cell.Y+cell.Height/2)*scale) <= 1, fmt.Sprintf("whitespace center (%g,%g), cell=%v scale=%g", cx, cy, cell, scale))
}
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
func saveCapture(img image.Image, path string) {
	f, err := os.Create(path)
	must(err)
	must(png.Encode(f, img))
	must(f.Close())
}
