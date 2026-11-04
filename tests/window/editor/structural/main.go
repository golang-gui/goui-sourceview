// P0 编辑辅助与大折叠结果的窗口验证。
// 环境：独立桌面和窗口管理器，原生排版；Linux XMODIFIERS=@im=none。
// 启动：GOWORK=off go run ./tests/window/editor/structural [-modern] [-dark]。
// 逐项通过 Window.DispatchEvent 验证子词、结构选区、语言换行、重新缩进、
// 合并行、大小写、尾部空白、一次撤销；剪贴板粘贴经平台 RequestText。
// 另验证共享文档的外部折叠隔离、重复发布，以及 10 MiB/十万候选的接收、
// 折叠后的真实编辑和取消。记录按键分发、事务/选区生效、后续绘制与心跳间隔。
// 可见初态为两个编辑器；只读镜像不能改变主视图的折叠意图。
// 本程序只在独立测试会话使用剪贴板，结束清空测试文本；不修改系统设置或文件。
// Basic/Modern、100%/150% 分别启动；物理 IME 和其他平台需要另行验收。
// 自动退出并打印 PASS；关闭窗口或断言失败返回非零。重新启动恢复初态。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"log"
	"runtime"
	"sort"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
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

type step struct {
	name  string
	run   func()
	ready func() bool
}

