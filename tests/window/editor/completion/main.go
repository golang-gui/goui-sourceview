// Completion panel verification. Requires a desktop/window manager and native
// fonts. Run: go run ./tests/window/editor/completion [-dark] [-case CASE]
// [-auto]. CASE: json, html, xml, long, unicode, scroll, wrapped, edge, navigation.
// navigation 必须带 -auto，验证词内左右移动复用弹窗、候选恢复、词边界与选区关闭、
// 嵌入语言确认、慢 Provider 的取消和过期结果、请求尚未显示时移动以及选区回调换文档。
// -basic selects Basic instead of Modern. -expect-surface transparent/opaque
// asserts the native surface mode; on X11 run opaque without a compositor to
// verify the alpha-unavailable fallback. Otherwise leave the expectation unset.
// Linux without an IME service: XMODIFIERS=@im=none. Initial state: an editable
// sample with a completion popup. Inspect aligned name/detail columns, muted
// smaller descriptions, prefix colors, a 1-DIP outline and ellipsis. Check
// the independently rounded selected background (Modern 4 DIP, Basic 2 DIP),
// its 2-DIP horizontal / 1-DIP vertical inset and clipped partially visible rows.
// light/dark and 1x/2x. Scroll has 32 items and a real draggable scrollbar; edge puts
// the owner near the workarea corner, where the popup must remain on screen.
// Up/Down, hover, wheel, Enter/Tab and clicking work without stealing editor
// focus. -auto checks semantic row bounds, stable width while scrolling,
// editor hover retention, keyboard/hover selection, independent pixel/line
// scrolling, thumb dragging to the last item, keyboard reveal, inert padding,
// pointer acceptance and undo via
// normal Window/Popover.DispatchEvent; READY means setup, not pixel acceptance.
// Native IME and physical shortcut interception require separate verification.
// 在 8 秒期限内等待弹窗完成首帧，再保存实际分配；原生 SizeEvent 可能把请求的
// DIP 尺寸按物理像素取整，首帧前的临时几何不能作为宽度稳定性的基准。
// No clipboard, files or settings change; edge moves only this program's window.
// Restart to reset. Close to exit; closing early with -auto is a failure.
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"log"
	"math"
	"runtime"
	"strings"
	"time"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
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
func selected(info gui.WidgetInfo) int {
	for i, row := range info.Children {
		if row.Selected {
			return i
		}
	}
	return -1
}
func role(info gui.WidgetInfo, wanted gui.Role) gui.WidgetInfo {
	if info.Role == wanted {
		return info
	}
	for _, child := range info.Children {
		if found := role(child, wanted); found.Role != "" {
			return found
		}
	}
	return gui.WidgetInfo{}
}

func candidates(popup gui.Popover) gui.WidgetInfo {
	return role(popup.Widget().Snapshot(), "completion-list")
}

