// 本地 GOUI 接入验收，需要桌面、原生字体和窗口管理器。
// 启动：go run ./tests/window/editor/goui_integration [-basic|-dark]。
// Linux 可设 XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software；GOUI_PLAT_SCALE=1/1.5。
// 初态：另一个输入框与带留白的 SourceView。自动通过正常 DispatchEvent 验证：
// 1. FocusOnPointer=false 与 CanContainFocus=false 不被正文拖选绕过，快照反映策略。
// 2. 恢复策略后点击聚焦，Widget.Surface 返回 owner；无 Window 管理能力也不作为正文契约。
// 3. F2 打开补全，另一组件借用共享瞬态面后 Enter 正常插入 LF，不能接受旧候选。
// 4. 撤销、重开补全、销毁旧 presenter、接受并撤销，连续三轮不遗留可见弹窗。
// 5. F6 跳转与 F7 查找打开子输入，Escape 返回正文，检查 Surface 字段迁移后的子树与焦点。
// 不访问剪贴板、文件或全局设置；使用独立原创样本，重启复位。20 秒超时/提前退出为失败。
// 成功输出 PASS 后自动退出；关闭主窗口也退出。模拟输入不替代真实 Alt/Option 或 IME 验收。
package main

import (
	"flag"
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/golang-gui/goui-sourceview/language"
	source "github.com/golang-gui/goui-sourceview/sourceview"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/theme/basic"
	"github.com/golang-gui/goui/theme/modern"
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
func check(ok bool, message string) {
	if !ok {
		log.Fatal(message)
	}
}
func hasRole(info gui.WidgetInfo, role gui.Role) bool {
	if info.Role == role {
		return true
	}
	for _, child := range info.Children {
		if hasRole(child, role) {
			return true
		}
	}
	return false
}
func completion(win gui.Window) gui.Popover {
	for _, popup := range win.Popovers() {
		if hasRole(popup.Widget().Snapshot(), "completion-list") {
			return popup
		}
	}
	return nil
}
func main() {
	runtime.LockOSThread()
	basicTheme := flag.Bool("basic", false, "Basic theme")
	dark := flag.Bool("dark", false, "Modern dark theme")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewGOUIIntegration")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	if *basicTheme {
		app.SetStyleSheet(basic.Sheet())
	}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 720, Height: 460}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView GOUI integration"))
	other := gui.NewTextInput()
	other.SetText("Focus stays here when pointer focus is blocked")
	v := source.NewSourceView()
	v.SetShowLineNumbers(false)
	v.SetFoldingEnabled(false)
	v.SetAutomaticCompletion(false)
	doc := source.NewDocument("tr")
	must(doc.SetLanguage(language.JSON))
	v.SetDocument(doc)
	bindings := v.KeyBindings()
	bindings[source.CommandComplete] = []gui.KeyGesture{{Key: gui.KeyF2}}
	bindings[source.CommandGoTo] = []gui.KeyGesture{{Key: gui.KeyF6}}
	bindings[source.CommandFind] = []gui.KeyGesture{{Key: gui.KeyF7}}
	must(v.SetKeyBindings(bindings))
	v.ConnectError(func(err error) { log.Fatal(err) })
	scroll := gui.NewScrollView()
	scroll.SetChild(v)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetPadding(geometry.UniformInsets(24))
	box.SetSpacing(12)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(other)
	box.AddChild(scroll)
	win.SetWidget(box)
	must(win.Show())
	primary, ok := gui.ModPrimary.Resolve()
	check(ok, "missing primary modifier")
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	click := func() {
		r := v.Snapshot().Bounds
		point := geometry.Point{X: r.X + 100, Y: r.Y + 18}
		must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerDown, Position: point, Button: events.PointerButtonLeft, Buttons: events.PointerButtonLeftDown}))
		must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerUp, Position: point, Button: events.PointerButtonLeft}))
	}
	stage, rounds, done := 0, 0, false
	var old gui.Popover
	timer := app.NewTimer()
	defer timer.Stop()
	deadline := time.Now().Add(20 * time.Second)
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), fmt.Sprintf("timeout stage=%d round=%d", stage, rounds))
		switch stage {
		case 0:
			if !win.Focused() || v.Rect().Height == 0 {
				return
			}
			check(v.Surface() == win && other.Surface() == win, "wrong Surface owner")
			check(win.SetFocusedWidget(other), "cannot prepare other focus")
			v.SetFocusOnPointer(false)
			click()
			check(win.FocusedWidget() == other && !v.Focused(), "selection bypassed FocusOnPointer=false")
			check(!v.Snapshot().FocusOnPointer, "snapshot lost pointer policy")
			v.SetFocusOnPointer(true)
			v.SetCanContainFocus(false)
			click()
			check(win.FocusedWidget() == other && !v.Snapshot().CanContainFocus, "selection bypassed focus containment")
			v.SetCanContainFocus(true)
			click()
			check(win.FocusedWidget() == v && v.Focused(), "normal pointer focus failed")
			stage++
		case 1:
			v.SetSelection(source.TextSelection{Anchor: 2, Caret: 2})
			key(events.KeyF2, 0)
			stage++
		case 2:
			old = completion(win)
			if old == nil {
				return
			}
			foreign := gui.NewTransientPopover(other)
			foreign.SetWidget(gui.NewLabel("Another component owns the transient slot"))
			must(foreign.Show())
			check(!old.Visible() && completion(win) == nil, "completion remained visible after shared replacement")
			foreign.Destroy()
			key(events.KeyEnter, 0)
			check(doc.Text() == "tr\n", "hidden completion consumed Enter after shared replacement")
			key(events.KeyZ, primary)
			check(doc.Text() == "tr", "normal newline undo failed")
			key(events.KeyF2, 0)
			stage++
		case 3:
			current := completion(win)
			if current == nil {
				return
			}
			old.Destroy()
			check(current.Visible() && len(win.Popovers()) == 1, "old presenter destroyed a newer completion")
			key(events.KeyEnter, 0)
			check(doc.Text() == "true", "fresh completion did not accept expected JSON candidate")
			key(events.KeyZ, primary)
			check(doc.Text() == "tr" && len(win.Popovers()) == 0, "completion acceptance undo or close failed")
			rounds++
			if rounds < 3 {
				stage = 1
			} else {
				stage++
			}
		case 4:
			key(events.KeyF6, 0)
			check(!v.Focused() && v.ContainsFocus() && win.FocusedWidget().Surface() == win, "go-to panel host/focus failed")
			stage++
		case 5:
			check(win.FocusedWidget().Rect().Width > 0, "go-to panel was not allocated")
			key(events.KeyEscape, 0)
			check(v.Focused(), "go-to Escape did not restore editor focus")
			key(events.KeyF7, 0)
			check(!v.Focused() && v.ContainsFocus(), "find did not focus its input")
			stage++
		case 6:
			key(events.KeyEscape, 0)
			check(v.Focused() && len(win.Popovers()) == 0 && doc.Text() == "tr", "final editor state changed")
			done = true
			timer.Stop()
			fmt.Printf("PASS goui-integration OS=%s basic=%t dark=%t rounds=%d\n", runtime.GOOS, *basicTheme, *dark, rounds)
			app.Quit()
		}
	})
	must(timer.Start(80 * time.Millisecond))
	must(app.Run())
	check(done, "window closed before verification completed")
}
