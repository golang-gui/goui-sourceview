// 多选区绑定的独立窗口验收，需要原生桌面/字体；运行 go run ./tests/window/editor/multibinding。
// Linux 可用 XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software；-basic/-dark 及 GOUI_PLAT_SCALE=1/1.5。
// 初始两个选区；外部状态、方向键回写、状态源切换、显式回调、单/多声明互斥、卸载/重挂载自动执行。
// 逐项断言控件身份、局部更新不重放其他属性、写回副本及订阅/回调释放；真实输入经 Window.DispatchEvent。
// 成功自动退出；失败/提前退出非零；重启复位。不访问剪贴板、文件或系统设置。
// Windows/macOS 原生运行与真实 IME 需另验；本程序不替代物理输入。
package main

import (
	"flag"
	"fmt"
	"log"
	"slices"
	"time"

	source "github.com/golang-gui/goui-sourceview/sourceview"
	sourceui "github.com/golang-gui/goui-sourceview/ui"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/style"
	"github.com/golang-gui/goui/theme/basic"
	"github.com/golang-gui/goui/theme/modern"
	base "github.com/golang-gui/goui/ui"
)

type selectionSource struct {
	state               base.State[[]sourceui.TextSelection]
	connections, writes int
	last                []sourceui.TextSelection
}

func (s *selectionSource) Get() []sourceui.TextSelection  { return s.state.Get() }
func (s *selectionSource) Set(v []sourceui.TextSelection) { s.writes++; s.last = v; s.state.Set(v) }
func (s *selectionSource) Connect(fn func()) signal.Handle {
	s.connections++
	return &connection{s.state.Connect(fn), s}
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

type editorView struct {
	*sourceui.SourceViewView
	updates *int
}

func (v *editorView) Update(ctx base.BuildContext, w gui.Widget) {
	*v.updates++
	v.SourceViewView.Update(ctx, w)
}
func check(ok bool, msg string) {
	if !ok {
		panic(msg)
	}
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
func pair(at int) []sourceui.TextSelection {
	return []sourceui.TextSelection{{Anchor: at, Caret: at}, {Anchor: at + 10, Caret: at + 10}}
}
func pending(v *source.SourceView) bool {
	return v.Snapshot().Attributes["goui.sourceview"].(map[string]any)["inputPending"] == true
}
func main() {
	basicTheme, dark := flag.Bool("basic", false, "Basic"), flag.Bool("dark", false, "Modern dark")
	flag.Parse()
	rules := modern.Rules(modern.Options{Dark: *dark})
	if *basicTheme {
		rules = basic.Rules()
	}
	doc := source.NewDocument("abcdefghij\nabcdefghij\n")
	a := &selectionSource{state: base.MakeState(pair(1))}
	b := &selectionSource{state: base.MakeState(pair(3))}
	scalar := base.MakeState(sourceui.TextSelection{Anchor: 2, Caret: 2})
	global, builds, updates, mode, step, callbacks := 0, 0, 0, 0, 0, 0
	started, complete := false, false
	var original *source.SourceView
	err := base.Run("org.golanggui.SourceViewMultiBinding", func(app base.App) base.RootView {
		global++
		if !started {
			started = true
			app.TimeoutFunc(20*time.Second, func() { panic(fmt.Sprintf("timeout step %d", step)) })
			app.TickFunc(100*time.Millisecond, func() {
				win := app.FindWindow("main")
				check(win != nil, "window lost")
				editor, _ := app.FindWidget("main", "editor").(*source.SourceView)
				if editor != nil && pending(editor) {
					return
				}
				switch step {
				case 0:
					check(editor != nil && slices.Equal(editor.Selections(), pair(1)) && a.connections == 1, "initial binding")
					original = editor
					editor.SetReadOnly(true)
					a.Set(pair(2))
				case 1:
					check(editor == original && slices.Equal(editor.Selections(), pair(2)), "external value")
					check(global == 1 && builds == 1 && updates == 1 && editor.ReadOnly() && a.writes == 1, "local update rebuilt or wrote back")
					win.SetFocusedWidget(editor)
					for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
						must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: events.KeyArrowRight}))
					}
				case 2:
					check(slices.Equal(a.state.Peek(), pair(3)) && a.writes == 2, "key writeback")
					a.last[0].Caret = 99
					check(editor.Selection().Caret == 3, "writeback aliases widget")
					mode = 1
					app.RequestUpdate()
				case 3:
					check(editor == original && slices.Equal(editor.Selections(), pair(3)) && a.connections == 0 && b.connections == 1 && b.writes == 0, "source switch")
					a.Set(pair(0))
					b.Set(pair(4))
				case 4:
					check(slices.Equal(editor.Selections(), pair(4)) && builds == 2 && updates == 2 && b.writes == 1, "new source local update")
					mode = 2
					app.RequestUpdate()
				case 5:
					b.Set(pair(5))
				case 6:
					check(slices.Equal(editor.Selections(), pair(5)) && callbacks == 0 && b.writes == 2, "external callback")
					must(editor.SetSelections(pair(6)))
					check(callbacks == 1 && b.writes == 2, "explicit callback did not replace writeback")
					mode = 3
					app.RequestUpdate()
				case 7:
					check(len(editor.Selections()) == 1 && editor.Selection().Caret == 2 && b.connections == 0, "scalar declaration retained multi binding")
					b.Set(pair(1))
					mode = 4
					app.RequestUpdate()
				case 8:
					check(slices.Equal(editor.Selections(), pair(2)), "multiple declaration did not replace scalar binding")
					scalar.Set(sourceui.TextSelection{Caret: 0})
					mode = 5
					app.RequestUpdate()
				case 9:
					check(editor == nil && original.Root() == nil && a.connections == 0 && b.connections == 0, "unmount retained subscription")
					writes := a.writes
					must(original.SetSelections(pair(0)))
					check(a.writes == writes && callbacks == 1, "unmount retained callback")
					a.Set(pair(4))
					mode = 6
					app.RequestUpdate()
				case 10:
					check(editor != nil && editor != original && editor.Document() == doc && slices.Equal(editor.Selections(), pair(4)) && a.connections == 1, "remount")
					complete = true
					fmt.Println("PASS multibinding local updates, writeback copies, source switching and teardown")
					app.Quit()
				}
				step++
			})
		}
		return base.Root().StyleSheet(style.Sheet(rules...)).Windows(base.Window("main").Size(720, 420).Content(base.Build(func() base.View {
			builds++
			if mode == 5 {
				return nil
			}
			view := sourceui.SourceView().ID("editor").Document(doc).AutomaticCompletion(false)
			switch mode {
			case 0, 6:
				view.BindSelections(a)
			case 1:
				view.BindSelections(b)
			case 2:
				view.BindSelections(b).OnSelections(func([]sourceui.TextSelection) { callbacks++ })
			case 3:
				view.BindSelections(b).Selection(sourceui.TextSelection{Anchor: 2, Caret: 2})
			case 4:
				view.BindSelection(&scalar).Selections(pair(2))
			}
			return base.ScrollView(&editorView{view, &updates}).Style("source-view")
		})))
	})
	if err != nil {
		log.Fatal(err)
	}
	check(complete && a.connections == 0 && b.connections == 0, "early exit or retained subscriptions")
}
