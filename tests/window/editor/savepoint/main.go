// 保存点窗口验收：需要桌面、原生排版与字体；不使用真实剪贴板或磁盘文件。
// 启动：XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software go run ./tests/window/editor/savepoint
// -basic/-dark 切换主题，GOUI_PLAT_SCALE=1/1.5；初态为共享 CRLF 文档的两个视图。
// 自动经正常 CommitText/Window.DispatchEvent 输入、撤销、重做；setter 仅准备场景。
// 断言初始 clean、保存边界、延迟旧保存完成、行尾往返、多光标原子编辑及失败保存不改基线。
// 底部显示当前 Modified；每步有完整正文/状态断言和超时，成功自动退出，提前关闭返回非零。
// 写入仅使用 io.Discard 与确定性失败 writer，不修改用户文件、系统设置或剪贴板。
// 重启复位；关闭窗口退出。真实 IME、物理键盘及 Windows/macOS 原生交互需另验。
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"time"

	source "github.com/golang-gui/goui-sourceview/sourceview"
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

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func main() {
	runtime.LockOSThread()
	basicTheme := flag.Bool("basic", false, "Basic theme")
	dark := flag.Bool("dark", false, "Modern dark")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewSavePoint")
	must(err)
	rules := modern.Rules(modern.Options{Dark: *dark})
	if *basicTheme {
		rules = basic.Rules()
	}
	app.SetStyleSheet(style.Sheet(rules...))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 900, Height: 600}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView save points"))
	d := source.NewDocument("ab\r\ncd")
	a, b := source.NewSourceView(), source.NewSourceView()
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	for _, v := range []*source.SourceView{a, b} {
		v.SetDocument(d)
		v.SetAutomaticCompletion(false)
		v.ConnectError(func(err error) { panic(err) })
		scroll := gui.NewScrollView()
		scroll.SetStyleName("source-view")
		scroll.SetChild(v)
		scroll.SetMainWeight(1)
		box.AddChild(scroll)
	}
	label := gui.NewLabel("Saved")
	box.AddChild(label)
	changes := 0
	h := d.ConnectModifiedChanged(func() {
		changes++
		label.SetText(fmt.Sprintf("Modified=%t", d.Modified()))
	})
	defer h.Disconnect()
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(a)
	primary, _ := gui.ModPrimary.Resolve()
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	pending := func() bool {
		return a.Snapshot().Attributes["goui.sourceview"].(map[string]any)["inputPending"] == true
	}
	save := func(w io.Writer) error {
		point := d.CaptureSavePoint()
		if _, err := d.WriteTo(w); err != nil {
			return err
		}
		return d.MarkSaved(point)
	}
	var old source.SavePoint
	type scene struct {
		name  string
		start func()
		ready func() bool
	}
	scenes := []scene{
		{"initial CRLF clean", func() { a.SetSelection(source.TextSelection{Anchor: 1, Caret: 1}) }, func() bool { return !d.Modified() && d.LineEnding() == source.LineEndingCRLF && changes == 0 }},
		{"type before save", func() { must(win.CommitText("x")) }, func() bool { return d.Text() == "axb\ncd" && d.Modified() && !pending() }},
		{"save baseline", func() { must(save(io.Discard)) }, func() bool { return !d.Modified() && changes == 2 }},
		{"continue after save", func() { must(win.CommitText("y")) }, func() bool { return d.Text() == "axyb\ncd" && d.Modified() && !pending() }},
		{"undo to save point", func() { key(events.KeyZ, primary) }, func() bool { return d.Text() == "axb\ncd" && !d.Modified() }},
		{"redo away from save point", func() { key(events.KeyZ, primary|events.ModifierShift) }, func() bool { return d.Text() == "axyb\ncd" && d.Modified() }},
		{"capture old point", func() { old = d.CaptureSavePoint(); must(win.CommitText("z")) }, func() bool { return d.Text() == "axyzb\ncd" && d.Modified() && !pending() }},
		{"old save completes", func() { must(d.MarkSaved(old)) }, func() bool { return d.Modified() && b.Document().Modified() }},
		{"undo to old saved state", func() { key(events.KeyZ, primary) }, func() bool { return d.Text() == "axyb\ncd" && !d.Modified() }},
		{"line ending differs", func() { must(d.SetLineEnding(source.LineEndingLF)) }, func() bool { return d.Modified() }},
		{"line ending round trip", func() { must(d.SetLineEnding(source.LineEndingCRLF)) }, func() bool { return !d.Modified() }},
		{"multi input", func() {
			must(a.SetSelections([]source.TextSelection{{Anchor: 1, Caret: 1}, {Anchor: 6, Caret: 6}}))
			must(win.CommitText("Q"))
		}, func() bool { return d.Text() == "aQxyb\ncQd" && d.Modified() && !pending() }},
		{"multi undo clean", func() { key(events.KeyZ, primary) }, func() bool { return d.Text() == "axyb\ncd" && !d.Modified() && len(a.Selections()) == 2 }},
		{"prepare failed save", func() { must(win.CommitText("F")) }, func() bool { return d.Text() == "aFxyb\ncFd" && d.Modified() && !pending() }},
		{"failed save retains baseline", func() {
			check(save(failedWriter{}) == io.ErrClosedPipe, "expected save failure")
			key(events.KeyZ, primary)
		}, func() bool { return d.Text() == "axyb\ncd" && !d.Modified() }},
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
			panic(fmt.Sprintf("timeout %s: text=%q modified=%t selections=%v", scenes[stage].name, d.Text(), d.Modified(), a.Selections()))
		}
		if !scenes[stage].ready() {
			return
		}
		log.Printf("PASS %s", scenes[stage].name)
		stage++
		if stage == len(scenes) {
			complete = true
			log.Printf("PASS savepoint OS=%s typography=%s scale=%s basic=%t dark=%t", runtime.GOOS, app.Typography().Name(), os.Getenv("GOUI_PLAT_SCALE"), *basicTheme, *dark)
			win.Destroy()
			return
		}
		started = time.Now()
		scenes[stage].start()
	})
	must(timer.Start(50 * time.Millisecond))
	app.Run()
	check(complete, "closed before save point verification completed")
}
