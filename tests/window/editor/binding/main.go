// SourceView 选区属性绑定与局部更新验收，需要桌面、原生排版和字体。
// 启动：go run ./tests/window/editor/binding；Linux 无输入法服务时设置 XMODIFIERS=@im=none。
// 可用 GOUI_PLAT_PAINTER=software/opengl、GOUI_PLAT_SCALE=1/2 重复验证。
// 初始：一个稳定 Document、一个绑定选区的编辑器。程序自动运行，无需手动操作。
// 逐项断言外部状态只更新选区，不重建父视图或重放 ReadOnly 等无关声明；
// 经 Window.DispatchEvent 的方向键回写选区；切换状态源会解除旧订阅；
// OnSelection 只替换回写，Selection 移除绑定且保留显式回调；
// 卸载后无订阅或回调，重新挂载恢复最新选区，文档和历史不丢失。
// 模型和 setter 只用于准备场景，模拟输入始终经正常窗口事件路径。
// 不读写文件、剪贴板或系统设置；重启复位。成功自动退出，提前关闭或断言失败返回非零。
// Windows/macOS 需原生运行；本程序不替代真实 IME、物理键盘及混合 DPI 验收。
package main

import (
	"fmt"
	"log"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	sourceui "github.com/golang-gui/goui-sourceview/ui"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/theme/modern"
	base "github.com/golang-gui/goui/ui"
)

type selectionSource struct {
	state               base.State[base.TextSelection]
	connections, writes int
}

func (s *selectionSource) Get() base.TextSelection { return s.state.Get() }
func (s *selectionSource) Set(value base.TextSelection) {
	s.writes++
	s.state.Set(value)
}
func (s *selectionSource) Connect(fn func()) signal.Handle {
	s.connections++
	return &connection{Handle: s.state.Connect(fn), owner: s}
}

type connection struct {
	signal.Handle
	owner *selectionSource
}

func (c *connection) Disconnect() {
	if c.owner != nil {
		c.owner.connections--
		c.owner = nil
	}
	c.Handle.Disconnect()
}

// 统计真正的 WidgetView.Update 次数，与仅应用属性的更新分开。
type editorView struct {
	*sourceui.SourceViewView
	updates *int
}

func (v *editorView) Update(ctx base.BuildContext, widget gui.Widget) {
	*v.updates++
	v.SourceViewView.Update(ctx, widget)
}

