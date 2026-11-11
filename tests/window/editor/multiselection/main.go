// 多选区独立窗口验收，需要桌面、原生排版和字体。启动：
// XMODIFIERS=@im=none GOUI_PLAT_PAINTER=software go run ./tests/window/editor/multiselection
// -basic/-dark 切换外观；GOUI_PLAT_SCALE=1/1.5。初态为共享文档的两个独立视图。
// setter 仅准备场景；中文输入、删除、导航、剪切/粘贴、撤销重做经 Window 的正常事件路径。
// 第三批额外检查连续输入合并、配对/跳过/删除和后台大小写/清理/重缩进。
// 断言完整正文、主/附加选区、视图隔离、历史、快照恢复和超过缓存页数的分散输入。
// 鼠标用字面 Alt / Alt+Shift 经正常事件分发，检查默认值及追加拖选/矩形选择。
// 默认与 -clipboard 是两条独立验收路径，临时快捷键不依赖此前场景。
// -clipboard 额外覆盖该显示的临时剪贴板，只在独立桌面启用。默认不访问剪贴板。
// 每步有明确条件与超时，成功自动关闭；提前退出或失败返回非零。不写文件或系统设置。
// 真实 IME 服务、物理键及 Windows/macOS 必须另验；重启恢复初态，关闭窗口退出。
package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/golang-gui/goui-sourceview/language"
	"log"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
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

// Observe the public tokenizer's normal final-line analysis without changing
// its results or driving the editor's private scheduler from the test.
type observedTokenizer struct {
	language.Tokenizer
	ready *atomic.Bool
}

