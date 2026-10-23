// 内置查找、替换和声明式生命周期验证。需要真实桌面、窗口管理器、原生排版和字体。
// 启动：go run ./tests/window/editor/find；Linux 无输入法服务时设置 XMODIFIERS=@im=none。
// 初始声明两个独立编辑器，不创建外部配套控件；内部栏首次打开时才创建。
// 自动通过 Window.DispatchEvent/CommitText 验证默认查找和替换键、选区取词、上下项、
// 选项、替换和撤销、只读、关闭、搜索背景、栏内拖选及其右键菜单与正文隔离。
// 验证无关重建保留输入、多视图隔离、解除绑定后应用可收到按键、移除后取消旧交互、
// 重挂载创建新子树、分块计数失效、回调更换文档及聚焦期间销毁窗口。
// 模型和方法调用仅准备场景，不作为模拟输入捷径。按实际日志记录后端和缩放。
// 不修改剪贴板、文件或系统设置；重启复位。成功自动关闭，提前关闭或断言失败以非零退出。
// 真实 IME、系统拦截和混合 DPI 仍需手工验证。
package main

import (
	"fmt"
	"image"
	"log"
	"runtime"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	sourceui "github.com/golang-gui/goui-sourceview/ui"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
	base "github.com/golang-gui/goui/ui"
)

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
func check(ok bool, msg string) {
	if !ok {
		log.Fatal(msg)
	}
}
func named(w gui.Widget, name string) gui.Widget {
	if w.Name() == name {
		return w
	}
	for _, c := range w.Children() {
		if found := named(c, name); found != nil {
			return found
		}
	}
	return nil
}
func main() {
	runtime.LockOSThread()
	doc := source.NewDocument("one ONE stone one")
	other := source.NewDocument("beta beta")
	show := true
	bindingsA, bindingsB := source.DefaultKeyBindings(), source.DefaultKeyBindings()
	started, complete := false, false
	var originalBar gui.Widget
	var originalOwner *source.SourceView
	var a, b *source.SourceView
	baseControllers, otherControllers, fallback, errors := 0, 0, 0, 0
	partialReset := false
	step := 0
	must(base.Run("org.golang-gui.SourceViewFindVerification", func(app base.App) base.RootView {
		if !started {
			started = true
			app.TickFunc(350*time.Millisecond, func() {
				win := app.FindWindow("main")
				check(win != nil, "window missing")
				a, _ = app.FindWidget("main", "a").(*source.SourceView)
				b = app.FindWidget("main", "b").(*source.SourceView)
				key := func(k events.Key, mods events.Modifiers) {
					for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
						must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
					}
				}
				primary, _ := gui.ModPrimary.Resolve()
				openReplace := func() {
					if runtime.GOOS == "darwin" {
						alt, _ := gui.ModAlt.Resolve()
						key(events.KeyF, primary|alt)
					} else {
						key(events.KeyH, primary)
					}
				}
				click := func(w gui.Widget) {
					check(w != nil, "click target missing")
					r := w.Snapshot().Bounds
					p := geometry.Point{X: r.X + r.Width/2, Y: r.Y + r.Height/2}
					for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
						must(win.DispatchEvent(events.PointerEvent{EventType: kind, Position: p, Button: events.PointerButtonLeft}))
					}
				}
				var bar gui.Widget
				if a != nil {
					bar = panel(a, "sourceview-find-bar")
				}
				count := func(want int) {
					check(bar != nil, "bar missing")
					state := bar.Snapshot().Attributes["sourceview.find"].(map[string]any)
					check(state["countComplete"] == true && state["count"] == want, fmt.Sprintf("step %d: completed match count=%v complete=%v, want %d; query=%+v", step, state["count"], state["countComplete"], want, a.Search()))
				}
				switch step {
				case 0:
					check(bar == nil, "closed editor eagerly created a find subtree")
					originalOwner = a
					baseControllers = len(a.EventControllers())
					otherControllers = len(b.EventControllers())
					s := gui.NewShortcut(gui.KeyGesture{Key: gui.KeyF, Modifiers: gui.ModPrimary})
					s.ConnectActivate(func() { fallback++ })
					win.Shortcuts().AddShortcut(s)
					win.SetFocusedWidget(a)
					a.SetSelection(textedit.Selection{Anchor: 3, Caret: 0})
					key(events.KeyF, primary)
					bar = panel(a, "sourceview-find-bar")
					originalBar = bar
					check(bar.Visible() && named(bar, "Find").Focused() && a.Search().Text == "one" && !doc.Model().CanUndo(), "query seed, focus or history")
				case 1:
					count(3)
					// 栏内拖选和右击不能触发正文的拖选或编辑菜单。
					input := named(bar, "Find").(*gui.TextView)
					selection := a.Selection()
					bounds := input.Snapshot().Bounds
					for i, kind := range []events.EventType{events.PointerDown, events.PointerMove, events.PointerUp} {
						x := bounds.X + 4
						if i > 0 {
							x = bounds.X + 24
						}
						must(win.DispatchEvent(events.PointerEvent{EventType: kind, Position: geometry.Point{X: x, Y: bounds.Y + bounds.Height/2}, Button: events.PointerButtonLeft}))
					}
					check(a.Selection() == selection && input.Focused() && input.Selection().Anchor != input.Selection().Caret, "find drag changed source selection or lost input focus")
					check(a.Cursor() == nil, "code text cursor leaked into the internal UI")
					for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
						must(win.DispatchEvent(events.PointerEvent{EventType: kind, Position: geometry.Point{X: bounds.X + 8, Y: bounds.Y + bounds.Height/2}, Button: events.PointerButtonRight}))
					}
					check(a.Selection() == selection && input.Focused(), "find right-click opened the source menu or moved its selection")
					check(len(win.Popovers()) == 1, "input context menu missing or duplicated")
					key(events.KeyEscape, 0)
					check(bar.Visible() && input.Focused() && len(win.Popovers()) == 0, "input menu dismissal closed the find UI")
					key(events.KeyEnter, 0)
					check(a.Selection() == (textedit.Selection{Anchor: 10, Caret: 13}), "find enter")
					key(events.KeyEnter, events.ModifierShift)
					check(a.Selection() == (textedit.Selection{Caret: 3}), "find shift-enter")
					click(named(bar, "Match case"))
					check(a.Search().IgnoreCase, "case toggle pointer")
				case 2:
					count(4)
					w := named(bar, "Whole word")
					check(w != nil, "word option missing")
					win.SetFocusedWidget(w)
					key(events.KeySpace, 0)
					check(a.Search().WholeWord, "word toggle keyboard")
					openReplace()
					check(named(bar, "Replacement").Visible(), "replace shortcut")
				case 3:
					count(3)
					field := named(bar, "Replacement")
					check(field != nil && field.Snapshot().Enabled, "replacement field unavailable")
					win.SetFocusedWidget(field)
					must(win.CommitText("X"))
					key(events.KeyEnter, 0)
					check(doc.Model().Text() == "X ONE stone one" && a.Selection() == (textedit.Selection{Anchor: 2, Caret: 5}), "replace and advance")
					win.SetFocusedWidget(a)
					key(events.KeyZ, primary)
					check(doc.Model().Text() == "one ONE stone one", "replace undo")
					win.SetFocusedWidget(named(bar, "Find"))
					key(events.KeyEscape, 0)
					check(!bar.Visible() && a.Focused() && a.Search().Text == "one" && !a.HighlightSearchMatches() && a.Cursor() == gui.CursorText, "close focus/query/highlights/cursor")
					key(events.KeyF3, 0)
					check(a.Selection() == (textedit.Selection{Anchor: 4, Caret: 7}), "closed-bar search navigation")
					check(!a.HighlightSearchMatches(), "F3 restored closed-bar highlights")
					a.SetReadOnly(true)
					openReplace()
				case 4:
					check(!named(bar, "Replacement").Snapshot().Enabled && !named(bar, "Replace all").Snapshot().Enabled, "readonly replacement enabled")
					a.SetReadOnly(false)
					app.RequestUpdate()
				case 5:
					check(bar == originalBar && bar.Visible() && a.Search().Text == "ONE" && len(a.EventControllers()) == baseControllers, "rebuild cleared state or duplicated controller")
					win.SetFocusedWidget(b)
					key(events.KeyF, primary)
					otherBar := panel(b, "sourceview-find-bar")
					check(otherBar != nil && otherBar != bar && otherBar.Parent() == b && bar.Parent() == a && b.Search().Text == "", "editors shared a companion or query")
					delete(bindingsA, source.CommandFind)
					app.RequestUpdate()
				case 6:
					check(len(a.EventControllers()) == baseControllers && len(b.EventControllers()) == otherControllers, "lazy creation changed owner controllers")
					a.HidePanel()
					check(!a.HighlightSearchMatches(), "hidden find retained search decorations")
					win.SetFocusedWidget(a)
					key(events.KeyF, primary)
					check(fallback == 1 && !bar.Visible(), "disabled Find binding was consumed")
					win.SetFocusedWidget(b)
					key(events.KeyF, primary)
					check(panelVisible(b, "sourceview-find-bar") && named(b, "Find").Focused(), "another view lost its own default bindings")
					show = false
					app.RequestUpdate()
				case 7:
					check(a == nil && originalOwner.Root() == nil && !originalBar.Visible(), "removed editor retained its owned companion")
					check(originalOwner.ShowFind() == source.ErrNotMounted, "unmounted editor opened private UI")
					b.HidePanel()
					delete(bindingsB, source.CommandFind)
					must(b.SetKeyBindings(bindingsB))
					win.SetFocusedWidget(b)
					key(events.KeyF, primary)
					check(fallback == 2 && !panelVisible(b, "sourceview-find-bar"), "removed binding intercepted Find")
					show = true
					bindingsA = source.DefaultKeyBindings()
					app.RequestUpdate()
				case 8:
					check(a != originalOwner && bar == nil, "editor remount retained an old private subtree")
					a.SetSelection(textedit.Selection{})
					win.SetFocusedWidget(a)
					key(events.KeyF, primary)
					bar = panel(a, "sourceview-find-bar")
					check(bar != nil && bar != originalBar, "remounted editor did not create a new owned bar")
					doc.Model().SetText(strings.Repeat("x", 200000))
					a.SetSearch(source.SearchQuery{Text: "x"})
					var handle interface{ Disconnect() }
					handle = win.ConnectFrame(func(time.Time) {
						state := bar.Snapshot().Attributes["sourceview.find"].(map[string]any)
						if state["countComplete"] == false && state["count"].(int) > 0 {
							partialReset = true
							handle.Disconnect()
							doc.Model().SetText("reset reset")
							a.SetSearch(source.SearchQuery{Text: "reset"})
						}
					})
				case 9:
					check(partialReset, "did not exercise partial count invalidation")
					count(2)
					openReplace()
					named(bar, "Replacement").(*gui.TextView).Model().SetText("R")
					a.SetSelection(textedit.Selection{})
					key(events.KeyEnter, 0)
				case 10:
					click(named(bar, "Replace all"))
					check(doc.Model().Text() == "R R", "replace all")
					win.SetFocusedWidget(a)
					key(events.KeyZ, primary)
					check(doc.Model().Text() == "reset reset", "replace-all atomic undo")
					openReplace()
					replacementDoc := source.NewDocument("keep reset")
					h := a.ConnectChange(func(textedit.Change) {
						a.SetDocument(replacementDoc)
						a.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
					})
					win.SetFocusedWidget(named(bar, "Replacement"))
					key(events.KeyEnter, 0)
					h.Disconnect()
					check(a.Document() == replacementDoc && replacementDoc.Model().Text() == "keep reset" && a.Selection() == (textedit.Selection{Anchor: 2, Caret: 2}), "replacement callback state overwritten")
					check(errors == 0, "internal companion errors were not forwarded exactly once")
					probe := app.FindWidget("main", "probe").(*scaleProbe)
					check(probe.scale > 0, "painter probe not rendered")
					log.Printf("PASS built-in find/replace, keyboard/pointer, counting, history, readonly, declarative rebuild/isolation/removal; typography=%s painter=%s scale=%g", gui.App.Typography().Name(), probe.painter, probe.scale)
					complete = true
					a.HidePanel()
					h = a.ConnectFocused(func(focused bool) {
						if !focused {
							win.Destroy()
						}
					})
					key(events.KeyF, primary)
					h.Disconnect()
					check(a.Destroyed() && b.Destroyed() && !bar.Visible(), "host did not release the internal UI/editor")
					check(a.ShowFind() == source.ErrNotMounted, "destroyed editor opened internal UI")
				}
				step++
			})
		}
		var first base.View
		if show {
			first = base.ScrollView(sourceui.SourceView().ID("a").Document(doc).KeyBindings(bindingsA).OnError(func(error) { errors++ })).Style("source-view").ID("a-scroll").MainWeight(1)
		}
		return base.Window("main").Title("Built-in SourceView find / replace").Size(760, 500).Content(
			base.VBox(
				first,
				base.ScrollView(sourceui.SourceView().ID("b").Document(other).KeyBindings(bindingsB)).Style("source-view").ID("b-scroll").MainWeight(1),
				newProbe(),
			).CrossAlign(layout.CrossStretch))
	}))
	check(complete, "find verification aborted")
}

type probeView struct{ base.ViewBase[probeView] }

func newProbe() base.View                                  { v := &probeView{}; v.Self = v; v.ID("probe"); return v }
func (v *probeView) Build() base.View                      { return v }
func (v *probeView) Mount(base.BuildContext) gui.Widget    { return &scaleProbe{} }
func (v *probeView) Update(base.BuildContext, gui.Widget)  {}
func (v *probeView) Unmount(base.BuildContext, gui.Widget) {}

type scaleProbe struct {
	gui.WidgetBase
	scale   float32
	painter string
}

func (p *scaleProbe) Measure(c layout.Constraint) layout.Measurement {
	return layout.Measured(c.Clamp(geometry.Size{Width: 1, Height: 1}))
}
func (p *scaleProbe) Paint(draw gui.Painter) {
	if p.scale != 0 {
		return
	}
	p.scale = draw.PixelScale()
	img, err := draw.NewImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	must(err)
	defer img.Destroy()
	p.painter = fmt.Sprintf("%T", img)
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
