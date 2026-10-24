// 超大单行文本交互验证，需要独立桌面、原生排版和字体，不使用 DevServer。
// 启动：go run ./tests/window/editor/paragraph -file /data/xuyang8/temp/test.json
// 加 -textview 验证 GOUI TextView；默认验证 SourceView。可选 -plain 关闭 JSON 高亮。
// -nowrap 验证水平长行：拖水平滚动条至中部，逐字符点击、左右键与连续拖选，
// 跨 4 KiB 片段时不得跳字、断裂选区或停住；Home/End 到逻辑行边界。
// -verify-end -nowrap 自动派发 End、三次 Left、三次 Right，在正常、最大化、恢复窗口下验证。
// 每一步检查真实选区位置，并要求 RenderWidget 中出现可见的洋红光标像素；
// 使用专用 Caret 色，与 JSON 语法颜色区分；sRGB 不透明输出，仅识别该色的覆盖像素，
// 不依赖字体金图。允许准备目标排版及光标闪烁的等待，超时或提前关闭失败；成功自动关闭。
// 此自动场景仅 SourceView，输入尾部应为至少三个 ASCII 字符，无额外文件写入。
// -verify-input 自动验证冷定位后立即 Left/提交文本的顺序，选区替换、删除、
// 换行、只读及撤销重做；可额外 -clipboard 测试末尾小选区的真实剪切/粘贴。
// -clipboard 会覆盖本显示的剪贴板，只应在独立测试桌面启用；默认不访问剪贴板。
// 文件始终只读，最后与原始全文精确比较；提前关闭或超时失败。可与 -nowrap 组合。
// 使用 GOUI_PLAT_PAINTER=software/opengl（Windows 另可 direct2d）及
// GOUI_PLAT_SCALE=1/1.5/2 选择环境；Linux 无 IME 服务可设 XMODIFIERS=@im=none。
// 初始：Modern、Integrated 窗口、自动换行、顶部工具栏、可编辑正文。
// 操作：等待 READY；点击正文输入、删除、撤销／重做；点击 Middle / End 后等待
// TEXT_READY 再编辑；拖动滚动条到中部；连续拖动窗口边缘改变宽高。
// 预期：逻辑文本始终未被人为插入 LF；首屏、远处和编辑后文字可延迟显示，
// 窗口持续响应。输入只修改当前光标附近，撤销恢复；保留语法颜色、选区与光标。
// 新建原生布局输入不得超过 8192 字节，超限直接失败；HEARTBEAT 记录事件循环
// 最大间隔，LOAD 单独记录同步模型加载时间，TEXT_READY 记录定位到可见文字的延迟。
// 记录 OS/WM、实际后端、缩放和每种操作；日志不能代替实际输入及截图验收。
// 副作用：只读指定文件，仅窗口和标准输出；不访问剪贴板、不修改系统设置。
// 平台字体、抗锯齿和 IME 可不同，远端需原生运行。重启复位；关闭窗口正常退出。
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"os"
	"runtime"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/typography"
	"github.com/golang-gui/goui/theme/modern"
)

type observedApplication struct {
	gui.Application
	context *observedContext
}

func (a observedApplication) Typography() typography.Context { return a.context }

type observedContext struct {
	typography.Context
	calls, maximum          int
	largest                 time.Duration
	paints, colors          int
	metricsTime, rasterTime time.Duration
	target                  string
	targetPaint             time.Time
}

func (c *observedContext) NewTextLayout(text string, f typography.TextFormat, w, h float32) (typography.TextLayout, error) {
	if len(text) > 8192 {
		panic(fmt.Sprintf("unbounded layout input: %d", len(text)))
	}
	c.maximum = max(c.maximum, len(text))
	c.calls++
	at := time.Now()
	l, err := c.Context.NewTextLayout(text, f, w, h)
	c.largest = max(c.largest, time.Since(at))
	if err != nil {
		return nil, err
	}
	return &observedLayout{TextLayout: l, context: c, text: text}, nil
}

type observedLayout struct {
	typography.TextLayout
	context *observedContext
	text    string
}