func (t observedTokenizer) Scan(in language.Input, state language.State) (language.Result, error) {
	result, err := t.Tokenizer.Scan(in, state)
	if err == nil && in.LineEnd && in.DocumentEnd && in.Text == "    " && in.Offset+result.Consumed == len(in.Text) {
		t.ready.Store(true)
	}
	return result, err
}

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
	// Every scene owns its temporary F8 binding. Optional clipboard scenes must
	// not be responsible for removing the preceding scene's command.
	bindScene := func(command source.Command) {
		bindings := source.DefaultKeyBindings()
		bindings[command] = []gui.KeyGesture{{Key: gui.KeyF8}}
		must(a.SetKeyBindings(bindings))
	}

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
	var cancellationAt time.Time
	var pasteAnalyzed atomic.Bool
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
			check(a.SelectionModifiers() == (source.SelectionModifiers{Add: gui.ModAlt, Column: gui.ModAlt | gui.ModShift}), "default Alt mouse gestures")
			a.SetWrapMode(source.WrapNone)
			a.SetFoldingEnabled(false)
			a.SetShowLineNumbers(false)
			d.SetText("abcdef\nabcdef\nabcdef")
			a.SetSelection(source.TextSelection{})
		}, func() bool { return !pending(a) }},
		step{"pointer rectangle", func() {
			mods := events.ModifierAlt | events.ModifierShift
			pointer(events.PointerDown, 1, 0, mods)
			pointer(events.PointerMove, 3, 2, mods)
			pointer(events.PointerUp, 3, 2, mods)
		}, func() bool {
			return slices.Equal(a.Selections(), []source.TextSelection{{Anchor: 15, Caret: 17}, {Anchor: 1, Caret: 3}, {Anchor: 8, Caret: 10}})
		}},
		step{"rectangle input", func() { must(win.CommitText("Z")) }, func() bool { return d.Text() == "aZdef\naZdef\naZdef" && !pending(a) }},
		step{"add pointer", func() {
			a.SetSelection(source.TextSelection{})
			mods := events.ModifierAlt
			pointer(events.PointerDown, 2, 1, mods)
			pointer(events.PointerUp, 2, 1, mods)
		}, func() bool { return slices.Equal(a.Selections(), []source.TextSelection{{Anchor: 8, Caret: 8}, {}}) }},
		step{"add pointer range", func() {
			pointer(events.PointerDown, 1, 2, events.ModifierAlt)
			pointer(events.PointerMove, 3, 2, events.ModifierAlt)
			pointer(events.PointerUp, 3, 2, events.ModifierAlt)
		}, func() bool {
			return slices.Equal(a.Selections(), []source.TextSelection{{Anchor: 13, Caret: 15}, {}, {Anchor: 8, Caret: 8}})
		}},
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

		step{"continuous typing", func() {
			d.SetLanguage(language.Plain)
			d.SetText("\n")
			set(a, []source.TextSelection{{}, {Anchor: 1, Caret: 1}})
			must(win.CommitText("a"))
			must(win.CommitText("中"))
			must(win.CommitText("b"))
		}, func() bool { return d.Text() == "a中b\na中b" && !pending(a) }},
		step{"continuous undo", func() { key(events.KeyZ, primary) }, func() bool { return d.Text() == "\n" && len(a.Selections()) == 2 }},
		step{"paired typing", func() {
			definition, err := language.New(language.Definition{Info: language.Info{ID: "window-test", Name: "Window test"}, Editing: language.EditingRules{AutoClosingPairs: []language.Pair{{Open: "(", Close: ")"}}, Brackets: []language.Pair{{Open: "(", Close: ")"}}}})
			must(err)
			d.SetLanguageDefinition(definition)
			must(win.CommitText("("))
		}, func() bool { return d.Text() == "()\n()" && !pending(a) }},
		step{"paired skip", func() { must(win.CommitText(")")) }, func() bool {
			return d.Text() == "()\n()" && slices.Equal(a.Selections(), []source.TextSelection{{Anchor: 2, Caret: 2}, {Anchor: 5, Caret: 5}}) && !pending(a)
		}},
		step{"pair reset", func() { key(events.KeyZ, primary) }, func() bool { return d.Text() == "\n" }},
		step{"paired deletion", func() { must(win.CommitText("(")); key(events.KeyBackspace, 0) }, func() bool { return d.Text() == "\n" && !pending(a) }},
		step{"multiple case", func() {
			d.SetText("ab\ncd")
			set(a, []source.TextSelection{{Anchor: 2, Caret: 0}, {Anchor: 3, Caret: 5}})
			bindScene(source.CommandUpperCase)
			key(events.KeyF8, 0)
		}, func() bool {
			return d.Text() == "AB\nCD" && slices.Equal(a.Selections(), []source.TextSelection{{Anchor: 2, Caret: 0}, {Anchor: 3, Caret: 5}})
		}},
		step{"multiple cleanup", func() {
			d.SetText("a \nb \nc ")
			set(a, []source.TextSelection{{}, {Anchor: 3, Caret: 3}})
			bindScene(source.CommandTrimTrailingWhitespace)
			key(events.KeyF8, 0)
		}, func() bool { return d.Text() == "a\nb\nc" && len(a.Selections()) == 2 }},
		step{"multiple join", func() {
			d.SetText("one\n two\nthree\n four")
			set(a, []source.TextSelection{{}, {Anchor: 9, Caret: 9}})
			bindScene(source.CommandJoinLines)
			key(events.KeyF8, 0)
		}, func() bool { return d.Text() == "one two\nthree four" && len(a.Selections()) == 2 }},
		step{"multiple reindent", func() {
			definition, err := language.New(language.Definition{Info: language.Info{ID: "indent-test", Name: "Indent test"}, Editing: language.EditingRules{Reindent: func(ctx context.Context, c language.ReindentContext) ([]language.LineIndent, error) {
				var result []language.LineIndent
				for row := c.StartLine; row <= c.EndLine; row++ {
					result = append(result, language.LineIndent{Line: row, Text: "  "})
				}
				return result, ctx.Err()
			}}})
			must(err)
			d.SetLanguageDefinition(definition)
			d.SetText("aa\nbb\ncc")
			set(a, []source.TextSelection{{}, {Anchor: 6, Caret: 6}})
			bindScene(source.CommandReindentSelection)
			key(events.KeyF8, 0)
		}, func() bool { return d.Text() == "  aa\nbb\n  cc" && len(a.Selections()) == 2 }},
		step{"prepare reindented paste", func() {
			if !*clipboard {
				return
			}
			// Wait for the public tokenizer to analyze the complete fixture.
			jsonLanguage, ok := language.Builtin(language.JSON)
			check(ok, "JSON definition missing")
			definition := jsonLanguage.Definition()
			definition.Tokenizer = observedTokenizer{definition.Tokenizer, &pasteAnalyzed}
			tracked, err := language.New(definition)
			must(err)
			pasteAnalyzed.Store(false)
			d.SetLanguageDefinition(tracked)
			d.SetText("  \n    ")
			set(a, []source.TextSelection{{Anchor: 2, Caret: 2}, {Anchor: 7, Caret: 7}})
		}, func() bool { return !*clipboard || pasteAnalyzed.Load() }},
		step{"multiple reindented paste", func() {
			if !*clipboard {
				return
			}
			must(app.Clipboard().SetText("{\n  \"a\":1\n}"))
			bindScene(source.CommandPasteReindented)
			key(events.KeyF8, 0)
		}, func() bool { return !*clipboard || d.Text() == "  {\n    \"a\":1\n  }\n    {\n      \"a\":1\n    }" }},
		step{"cancel background set", func() {
			d.SetText("one\n two")
			set(a, []source.TextSelection{{Anchor: 0, Caret: 3}, {Anchor: 5, Caret: 8}})
			bindScene(source.CommandUpperCase)
			key(events.KeyF8, 0)
			a.SetSelection(source.TextSelection{})
			cancellationAt = time.Now()
		}, func() bool {
			return time.Since(cancellationAt) > 200*time.Millisecond && d.Text() == "one\n two" && len(a.Selections()) == 1
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
			panic(fmt.Sprintf("timeout %s: bytes=%d prefix=%q selections=%v pending=%t", steps[stage].name, d.Len(), d.Text()[:min(128, d.Len())], a.Selections(), pending(a)))
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
