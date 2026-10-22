// Eight-language editor UI verification. Requires a desktop/window manager,
// native typography and fonts. Run: go run ./tests/window/editor/localization
// (Linux without IME service: XMODIFIERS=@im=none). -capture-dir accepts an
// existing directory outside the repository. Initial state: one editor with an
// optional FindBar. Key input uses Window.DispatchEvent/CommitText; setters only
// prepare state and choose languages/themes. Automatically checks all eight
// languages against literal testdata, including an already open context menu,
// input names, hints, completed counts, focus, selection/history preservation,
// long menu labels, narrow/restored FindBar and detach/reattach. Menu navigation
// and execution use normal keys. No clipboard or system settings changes.
// Captures are observational, not pixel goldens. Native IME, physical keys,
// fractional/mixed DPI, RTL mirroring and mother-tongue review remain manual.
// Restart to reset; closes on success, exits nonzero on failure or early close.
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

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/mlang"
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
}

func main() {
	runtime.LockOSThread()
	out := flag.String("capture-dir", "", "existing temporary directory for PNGs")
	flag.Parse()
	var languages []translation
	must(json.Unmarshal(fixture, &languages))
	check(len(languages) == 8, "missing language fixtures")
	for _, entry := range languages {
		check(len(entry.Menu) == 16, "missing menu fixtures")
	}
	previous := mlang.Language()
	defer func() { must(mlang.SetLanguage(previous)) }()
	must(mlang.SetLanguage("en"))
	app, err := gui.NewApplication("org.golang-gui.SourceViewLocalization")
	must(err)
	app.SetStyleSheet(modern.Sheet(modern.Options{}))
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
	bar := source.NewFindBar()
	bar.SetTarget(v)
	overlay.AddOverlay(bar)
	overlay.ConnectOverlayPosition(bar, func(available, size geometry.Size, p *geometry.Point) {
		p.X = max(0, available.Width-size.Width-8)
	})
	win.SetWidget(overlay)
	must(win.Show())
	primary, _ := gui.ModPrimary.Resolve()
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
	}
	openReplace := func() {
		if runtime.GOOS == "darwin" {
			alt, _ := gui.ModAlt.Resolve()
			key(events.KeyF, primary|alt)
		} else {
			key(events.KeyH, primary)
		}
	}
	find := named(bar, "Find").(*gui.TextView)
	replacement := named(bar, "Replacement").(*gui.TextView)
	var selected textedit.Selection
	var inputSelected gui.TextSelection
	var revision uint64
	var text string
	query := source.SearchQuery{Text: "one", IgnoreCase: true, WholeWord: true}
	assertState := func() {
		check(v.Document().Model().Text() == text && uint64(v.Document().Model().Revision()) == revision && v.Document().Model().CanUndo() && !v.Document().Model().CanRedo(), "language change altered document/history")
		check(v.Selection() == selected && v.Search() == query && bar.Replacement() == "replacement 中" && bar.Opened() && bar.ReplaceMode(), "language change altered selection/search/replacement")
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
	complete := false
	timer := app.NewTimer()
	defer timer.Stop()
	timer.ConnectTimeout(func() {
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
			phase++
		case 4:
			must(mlang.SetLanguage(languages[index].Tag))
			phase++
		case 5:
			entry := languages[index]
			assertBar(entry)
			check(len(win.Popovers()) == 1 && win.Popovers()[0] == menu, "language change recreated menu")
			var labels []string
			var states []bool
			var selectAllSelected bool
			walk(menu.Widget(), func(w gui.Widget) {
				info := w.Snapshot()
				if info.Role == gui.RoleMenuItem && info.Visible {
					labels = append(labels, info.Text)
					states = append(states, info.Enabled)
					if info.Text == entry.Menu[11] {
						selectAllSelected = info.Selected
					}
				}
				if label, ok := w.(*gui.Label); ok && label.Visible() {
					width := label.Measure(layout.Unbounded()).Width
					check(label.Rect().Width+0.5 >= width, "translated menu label was clipped")
				}
			})
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
				overlay.RemoveOverlay(bar)
				must(mlang.SetLanguage("ja"))
				check(replacement.Name() == entry.Replacement, "detached bar retained language listener")
				overlay.AddOverlay(bar)
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
			phase++
		case 10:
			assertBar(languages[4])
			complete = true
			log.Printf("PASS localization history/focus/menu bounds/reattach/narrow/theme; typography=%s", app.Typography().Name())
			win.Destroy()
		}
	})
	must(timer.Start(160 * time.Millisecond))
	app.Run()
	check(complete, "localization verification aborted")
	must(mlang.SetLanguage("ar"))
	check(replacement.Name() == languages[4].Replacement, "destroyed bar retained language listener")
}
