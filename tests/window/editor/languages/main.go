// 内置语言集成验证，需要桌面、窗口管理器、原生排版及字体。
// 启动：go run ./tests/window/editor/languages [-dark]；Linux 无 IME 服务时设置
// -language zig 等参数只运行该语言的补全/引用场景；省略时验收全部场景。
// XMODIFIERS=@im=none。初始为一个可编辑视图，程序自动切换文档，经正常事件路径
// 触发补全、方向键选中、Enter 接受、撤销，再输入引用与括号并断言文本和光标。
// Java/C#/Kotlin/Swift/Dart 三引号不能多插闭合引用，Rust 生命周期不能补成字符引用。
// Swift/R 动态原始引用手工输入完整定界符，Dart 原始引用尾反斜杠不转义闭合引号。
// 另检查只读拒绝、语义文本/选区；全部断言完成后自动关闭，提前关闭或失败则非零退出。
// 补全逐个等待已分配的可视区域，最多 8 秒；Show 返回不能代替布局完成。
// 记录实际 typography/painter/scale；颜色另有原生位图断言，不能用窗口启动代替像素验收。
// 不访问剪贴板、文件或设置；重新启动复位。真实 IME 和系统快捷键仍需手工验证。
package main

import (
	"flag"
	"fmt"
	"image"
	"log"
	"runtime"
	"time"
	"unicode/utf8"

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
func completionInfo(info gui.WidgetInfo) gui.WidgetInfo {
	if info.Role == "completion-list" {
		return info
	}
	for _, child := range info.Children {
		if found := completionInfo(child); found.Role != "" {
			return found
		}
	}
	return gui.WidgetInfo{}
}

func main() {
	runtime.LockOSThread()
	dark := flag.Bool("dark", false, "Modern dark appearance")
	filter := flag.String("language", "", "run completion and quote cases for one language ID")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewLanguages")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 720, Height: 420}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView language integration"))
	v := source.NewSourceView()
	bindings := v.KeyBindings()
	bindings[source.CommandComplete] = []gui.KeyGesture{{Key: gui.KeyF2}}
	must(v.SetKeyBindings(bindings))
	v.SetAutomaticCompletion(false)
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(v)
	scroll.SetMainWeight(1)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(gui.NewLabel("Built-in languages — completion, quotes and history"))
	box.AddChild(scroll)
	probe := &scaleProbe{}
	box.AddChild(probe)
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(v)
	primary, _ := gui.ModPrimary.Resolve()
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	fixtures := []struct {
		id         language.ID
		text, want string
	}{
		{language.TOML, "enabled = tr", "enabled = true"},
		{language.YAML, "enabled: nu", "enabled: null"},
		{language.Shell, "pri", "printf"},
		{language.JSON, "tr", "true"}, {language.XML, "<x xml:l", "<x xml:lang"}, {language.HTML, "<di", "<div"},
		{language.JSONC, "tr", "true"}, {language.HTTP, "GE", "GET"},
		{language.JSONLines, "{\ntr", "{\ntrue"},
		{language.Makefile, "incl", "include"}, {language.CMake, "proj", "project"},
		{language.Nginx, "serv", "server"}, {language.INI, "ye", "yes"},
		{language.Dockerfile, "FRO", "FROM"}, {language.SQL, "SEL", "SELECT"},
		{language.Protobuf, "mes", "message"}, {language.Go, "pack", "package"},
		{language.Python, "whil", "while"}, {language.JavaScript, "cons", "const"},
		{language.TypeScript, "interf", "interface"}, {language.CSS, ".x { backg", ".x { background"},
		{language.C, "retu", "return"}, {language.Cpp, "conste", "constexpr"},
		{language.Java, "synch", "synchronized"}, {language.Rust, "matc", "match"},
		{language.CSharp, "namesp", "namespace"}, {language.Lua, "funct", "function"},
		{language.PowerShell, "funct", "function"},
		{language.Ruby, "unles", "unless"},
		{language.PHP, "<?php retu", "<?php return"},
		{language.JSX, "cons", "const"}, {language.TSX, "interf", "interface"},
		{language.JSX, "const x=<di", "const x=<div"}, {language.TSX, "const x=<A classN", "const x=<A className"},
		{language.Kotlin, "overr", "override"},
		{language.Swift, "filepr", "fileprivate"},
		{language.Dart, "requi", "required"},
		{language.R, "funct", "function"},
		{language.Zig, "comp", "comptime"},
		{language.ObjectiveC, "@interf", "@interface"},
		{language.ObjectiveCpp, "namesp", "namespace"},
		{language.Scala, "giv", "given"},
		{language.Perl, "packa", "package"},
	}
	var steps []func()
	var pendingViewport func() bool
	var viewportDeadline time.Time
	matched := 0
	for _, tc := range fixtures {
		if *filter != "" && string(tc.id) != *filter {
			continue
		}
		matched++
		steps = append(steps, func() {
			d := source.NewDocument(tc.text)
			must(d.SetLanguage(tc.id))
			v.SetDocument(d)
			v.SetSelection(textedit.Selection{Anchor: len(tc.text), Caret: len(tc.text)})
			win.SetFocusedWidget(v)
		})
		steps = append(steps, func() {
			key(events.KeyF2, 0)
			check(len(win.Popovers()) == 1, "missing completion for "+string(tc.id))
			viewportDeadline = time.Now().Add(8 * time.Second)
			pendingViewport = func() bool {
				check(len(win.Popovers()) == 1, "completion closed before layout for "+string(tc.id))
				info := completionInfo(win.Popovers()[0].Widget().Snapshot())
				return info.Bounds.Width > 0 && info.Bounds.Height > 0
			}
		})
		steps = append(steps, func() {
			info := completionInfo(win.Popovers()[0].Widget().Snapshot())
			found := -1
			for i, row := range info.Children {
				if row.Text == tc.want[len(tc.text)-len(completionPrefix(tc.id, tc.text)):] {
					found = i
					break
				}
			}
			check(found >= 0, "missing expected proposal for "+string(tc.id))
			for i := 0; i < found; i++ {
				key(events.KeyArrowDown, 0)
			}
			key(events.KeyEnter, 0)
			check(v.Document().Model().Text() == tc.want, "wrong completion for "+string(tc.id))
			key(events.KeyZ, primary)
			check(v.Document().Model().Text() == tc.text, "completion undo failed")
			log.Printf("PASS %s completion and undo", tc.id)
		})
	}
	check(matched > 0, "selected language has no completion case")
	for _, tc := range []struct {
		id                  language.ID
		prefix, typed, want string
	}{
		{language.TOML, "text = ", `"""`, `text = """`},
		{language.YAML, "text: ", "'it''s'", "text: 'it''s'"},
		{language.Shell, "echo ", "'\\'", "echo '\\'"},
		{language.Python, "text = ", `"""`, `text = """`},
		{language.Go, "text := ", "`raw`", "text := `raw`"},
		{language.Java, "String text = ", `"""`, `String text = """`},
		{language.CSharp, "var text = ", `"""`, `var text = """`},
		{language.Rust, "fn get<", "'a", "fn get<'a"},
		{language.PowerShell, "$x = ", "'it''s'", "$x = 'it''s'"},
		{language.Ruby, "x = ", "'it\\'s'", "x = 'it\\'s'"},
		{language.PHP, "<?php $x = ", "'it\\'s'", "<?php $x = 'it\\'s'"},
		{language.JSX, "const x=<A title=", "\"C:\\\"", "const x=<A title=\"C:\\\""},
		{language.JSX, "const x=<A>text ", "'", "const x=<A>text '"},
		{language.Kotlin, "val s = ", `"""`, `val s = """`},
		{language.Swift, "let s = ", `"""`, `let s = """`},
		{language.Swift, "let s = #", `"raw"#`, `let s = #"raw"#`},
		{language.Dart, "var s = ", "'''", "var s = '''"},
		{language.Dart, "var s = ", "r'\\'", "var s = r'\\'"},
		{language.R, "x <- ", `r"--(raw)--"`, `x <- r"--(raw)--"`},
		{language.R, "x <- ", "'it\\'s'", "x <- 'it\\'s'"},
		{language.Zig, "const s = ", "\"a\\\"b\"", "const s = \"a\\\"b\""},
		{language.ObjectiveC, "id s = ", `@"hello"`, `id s = @"hello"`},
		{language.ObjectiveCpp, "id s = ", `@"hello"`, `id s = @"hello"`},
		{language.Scala, "val s = ", `"""`, `val s = """`},
		{language.Scala, "val code = ", "'{true}", "val code = '{true}"},
		{language.Perl, "my $s = ", "'it\\'s'", "my $s = 'it\\'s'"},
		{language.PowerShell, "$x = ", "\"a`\"b\"", "$x = \"a`\"b\""},
		{language.PowerShell, "$x = ", "\"C:\\\"", "$x = \"C:\\\""},
	} {
		if *filter != "" && string(tc.id) != *filter {
			continue
		}
		steps = append(steps, func() {
			d := source.NewDocument(tc.prefix)
			must(d.SetLanguage(tc.id))
			v.SetDocument(d)
			v.SetSelection(textedit.Selection{Anchor: len(tc.prefix), Caret: len(tc.prefix)})
		})
		for _, r := range tc.typed {
			steps = append(steps, func() { must(win.CommitText(string(r))) })
		}
		steps = append(steps, func() {
			check(v.Document().Model().Text() == tc.want && v.Selection().Caret == len(tc.want), "quote typing failed: "+string(tc.id))
			log.Printf("PASS %s quote typing", tc.id)
		})
	}
	steps = append(steps, func() {
		d := source.NewDocument("")
		must(d.SetLanguage(language.PowerShell))
		v.SetDocument(d)
		v.SetSelection(textedit.Selection{})
	})
	steps = append(steps, func() {
		must(win.CommitText(`"`))
		must(win.CommitText("a`"))
		must(win.CommitText(`"`))
		check(v.Document().Model().Text() == "\"a`\"\"" && v.Selection().Caret == 4, "escaped PowerShell quote consumed generated closer")
	})
	steps = append(steps, func() {
		d := source.NewDocument("")
		must(d.SetLanguage(language.Shell))
		v.SetDocument(d)
		v.SetSelection(textedit.Selection{})
	})
	steps = append(steps, func() {
		must(win.CommitText("{"))
		check(v.Document().Model().Text() == "{}" && v.Selection().Caret == 1, "bracket insertion failed")
		v.SetReadOnly(true)
		check(win.CommitText("x") != nil, "readonly accepted text commit")
		check(v.Document().Model().Text() == "{}", "readonly edited text")
		v.SetReadOnly(false)
		check(v.Snapshot().TextEditing != nil, "missing editing semantics")
	})
	complete := false
	at := 0
	timer := app.NewTimer()
	timer.ConnectTimeout(func() {
		if pendingViewport != nil {
			if !pendingViewport() {
				check(time.Now().Before(viewportDeadline), "completion viewport was not allocated within 8 seconds")
				return
			}
			pendingViewport = nil
		}
		if at < len(steps) {
			steps[at]()
			at++
			return
		}
		check(probe.scale > 0, "painter not ready")
		log.Printf("PASS language integration; typography=%s painter=%s scale=%g dark=%t", app.Typography().Name(), probe.painter, probe.scale, *dark)
		complete = true
		win.Destroy()
	})
	must(timer.Start(160 * time.Millisecond))
	app.Run()
	check(complete, "language integration aborted")
}
func completionPrefix(id language.ID, text string) string {
	l, ok := language.Builtin(id)
	check(ok, "unknown language")
	at := len(text)
	for at > 0 {
		r, n := utf8.DecodeLastRuneInString(text[:at])
		if !l.IsWord(r) {
			break
		}
		at -= n
	}
	return text[at:]
}

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
