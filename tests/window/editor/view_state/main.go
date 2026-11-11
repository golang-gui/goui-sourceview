// 视图状态的独立原生窗口验收。环境：桌面/窗口管理器、原生排版和字体；
// Linux 可用 XMODIFIERS=@im=none go run ./tests/window/editor/view_state。
// -basic/-dark 选择主题，另用 GOUI_PLAT_SCALE=1/1.5 验证。初态为两个共享
// Document 的独立视图；通过正常 Window.DispatchEvent 点击保存/恢复按钮，
// 检查反向选区、屏外光标、滚动锚点、结构/长行折叠及只读视图隔离；随后卸载
// 重建目标视图、调整原生窗口宽高与字体，并恢复约 10 MiB 超长单行中段（包含 RTL
// 文本）。检查实际首屏到达时间、事件循环间隔、输入取消和过期拒绝。滚轮、
// 滚动条拖动及 CommitText 走正常事件路径。正文历史只在最后的明确输入时变化。
// 断言实际语义和滚动位置；时间用于观测而非跨机器性能门槛。不访问用户文件、
// 剪贴板或系统设置。成功自动关闭，超时/提前退出/断言失败非零退出；重启复位。
// 原生 IME、物理输入及 macOS/Windows 需单独验证；本程序不代表这些平台通过。
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"slices"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview/sourceview"
	sourceui "github.com/golang-gui/goui-sourceview/ui"
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
func closeEnough(a, b float32) bool { return math.Abs(float64(a-b)) <= 1 }
func ready(v *source.SourceView) bool {
	m := v.Snapshot().Attributes["goui.sourceview"].(map[string]any)
	return !m["layoutPending"].(bool) && !m["foldingPending"].(bool)
}
func collapsed(v *source.SourceView, line int) bool {
	for _, f := range v.FoldingRegions() {
		if f.StartLine == line {
			return f.Collapsed
		}
	}
	return false
}