func (l *observedLayout) Rasterize(scale float32, region image.Rectangle, buf []byte) (typography.TextBitmap, error) {
	at := time.Now()
	bitmap, err := l.TextLayout.Rasterize(scale, region, buf)
	l.context.rasterTime = max(l.context.rasterTime, time.Since(at))
	if err == nil {
		l.context.paints++
		if l.context.target != "" && strings.Contains(l.text, l.context.target) {
			l.context.targetPaint = time.Now()
		}
	}
	return bitmap, err
}
func (l *observedLayout) MeasureMetrics() ([]typography.TextLine, []typography.TextCluster) {
	at := time.Now()
	lines, clusters := l.TextLayout.MeasureMetrics()
	l.context.metricsTime = max(l.context.metricsTime, time.Since(at))
	return lines, clusters
}
func (l *observedLayout) SetTextColor(start, length int, c color.Color) {
	l.context.colors++
	l.TextLayout.SetTextColor(start, length, c)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	runtime.LockOSThread()
	file := flag.String("file", "", "single-line UTF-8 file to read")
	textview := flag.Bool("textview", false, "test GOUI TextView")
	plain := flag.Bool("plain", false, "disable JSON highlighting")
	nowrap := flag.Bool("nowrap", false, "disable line wrapping")
	verifyEnd := flag.Bool("verify-end", false, "verify End/Left/Right caret visibility and resize (SourceView, -nowrap)")
	verifyInput := flag.Bool("verify-input", false, "verify rapid navigation/editing on the original long document")
	clipboard := flag.Bool("clipboard", false, "also verify small native cut/paste selections (disposable desktop)")
	flag.Parse()
	if *verifyInput && (*textview || *verifyEnd) {
		panic("-verify-input requires SourceView and cannot combine with -verify-end")
	}
	if *verifyEnd && (*textview || !*nowrap) {
		panic("-verify-end requires SourceView and -nowrap")
	}
	data, err := os.ReadFile(*file)
	must(err)
	app, err := gui.NewApplication("org.golang-gui.LongParagraph")
	must(err)
	context := &observedContext{Context: app.Typography()}
	gui.App = observedApplication{app, context}
	app.SetStyleSheet(modern.Sheet(modern.Options{}))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1000, Height: 700}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("GOUI Long Paragraph"))
	model := gui.NewTextModel("")
	var widget gui.Widget
	var setSelection func(textedit.Selection)
	if *textview {
		v := gui.NewTextView()
		v.SetModel(model)
		v.SetWrapMode(gui.WrapWordChar)
		if *nowrap {
			v.SetWrapMode(gui.WrapNone)
		}

		widget = v
		setSelection = v.SetSelection
	} else {
		v := source.NewSourceView()
		v.SetDocument(source.NewDocumentWithModel(model))
		v.SetWrapMode(source.WrapWordChar)
		if *nowrap {
			v.SetWrapMode(source.WrapNone)
		}
		if !*plain {
			must(v.Document().SetLanguage(language.JSON))
		}
		if *verifyEnd || *verifyInput {
			v.SetAutomaticCompletion(false)
			must(v.SetWhitespaceMode(source.WhitespaceAll))
			colors := scheme.Light().Definition()
			colors.Caret = color.NRGBA{R: 255, B: 255, A: 255}
			custom, err := scheme.New(colors)
			must(err)
			v.SetScheme(custom)
		}
		widget = v
		setSelection = v.SetSelection
	}
	widget.SetID("editor")
	scroll := gui.NewScrollView()
	scroll.SetStyleName("text-view")
	if !*textview {
		scroll.SetStyleName("source-view")
	}
	scroll.SetChild(widget)
	scroll.SetMainWeight(1)
	var requested int
	var issued time.Time
	toolbar := gui.NewLinearBox(layout.DirectionHorizontal)
	toolbar.SetSpacing(8)
	for _, item := range []struct {
		name string
		part int
	}{{"Start", 0}, {"Middle", 1}, {"End", 2}} {
		button := gui.NewButton()
		button.SetID(item.name)
		button.SetChild(gui.NewLabel(item.name))
		button.ConnectClicked(func() {
			requested = model.Len() * item.part / 2
			for !model.IsRuneBoundary(requested) {
				requested--
			}
			a, b := max(0, requested-24), min(model.Len(), requested+24)
			for !model.IsRuneBoundary(a) {
				a++
			}
			for !model.IsRuneBoundary(b) {
				b--
			}
			context.target, _ = model.Slice(textedit.Range{Start: a, End: b})
			context.targetPaint = time.Time{}
			issued = time.Now()
			setSelection(textedit.Selection{Anchor: requested, Caret: requested})
			win.SetFocusedWidget(widget)
			fmt.Printf("SELECT at=%d handled=%s\n", requested, time.Since(issued))
		})
		toolbar.AddChild(button)
	}
	header := gui.NewHeaderBar()
	header.SetChild(gui.NewLabel("Large single-line text"))
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.SetSpacing(8)
	box.AddChild(header)
	box.AddChild(toolbar)
	box.AddChild(scroll)
	win.SetWidget(box)
	loaded, ready := false, false
	var original string
	load := app.NewTimer()
	defer load.Stop()
	load.ConnectTimeout(func() {
		at := time.Now()
		text := string(data)
		model.SetText(text)
		if *verifyInput {
			original = text
		}
		data = nil
		loaded = true
		win.SetFocusedWidget(widget)
		fmt.Printf("LOAD bytes=%d elapsed=%s\n", model.Len(), time.Since(at))
	})
	must(load.StartOnce(100 * time.Millisecond))
	last := time.Now()
	largest := time.Duration(0)
	lastRevision := uint64(0)
	lastSelection := textedit.Selection{}
	lastRect := geometry.Rectangle{}
	lastVisible := textedit.Range{}
	lastScroll := geometry.Point{}
	verified := false
	var verify func()
	if *verifyEnd {
		verify = endVerification(win, widget, scroll, model, &verified)
	} else if *verifyInput {
		verify = inputVerification(win, widget.(*source.SourceView), model, func() string { return original }, *clipboard, *nowrap, &verified)
	}
	heartbeat := app.NewTimer()
	defer heartbeat.Stop()
	heartbeat.ConnectTimeout(func() {
		now := time.Now()
		gap := now.Sub(last)
		last = now
		if gap > largest {
			largest = gap
			if loaded {
				fmt.Printf("HEARTBEAT max=%s nativeCalls=%d maxInput=%d\n", largest, context.calls, context.maximum)
			}
		}
		if !loaded {
			return
		}
		pos := geometry.Point{X: scroll.ScrollX(), Y: scroll.ScrollY()}
		if pos != lastScroll {
			fmt.Printf("SCROLL value=%v\n", pos)
			lastScroll = pos
		}
		info := widget.Snapshot().TextEditing
		if info == nil {
			return
		}
		if info.VisibleRange != lastVisible {
			fmt.Printf("VISIBLE range=%v\n", info.VisibleRange)
			lastVisible = info.VisibleRange
		}
		if rect := widget.Rect(); rect != lastRect {
			fmt.Printf("SIZE rect=%v\n", rect)
			lastRect = rect
		}
		if info.Selection != lastSelection {
			fmt.Printf("SELECTION value=%v\n", info.Selection)
			lastSelection = info.Selection
		}
		if info != nil && info.VisibleRange.End > info.VisibleRange.Start {
			if !ready && context.maximum >= 1024 {
				ready = true
				largest = 0
				fmt.Printf("READY textview=%v native=%s maxInput=%d\n", *textview, context.Name(), context.maximum)
			}
			if !issued.IsZero() && !context.targetPaint.IsZero() && ((requested >= info.VisibleRange.Start && requested < info.VisibleRange.End) || (requested == model.Len() && info.VisibleRange.End == model.Len())) {
				fmt.Printf("TEXT_READY at=%d elapsed=%s range=%v\n", requested, time.Since(issued), info.VisibleRange)
				issued = time.Time{}
			}
		}
		if verify != nil && ready {
			verify()
		}
		if model.Revision() != lastRevision {
			lastRevision = model.Revision()
			fmt.Printf("EDIT revision=%d bytes=%d selection=%v\n", lastRevision, model.Len(), info.Selection)
		}
	})
	must(heartbeat.Start(16 * time.Millisecond))
	must(win.Show())
	app.Run()
	if (*verifyEnd || *verifyInput) && !verified {
		panic("End verification closed before completion")
	}
	fmt.Printf("CLOSED maxGap=%s nativeMaxInput=%d nativeCalls=%d largestCreate=%s paints=%d colors=%d largestMetrics=%s largestRaster=%s\n", largest, context.maximum, context.calls, context.largest, context.paints, context.colors, context.metricsTime, context.rasterTime)
}

