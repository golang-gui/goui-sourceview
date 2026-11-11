// Eight-language editor UI verification. Requires a desktop/window manager,
// native typography and fonts. Run: go run ./tests/window/editor/localization
// (Linux without IME service: XMODIFIERS=@im=none). -capture-dir accepts an
// existing directory outside the repository. Initial state: one editor with an
// built-in find/replace UI. Key input uses Window.DispatchEvent/CommitText; setters only
// prepare state and choose languages/themes. Automatically checks all eight
// languages against literal testdata, including an already open context menu,
// input names, hints, completed counts, focus, selection/history preservation,
// long menu labels, narrow/restored FindBar and detach/reattach. Menu navigation
// and execution use normal keys. No clipboard or system settings changes.
// Captures are observational, not pixel goldens. Native IME, physical keys,
// mixed DPI, RTL mirroring and mother-tongue review remain manual.
// Restart to reset; closes on success, exits nonzero on failure or early close.
// Open-menu locale changes wait for completed frames with stable allocations;
// adding/removing the shortcut column may resize the native popup asynchronously.
// 另检查十个查找/替换按钮的八语言 Tooltip 及语义；用正常指针事件悬停图标子控件，
// 等待真实提示弹出，检查语言切换、关闭清理及定位栏关闭按钮，不抢编辑焦点。
// -basic/-dark 选择悬停提示场景的主题；另以 GOUI_PLAT_SCALE=1/1.5 运行。
package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	source "github.com/golang-gui/goui-sourceview/sourceview"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/mlang"
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
func walk(w gui.Widget, visit func(gui.Widget)) {
	visit(w)
	for _, child := range w.Children() {
		walk(child, visit)
	}
}
func named(w gui.Widget, name string) gui.Widget {
	var result gui.Widget
	walk(w, func(w gui.Widget) {
		if w.Name() == name {
			result = w
		}
	})
	return result
}

//go:embed testdata/languages.json
var fixture []byte

type translation struct {
	Tag, Find, Replacement, Replace, Count string
	Menu                                   []string
	Buttons                                []string
}