// ScrollView's semantic snapshot omits structural bars. Locate the real
// public control through the ordinary widget tree for pointer verification.
func verticalBar(w gui.Widget) *gui.ScrollBar {
	if bar, ok := w.(*gui.ScrollBar); ok && bar.Visible() && bar.Orientation() == layout.DirectionVertical {
		return bar
	}
	for _, child := range w.Children() {
		if found := verticalBar(child); found != nil {
			return found
		}
	}
	return nil
}
func main() {
	runtime.LockOSThread()
	dark := flag.Bool("dark", false, "dark code scheme")
	scenario := flag.String("case", "json", "json/html/xml/long/unicode/scroll/wrapped/edge/navigation")
	auto := flag.Bool("auto", false, "verify input and close automatically")
	basicTheme := flag.Bool("basic", false, "use the official Basic sheet")
	surface := flag.String("expect-surface", "", "assert transparent or opaque popup creation")
	flag.Parse()
	check(*surface == "" || *surface == "transparent" || *surface == "opaque", "invalid expected surface")
	text, prefix, id := "{\n    \"value\": tr", "tr", language.JSON
	switch *scenario {
	case "json", "long", "scroll":
	case "navigation":
		check(*auto, "navigation requires -auto")
		text, prefix, id = "trade trace true\ntruffle tail", "tr", language.Plain
	case "html":
		text, prefix, id = "<di", "di", language.HTML
	case "xml":
		text, prefix, id = "<node xml:", "xml:", language.XML
	case "unicode":
		text, prefix, id = "中", "中", language.Plain
	case "wrapped":
		text = "tr" + strings.Repeat("a", 100)
		prefix = text
	case "edge":
		text = strings.Repeat("\n", 8) + strings.Repeat(" ", 40) + "tr"
	default:
		log.Fatalf("unknown case %q", *scenario)
	}
	app, err := gui.NewApplication("org.golang-gui.SourceViewCompletionPreview")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *dark}))
	if *basicTheme {
		app.SetStyleSheet(basic.Sheet())
	}
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 700, Height: 340}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("GOUI SourceView completion preview"))
	doc := source.NewDocument(text)
	must(doc.SetLanguage(id))
	v := source.NewSourceView()
	bindings := v.KeyBindings()
	bindings[source.CommandComplete] = []gui.KeyGesture{{Key: gui.KeyF2}}
	must(v.SetKeyBindings(bindings))
	v.SetDocument(doc)
	v.SetAcceptsTab(true)

	if *scenario == "wrapped" {
		v.SetWrapMode(source.WrapChar)
	}
	var items []source.Completion
	switch *scenario {
	case "html":
		items = []source.Completion{{Label: "dialog", Detail: "HTML"}, {Label: "dir", Detail: "HTML"}}
	case "wrapped":
		items = []source.Completion{{Label: prefix + "Value", Detail: "A wrapped identifier"}, {Label: prefix + "Variant", Detail: "Another wrapped identifier"}}
	case "json", "edge":
		items = []source.Completion{{Label: "transform", Detail: "Apply a transformation to the current value"}, {Label: "translate", Detail: "Translate a value"}, {Label: "trim", Detail: "Remove whitespace"}, {Label: "truncate", Detail: "Limit the length of text"}}
	case "long":
		items = []source.Completion{{Label: "tr" + strings.Repeat("ansformation", 20), Detail: strings.Repeat("A long description 中文 e\u0301 ", 30)}, {Label: "trim", Detail: "Remove whitespace"}}
	case "unicode":
		items = []source.Completion{{Label: "中文标识符", Detail: "中文说明 · e\u0301 · 👩‍💻"}, {Label: "中" + strings.Repeat("文标识符", 20), Detail: "שלום · multilingual description"}}
	case "scroll":
		for i := 0; i < 32; i++ {
			items = append(items, source.Completion{Label: fmt.Sprintf("transform%02d", i), Detail: "document word"})
		}
		items[31].Detail = "A considerably longer description outside the first page"
	}
	if items != nil {
		v.SetCompletionProvider(&source.CompletionProvider{Complete: func(context.Context, source.CompletionContext) ([]source.Completion, error) { return items, nil }})
	}
	caret := len(text)
	if *scenario == "navigation" {
		caret = strings.Index(text, "truffle") + len(prefix)
	}
	v.SetSelection(textedit.Selection{Anchor: caret, Caret: caret})
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(v)
	scroll.SetMainWeight(1)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(gui.NewLabel("Completion · " + *scenario + "   F2 · ↑/↓ · Enter/Tab · Esc"))
	box.AddChild(scroll)
	probe := &scaleProbe{}
	box.AddChild(probe)
	win.SetWidget(box)
	must(win.Show())
	win.SetFocusedWidget(v)
	if *scenario == "edge" {
		area, err := win.WorkAreaAt(geometry.Point{})
		must(err)
		must(win.SetPosition(nil, geometry.Point{X: max(0, area.Width-720), Y: max(0, area.Height-360)}))
	}
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	primary, _ := gui.ModPrimary.Resolve()
	if *scenario == "navigation" {
		verifyNavigation(app, win, v, doc, key, probe, *dark)
		return
	}
	var popup gui.Popover
	var observed gui.Popover
	popupFrames := 0
	var initialWidth float32
	complete := false
	var startAuto func()
	opened := app.NewTimer()
	opened.ConnectTimeout(func() {
		v.RevealSelection()
		key(events.KeyF2, 0)
	})
	must(opened.StartOnce(750 * time.Millisecond))
	ready := app.NewTimer()
	defer ready.Stop()
	deadline := time.Now().Add(8 * time.Second)
	ready.ConnectTimeout(func() {
		if len(win.Popovers()) == 0 {
			check(time.Now().Before(deadline), "completion did not open")
			return
		}
		popup = win.Popovers()[0]
		check(popup != nil && win.Focused() && v.Focused() && probe.scale > 0, "native popup/focus/painter not ready")
		if observed != popup {
			observed, popupFrames = popup, 0
			popup.ConnectFrame(func(time.Time) { popupFrames++ })
			must(popup.RequestPaint())
			return
		}
		// ConnectFrame 在绘制之前通知；下一个定时器回调再读取已完成帧的几何。
		if popupFrames == 0 {
			check(time.Now().Before(deadline), "completion did not paint its viewport")
			return
		}
		check(!popup.Modal() && popup.StyleName() == "sourceview.completion", "completion changed keyboard policy or lost shell style")
		if *surface != "" {
			check(popup.Transparent() == (*surface == "transparent"), "unexpected transparent/opaque fallback")
		}
		info := candidates(popup)
		if info.Bounds.Width <= 0 || info.Bounds.Height <= 0 {
			check(time.Now().Before(deadline), "completion was shown without a laid-out viewport")
			return
		}
		ready.Stop()
		check(info.Role == "completion-list" && len(info.Children) > 0 && selected(info) >= 0, fmt.Sprintf("completion rows not ready: %+v; scroll=%+v", info, role(popup.Widget().Snapshot(), gui.RoleScrollView)))
		initialWidth = info.Bounds.Width
		if *scenario == "scroll" {
			for i := 0; i < 12; i++ {
				key(events.KeyArrowDown, 0)
			}
			info = candidates(popup)
			check(info.VisibleStart > 0 && info.Bounds.Width == initialWidth, "scroll changed width or hid selection")
		}
		log.Printf("READY case=%s dark=%t basic=%t transparent=%t typography=%s painter=%s scale=%g width=%g rows=%d first=%d", *scenario, *dark, *basicTheme, popup.Transparent(), app.Typography().Name(), probe.painter, probe.scale, initialWidth, len(info.Children), info.VisibleStart)
		if startAuto != nil {
			startAuto()
		}
	})
	must(ready.Start(100 * time.Millisecond))
	if *auto {
		startAuto = func() {
			hover := app.NewTimer()
			hover.ConnectTimeout(func() {
				b := v.Snapshot().Bounds
				must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerMove, Position: geometry.Point{X: b.X + 40, Y: b.Y + 40}}))
			})
			must(hover.StartOnce(350 * time.Millisecond))
			leave := app.NewTimer()
			leave.ConnectTimeout(func() {
				check(len(win.Popovers()) == 1, "editor hover closed completion")
				must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerMove, Position: geometry.Point{X: 10, Y: 5}}))
			})
			must(leave.StartOnce(2000 * time.Millisecond))
			verify := app.NewTimer()
			verify.ConnectTimeout(func() {
				check(len(win.Popovers()) == 1, "editor hover leave closed completion")
				info := candidates(popup)
				check(info.Bounds.Width == initialWidth, "panel width changed after paint")
				// Padding is inside the native surface, but must not activate a candidate.
				for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
					must(popup.DispatchEvent(events.PointerEvent{EventType: kind, Button: events.PointerButtonLeft, Position: geometry.Point{X: popup.Widget().Snapshot().Bounds.X + 1, Y: info.Children[0].Bounds.Y + info.Children[0].Bounds.Height/2}}))
				}
				check(len(win.Popovers()) == 1 && doc.Model().Text() == text, "padding accepted a candidate")
				key(events.KeyArrowDown, 0)
				key(events.KeyArrowUp, 0)
				info = candidates(popup)
				before := selected(info)
				check(before >= 0, "keyboard selection disappeared")
				first := info.Children[0].Bounds
				point := geometry.Point{X: first.X + first.Width/2, Y: first.Y + first.Height/2}
				must(popup.DispatchEvent(events.PointerEvent{EventType: events.PointerMove, Position: point}))
				info = candidates(popup)
				check(selected(info) == 0, "hover selection failed")
				chosen := info.Children[0].Text
				rowHeight := info.Children[1].Bounds.Height
				sv := role(popup.Widget().Snapshot(), gui.RoleScrollView)
				must(popup.DispatchEvent(events.WheelEvent{Position: point, DeltaY: 7}))
				info = candidates(popup)
				check(selected(info) >= 0 && info.Children[selected(info)].Text == chosen, "wheel changed the selected candidate")
				check(sv.Role == gui.RoleScrollView, "completion did not reuse ScrollView")
				if *scenario == "scroll" {
					after := role(popup.Widget().Snapshot(), gui.RoleScrollView)
					check(after.ScrollY == sv.ScrollY+7, "pixel wheel ignored its magnitude")
					must(popup.DispatchEvent(events.WheelEvent{Position: point, DeltaY: 2, Mode: events.WheelDeltaLine}))
					after = role(popup.Widget().Snapshot(), gui.RoleScrollView)
					check(after.ScrollY == float32(math.Round(float64(sv.ScrollY+7+2*rowHeight))), "line wheel did not use candidate row height")
					must(popup.DispatchEvent(events.WheelEvent{Position: point, DeltaY: -100000}))
					control := verticalBar(popup.Widget())
					check(control != nil, "missing real scrollbar")
					bar := control.Snapshot().Bounds
					check(bar.Width > 0 && bar.Height > 0, "missing real scrollbar")
					grab := geometry.Point{X: bar.X + bar.Width/2, Y: bar.Y + 5}
					must(popup.DispatchEvent(events.PointerEvent{EventType: events.PointerDown, Button: events.PointerButtonLeft, Position: grab}))
					grab.Y = bar.Y + bar.Height - 1
					must(popup.DispatchEvent(events.PointerEvent{EventType: events.PointerMove, Buttons: events.PointerButtonLeftDown, Position: grab}))
					must(popup.DispatchEvent(events.PointerEvent{EventType: events.PointerUp, Button: events.PointerButtonLeft, Position: grab}))
					after = role(popup.Widget().Snapshot(), gui.RoleScrollView)
					check(after.ScrollY == float32(math.Floor(float64(after.MaxScrollY))), "thumb drag did not reach the last candidate")
					info = candidates(popup)
					check(info.VisibleEnd == 32 && info.Children[len(info.Children)-1].Text == "transform31", "drag lost final candidate")
					// Keyboard navigation resumes from the chosen item, independently
					// of manual browsing, and reveals that next item again.
					key(events.KeyArrowDown, 0)
					info = candidates(popup)
					expected := ""
					for i, item := range items {
						if item.Label == chosen {
							expected = items[(i+1)%len(items)].Label
						}
					}
					check(selected(info) >= 0 && info.Children[selected(info)].Text == expected, "scrolling changed keyboard selection or reveal")
				}
				target := info.Children[1]
				// The visual selection inset must not shrink the full row hit area.
				point = geometry.Point{X: target.Bounds.X + .5, Y: target.Bounds.Y + .5}
				check(v.Focused() && win.FocusedWidget() == v, "popup stole editor focus")
				for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
					must(popup.DispatchEvent(events.PointerEvent{EventType: kind, Button: events.PointerButtonLeft, Position: point}))
				}
				expected := text[:caret-len(prefix)] + target.Text
				check(len(win.Popovers()) == 0 && doc.Model().Text() == expected, "pointer completion inserted wrong text")
				key(events.KeyZ, primary)
				check(doc.Model().Text() == text, "completion undo failed")
				log.Print("PASS stable size, editor hover retention, keyboard/hover, pixel/line wheel, scrollbar drag, reveal, padding, pointer acceptance, focus and undo")
				complete = true
				win.Destroy()
			})
			must(verify.StartOnce(1550 * time.Millisecond))
		}
	}
	app.Run()
	if *auto && !complete {
		log.Fatal("verification aborted")
	}
}