// 验证通过正常 Window 事件分发执行，不直接调用导航或改变模型。
func endVerification(win gui.Window, widget gui.Widget, scroll *gui.ScrollView, model *textedit.Model, complete *bool) func() {
	keys := []events.Key{events.KeyEnd, events.KeyArrowLeft, events.KeyArrowLeft, events.KeyArrowLeft, events.KeyArrowRight, events.KeyArrowRight, events.KeyArrowRight}
	states := []gui.WindowState{gui.WindowStateNormal, gui.WindowStateMaximized, gui.WindowStateNormal}
	var previousSize geometry.Size
	step, allocation := -1, 0
	expected := 0
	deadline := time.Now().Add(30 * time.Second)
	issue := func() {
		if step == 0 {
			expected = model.Len()
		} else if step <= 3 {
			expected--
		} else {
			expected++
		}
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: keys[step]}))
		}
		actual := widget.Snapshot().TextEditing
		if actual == nil {
			panic("missing editing snapshot")
		}
		if actual.Selection.Anchor != expected || actual.Selection.Caret != expected {
			panic(fmt.Sprintf("navigation changed byte position: step=%d got=%v want=%d", step, actual.Selection, expected))
		}
		deadline = time.Now().Add(20 * time.Second)
	}
	return func() {
		if !time.Now().Before(deadline) {
			panic(fmt.Sprintf("caret not visible after End/arrow: allocation=%d step=%d selection=%d scroll=%g viewport=%v", allocation, step, expected, scroll.ScrollX(), widget.Rect()))
		}
		if step < 0 {
			if allocation > 0 && widget.Rect().Size == previousSize {
				return
			}
			step = 0
			issue()
			return
		}
		image, err := gui.RenderWidget(widget, 0)
		must(err)
		found := false
		maxInkX := -1
		scale := float64(image.Bounds().Dx()) / float64(widget.Rect().Width)
		// 只检索正文光标颜色，单个部分覆盖像素也要与明亮/深色语法颜色分开。
		for y := 0; y < min(image.Bounds().Dy(), int(64*scale)); y++ {
			for x := 0; x < image.Bounds().Dx(); x++ {
				r, g, b, a := image.At(x, y).RGBA()
				if a == 65535 && r > 64000 && b > 64000 && g < 58000 {
					found = true
					maxInkX = max(maxInkX, x)
				}
			}
		}
		if !found || float64(maxInkX) >= float64(image.Bounds().Dx())-2*scale {
			return
		}
		fmt.Printf("CARET_VISIBLE allocation=%d step=%d selection=%d scroll=%g viewport=%v\n", allocation, step, expected, scroll.ScrollX(), widget.Rect().Size)
		step++
		if step < len(keys) {
			issue()
			return
		}
		allocation++
		if allocation == len(states) {
			*complete = true
			fmt.Println("PASS End/Left/Right caret pixels and resize")
			win.Destroy()
			return
		}
		previousSize = widget.Rect().Size
		win.RequestState(states[allocation])
		deadline = time.Now().Add(10 * time.Second)
		step = -1
	}
}