func main() {
	runtime.LockOSThread()
	basicTheme := flag.Bool("basic", false, "Basic theme")
	dark := flag.Bool("dark", false, "Modern dark theme")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewState")
	must(err)
	fontSize := float32(12)
	setStyle := func() {
		rules := modern.Rules(modern.Options{Dark: *dark})
		if *basicTheme {
			rules = basic.Rules()
		}
		rules = append(rules, style.Name("source-view").FontFamily("monospace").FontSize(fontSize))
		app.SetStyleSheet(style.Sheet(rules...))
	}
	setStyle()
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1000, Height: 800}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView state verification"))
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	label := gui.NewLabel("Save selection, viewport and folds; restore into an independent view")
	box.AddChild(label)
	tools := gui.NewLinearBox(layout.DirectionHorizontal)
	box.AddChild(tools)
	save, restore := gui.NewButton(), gui.NewButton()
	save.SetChild(gui.NewLabel("Save upper view"))
	restore.SetChild(gui.NewLabel("Restore lower view"))
	tools.AddChild(save)
	tools.AddChild(restore)
	newView := func() *source.SourceView {
		v := source.NewSourceView()
		v.SetAutomaticCompletion(false)
		v.SetWrapMode(source.WrapWordChar)
		v.ConnectError(func(err error) { panic(err) })
		return v
	}
	left, right := newView(), newView()
	upper, lower := gui.NewScrollView(), gui.NewScrollView()
	for _, host := range []*gui.ScrollView{upper, lower} {
		host.SetStyleName("source-view")
		host.SetMainWeight(1)
		box.AddChild(host)
	}
	upper.SetChild(left)
	lower.SetChild(right)
	var text strings.Builder
	for i := range 3000 {
		if i == 20 {
			text.WriteString(strings.Repeat("long header words ", 1000))
		} else {
			fmt.Fprintf(&text, "line %04d: source text", i)
		}
		text.WriteByte('\n')
	}
	doc := source.NewDocument(text.String())
	left.SetDocument(doc)
	right.SetDocument(doc)
	var saved *sourceui.ViewState
	frames, restoreFrame := 0, 0
	win.ConnectFrame(func(time.Time) { frames++ })
	var restoreStart, lastBeat time.Time
	var maxGap time.Duration
	save.ConnectClicked(func() { saved, err = left.SaveState(); must(err) })
	restore.ConnectClicked(func() {
		restoreFrame = frames
		restoreStart, lastBeat, maxGap = time.Now(), time.Now(), 0
		focused := win.FocusedWidget()
		must(right.RestoreState(saved))
		check(win.FocusedWidget() == focused, "restore acquired focus")
	})
	win.SetWidget(box)
	must(win.Show())
	click := func(w gui.Widget) {
		p := w.Snapshot().Bounds.Center()
		for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
			must(win.DispatchEvent(events.PointerEvent{EventType: kind, Position: p, Button: events.PointerButtonLeft}))
		}
	}
	wheel := func(w gui.Widget, delta float32) {
		must(win.DispatchEvent(events.WheelEvent{Position: w.Snapshot().Bounds.Center(), DeltaY: delta, Mode: events.WheelDeltaPixel}))
	}
	type step struct {
		name string
		run  func() bool
	}
	var steps []step
	add := func(name string, f func()) { steps = append(steps, step{name, func() bool { f(); return true }}) }
	wait := func(name string, f func() bool) { steps = append(steps, step{name, f}) }
	var expected source.TextSelection
	var top int
	var y float32
	var oldSize geometry.Size
	var beforeRevision uint64
	var visibleAt time.Time
	add("prepare folds", func() {
		f, err := source.NewFolds(doc.Text(), []source.FoldRange{{StartLine: 10, EndLine: 15}, {StartLine: 1000, EndLine: 1005}})
		must(err)
		_, err = doc.SetFolds(doc.Revision(), f)
		must(err)
		win.SetFocusedWidget(left)
	})
	wait("candidates", func() bool { return len(left.FoldingRegions()) == 2 && ready(left) && ready(right) })
	add("measure manual long line", func() {
		r, err := doc.LineRange(20)
		must(err)
		left.SetSelection(source.TextSelection{Anchor: r.Start, Caret: r.Start})
		left.RevealSelection()
	})
	wait("long line geometry", func() bool {
		return ready(left) && strings.Contains(left.Snapshot().TextEditing.VisibleText, "long header")
	})
	add("save folds and viewport", func() {
		check(left.CollapseLine(20), "manual collapse setup")
		check(left.Fold(10) && left.Fold(1000), "structure collapse setup")
		expected = source.TextSelection{Anchor: 6, Caret: 1}
		left.SetSelection(expected)
	})
	wait("selection revealed", func() bool { return ready(left) && left.Snapshot().TextEditing.VisibleRange.Start == 0 })
	add("scroll away from caret", func() { upper.SetScrollY(5007) })
	wait("source viewport", func() bool { return ready(left) && upper.ScrollY() > 4000 })
	add("save and restore by pointer", func() {
		top, y = left.Snapshot().TextEditing.VisibleRange.Start, upper.ScrollY()
		beforeRevision = doc.Revision()
		click(save)
		right.SetReadOnly(true)
		click(restore)
	})
	wait("restored viewport and folds", func() bool {
		if frames < restoreFrame+2 || !ready(right) || right.Snapshot().TextEditing.VisibleRange.Start != top || !closeEnough(lower.ScrollY(), y) {
			return false
		}
		check(right.Selection() == expected && collapsed(right, 10) && collapsed(right, 1000) && slices.Equal(right.CollapsedLines(), []int{20}), "selection/folds roundtrip")
		check(doc.Revision() == beforeRevision && !doc.CanUndo(), "restore changed history")
		check(left.Unfold(10) && collapsed(right, 10), "fold isolation")
		log.Printf("PASS initial viewport selection/folds/read-only; visible after %s, largest timer gap %s", time.Since(restoreStart), maxGap)
		return true
	})
	add("unmount and rebuild", func() {
		lower.SetChild(nil)
		right = newView()
		right.SetDocument(doc)
		lower.SetChild(right)
		click(restore)
	})
	wait("rebuilt viewport", func() bool {
		return ready(right) && right.Snapshot().TextEditing.VisibleRange.Start == top && right.Selection() == expected && collapsed(right, 10)
	})
	add("resize and font", func() {
		oldSize = right.Rect().Size
		win.RequestState(gui.WindowStateMaximized)
		fontSize = 16
		setStyle()
		click(restore)
	})
	wait("resized anchor", func() bool {
		return win.State() == gui.WindowStateMaximized && ready(right) && right.Rect().Size != oldSize && right.Snapshot().TextEditing.VisibleRange.Start == top
	})
	add("restore window size", func() { win.RequestState(gui.WindowStateNormal) })
	wait("window size settled", func() bool { return win.State() == gui.WindowStateNormal && ready(left) && ready(right) })
	add("large wrapped single line", func() {
		fontSize = 12
		setStyle()
		doc = source.NewDocument(strings.Repeat("word 0123456789 中文 אבג ", 350000))
		check(doc.Len() >= 10<<20, "large fixture too small")
		left.SetDocument(doc)
		right.SetDocument(doc)
		at := doc.Len() / 2
		for !doc.IsRuneBoundary(at) {
			at++
		}
		left.SetSelection(source.TextSelection{Anchor: at, Caret: at})
		left.RevealSelection()
	})
	wait("large source middle", func() bool {
		return ready(left) && left.Snapshot().TextEditing.VisibleRange.Start > doc.Len()/3
	})
	add("large source scroll", func() { wheel(left, 83) })
	wait("large source stable", func() bool { return ready(left) })
	add("large snapshot", func() {
		top = left.Snapshot().TextEditing.VisibleRange.Start
		expected = left.Selection()
		click(save)
		click(restore)
	})
	wait("large actual visible viewport", func() bool {
		if frames < restoreFrame+2 || !ready(right) || right.Snapshot().TextEditing.VisibleRange.Start != top {
			return false
		}
		check(right.Selection() == expected && doc.Revision() == 0 && !doc.CanUndo(), "large snapshot selection/history")
		log.Printf("PASS large %d bytes midpoint viewport; visible after %s, largest timer gap %s; upper=%d lower=%d", doc.Len(), time.Since(restoreStart), maxGap, top, right.Snapshot().TextEditing.VisibleRange.Start)
		visibleAt = time.Now()
		return true
	})
	wait("observe restored desktop", func() bool { return time.Since(visibleAt) > 240*time.Millisecond })
	add("wheel cancels pending restore", func() {
		lower.SetScrollY(0)
		click(restore)
		wheel(right, 120)
		y = lower.ScrollY()
	})
	wait("wheel remains authoritative", func() bool {
		if !ready(right) {
			return false
		}
		check(closeEnough(lower.ScrollY(), y) && right.Snapshot().TextEditing.VisibleRange.Start < 10000, "late restore overrode wheel")
		return true
	})
	add("scrollbar cancels restore", func() {
		click(restore)
		var bar gui.Widget
		var walk func(gui.Widget)
		walk = func(w gui.Widget) {
			if w.Snapshot().Role == gui.RoleScrollBar && w.Rect().Height > w.Rect().Width {
				bar = w
			}
			for _, c := range w.Children() {
				walk(c)
			}
		}
		walk(lower)
		check(bar != nil, "vertical scrollbar absent")
		p := bar.Snapshot().Bounds
		before := lower.ScrollY()
		from := geometry.Point{X: p.Center().X, Y: p.Y + 5}
		must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerDown, Position: from, Button: events.PointerButtonLeft}))
		from.Y += 30
		must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerMove, Position: from, Buttons: events.PointerButtonLeftDown}))
		must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerUp, Position: from, Button: events.PointerButtonLeft}))
		y = lower.ScrollY()
		check(y != before, "scrollbar drag did not move the viewport")
	})
	wait("scrollbar remains authoritative", func() bool {
		if !ready(right) {
			return false
		}
		check(closeEnough(lower.ScrollY(), y), "late restore overrode scrollbar")
		return true
	})
	add("input cancels and invalidates state", func() {
		click(restore)
		win.SetFocusedWidget(right)
		must(win.CommitText("x"))
		check(errors.Is(left.RestoreState(saved), source.ErrOperationChanged), "stale state accepted after shared edit")
	})
	wait("input completed", func() bool { return ready(right) && doc.Revision() == 1 })
	add("final assertions", func() {
		check(doc.CanUndo(), "normal CommitText did not edit")
		log.Printf("PASS view state; OS=%s typography=%s scale=%s basic=%t dark=%t", runtime.GOOS, app.Typography().Name(), os.Getenv("GOUI_PLAT_SCALE"), *basicTheme, *dark)
	})
	index, complete, stable := 0, false, 0
	deadline := time.Now().Add(20 * time.Second)
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		now := time.Now()
		if !lastBeat.IsZero() {
			maxGap = max(maxGap, now.Sub(lastBeat))
			lastBeat = now
		}
		if now.After(deadline) {
			panic(fmt.Sprintf("step %d %s timeout; upper=%v lower=%v y=%v/%v bounds=%v/%v metadata=%v/%v", index, steps[index].name, left.Snapshot().TextEditing.VisibleRange, right.Snapshot().TextEditing.VisibleRange, upper.ScrollY(), lower.ScrollY(), left.Rect(), right.Rect(), left.Snapshot().Attributes, right.Snapshot().Attributes))
		}
		if index == len(steps) {
			complete = true
			win.Destroy()
			return
		}
		if steps[index].run() {
			index++
			stable = 0
			deadline = now.Add(20 * time.Second)
		} else {
			stable++
			if stable == 1 {
				log.Print("WAIT ", steps[index].name)
			}
		}
	})
	must(timer.Start(40 * time.Millisecond))
	app.Run()
	check(complete, "closed before verification completed")
}