func main() {
	runtime.LockOSThread()
	modernFlag := flag.Bool("modern", false, "Modern theme")
	dark := flag.Bool("dark", false, "Dark Modern theme")
	flag.Parse()
	// 固定生成样本不依赖网络；候选端点预先构造，接收和编辑仍在真实窗口测量。
	largeText := strings.Repeat("block\n"+strings.Repeat("a", 100)+"\n", 100000)
	large := source.NewDocument(largeText)
	ranges := make([]source.FoldRange, 100000)
	for i := range ranges {
		ranges[i] = source.FoldRange{StartLine: 2 * i, EndLine: 2*i + 1}
	}
	largeFolds, err := source.NewFolds(largeText, ranges)
	must(err)
	app, err := gui.NewApplication("org.golanggui.SourceViewAssistance")
	must(err)
	if *modernFlag {
		app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 940, Height: 700}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView P0 editing verification"))
	v, mirror := source.NewSourceView(), source.NewSourceView()
	v.SetID("editor")
	mirror.SetReadOnly(true)
	doc := source.NewDocument("")
	v.SetDocument(doc)
	mirror.SetDocument(doc)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	status := gui.NewLabel("P0: folding · structural selection · indentation · editing")
	box.AddChild(status)
	for _, view := range []*source.SourceView{v, mirror} {
		scroll := gui.NewScrollView()
		scroll.SetStyleName("source-view")
		scroll.SetChild(view)
		scroll.SetMainWeight(1)
		box.AddChild(scroll)
	}
	probe := &paintProbe{}
	box.AddChild(probe)
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(v)
	expectLimit := false
	limitErrors := 0
	v.ConnectError(func(err error) {
		if expectLimit && errors.Is(err, source.ErrOperationLimit) {
			limitErrors++
			return
		}
		log.Fatal("editor: ", err)
	})
	key := func(k events.Key) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k}))
		}
	}
	invoke := func(c source.Command) {
		must(v.SetKeyBindings(source.KeyBindings{c: {{Key: gui.KeyF4}}}))
		key(events.KeyF4)
	}
	caret := func(at int) { v.SetSelection(textedit.Selection{Anchor: at, Caret: at}) }
	setup := func(text string, id language.ID, s textedit.Selection) {
		v.SetDocument(doc)
		mirror.SetDocument(doc)
		doc.Model().SetText(text)
		must(doc.SetLanguage(id))
		v.SetSelection(s)
		win.SetFocusedWidget(v)
	}
	var steps []step
	add := func(name string, run func(), ready func() bool) { steps = append(steps, step{name, run, ready}) }
	textIs := func(want string) func() bool { return func() bool { return v.Document().Model().Text() == want } }
	add("subword", func() {
		setup("getHTTPResponse2", language.Go, textedit.Selection{})
		invoke(source.CommandMoveWordPartRight)
		check(v.Selection().Caret == 3, "subword move")
		invoke(source.CommandSelectWordPartRight)
		check(v.Selection() == (textedit.Selection{Anchor: 3, Caret: 7}), "subword select")
		invoke(source.CommandDeleteWordPartRight)
		check(doc.Model().Text() == "getResponse2", "subword delete")
		invoke(source.CommandUndo)
		check(doc.Model().Text() == "getHTTPResponse2", "subword undo")
	}, nil)
	add("selection setup", func() { setup(`{"a": ["alphaBeta",2]}`, language.JSON, textedit.Selection{Anchor: 10, Caret: 10}) }, nil)
	add("structural roundtrip", func() {
		before := v.Selection()
		invoke(source.CommandExpandSelection)
		got, _ := doc.Model().Slice(v.Selection().Range())
		check(got == "alphaBeta", "selection word: "+got)
		invoke(source.CommandExpandSelection)
		got, _ = doc.Model().Slice(v.Selection().Range())
		check(got == `"alphaBeta"`, "selection literal: "+got)
		invoke(source.CommandShrinkSelection)
		invoke(source.CommandShrinkSelection)
		check(v.Selection() == before, "selection restore")
	}, nil)
	for _, tc := range []struct {
		id         language.ID
		text, want string
	}{{language.Python, "if ready:", "if ready:\n    "}, {language.YAML, "- item", "- item\n- "}, {language.TOML, "[table]", "[table]\n"}, {language.JSON, "{", "{\n    "}, {language.XML, "<node>", "<node>\n    "}, {language.YAML, "- key:", "- key:\n    "}, {language.YAML, "text: |2", "text: |2\n    "}} {
		add("newline setup "+string(tc.id), func() { setup(tc.text, tc.id, textedit.Selection{Anchor: len(tc.text), Caret: len(tc.text)}) }, nil)
		add("newline "+string(tc.id), func() {
			key(events.KeyEnter)
			check(doc.Model().Text() == tc.want, fmt.Sprintf("newline %s: %q", tc.id, doc.Model().Text()))
			invoke(source.CommandUndo)
			check(doc.Model().Text() == tc.text, "newline undo")
		}, nil)
	}
	original := "{\n\"items\": [\n1,\n2\n]\n}"
	indented := "{\n    \"items\": [\n        1,\n        2\n    ]\n}"
	add("reindent setup", func() { setup(original, language.JSON, textedit.Selection{Anchor: len(original)}) }, nil)
	add("reindent", func() { invoke(source.CommandReindentSelection) }, textIs(indented))
	add("reindent undo", func() {
		invoke(source.CommandUndo)
		check(doc.Model().Text() == original && v.Selection() == (textedit.Selection{Anchor: len(original)}), "reindent transaction")
		invoke(source.CommandRedo)
		check(doc.Model().Text() == indented, "reindent redo")
	}, nil)
	customStarted := make(chan struct{}, 1)
	customDone := make(chan struct{}, 1)
	add("custom reindent setup", func() {
		setup(original, language.JSON, textedit.Selection{Anchor: len(original)})
		base, _ := language.Builtin(language.JSON)
		definition := base.Definition()
		definition.Editing.Reindent = func(ctx context.Context, c language.ReindentContext) ([]language.LineIndent, error) {
			if c.Text != original || c.StartLine != 0 || c.EndLine != 5 {
				return nil, fmt.Errorf("custom reindent context mismatch")
			}
			return []language.LineIndent{{Line: 1, Text: "  "}}, nil
		}
		custom, err := language.New(definition)
		must(err)
		doc.SetLanguageDefinition(custom)
	}, nil)
	add("custom reindent", func() { invoke(source.CommandReindentSelection) }, textIs("{\n  \"items\": [\n1,\n2\n]\n}"))
	add("custom worker setup", func() {
		setup(original, language.JSON, textedit.Selection{Anchor: len(original)})
		base, _ := language.Builtin(language.JSON)
		definition := base.Definition()
		definition.Editing.Reindent = func(ctx context.Context, c language.ReindentContext) ([]language.LineIndent, error) {
			customStarted <- struct{}{}
			<-ctx.Done()
			customDone <- struct{}{}
			return []language.LineIndent{{Line: 1, Text: "       "}}, nil
		}
		custom, err := language.New(definition)
		must(err)
		doc.SetLanguageDefinition(custom)
	}, nil)
	add("custom worker running", func() { invoke(source.CommandReindentSelection) }, func() bool { return len(customStarted) == 1 })
	add("custom worker cancellation", func() { caret(0) }, func() bool { return len(customDone) == 1 })
	add("custom cancellation result", func() { check(doc.Model().Text() == original, "custom stale result applied") }, nil)
	add("reindent cancellation", func() {
		setup(original, language.JSON, textedit.Selection{Caret: len(original)})
		invoke(source.CommandReindentSelection)
		caret(0)
		must(win.CommitText("x"))
	}, textIs("x"+original))
	add("cancel stayed cancelled", func() { check(doc.Model().Text() == "x"+original, "late reindent applied") }, nil)
	for _, tc := range []struct {
		name, text, want string
		command          source.Command
		s                textedit.Selection
	}{
		{"join", "one\n  two", "one two", source.CommandJoinLines, textedit.Selection{}},
		{"trim", "a \t\nb\u00a0\t\n", "a\nb\u00a0\n", source.CommandTrimTrailingWhitespace, textedit.Selection{}},
		{"upper", "café", "CAFÉ", source.CommandUpperCase, textedit.Selection{Anchor: 5}},
		{"lower", "İABC", "iabc", source.CommandLowerCase, textedit.Selection{Caret: 5}},
	} {
		add(tc.name+" setup", func() { setup(tc.text, language.Plain, tc.s) }, nil)
		add(tc.name, func() { invoke(tc.command) }, textIs(tc.want))
		add(tc.name+" undo", func() {
			invoke(source.CommandUndo)
			check(doc.Model().Text() == tc.text && v.Selection() == tc.s, tc.name+" undo selection")
		}, nil)
	}
	add("paste setup", func() {
		setup("  ", language.JSON, textedit.Selection{Anchor: 2, Caret: 2})
		must(app.Clipboard().SetText("    {\n      \"x\": 1\n    }"))
	}, nil)
	add("paste indent", func() { invoke(source.CommandPasteReindented) }, textIs("  {\n    \"x\": 1\n  }"))
	add("paste undo", func() {
		invoke(source.CommandUndo)
		check(doc.Model().Text() == "  " && v.Selection().Caret == 2, "paste undo")
	}, nil)
	add("literal paste setup", func() {
		setup("  ", language.Go, textedit.Selection{Anchor: 2, Caret: 2})
		must(app.Clipboard().SetText("s := `a\n  literal`"))
	}, nil)
	add("literal paste", func() { invoke(source.CommandPasteReindented) }, textIs("  s := `a\n  literal`"))
	var smallFolds *source.Folds
	add("external folds setup", func() {
		setup(original, language.JSON, textedit.Selection{})
		smallFolds, err = source.NewFolds(original, []source.FoldRange{{StartLine: 0, EndLine: 5}, {StartLine: 1, EndLine: 4}})
		must(err)
		_, err = doc.SetFolds(doc.Model().Revision(), smallFolds)
		must(err)
	}, func() bool { return len(v.FoldingRegions()) == 2 && len(mirror.FoldingRegions()) == 2 })
	add("external folds republish", func() {
		check(v.Fold(1), "fold child")
		_, err = doc.SetFolds(doc.Model().Revision(), smallFolds)
		must(err)
	}, func() bool {
		a, b := v.FoldingRegions(), mirror.FoldingRegions()
		return len(a) == 2 && len(b) == 2 && a[1].Collapsed && !b[1].Collapsed
	})
	add("large setup", func() {
		v.SetDocument(large)
		mirror.SetDocument(source.NewDocument("Large snapshot: 100,000 fold candidates"))
		v.SetWrapMode(source.WrapNone)
		caret(0)
	}, nil)
	add("large fold publication", func() { _, err = large.SetFolds(large.Model().Revision(), largeFolds); must(err) }, func() bool {
		return v.Snapshot().Attributes["goui.sourceview"].(map[string]any)["foldRegionCount"] == 100000
	})
	add("large folded edit", func() {
		check(v.Fold(100000), "large middle fold")
		caret(0)
		must(win.CommitText("x"))
		f := v.FoldingRegions()
		check(len(f) == 99999 && f[49999].StartLine == 100000 && f[49999].Collapsed, "large edit mapping")
	}, nil)
	add("large undo", func() { invoke(source.CommandUndo); check(large.Model().Len() == len(largeText), "large undo") }, nil)
	add("large cancel publication", func() {
		_, err = large.SetFolds(large.Model().Revision(), largeFolds)
		must(err)
		must(win.CommitText("y"))
	}, nil)
	add("large cancellation result", func() { f := v.FoldingRegions(); check(len(f) == 99999, "stale large snapshot applied") }, nil)
	record := "{\n  \"text\": \"" + strings.Repeat("a", 10400) + "\"\n}"
	largeJSON := "[\n" + strings.Repeat(record+",\n", 999) + record + "\n]"
	add("large builtin JSON setup", func() {
		setup(largeJSON, language.JSON, textedit.Selection{})
		_, err = doc.SetFolds(doc.Model().Revision(), nil)
		must(err)
		mirror.SetDocument(source.NewDocument("Built-in JSON folding · 10 MiB"))
		v.SetWrapMode(source.WrapNone)
	}, nil)
	add("large builtin JSON analysis", func() {}, func() bool {
		return v.Snapshot().Attributes["goui.sourceview"].(map[string]any)["foldRegionCount"] == 1001
	})
	add("large builtin JSON edit", func() {
		must(win.CommitText(" "))
		check(doc.Model().Len() == len(largeJSON)+1, "large JSON input")
		invoke(source.CommandUndo)
		check(doc.Model().Len() == len(largeJSON), "large JSON undo")
	}, nil)
	largeTrim := strings.Repeat(strings.Repeat("x", 2500)+" \n", 4096)
	add("large trim setup", func() { setup(largeTrim, language.Plain, textedit.Selection{}); v.SetWrapMode(source.WrapNone) }, nil)
	add("large trim 4096 edits", func() { invoke(source.CommandTrimTrailingWhitespace) }, func() bool { return doc.Model().Len() == len(largeTrim)-4096 })
	add("large trim undo", func() { invoke(source.CommandUndo); check(doc.Model().Len() == len(largeTrim), "large trim undo") }, nil)
	add("batch limit setup", func() { setup(strings.Repeat("x \n", 4097), language.Plain, textedit.Selection{}); expectLimit = true }, nil)
	add("batch limit", func() { invoke(source.CommandTrimTrailingWhitespace) }, func() bool { return limitErrors == 1 })
	add("batch limit atomic", func() {
		check(doc.Model().Len() == 4097*3 && !doc.Model().CanUndo(), "over-limit operation changed text/history")
		expectLimit = false
	}, nil)
	add("long line setup", func() {
		setup(strings.Repeat("alpha ", 1800000), language.Plain, textedit.Selection{Anchor: 4, Caret: 0})
		v.SetWrapMode(source.WrapWordChar)
	}, nil)
	add("long line case", func() { invoke(source.CommandUpperCase) }, func() bool { s, _ := doc.Model().Slice(textedit.Range{End: 6}); return s == "ALPHa " })
	add("long line undo", func() {
		invoke(source.CommandUndo)
		s, _ := doc.Model().Slice(textedit.Range{End: 6})
		check(s == "alpha ", "long-line undo")
	}, nil)
	complete := false
	index := -1
	started := time.Now()
	last := started
	var gaps []time.Duration
	var elapsed []time.Duration
	var stepGaps []time.Duration
	var readyAt time.Duration
	timer := app.NewTimer()
	timer.ConnectTimeout(func() {
		now := time.Now()
		if index >= 0 {
			stepGaps = append(stepGaps, now.Sub(last))
		}
		gaps = append(gaps, now.Sub(last))
		last = now
		if index >= 0 {
			s := steps[index]
			if time.Since(started) > 30*time.Second {
				log.Fatalf("timeout: %s", s.name)
			}
			if readyAt == 0 && (s.ready == nil || s.ready()) {
				readyAt = time.Since(started)
			}
			if time.Since(started) < 250*time.Millisecond {
				return
			}
			if s.ready != nil && !s.ready() {
				return
			}
			elapsed = append(elapsed, time.Since(started))
			sort.Slice(stepGaps, func(i, j int) bool { return stepGaps[i] < stepGaps[j] })
			log.Printf("step=%q ready=%s settled=%s paints=%d heartbeat_max=%s", s.name, readyAt, time.Since(started), probe.paints, stepGaps[len(stepGaps)-1])
		}
		index++
		if index == len(steps) {
			sort.Slice(gaps, func(i, j int) bool { return gaps[i] < gaps[j] })
			log.Printf("PASS P0: typography=%s painter=%s scale=%g modern=%t dark=%t heartbeat_p95=%s max=%s", app.Typography().Name(), probe.painter, probe.scale, *modernFlag, *dark, gaps[len(gaps)*95/100], gaps[len(gaps)-1])
			must(app.Clipboard().SetText(""))
			complete = true
			timer.Stop()
			win.Destroy()
			return
		}
		s := steps[index]
		status.SetText(s.name)
		started = time.Now()
		stepGaps = nil
		readyAt = 0
		s.run()
		log.Printf("step=%q dispatch=%s", s.name, time.Since(started))
		probe.RequestPaint()
	})
	must(timer.Start(10 * time.Millisecond))
	app.Run()
	check(complete, "verification aborted")
}

type paintProbe struct {
	gui.WidgetBase
	scale   float32
	painter string
	paints  int
}

func (p *paintProbe) Measure(c layout.Constraint) layout.Measurement {
	return layout.Measured(c.Clamp(geometry.Size{Width: 1, Height: 1}))
}
func (p *paintProbe) Paint(draw gui.Painter) {
	p.paints++
	if p.scale != 0 {
		return
	}
	p.scale = draw.PixelScale()
	img, err := draw.NewImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	must(err)
	defer img.Destroy()
	p.painter = fmt.Sprintf("%T", img)
}
