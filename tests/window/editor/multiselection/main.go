// 多选区独立窗口验收，需要桌面、原生排版和字体。启动：
// XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software go run ./tests/window/editor/multiselection
// -basic/-dark 切换外观；GOUI_PLAT_SCALE=1/1.5。初态为共享文档的两个独立视图。
// setter 仅准备场景；中文输入、删除、导航、剪切/粘贴、撤销重做经 Window 的正常事件路径。
// 断言完整正文、主/附加选区、视图隔离、历史、快照恢复和超过缓存页数的分散输入。
// -clipboard 额外覆盖该显示的临时剪贴板，只在独立桌面启用。默认不访问剪贴板。
// 每步有明确条件与超时，成功自动关闭；提前退出或失败返回非零。不写文件或系统设置。
// 真实 IME 服务、物理键及 Windows/macOS 必须另验；重启恢复初态，关闭窗口退出。
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"slices"
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
func main() {
	runtime.LockOSThread()
	basicTheme := flag.Bool("basic", false, "Basic theme")
	dark := flag.Bool("dark", false, "Modern dark")
	clipboard := flag.Bool("clipboard", false, "use disposable native clipboard")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewMultiple")
	must(err)
	rules := modern.Rules(modern.Options{Dark: *dark})
	if *basicTheme {
		rules = basic.Rules()
	}
	rules = append(rules, style.Name("source-view").FontFamily("monospace").FontSize(12))
	app.SetStyleSheet(style.Sheet(rules...))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1000, Height: 800}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView multiple selections"))
	d := source.NewDocument("one two\nthree four\nfive six")
	a, b := source.NewSourceView(), source.NewSourceView()
	a.SetDocument(d)
	b.SetDocument(d)
	for _, v := range []*source.SourceView{a, b} {
		v.SetAutomaticCompletion(false)
		v.SetAcceptsTab(true)
		v.ConnectError(func(err error) { panic(err) })
	}
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	for _, v := range []*source.SourceView{a, b} {
		scroll := gui.NewScrollView()
		scroll.SetStyleName("source-view")
		scroll.SetChild(v)
		scroll.SetMainWeight(1)
		box.AddChild(scroll)
	}
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(a)
	primary, _ := gui.ModPrimary.Resolve()
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	set := func(v *source.SourceView, s []source.TextSelection) { must(v.SetSelections(s)) }
	pending := func(v *source.SourceView) bool {
		m := v.Snapshot().Attributes["goui.sourceview"].(map[string]any)
		return m["inputPending"] == true
	}
	ref, err := app.Typography().NewTextLayout("abcdef", typography.TextFormat{Font: typography.FontInfo{Family: "monospace", Size: 12}}, 1000, 1000)
	must(err)
	lines, clusters := ref.MeasureMetrics()
	advance, lineHeight := clusters[0].Width, lines[0].Height
	ref.Destroy()
	pointer := func(kind events.EventType, column, row int, mods events.Modifiers) {
		bounds := a.Snapshot().Bounds
		event := events.PointerEvent{EventType: kind, Position: geometry.Point{X: bounds.X + 6 + float32(column)*advance, Y: bounds.Y + 6 + (float32(row)+.5)*lineHeight}, Modifiers: mods}
		if kind == events.PointerMove {
			event.Buttons = events.PointerButtonLeftDown
		} else {
			event.Button = events.PointerButtonLeft
		}
		must(win.DispatchEvent(event))
	}
	original := d.Text()
	before := []source.TextSelection{{Anchor: 13, Caret: 8}, {Anchor: 0, Caret: 3}}
	after := []source.TextSelection{{Anchor: 11, Caret: 11}, {Anchor: 3, Caret: 3}}
	var saved *source.ViewState
	var longOriginal, longWant string
	var longSet []source.TextSelection
	type step struct {
		name  string
		start func()
		ready func() bool
	}
	steps := []step{
		{"input", func() {
			set(a, before)
			set(b, []source.TextSelection{{Anchor: 18, Caret: 18}, {Anchor: 22, Caret: 22}})
			must(win.CommitText("中"))
		}, func() bool { return d.Text() == "中 two\n中 four\nfive six" && !pending(a) }},
		{"undo", func() {
			check(slices.Equal(a.Selections(), after), "input selections")
			check(len(b.Selections()) == 2, "other view lost set")
			key(events.KeyZ, primary)
		}, func() bool { return d.Text() == original && slices.Equal(a.Selections(), before) }},
		{"redo", func() { key(events.KeyZ, primary|events.ModifierShift) }, func() bool { return d.Text() == "中 two\n中 four\nfive six" && slices.Equal(a.Selections(), after) }},
		{"save", func() { saved, err = a.SaveState(); must(err); a.SetSelection(source.TextSelection{}) }, func() bool { return len(a.Selections()) == 1 }},
		{"restore", func() { must(a.RestoreState(saved)) }, func() bool { return slices.Equal(a.Selections(), after) }},
		{"delete", func() { key(events.KeyBackspace, 0) }, func() bool { return d.Text() == " two\n four\nfive six" && !pending(a) }},
		{"restore text", func() { key(events.KeyZ, primary) }, func() bool { return d.Text() == "中 two\n中 four\nfive six" && slices.Equal(a.Selections(), after) }},
		{"navigation", func() { key(events.KeyArrowRight, 0) }, func() bool {
			return slices.Equal(a.Selections(), []source.TextSelection{{Anchor: 12, Caret: 12}, {Anchor: 4, Caret: 4}}) && !pending(a)
		}},
	}
	if *clipboard {
		steps = append(steps,
			step{"copy cut", func() {
				d.SetText("ab\ncd\nef")
				set(a, []source.TextSelection{{Anchor: 6, Caret: 7}, {Anchor: 0, Caret: 1}, {Anchor: 3, Caret: 4}})
				key(events.KeyC, primary)
				key(events.KeyX, primary)
			}, func() bool { return d.Text() == "b\nd\nf" && !pending(a) }},
			step{"distributed paste", func() { key(events.KeyV, primary) }, func() bool { return d.Text() == "ab\ncd\nef" && !pending(a) }},
		)
	}
	steps = append(steps,
		step{"prepare rectangle", func() {
			a.SetWrapMode(source.WrapNone)
			a.SetFoldingEnabled(false)
			a.SetShowLineNumbers(false)
			d.SetText("abcdef\nabcdef\nabcdef")
			a.SetSelection(source.TextSelection{})
		}, func() bool { return !pending(a) }},
		step{"pointer rectangle", func() {
			mods, _ := a.SelectionModifiers().Column.Resolve()
			pointer(events.PointerDown, 1, 0, mods)
			pointer(events.PointerMove, 3, 2, mods)
			pointer(events.PointerUp, 3, 2, mods)
		}, func() bool {
			return slices.Equal(a.Selections(), []source.TextSelection{{Anchor: 15, Caret: 17}, {Anchor: 1, Caret: 3}, {Anchor: 8, Caret: 10}})
		}},
		step{"rectangle input", func() { must(win.CommitText("Z")) }, func() bool { return d.Text() == "aZdef\naZdef\naZdef" && !pending(a) }},
		step{"add pointer", func() {
			a.SetSelection(source.TextSelection{})
			mods, _ := a.SelectionModifiers().Add.Resolve()
			pointer(events.PointerDown, 2, 1, mods)
			pointer(events.PointerUp, 2, 1, mods)
		}, func() bool { return slices.Equal(a.Selections(), []source.TextSelection{{Anchor: 8, Caret: 8}, {}}) }},
		step{"keyboard column", func() {
			a.SetSelection(source.TextSelection{Anchor: 1, Caret: 1})
			key(events.KeyArrowRight, primary|events.ModifierAlt|events.ModifierShift)
			key(events.KeyArrowDown, events.ModifierControl|events.ModifierShift)
		}, func() bool {
			return slices.Equal(a.Selections(), []source.TextSelection{{Anchor: 7, Caret: 8}, {Anchor: 1, Caret: 2}}) && !pending(a)
		}},
		step{"occurrence", func() {
			a.SetSelection(source.TextSelection{Anchor: 1, Caret: 2})
			key(events.KeyL, primary|events.ModifierShift)
		}, func() bool { return len(a.Selections()) == 3 && !pending(a) }},
		step{"add cursor", func() {
			a.SetSelection(source.TextSelection{Anchor: 1, Caret: 1})
			key(events.KeyArrowDown, primary|events.ModifierAlt)
		}, func() bool {
			return slices.Equal(a.Selections(), []source.TextSelection{{Anchor: 7, Caret: 7}, {Anchor: 1, Caret: 1}}) && !pending(a)
		}},
		step{"dispersed long input", func() {
			a.SetWrapMode(source.WrapNone)
			a.SetFoldingEnabled(false)
			longOriginal = strings.Repeat("ab ", 100000)
			d.SetText(longOriginal)
			longSet = make([]source.TextSelection, 24)
			for i := range longSet {
				at := 100 + i*8192
				longSet[i] = source.TextSelection{Anchor: at, Caret: at}
			}
			set(a, longSet)
			longWant = longOriginal
			for i := len(longSet) - 1; i >= 0; i-- {
				at := longSet[i].Caret
				longWant = longWant[:at] + "X" + longWant[at:]
			}
			must(win.CommitText("X"))
		}, func() bool { return d.Text() == longWant && !pending(a) }},
		step{"dispersed undo", func() { key(events.KeyZ, primary) }, func() bool { return d.Text() == longOriginal && slices.Equal(a.Selections(), longSet) }},
		step{"cancel", func() { must(win.CommitText("Y")); a.SetSelection(source.TextSelection{}); key(events.KeyEscape, 0) }, func() bool { return d.Text() == longOriginal && !pending(a) && len(a.Selections()) == 1 }},
	)
	timer := app.NewTimer()
	defer timer.Stop()
	stage := -1
	started := time.Now()
	complete := false
	timer.ConnectTimeout(func() {
		if stage < 0 {
			stage = 0
			started = time.Now()
			steps[stage].start()
			return
		}
		if time.Since(started) > 25*time.Second {
			panic(fmt.Sprintf("timeout %s: bytes=%d selections=%v pending=%t", steps[stage].name, d.Len(), a.Selections(), pending(a)))
		}
		if !steps[stage].ready() {
			return
		}
		log.Printf("PASS %s", steps[stage].name)
		stage++
		if stage == len(steps) {
			complete = true
			log.Printf("PASS multiselection OS=%s typography=%s scale=%s basic=%t dark=%t clipboard=%t", runtime.GOOS, app.Typography().Name(), os.Getenv("GOUI_PLAT_SCALE"), *basicTheme, *dark, *clipboard)
			win.Destroy()
			return
		}
		started = time.Now()
		steps[stage].start()
	})
	must(timer.Start(40 * time.Millisecond))
	app.Run()
	check(complete, "closed before completing verification")
}