func main() {
	runtime.LockOSThread()
	out := flag.String("capture-dir", "", "existing temporary directory for PNGs")
	basicTheme := flag.Bool("basic", false, "Basic appearance for tooltip checks")
	darkTheme := flag.Bool("dark", false, "Modern dark appearance for tooltip checks")
	flag.Parse()
	var languages []translation
	must(json.Unmarshal(fixture, &languages))
	check(len(languages) == 8, "missing language fixtures")
	for _, entry := range languages {
		check(len(entry.Menu) == 16, "missing menu fixtures")
		check(len(entry.Buttons) == 10, "missing button tooltip fixtures")
	}
	previous := mlang.Language()
	defer func() { must(mlang.SetLanguage(previous)) }()
	must(mlang.SetLanguage("en"))
	app, err := gui.NewApplication("org.golang-gui.SourceViewLocalization")
	must(err)
	setTheme := func() {
		if *basicTheme {
			app.SetStyleSheet(basic.Sheet())
		} else {
			app.SetStyleSheet(modern.Sheet(modern.Options{Dark: *darkTheme}))
		}
	}
	setTheme()
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 850, Height: 540}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle("SourceView — eight-language UI"))
	v := source.NewSourceView()
	v.SetDocument(source.NewDocument("one ONE stone one\nlast"))
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(v)
	overlay := gui.NewOverlay()
	overlay.SetChild(scroll)
	var bar gui.Widget
	win.SetWidget(overlay)
	must(win.Show())
	primary, _ := gui.ModPrimary.Resolve()
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
		bar = panel(v, "sourceview-find-bar")
	}
	openReplace := func() {
		if runtime.GOOS == "darwin" {
			alt, _ := gui.ModAlt.Resolve()
			key(events.KeyF, primary|alt)
		} else {
			key(events.KeyH, primary)
		}
	}
	var find, replacement *gui.TextView
	var selected textedit.Selection
	var inputSelected gui.TextSelection
	var revision uint64
	var text string
	query := source.SearchQuery{Text: "one", IgnoreCase: true, WholeWord: true}
	assertState := func() {
		check(v.Document().Model().Text() == text && uint64(v.Document().Model().Revision()) == revision && v.Document().Model().CanUndo() && !v.Document().Model().CanRedo(), "language change altered document/history")
		check(v.Selection() == selected && v.Search() == query && replacement.Model().Text() == "replacement 中" && panelVisible(v, "sourceview-find-bar") && replacement.Visible(), "language change altered selection/search/replacement")
	}
	assertBar := func(entry translation) {
		assertState()
		check(find.Name() == entry.Find && replacement.Name() == entry.Replacement, "input names were not translated")
		check(replacement.Selection() == inputSelected, "language change altered input selection")
		var count bool
		walk(bar, func(w gui.Widget) {
			if w.StyleName() == "sourceview.find.status" {
				count = w.(*gui.Label).Text() == entry.Count
			}
		})
		check(find.Placeholder() == entry.Find && replacement.Placeholder() == entry.Replace && count, fmt.Sprintf("%s placeholders/count mismatch: %q/%q", entry.Tag, find.Placeholder(), replacement.Placeholder()))
		check(find.Snapshot().TextEditing.Placeholder == entry.Find && replacement.Snapshot().TextEditing.Placeholder == entry.Replace, "placeholder semantics were not translated")
		check(find.Rect().Width > 0 && replacement.Rect().Width > 0, "translated status displaced input")
		for _, text := range entry.Buttons {
			w := named(bar, text)
			check(w != nil && w.Tooltip() == text && w.Snapshot().Tooltip == text, fmt.Sprintf("%s 按钮 %q 的提示或语义不正确", entry.Tag, text))
		}
		state := bar.Snapshot().Attributes["sourceview.find"].(map[string]any)
		check(state["count"] == 3 && state["countComplete"] == true, "language change invalidated count")
	}
	capture := func(w gui.Widget, name string) {
		if *out == "" {
			return
		}
		img, err := gui.RenderWidget(w, 0)
		must(err)
		f, err := os.Create(filepath.Join(*out, name+".png"))
		must(err)
		must(png.Encode(f, img))
		must(f.Close())
	}
	phase, index := 0, 0
	scale := float32(1)
	var menu gui.Popover
	var enabled []bool
	menuFrames, previousMenuFrame := 0, 0
	var menuAllocation []geometry.Rectangle
	var tooltipFocus gui.Widget
	var tooltipStarted time.Time
	var tooltipTarget gui.Widget
	var tooltipBounds geometry.Rectangle
	hover := func(w gui.Widget) {
		check(w != nil && w.Visible(), "缺少悬停目标")
		// 按钮中心可以命中 Icon/Label；GOUI 必须向祖先查找提示。
		tooltipTarget, tooltipBounds = w, w.Snapshot().Bounds
		must(win.DispatchEvent(events.PointerEvent{EventType: events.PointerMove, Position: tooltipBounds.Center()}))
		tooltipFocus = win.FocusedWidget()
		tooltipStarted = time.Now()
	}
	tooltipReady := func() bool {
		if bounds := tooltipTarget.Snapshot().Bounds; bounds != tooltipBounds {
			// 译文改变计数宽度时按钮会移位；重新悬停同一按钮，而非等待旧坐标上的提示。
			check(win.FocusedWidget() == tooltipFocus, "提示重定位前输入焦点已改变")
			log.Printf("tooltip target moved: %q %v -> %v", tooltipTarget.Name(), tooltipBounds, bounds)
			hover(tooltipTarget)
			return false
		}
		return time.Since(tooltipStarted) >= time.Second
	}
	tooltip := func() (string, int) {
		var text string
		count := 0
		for _, p := range win.Popovers() {
			walk(p.Widget(), func(w gui.Widget) {
				if info := w.Snapshot(); info.Role == gui.RoleTooltip {
					text, count = info.Text, count+1
					check(!p.Modal() && !w.Focusable(), "提示取得了交互焦点")
				}
			})
		}
		return text, count
	}
	assertTooltip := func(want string) {
		text, count := tooltip()
		if want == "" {
			check(count == 0, "旧提示没有关闭")
		} else {
			check(count == 1 && text == want, fmt.Sprintf("提示=%q，数量=%d，期望=%q", text, count, want))
			check(win.FocusedWidget() == tooltipFocus, "提示抢走输入焦点")
		}
	}
	complete := false
	deadline := time.Now().Add(45 * time.Second)
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
		check(time.Now().Before(deadline), fmt.Sprintf("localization timeout phase=%d language=%d", phase, index))
		switch phase {
		case 0:
			img, err := gui.RenderWidget(overlay, 0)
			must(err)
			scale = float32(img.Bounds().Dx()) / overlay.Rect().Width
			win.SetFocusedWidget(v)
			v.SetSelection(textedit.Selection{Anchor: v.Document().Model().Len(), Caret: v.Document().Model().Len()})
			must(win.CommitText("!"))
			v.SetSelection(textedit.Selection{Caret: 3})
			openReplace()
			find = named(bar, "Find").(*gui.TextView)
			replacement = named(bar, "Replacement").(*gui.TextView)
			v.SetSearch(query)
			win.SetFocusedWidget(replacement)
			must(win.CommitText("replacement 中"))
			replacement.SetSelection(gui.TextSelection{Anchor: 2, Caret: 8})
			selected, inputSelected = v.Selection(), replacement.Selection()
			text, revision = v.Document().Model().Text(), uint64(v.Document().Model().Revision())
			phase++
		case 1:
			must(mlang.SetLanguage(languages[index].Tag))
			phase++
		case 2:
			entry := languages[index]
			assertBar(entry)
			check(win.FocusedWidget() == replacement, "language change stole input focus")
			capture(overlay, "find-"+entry.Tag)
			log.Printf("PASS find %s", entry.Tag)
			index++
			phase = 1
			if index == len(languages) {
				index, phase = 0, 3
				must(mlang.SetLanguage("en"))
				win.SetFocusedWidget(v)
				key(events.KeyF10, events.ModifierShift)
				key(events.KeyEnd, 0)
			}
		case 3:
			check(len(win.Popovers()) == 1, "context menu not open")
			menu = win.Popovers()[0]
			menu.ConnectFrame(func(time.Time) { menuFrames++ })
			phase++
		case 4:
			must(mlang.SetLanguage(languages[index].Tag))
			menuAllocation = nil
			must(menu.RequestPaint())
			phase++
		case 5:
			entry := languages[index]
			assertBar(entry)
			check(len(win.Popovers()) == 1 && win.Popovers()[0] == menu, "language change recreated menu")
			var labels []string
			var states []bool
			var selectAllSelected bool
			var allocation []geometry.Rectangle
			clipped := false
			walk(menu.Widget(), func(w gui.Widget) {
				info := w.Snapshot()
				if info.Role == gui.RoleMenuItem && info.Visible {
					labels = append(labels, info.Text)
					allocation = append(allocation, info.Bounds)
					states = append(states, info.Enabled)
					if info.Text == entry.Menu[11] {
						selectAllSelected = info.Selected
					}
				}
				if label, ok := w.(*gui.Label); ok && label.Visible() {
					width := label.Measure(layout.Unbounded()).Width
					clipped = clipped || label.Rect().Width+0.5 < width
					allocation = append(allocation, label.Snapshot().Bounds)
				}
			})
			must(menu.RequestPaint())
			if previousMenuFrame == menuFrames {
				return
			}
			previousMenuFrame = menuFrames
			stable := slices.Equal(menuAllocation, allocation)
			menuAllocation = allocation
			if !stable {
				return
			}
			check(!clipped, "translated menu label was clipped")
			check(slices.Equal(labels, entry.Menu), fmt.Sprintf("%s menu=%q want=%q", entry.Tag, labels, entry.Menu))
			check(selectAllSelected, "language change lost menu navigation")
			if enabled == nil {
				enabled = states
			} else {
				check(slices.Equal(enabled, states), "language change altered command availability")
			}
			capture(menu.Widget(), "menu-"+entry.Tag)
			log.Printf("PASS open menu %s", entry.Tag)
			index++
			phase = 4
			if index == len(languages) {
				key(events.KeyEnter, 0)
				check(v.Selection() == (textedit.Selection{Caret: len(text)}), "translated select-all command did not execute")
				v.SetSelection(selected)
				overlay.SetChild(nil)
				must(mlang.SetLanguage("ja"))
				check(replacement.Name() == entry.Replacement, "detached bar retained language listener")
				overlay.SetChild(scroll)
				must(v.ShowReplace())
				phase = 6
			}
		case 6:
			assertBar(languages[2])
			must(mlang.SetLanguage("fr"))
			must(win.DispatchEvent(events.SizeEvent{Width: 260, Height: 540, PixelWidth: 260 * scale, PixelHeight: 540 * scale}))
			phase++
		case 7:
			assertBar(languages[5])
			must(win.DispatchEvent(events.SizeEvent{Width: 850, Height: 540, PixelWidth: 850 * scale, PixelHeight: 540 * scale}))
			phase++
		case 8:
			assertBar(languages[5])
			check(named(bar, "Rechercher") == find && named(bar, "Texte de remplacement") == replacement, "resize/remount recreated input")
			app.SetStyleSheet(modern.Sheet(modern.Options{Dark: true}))
			phase++
		case 9:
			assertBar(languages[5])
			capture(overlay, "find-dark-fr")
			win.SetWidget(nil)
			must(mlang.SetLanguage("de"))
			check(replacement.Name() == languages[5].Replacement, "unmounted bar retained language listener")
			win.SetWidget(overlay)
			must(v.ShowReplace())
			phase++
		case 10:
			assertBar(languages[4])
			must(mlang.SetLanguage("en"))
			setTheme()
			phase++
		case 11:
			hover(named(bar, "Match case"))
			phase++
		case 12:
			if !tooltipReady() {
				return
			}
			assertTooltip("Match case")
			capture(win.Popovers()[0].Widget(), "tooltip-en")
			must(mlang.SetLanguage("zh-CN"))
			assertTooltip("")
			tooltipStarted = time.Now()
			phase++
		case 13:
			if !tooltipReady() {
				return
			}
			assertTooltip("区分大小写")
			capture(win.Popovers()[0].Widget(), "tooltip-zh")
			v.HidePanel()
			phase++
		case 14:
			assertTooltip("")
			must(v.ShowGoTo())
			phase++
		case 15:
			goTo := panel(v, "sourceview-goto-bar")
			check(goTo != nil, "定位栏未创建")
			hover(named(goTo, "关闭"))
			phase++
		case 16:
			if !tooltipReady() {
				return
			}
			assertTooltip("关闭")
			must(mlang.SetLanguage("de"))
			tooltipStarted = time.Now()
			phase++
		case 17:
			if !tooltipReady() {
				return
			}
			assertTooltip("Schließen")
			position := tooltipTarget.Snapshot().Bounds.Center()
			for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
				must(win.DispatchEvent(events.PointerEvent{EventType: kind, Position: position, Button: events.PointerButtonLeft}))
			}
			check(!panelVisible(v, "sourceview-goto-bar"), "提示拦截了关闭按钮点击")
			phase++
		case 18:
			assertTooltip("")
			complete = true
			log.Printf("PASS localization history/focus/menu bounds/reattach/narrow/theme/tooltips; typography=%s", app.Typography().Name())
			win.Destroy()
		}
	})
	must(timer.Start(160 * time.Millisecond))
	app.Run()
	check(complete, "localization verification aborted")
	must(mlang.SetLanguage("ar"))
	check(replacement.Name() == languages[4].Replacement, "destroyed bar retained language listener")
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
