// 超大单行文本交互验证，需要独立桌面、原生排版和字体，不使用 DevServer。
// 启动：go run ./tests/window/editor/paragraph -file /data/xuyang8/temp/test.json
// 加 -textview 验证 GOUI TextView；默认验证 SourceView。可选 -plain 关闭 JSON 高亮。
// -nowrap 验证水平长行：拖水平滚动条至中部，逐字符点击、左右键与连续拖选，
// 跨 4 KiB 片段时不得跳字、断裂选区或停住；Home/End 到逻辑行边界。
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
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
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
	flag.Parse()
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
	load := app.NewTimer()
	defer load.Stop()
	load.ConnectTimeout(func() {
		at := time.Now()
		model.SetText(string(data))
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
		if model.Revision() != lastRevision {
			lastRevision = model.Revision()
			fmt.Printf("EDIT revision=%d bytes=%d selection=%v\n", lastRevision, model.Len(), info.Selection)
		}
	})
	must(heartbeat.Start(16 * time.Millisecond))
	must(win.Show())
	app.Run()
	fmt.Printf("CLOSED maxGap=%s nativeMaxInput=%d nativeCalls=%d largestCreate=%s paints=%d colors=%d largestMetrics=%s largestRaster=%s\n", largest, context.maximum, context.calls, context.largest, context.paints, context.colors, context.metricsTime, context.rasterTime)
}