// 连续输入在同一事件循环回调中发出，不能靠等待排版掩盖旧片段导航。
func inputVerification(win gui.Window, v *source.SourceView, model *textedit.Model, original func() string, clipboard, nowrap bool, complete *bool) func() {
	primary, _ := gui.ModPrimary.Resolve()
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	stage := -1
	n := 0
	tail := ""
	deadline := time.Now().Add(45 * time.Second)
	matches := func(length int, suffix string) bool {
		if model.Len() != length {
			return false
		}
		text, err := model.Slice(textedit.Range{Start: length - len(suffix), End: length})
		must(err)
		return text == suffix
	}
	return func() {
		if !time.Now().Before(deadline) {
			panic(fmt.Sprintf("input verification timeout: stage=%d bytes=%d selection=%v", stage, model.Len(), v.Selection()))
		}
		switch stage {
		case -1:
			n = model.Len()
			source := original()
			if n < 3 || len(source) != n || source[n-1] >= 128 || source[n-2] >= 128 || source[n-3] >= 128 {
				panic("verification fixture must be normalized and end in three ASCII bytes")
			}
			tail = source[n-1:]
			endMods := primary
			if nowrap {
				endMods = 0
			}
			key(events.KeyEnd, endMods)
			key(events.KeyArrowLeft, 0)
			must(win.CommitText("Z"))
			if model.Len() != n || v.Selection().Caret != n {
				panic(fmt.Sprintf("cold input used an unrelated window: bytes=%d selection=%v", model.Len(), v.Selection()))
			}
			stage = 0
		case 0:
			if !matches(n+1, "Z"+tail) || v.Selection().Caret != n {
				return
			}
			key(events.KeyArrowLeft, events.ModifierShift)
			must(win.CommitText("Q"))
			stage = 1
		case 1:
			if !matches(n+1, "Q"+tail) {
				return
			}
			key(events.KeyZ, primary)
			stage = 2
		case 2:
			if !matches(n+1, "Z"+tail) {
				return
			}
			if v.Selection().Range() != (textedit.Range{Start: n - 1, End: n}) {
				panic("replacement undo lost its selection")
			}
			key(events.KeyZ, primary|events.ModifierShift)
			stage = 3
		case 3:
			if !matches(n+1, "Q"+tail) {
				return
			}
			key(events.KeyBackspace, 0)
			stage = 4
		case 4:
			if !matches(n, tail) {
				return
			}
			key(events.KeyArrowRight, 0)
			key(events.KeyArrowLeft, 0)
			key(events.KeyDelete, 0)
			stage = 5
		case 5:
			if !matches(n-1, original()[n-2:n-1]) {
				return
			}
			key(events.KeyZ, primary)
			stage = 6
		case 6:
			if !matches(n, tail) {
				return
			}
			if !model.EqualText(original()) {
				panic("editing/undo changed unrelated original bytes")
			}
			if clipboard {
				key(events.KeyArrowRight, events.ModifierShift)
				key(events.KeyX, primary)
				stage = 7
			} else {
				stage = 9
			}
		case 7:
			if !matches(n-1, original()[n-2:n-1]) {
				return
			}
			key(events.KeyV, primary)
			stage = 8
		case 8:
			if !matches(n, tail) {
				return
			}
			stage = 9
		case 9:
			key(events.KeyEnd, primary)
			key(events.KeyEnter, 0)
			must(win.CommitText("Z"))
			stage = 10
		case 10:
			if !matches(n+2, "\nZ") {
				return
			}
			key(events.KeyZ, primary)
			key(events.KeyZ, primary)
			stage = 11
		case 11:
			if !matches(n, tail) {
				return
			}
			v.SetReadOnly(true)
			if win.CommitText("bad") == nil {
				panic("readonly accepted text")
			}
			key(events.KeyBackspace, 0)
			key(events.KeyDelete, 0)
			key(events.KeyZ, primary)
			if !model.EqualText(original()) {
				panic("readonly or history corrupted original content")
			}
			v.SetReadOnly(false)
			*complete = true
			fmt.Println("PASS cold End/Left/typing, replacement, deletion, newline, readonly, clipboard and undo/redo; original bytes restored")
			win.Destroy()
		}
	}
}