func require(ok bool, message string) {
	if !ok {
		panic(message)
	}
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	doc := source.NewDocument("one two three\nsecond line")
	a := &selectionSource{state: base.MakeState(base.TextSelection{Anchor: 1, Caret: 1})}
	b := &selectionSource{state: base.MakeState(base.TextSelection{Anchor: 7, Caret: 7})}
	started, complete := false, false
	global, builds, updates, callbacks, mode, step := 0, 0, 0, 0, 0, 0
	var original *source.SourceView
	err := base.Run("org.golang-gui.SourceViewBinding", func(app base.App) base.RootView {
		global++
		if !started {
			started = true
			app.TimeoutFunc(15*time.Second, func() { panic("选区绑定验证超时") })
			app.TickFunc(250*time.Millisecond, func() {
				win := app.FindWindow("main")
				require(win != nil, "主窗口丢失")
				editor, _ := app.FindWidget("main", "editor").(*source.SourceView)
				key := func(key events.Key, modifiers events.Modifiers) {
					for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
						must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: key, Modifiers: modifiers}))
					}
				}
				move := func(offset int) base.TextSelection { return base.TextSelection{Anchor: offset, Caret: offset} }
				switch step {
				case 0:
					require(editor != nil && editor.Selection() == move(1) && a.connections == 1, "初始选区或订阅错误")
					original = editor
					editor.SetReadOnly(true)
					a.Set(move(4))
				case 1:
					require(editor == original && editor.Selection() == move(4), "外部选区未局部应用")
					require(global == 1 && builds == 1 && updates == 1 && a.writes == 1 && editor.ReadOnly(), "属性更新重建或重放无关声明，或发生回写")
					win.SetFocusedWidget(editor)
					key(events.KeyArrowRight, 0)
					require(a.state.Peek() == move(5) && a.writes == 2, "方向键未回写选区")
					mode = 1
					app.RequestUpdate()
				case 2:
					require(editor == original && editor.Selection() == move(7), "切换状态源替换了控件或选区错误")
					require(a.connections == 0 && b.connections == 1 && b.writes == 0, "切换状态源未解除旧订阅或发生回写")
					a.Set(move(2))
					b.Set(move(8))
				case 3:
					require(editor.Selection() == move(8) && builds == 2 && updates == 2 && b.writes == 1, "旧订阅残留或新属性更新错误")
					mode = 2
					app.RequestUpdate()
				case 4:
					b.Set(move(10))
				case 5:
					require(editor.Selection() == move(10) && b.writes == 2 && callbacks == 0, "外部应用触发了用户回调")
					key(events.KeyArrowRight, 0)
					require(callbacks == 1 && b.state.Peek() == move(10), "OnSelection 未替换绑定回写")
					mode = 3
					app.RequestUpdate()
				case 6:
					require(editor.Selection() == move(3) && b.connections == 0 && callbacks == 1, "Selection 未移除读绑定")
					b.Set(move(12))
					mode = 4
					app.RequestUpdate()
				case 7:
					require(editor.Selection() == move(2) && a.connections == 0 && b.connections == 0, "静态 Selection 残留订阅")
					key(events.KeyArrowRight, 0)
					require(callbacks == 2 && a.writes == 3, "静态 Selection 清除了显式回调或残留回写")
					must(win.CommitText("X"))
					require(doc.Model().Text() == "oneX two three\nsecond line", "文本提交失败")
					primary, _ := gui.ModPrimary.Resolve()
					key(events.KeyZ, primary)
					require(doc.Model().Text() == "one two three\nsecond line", "撤销失败")
					mode = 0
					app.RequestUpdate()
				case 8:
					require(editor == original && a.connections == 1, "卸载前未恢复有效订阅")
					mode = 5
					app.RequestUpdate()
				case 9:
					// UI 卸载断开声明连接并脱离 Root，不冒充 GUI 的最终销毁。
					require(editor == nil && original.Root() == nil && a.connections == 0 && b.connections == 0, "卸载残留控件或订阅")
					before, writes := callbacks, a.writes
					original.SetSelection(move(0))
					require(callbacks == before && a.writes == writes, "卸载后旧控件仍调用声明式连接")
					a.Set(move(6))
					mode = 0
					app.RequestUpdate()
				case 10:
					require(editor != nil && editor != original && editor.Document() == doc && editor.Selection() == move(6), "重挂载未恢复文档和最新选区")
					require(doc.Model().CanRedo(), "重建清除了文档历史")
					complete = true
					fmt.Println("PASS SourceView local selection binding, modifier order, source switching, teardown and history")
					app.Quit()
				}
				step++
			})
		}
		return base.Root().StyleSheet(modern.FromSettings(app.Settings())).Windows(base.Window("main").Title("SourceView selection binding").Size(720, 420).
			Content(base.Build(func() base.View {
				builds++
				if mode == 5 {
					return nil
				}
				v := sourceui.SourceView().ID("editor").Document(doc).AutomaticCompletion(false)
				switch mode {
				case 0:
					v.BindSelection(a)
				case 1:
					v.BindSelection(b)
				case 2:
					v.BindSelection(b).OnSelection(func(base.TextSelection) { callbacks++ })
				case 3:
					v.BindSelection(b).Selection(base.TextSelection{Anchor: 3, Caret: 3})
				case 4:
					v.BindSelection(a).OnSelection(func(base.TextSelection) { callbacks++ }).Selection(base.TextSelection{Anchor: 2, Caret: 2})
				}
				return base.ScrollView(&editorView{SourceViewView: v, updates: &updates}).Style("source-view")
			})))
	})
	if err != nil {
		log.Fatal(err)
	}
	require(complete && a.connections == 0 && b.connections == 0, "提前退出或退出后残留订阅")
}