// 所有交互均走窗口输入；模型、Provider 和选区 setter 只用于准备场景或回调重入。
func verifyNavigation(app gui.Application, win gui.Window, v *source.SourceView, doc *source.Document, key func(events.Key, events.Modifiers), probe *scaleProbe, dark bool) {
	original := doc.Model().Text()
	expectedFailure := fmt.Errorf("verification provider failure")
	failures := 0
	handle := v.ConnectError(func(err error) {
		check(err == expectedFailure, fmt.Sprintf("unexpected completion error: %v", err))
		failures++
	})
	defer handle.Disconnect()
	start := strings.Index(original, "truffle")
	primary, _ := gui.ModPrimary.Resolve()
	var popup gui.Popover
	var steps []func() bool
	next := func(f func()) { steps = append(steps, func() bool { f(); return true }) }
	ready := func() bool {
		if len(win.Popovers()) == 0 {
			return false
		}
		info := candidates(win.Popovers()[0])
		return len(info.Children) > 0 && info.Attributes["sourceview.completion.pending"] != true
	}
	labels := func() []string {
		var labels []string
		for _, row := range candidates(win.Popovers()[0]).Children {
			labels = append(labels, row.Text)
		}
		return labels
	}
	selectedText := func() string {
		info := candidates(win.Popovers()[0])
		i := selected(info)
		if i < 0 {
			return ""
		}
		return info.Children[i].Text
	}
	next(func() { key(events.KeyF2, 0) })
	steps = append(steps, ready)
	next(func() {
		popup = win.Popovers()[0]
		check(strings.Join(labels(), ",") == "trace,trade,true", "initial default candidates")
		key(events.KeyArrowDown, 0)
		key(events.KeyArrowDown, 0)
		check(selectedText() == "true", "candidate setup")
		key(events.KeyArrowRight, 0)
		check(len(win.Popovers()) == 1 && win.Popovers()[0] == popup && v.Selection().Caret == start+3, "right arrow closed/recreated completion or failed to move caret")
	})
	steps = append(steps, ready)
	next(func() {
		check(strings.Join(labels(), ",") == "true" && selectedText() == "true", "right arrow did not refilter")
		key(events.KeyArrowLeft, 0)
	})
	steps = append(steps, ready)
	next(func() {
		check(win.Popovers()[0] == popup && strings.Join(labels(), ",") == "trace,trade,true" && selectedText() == "true", "left arrow failed to restore candidates or selection")
		key(events.KeyArrowLeft, 0)
		key(events.KeyArrowLeft, 0)
	})
	steps = append(steps, ready)
	next(func() {
		check(win.Popovers()[0] == popup && v.Selection().Caret == start && len(labels()) == 4, "word-start completion did not retain empty prefix")
		key(events.KeyArrowLeft, 0)
		check(len(win.Popovers()) == 0, "leaving word range kept completion")
		v.SetSelection(textedit.Selection{Anchor: start + 2, Caret: start + 2})
		key(events.KeyF2, 0)
	})
	steps = append(steps, ready)
	next(func() {
		key(events.KeyArrowLeft, events.ModifierShift)
		check(len(win.Popovers()) == 0 && v.Selection().Anchor != v.Selection().Caret, "selection extension retained completion")
		v.SetSelection(textedit.Selection{Anchor: start + 2, Caret: start + 2})
		key(events.KeyF2, 0)
	})
	steps = append(steps, ready)
	next(func() {
		key(events.KeyArrowRight, 0)
		key(events.KeyArrowRight, 0)
		check(len(win.Popovers()) == 0 && doc.Model().Text() == original, "empty default result kept the popup or changed text")
		v.SetSelection(textedit.Selection{Anchor: start + 2, Caret: start + 2})
		key(events.KeyF2, 0)
	})
	steps = append(steps, ready)
	next(func() { key(events.KeyArrowRight, 0) })
	steps = append(steps, ready)
	next(func() {
		key(events.KeyEnter, 0)
		check(doc.Model().Text() == "trade trace true\ntrue tail" && len(win.Popovers()) == 0, "completion after movement used a stale edit range")
		key(events.KeyZ, primary)
		check(doc.Model().Text() == original && !doc.Model().CanUndo(), "completion after movement was not one undo step")
		must(doc.SetLanguage(language.Markdown))
		doc.Model().SetText("```json\ntr\n```")
		v.SetSelection(textedit.Selection{Anchor: 10, Caret: 10})
	})
	// 先让事件循环完成围栏行的增量词法状态；补全不对未知嵌入上下文猜测语言。
	next(func() { key(events.KeyF2, 0) })
	steps = append(steps, ready)
	next(func() { key(events.KeyArrowLeft, 0) })
	steps = append(steps, ready)
	next(func() {
		check(strings.Join(labels(), ",") == "true", "embedded JSON lost completion after movement")
		key(events.KeyEnter, 0)
		check(doc.Model().Text() == "```json\ntrue\n```", "embedded language completion did not commit")
	})
	requests := make(chan source.CompletionContext, 16)
	nextRequest := func() source.CompletionContext {
		select {
		case c := <-requests:
			return c
		default:
			log.Fatal("completed provider did not record its request")
			return source.CompletionContext{}
		}
	}
	gate := make(chan struct{})
	next(func() {
		must(doc.SetLanguage(language.Plain))
		doc.Model().SetText("word tail")
		v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
		v.SetCompletionProvider(&source.CompletionProvider{Complete: func(ctx context.Context, c source.CompletionContext) ([]source.Completion, error) {
			requests <- c
			if c.Prefix == "w" {
				<-gate // 故意不合作的旧请求，用于确认迟到数据仍被丢弃。
				return []source.Completion{{Label: "zebra"}, {Label: "alpha"}}, nil
			}
			return []source.Completion{{Label: "worker"}, {Label: "wonder"}}, nil
		}})
		key(events.KeyF2, 0)
	})
	steps = append(steps, ready)
	next(func() {
		popup = win.Popovers()[0]
		nextRequest()
		key(events.KeyArrowLeft, 0)
	})
	steps = append(steps, func() bool {
		select {
		case c := <-requests:
			check(c.Caret == 1 && c.Prefix == "w" && c.Range == (textedit.Range{Start: 0, End: 4}), "provider received stale cursor context")
			return true
		default:
			return false
		}
	})
	next(func() {
		check(len(win.Popovers()) == 1 && win.Popovers()[0] == popup, "pending refresh discarded popup")
		info := candidates(popup)
		check(info.Attributes["sourceview.completion.pending"] == true && !info.Children[0].Enabled && len(info.Children[0].Actions) == 0, "pending candidate exposed an action")
		key(events.KeyEnter, 0)
		check(doc.Model().Text() == "word tail" && win.Popovers()[0] == popup, "Enter committed a pending candidate")
		key(events.KeyArrowRight, 0)
		close(gate)
	})
	steps = append(steps, ready)
	next(func() {
		check(win.Popovers()[0] == popup && strings.Join(labels(), ",") == "worker,wonder", "late provider result replaced latest candidates")
		c := nextRequest()
		check(c.Caret == 2 && c.Prefix == "wo", "latest request was lost")
		key(events.KeyArrowLeft, 0)
	})
	steps = append(steps, ready)
	next(func() {
		check(win.Popovers()[0] == popup && strings.Join(labels(), ",") == "zebra,alpha", "editor filtered or reordered provider results")
		nextRequest()
		key(events.KeyEnter, 0)
		check(doc.Model().Text() == "zebra tail", "provider completion after movement used stale range")
		key(events.KeyZ, primary)
		check(doc.Model().Text() == "word tail", "provider completion undo")
	})
	next(func() {
		v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
		v.SetCompletionProvider(&source.CompletionProvider{Complete: func(ctx context.Context, c source.CompletionContext) ([]source.Completion, error) {
			if c.Prefix == "w" {
				return nil, expectedFailure
			}
			return []source.Completion{{Label: "worker"}}, nil
		}})
		key(events.KeyF2, 0)
	})
	steps = append(steps, ready)
	next(func() { key(events.KeyArrowLeft, 0) })
	steps = append(steps, func() bool {
		if failures == 0 {
			return false
		}
		check(failures == 1 && len(win.Popovers()) == 0 && doc.Model().Text() == "word tail", "provider failure kept a pending popup or was reported more than once")
		return true
	})
	initialGate := make(chan struct{})
	next(func() {
		v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
		v.SetCompletionProvider(&source.CompletionProvider{Complete: func(ctx context.Context, c source.CompletionContext) ([]source.Completion, error) {
			requests <- c
			if c.Prefix == "wo" {
				<-initialGate
				return []source.Completion{{Label: "stale"}}, nil
			}
			return []source.Completion{{Label: "latest"}}, nil
		}})
		key(events.KeyF2, 0)
	})
	steps = append(steps, func() bool {
		select {
		case <-requests:
			return true
		default:
			return false
		}
	})
	next(func() {
		check(len(win.Popovers()) == 0, "initial gated request unexpectedly displayed")
		key(events.KeyArrowLeft, 0)
		check(v.Selection().Caret == 1, "pending initial request swallowed cursor movement")
		close(initialGate)
	})
	steps = append(steps, ready)
	next(func() {
		check(strings.Join(labels(), ",") == "latest", "initial cancelled request was published")
		nextRequest()
		once := true
		h := v.ConnectSelection(func(textedit.Selection) {
			if !once {
				return
			}
			once = false
			v.SetDocument(source.NewDocument("keep"))
			v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
		})
		key(events.KeyArrowRight, 0)
		h.Disconnect()
		check(len(win.Popovers()) == 0 && v.Document().Model().Text() == "keep" && v.Selection().Caret == 2, "navigation callback state was overwritten")
	})
	index, done := 0, false
	deadline := time.Now().Add(10 * time.Second)
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), fmt.Sprintf("navigation step %d timed out; popups=%d text=%q", index, len(win.Popovers()), v.Document().Model().Text()))
		if index < len(steps) {
			if steps[index]() {
				index++
				deadline = time.Now().Add(10 * time.Second)
			}
			return
		}
		done = true
		check(failures == 1, "missing or duplicate completion error")
		log.Printf("PASS completion navigation, stable popup, refilter, selection, undo, embedded language, delayed/stale providers and reentry; typography=%s painter=%s scale=%g dark=%t", app.Typography().Name(), probe.painter, probe.scale, dark)
		win.Destroy()
	})
	must(timer.Start(100 * time.Millisecond))
	app.Run()
	check(done, "navigation verification aborted")
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
