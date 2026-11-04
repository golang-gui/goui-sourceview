// 外部语义分析验收：go run ./tests/window/editor/semantic [-dark]。
// 需要桌面、原生字体；Linux 使用独立 X11、合成器及 1/1.5 缩放。
// 初始为小型 Go 文件，分析器运行在控件外。自动通过 Window.DispatchEvent / CommitText
// 改变变量类型和结构体成员、请求并确认补全、快速连续输入，验证修订一致性。
// RenderWidget 在正文中统计指定语义前景的 NRGBA 实心像素（每类至少 5 个），
// 不依赖字形边缘抗锯齿或跨平台金图。确认外部着色确实应用到了绘制。
// 延迟发布独立颜色层必须保留原补全弹窗、选中项目；显式刷新须原位更新。
// 30 秒未完成、提前关闭或断言失败返回非零，成功自动退出。重启复位。
// 不执行源码，不修改文件、剪贴板或系统配置；关闭时取消后台分析。
package main

import (
	"context"
	"flag"
	"fmt"
	"image/color"
	"log"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/example/semantic/analysis"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/style"
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
func role(info gui.WidgetInfo, want gui.Role) gui.WidgetInfo {
	if info.Role == want {
		return info
	}
	for _, child := range info.Children {
		if found := role(child, want); found.Role == want {
			return found
		}
	}
	return gui.WidgetInfo{}
}
func main() {
	runtime.LockOSThread()
	dark := flag.Bool("dark", false, "dark appearance")
	flag.Parse()
	app, err := gui.NewApplication("org.golanggui.SourceViewSemanticTest")
	must(err)
	app.SetStyleSheet(style.Sheet(append(modern.Rules(modern.Options{Dark: *dark}), analysis.Rules(*dark)...)...))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 900, Height: 500}, Chrome: gui.WindowChromeIntegrated})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("Semantic interface verification"))
	const sample = "package main\ntype Person struct { Name string; Age int }\nfunc main(){\n value := \"hello\"\n person := Person{}\n println(value)\n println(person.Name)\n}\n"
	doc := source.NewDocument(sample)
	must(doc.SetLanguage(language.Go))
	v := source.NewSourceView()
	v.SetDocument(doc)
	keys := v.KeyBindings()
	keys[source.CommandComplete] = []gui.KeyGesture{{Key: gui.KeyF2}}
	must(v.SetKeyBindings(keys))
	published := 0
	var result analysis.Result
	service := analysis.NewService(app.Post, func(r analysis.Result, err error) {
		must(err)
		check(r.Snapshot.DocumentID == doc.ID() && r.Snapshot.Revision == doc.Model().Revision(), "过期结果进入 GUI")
		applied, err := doc.SetHighlights(r.Snapshot.Revision, r.Highlights)
		must(err)
		check(applied, "当前颜色快照未应用")
		applied, err = doc.SetFolds(r.Snapshot.Revision, r.Folds)
		must(err)
		check(applied, "当前折叠快照未应用")
		result = r
		published++
	})
	defer service.Close()
	win.ConnectDestroy(service.Close)
	var filtered atomic.Bool
	provider := service.Provider()
	original := provider.Complete
	provider.Complete = func(ctx context.Context, c source.CompletionContext) ([]source.Completion, error) {
		items, err := original(ctx, c)
		if filtered.Load() {
			check(c.Explicit && c.Trigger == "", "刷新没有保留手动请求语义")
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
			for _, item := range items {
				if item.Label == "Score" {
					return []source.Completion{item}, err
				}
			}
			return nil, err
		}
		return items, err
	}
	v.SetCompletionProvider(provider)
	v.ConnectError(func(err error) { panic(err) })
	submit := func(delay time.Duration) {
		service.Submit(analysis.Snapshot{DocumentID: doc.ID(), Revision: doc.Model().Revision(), Text: doc.Model().Text()}, delay)
	}
	analysisDelay := 150 * time.Millisecond
	v.ConnectChange(func(textedit.Change) { submit(analysisDelay) })
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(v)
	scroll.SetMainWeight(1)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	header := gui.NewHeaderBar()
	header.SetChild(gui.NewLabel("External syntax/types → colors and completion"))
	box.AddChild(header)
	box.AddChild(scroll)
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(v)
	submit(0)
	key := func(k events.Key) {
		must(win.DispatchEvent(events.KeyEvent{EventType: events.KeyDown, Key: k}))
		must(win.DispatchEvent(events.KeyEvent{EventType: events.KeyUp, Key: k}))
	}
	replace := func(old, text string) {
		at := strings.Index(doc.Model().Text(), old)
		check(at >= 0, "找不到替换片段")
		v.SetSelection(textedit.Selection{Anchor: at, Caret: at + len(old)})
		must(win.CommitText(text))
	}
	completeMember := func() {
		at := strings.Index(doc.Model().Text(), "println(person.") + len("println(person.")
		v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
		key(events.KeyF2)
	}
	labels := func() []string {
		if len(win.Popovers()) == 0 {
			return nil
		}
		info := role(win.Popovers()[0].Widget().Snapshot(), "completion-list")
		if info.Attributes["sourceview.completion.pending"] == true {
			return nil
		}
		var out []string
		for _, row := range info.Children {
			out = append(out, row.Text)
		}
		return out
	}
	has := func(items []string, value string) bool {
		for _, item := range items {
			if item == value {
				return true
			}
		}
		return false
	}
	countColor := func(want color.NRGBA) int {
		img, err := gui.RenderWidget(v, 1)
		must(err)
		n := 0
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				if color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA) == want {
					n++
				}
			}
		}
		return n
	}
	pink, blue := color.NRGBA{0xd5, 0x2a, 0x91, 255}, color.NRGBA{0x2d, 0x7e, 0xea, 255}
	if *dark {
		pink, blue = color.NRGBA{0xf0, 0x7a, 0xc4, 255}, color.NRGBA{0x69, 0xa7, 0xff, 255}
	}
	ready := func() bool { return published > 0 && result.Snapshot.Revision == doc.Model().Revision() }
	stage, targetPublications := 0, 0
	var kept gui.Popover
	done := false
	deadline := time.Now().Add(30 * time.Second)
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), fmt.Sprintf("阶段 %d 超时", stage))
		switch stage {
		case 0:
			if !ready() || countColor(pink) < 5 {
				return
			}
			replace(`"hello"`, "42")
		case 1:
			if !ready() || countColor(blue) < 5 {
				return
			}
			check(countColor(pink) == 0, "旧语义颜色未被替换")
			completeMember()
		case 2:
			items := labels()
			if len(items) == 0 {
				return
			}
			check(len(items) == 2 && items[0] == "Age" && items[1] == "Name", "成员补全不是类型检查结果")
			key(events.KeyEnter)
			check(strings.Contains(doc.Model().Text(), "println(person.Age)"), "确认补全没有替换整个标识符")
		case 3:
			if !ready() {
				return
			}
			analysisDelay = 800 * time.Millisecond
			replace("Age int", "Score int")
			completeMember()
			targetPublications = published + 1
		case 4:
			items := labels()
			if len(items) == 0 {
				return
			}
			check(has(items, "Score") && !has(items, "Age"), "修改类型后补全未更新")
			key(events.KeyArrowDown)
			kept = win.Popovers()[0]
		case 5:
			if published < targetPublications {
				return
			}
			check(len(win.Popovers()) == 1 && win.Popovers()[0] == kept, "延迟颜色关闭或替换了补全弹窗")
			filtered.Store(true)
			must(v.RefreshCompletion())
			must(v.RefreshCompletion()) // 合并取消中的请求，只保留最新一项。
			check(role(kept.Widget().Snapshot(), "completion-list").Attributes["sourceview.completion.pending"] == true, "刷新未标记等待状态")
			before := doc.Model().Text()
			key(events.KeyEnter)
			check(doc.Model().Text() == before, "等待时提交了旧候选")
			analysisDelay = 150 * time.Millisecond
		case 6:
			if len(labels()) == 0 {
				return
			}
			check(len(win.Popovers()) == 1 && win.Popovers()[0] == kept, "刷新重建了弹窗")
			check(len(labels()) == 1 && labels()[0] == "Score", "外部索引更新没有替换候选")
			key(events.KeyEnter)
			check(strings.Contains(doc.Model().Text(), "println(person.Score)"), "刷新没有保留已选中的 Score")
			filtered.Store(false)
			replace("42", `"done"`)
			replace(`"done"`, "99")
		case 7:
			if !ready() || countColor(blue) < 5 {
				return
			}
			check(countColor(pink) == 0, "旧异步着色覆盖了新修订")
			submit(2 * time.Second)
			must(v.RefreshCompletion())
			check(len(win.Popovers()) == 0, "无会话刷新打开了弹窗")
			done = true
			log.Printf("PASS semantic colors, member completion, type updates, revision guards, retained popup and refreshed selection; dark=%v", *dark)
			win.Destroy()
			return
		}
		stage++
	})
	must(timer.Start(100 * time.Millisecond))
	app.Run()
	service.Close()
	service.Wait()
	check(done, "窗口提前关闭")
}
