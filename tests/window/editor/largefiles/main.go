// Medium/large-file functional verification. Requires a desktop/window manager,
// native typography and fonts. Run: go run ./tests/window/editor/largefiles
// -language json -mib 10. Also use xml/html and 50 MiB; 100 MiB JSON is a probe.
// Linux without IME: XMODIFIERS=@im=none. -data-dir may name an existing temporary
// directory outside the repository; generated files are deleted on normal exit.
// Optional -capture-dir writes observational PNGs to an existing temporary directory.
// Initial state is an empty editor, closed FindBar and default Modern Light.
// The current synchronous ReadFile/SetText path loads a generated real file.
// Normal Window.DispatchEvent/CommitText exercises document edges, Unicode
// insert/delete, history, line edits/indent, sparse search/replace, readonly,
// shared views, remount, wrapping and theme changes. Public setters only prepare
// scenarios. Automatic completion is disabled to isolate these operations.
// Content is compared in full against original file slices plus independently
// specified edits; visible excerpts and head/middle/tail native syntax colors
// have assertions. Timings include assertions and are observations, not latency
// or memory acceptance thresholds. Each wait is bounded; early close fails.
// Independent display failures are collected so remaining content checks run;
// any failed assertion still makes the final exit status nonzero.
// No clipboard/system settings changes. Restart to reset. Real IME, physical
// shortcuts, very long lines and mixed/fractional DPI need separate verification.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui-sourceview/language"
	"github.com/golang-gui/goui-sourceview/scheme"
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/layout"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/typography"
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

type expectedEdit struct {
	start, end int
	text       string
}
type step struct {
	name string
	run  func() bool
}

func named(w gui.Widget, name string) gui.Widget {
	if w.Name() == name {
		return w
	}
	for _, child := range w.Children() {
		if found := named(child, name); found != nil {
			return found
		}
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Print("FAIL ", err)
		os.Exit(1)
	}
}

func run() (result error) {
	runtime.LockOSThread()
	currentStep := "setup"
	defer func() {
		if p := recover(); p != nil {
			result = fmt.Errorf("%s: %v", currentStep, p)
		}
	}()
	idFlag := flag.String("language", "json", "json/xml/html")
	mib := flag.Int("mib", 10, "approximate file size in MiB, 1..100")
	dir := flag.String("data-dir", "", "existing temporary directory for generated files")
	out := flag.String("capture-dir", "", "existing temporary directory for observational PNGs")
	flag.Parse()
	check(*mib >= 1 && *mib <= 100, "mib must be in 1..100")
	id := language.ID(*idFlag)
	data, err := generateSample(*dir, id, *mib<<20)
	must(err)
	defer os.Remove(data.path)
	app, err := gui.NewApplication("org.golang-gui.SourceViewLargeFiles")
	must(err)
	ctx := &observedTypography{Context: app.Typography(), lines: make(map[string]*observedLine)}
	sourceTimerTicks := 0
	gui.App = observedApplication{Application: app, context: ctx, timerTicks: &sourceTimerTicks}
	app.SetStyleSheet(modern.Sheet(modern.Options{}))
	win, err := app.NewWindow(&gui.WindowOptions{Size: geometry.Size{Width: 1000, Height: 620}})
	must(err)
	defer win.Destroy()
	must(win.SetTitle(fmt.Sprintf("SourceView large files — %s / %d MiB", id, *mib)))
	doc := source.NewDocument("")
	v := source.NewSourceView()
	v.ConnectError(func(err error) { panic(err) })
	v.SetDocument(doc)
	v.SetAcceptsTab(true)
	v.SetAutomaticCompletion(false)
	scroll := gui.NewScrollView()
	scroll.SetStyleName("source-view")
	scroll.SetChild(v)
	overlay := gui.NewOverlay()
	overlay.SetMainWeight(1)
	overlay.SetChild(scroll)
	var bar gui.Widget
	row := gui.NewLinearBox(layout.DirectionHorizontal)
	row.SetCrossAlign(layout.CrossStretch)
	row.SetMainWeight(1)
	row.AddChild(overlay)
	box := gui.NewLinearBox(layout.DirectionVertical)
	box.SetCrossAlign(layout.CrossStretch)
	box.AddChild(row)
	box.AddChild(&paintProbe{})
	win.SetWidget(box)
	must(win.Show())
	key := func(k events.Key, mods events.Modifiers) {
		for _, kind := range []events.EventType{events.KeyDown, events.KeyUp} {
			must(win.DispatchEvent(events.KeyEvent{EventType: kind, Key: k, Modifiers: mods}))
		}
		bar = panel(v, "sourceview-find-bar")
	}
	primary, _ := gui.ModPrimary.Resolve()
	alt, _ := gui.ModAlt.Resolve()
	undo := func() { win.SetFocusedWidget(v); key(events.KeyZ, primary) }
	redo := func() { win.SetFocusedWidget(v); key(events.KeyZ, primary|events.ModifierShift) }
	edge := func(end bool) {
		k := events.KeyHome
		if end {
			k = events.KeyEnd
		}
		if runtime.GOOS == "darwin" {
			k = events.KeyArrowUp
			if end {
				k = events.KeyArrowDown
			}
		}
		key(k, primary)
	}
	click := func(w gui.Widget) {
		check(w != nil, "missing pointer target")
		r := w.Snapshot().Bounds
		p := geometry.Point{X: r.X + r.Width/2, Y: r.Y + r.Height/2}
		for _, kind := range []events.EventType{events.PointerDown, events.PointerUp} {
			must(win.DispatchEvent(events.PointerEvent{EventType: kind, Position: p, Button: events.PointerButtonLeft}))
		}
	}
	var original string
	lines := 0
	assertContent := func(edits ...expectedEdit) {
		model := doc.Model()
		at, previous, wantLines := 0, 0, lines
		part := func(want string) {
			for start := 0; start < len(want); {
				end := min(len(want), start+(1<<20))
				for end < len(want) && !utf8.RuneStart(want[end]) {
					end--
				}
				got, err := model.Slice(textedit.Range{Start: at, End: at + end - start})
				must(err)
				check(got == want[start:end], fmt.Sprintf("content differs at byte %d", at))
				at += end - start
				start = end
			}
		}
		for _, edit := range edits {
			check(edit.start >= previous && edit.end >= edit.start, "invalid independent edit expectation")
			part(original[previous:edit.start])
			part(edit.text)
			wantLines += strings.Count(edit.text, "\n") - strings.Count(original[edit.start:edit.end], "\n")
			previous = edit.end
		}
		part(original[previous:])
		check(model.Len() == at && model.LineCount() == wantLines, "length or logical line count differs")
	}
	assertVisible := func(view *source.SourceView, at int, marker string) {
		info := view.Snapshot().TextEditing
		check(info != nil, "visible editing snapshot unavailable")
		check(info.VisibleRange.Start <= at && info.VisibleRange.End >= at && strings.Contains(info.VisibleText, marker), fmt.Sprintf("caret/marker not in visible excerpt: expected=%d marker=%q range=%+v selection=%+v bounds=%+v", at, marker, info.VisibleRange, info.Selection, view.Snapshot().Bounds))
		text, err := doc.Model().Slice(textedit.Range{Start: info.VisibleRange.Start, End: info.VisibleRange.End})
		must(err)
		check(text == info.VisibleText && info.Length == doc.Model().Len(), "visible excerpt does not match model")
	}
	assertSyntax := func(marker string) bool {
		for text, line := range ctx.lines {
			if !strings.Contains(text, marker) {
				continue
			}
			needle, delta, kind := "\"row\"", 1, language.KindProperty
			if id == language.XML {
				needle, delta, kind = "<row", 1, language.KindTag
			}
			if id == language.HTML {
				needle, delta, kind = "<p", 1, language.KindTag
			}
			at := strings.Index(text, needle) + delta
			if at >= delta && at < len(line.colors) && line.colors[at] == scheme.Light().Color(kind) {
				if *out != "" {
					img, err := gui.RenderWidget(overlay, 0)
					must(err)
					f, err := os.Create(filepath.Join(*out, fmt.Sprintf("%s-%d-%s.png", id, *mib, marker)))
					must(err)
					err = png.Encode(f, img)
					closeErr := f.Close()
					must(err)
					must(closeErr)
				}
				return true
			}
		}
		return false
	}
	var steps []step
	var failures []string
	frames, changedFrame := 0, 0
	win.ConnectFrame(func(time.Time) { frames++ })
	layoutReady := func(view *source.SourceView) bool {
		meta, _ := view.Snapshot().Attributes["goui.sourceview"].(map[string]any)
		pending, _ := meta["layoutPending"].(bool)
		if pending || frames < changedFrame+2 {
			return false
		}
		return true
	}
	// Display assertions do not mutate the model. Keep their failure status
	// while allowing independent remaining scenarios to run.
	recordDisplay := func(name string, fn func()) {
		defer func() {
			if p := recover(); p != nil {
				message := fmt.Sprintf("%s: %v", name, p)
				failures = append(failures, message)
				log.Print("FAIL display ", message)
			}
		}()
		fn()
	}
	add := func(name string, fn func()) { steps = append(steps, step{name, func() bool { fn(); return true }}) }
	wait := func(name string, fn func() bool) { steps = append(steps, step{name, fn}) }
	afterLayout := func(name string, view *source.SourceView, fn func()) {
		wait(name, func() bool {
			if !layoutReady(view) {
				return false
			}
			fn()
			return true
		})
	}
	add("read file and mount", func() {
		start := time.Now()
		bytes, err := os.ReadFile(data.path)
		must(err)
		original = string(bytes)
		lines = strings.Count(original, "\n") + 1
		doc.Model().SetText(original)
		must(doc.SetLanguage(id))
		win.SetFocusedWidget(v)
		assertContent()
		check(!doc.Model().CanUndo(), "initial load created undo history")
		log.Printf("LOADED language=%s bytes=%d lines=%d read/model/verification=%s typography=%s", id, len(original), lines, time.Since(start), app.Typography().Name())
	})
	add("edit without waiting for highlight", func() {
		v.SetSelection(textedit.Selection{})
		must(win.CommitText(" "))
		assertContent(expectedEdit{0, 0, " "})
	})
	add("undo early edit", func() { undo(); assertContent() })
	for slot, marker := range markers {
		at := data.positions[slot]
		add("reveal "+marker, func() { v.SetSelection(textedit.Selection{Anchor: at, Caret: at}); v.RevealSelection() })
		wait("visible and syntax "+marker, func() bool {
			if !layoutReady(v) {
				return false
			}
			assertVisible(v, at, marker)
			return assertSyntax(marker)
		})
		insert := "中文 é 👩‍💻 "
		add("type "+marker, func() {
			must(win.CommitText(insert))
			assertContent(expectedEdit{at, at, insert})
			check(v.Selection().Caret == at+len(insert), "Unicode insertion caret")
		})
		add("undo "+marker, func() { undo(); assertContent(); check(v.Selection().Caret == at, "undo insertion caret") })
		add("redo "+marker, func() { redo(); assertContent(expectedEdit{at, at, insert}) })
		add("restore "+marker, func() { undo(); assertContent(); key(events.KeyDelete, 0); assertContent(expectedEdit{at, at + 1, ""}) })
		add("undo delete "+marker, func() { undo(); assertContent() })
	}
	add("document end key", func() { edge(true); check(v.Selection().Caret == len(original), "end shortcut") })
	afterLayout("end visible", v, func() {
		assertVisible(v, len(original), "")
		check(scroll.ScrollY() > 0, "end did not scroll")
		edge(false)
		check(v.Selection().Caret == 0, "home shortcut")
	})
	add("page navigation", func() {
		key(events.KeyPageDown, 0)
		check(v.Selection().Caret > 0, "PageDown did not navigate")
		key(events.KeyPageUp, 0)
	})
	mid := data.positions[1]
	add("move middle line down", func() {
		v.SetSelection(textedit.Selection{Anchor: mid, Caret: mid})
		key(events.KeyArrowDown, alt)
		swapped := original[data.middleEnd:data.followingEnd] + original[data.middleStart:data.middleEnd]
		assertContent(expectedEdit{data.middleStart, data.followingEnd, swapped})
	})
	add("undo line move", func() { undo(); assertContent() })
	add("duplicate middle line", func() {
		v.SetSelection(textedit.Selection{Anchor: mid, Caret: mid})
		mods := gui.ModAlt | gui.ModShift
		if runtime.GOOS == "linux" {
			mods |= gui.ModControl
		}
		resolved, ok := mods.Resolve()
		check(ok, "duplicate modifiers unavailable")
		key(events.KeyArrowDown, resolved)
		assertContent(expectedEdit{data.middleEnd, data.middleEnd, original[data.middleStart:data.middleEnd]})
	})
	add("undo line duplicate", func() { undo(); assertContent() })
	add("delete middle line", func() {
		v.SetSelection(textedit.Selection{Anchor: mid, Caret: mid})
		key(events.KeyK, primary|events.ModifierShift)
		assertContent(expectedEdit{data.middleStart, data.middleEnd, ""})
	})
	add("undo line delete", func() { undo(); assertContent() })
	indentEnd := 0
	add("indent one hundred lines", func() {
		indentEnd = data.middleStart
		for range 100 {
			at := strings.IndexByte(original[indentEnd:], '\n')
			check(at >= 0, "missing indent fixture")
			indentEnd += at + 1
		}
		v.SetSelection(textedit.Selection{Anchor: data.middleStart, Caret: indentEnd})
		key(events.KeyTab, 0)
		indented := "    " + strings.ReplaceAll(original[data.middleStart:indentEnd-1], "\n", "\n    ") + "\n"
		assertContent(expectedEdit{data.middleStart, indentEnd, indented})
	})
	add("undo whole indent once", func() { undo(); assertContent() })
	add("open find", func() {
		v.SetSelection(textedit.Selection{})
		key(events.KeyF, primary)
		check(panelVisible(v, "sourceview-find-bar") && named(bar, "Find").Focused(), "find shortcut")
		key(events.KeyA, primary)
		must(win.CommitText(searchText))
	})
	wait("completed find count", func() bool {
		state := bar.Snapshot().Attributes["sourceview.find"].(map[string]any)
		if state["countComplete"] != true {
			return false
		}
		check(state["count"] == 3, "sparse search count")
		return true
	})
	var matches []int
	add("find next and previous", func() {
		for at := 0; ; {
			next := strings.Index(original[at:], searchText)
			if next < 0 {
				break
			}
			at += next
			matches = append(matches, at)
			at += len(searchText)
		}
		check(len(matches) == 3, "independent search fixture")
		for _, at := range matches {
			key(events.KeyEnter, 0)
			check(v.Selection().Range() == (textedit.Range{Start: at, End: at + len(searchText)}), "find next range")
		}
		key(events.KeyEnter, events.ModifierShift)
		check(v.Selection().Range().Start == matches[1], "find previous range")
		key(events.KeyEscape, 0)
		check(v.Focused(), "close find focus")
		v.SetSelection(textedit.Selection{})
		key(events.KeyF3, 0)
	})
	add("open replace", func() {
		if runtime.GOOS == "darwin" {
			key(events.KeyF, primary|alt)
		} else {
			key(events.KeyH, primary)
		}
		check(named(bar, "Replacement").Visible(), "replace shortcut")
		win.SetFocusedWidget(named(bar, "Replacement"))
		must(win.CommitText("REPLACED"))
		key(events.KeyEnter, 0)
		assertContent(expectedEdit{matches[0], matches[0] + len(searchText), "REPLACED"})
	})
	add("undo one replacement", func() {
		undo()
		assertContent()
		redo()
		assertContent(expectedEdit{matches[0], matches[0] + len(searchText), "REPLACED"})
		undo()
		assertContent()
	})
	add("replace all", func() {
		click(named(bar, "Replace all"))
		var edits []expectedEdit
		for _, at := range matches {
			edits = append(edits, expectedEdit{at, at + len(searchText), "REPLACED"})
		}
		assertContent(edits...)
	})
	add("undo all once", func() { undo(); assertContent(); v.HidePanel(); v.SetSearch(source.SearchQuery{}) })
	add("readonly input and history", func() {
		v.SetReadOnly(true)
		revision := doc.Model().Revision()
		check(win.CommitText("blocked") != nil, "readonly accepted text")
		key(events.KeyDelete, 0)
		key(events.KeyZ, primary)
		check(doc.Model().Revision() == revision, "readonly changed revision")
		assertContent()
		v.SetReadOnly(false)
	})
	second := source.NewSourceView()
	second.ConnectError(func(err error) { panic(err) })
	second.SetDocument(doc)
	second.SetReadOnly(true)
	otherScroll := gui.NewScrollView()
	otherScroll.SetStyleName("source-view")
	otherScroll.SetMainWeight(1)
	otherScroll.SetChild(second)
	add("mount second view", func() {
		row.AddChild(otherScroll)
		second.SetSelection(textedit.Selection{Anchor: data.positions[2], Caret: data.positions[2]})
		second.RevealSelection()
	})
	afterLayout("new shared tail visible", second, func() {
		recordDisplay("new shared tail", func() { assertVisible(second, data.positions[2], markers[2]) })
	})
	add("shared view edit", func() {
		v.SetSelection(textedit.Selection{Anchor: mid, Caret: mid})
		win.SetFocusedWidget(v)
		must(win.CommitText("共享"))
		assertContent(expectedEdit{mid, mid, "共享"})
		check(second.Selection().Caret == data.positions[2]+len("共享"), "shared selection mapping")
	})
	afterLayout("shared snapshot", second, func() {
		recordDisplay("shared tail after edit", func() { assertVisible(second, data.positions[2]+len("共享"), markers[2]) })
		undo()
		assertContent()
		check(second.Selection().Caret == data.positions[2], "shared undo mapping")
		row.RemoveChild(otherScroll)
		otherScroll.SetChild(nil)
	})
	add("detach and remount", func() {
		v.SetSelection(textedit.Selection{Anchor: mid, Caret: mid})
		scroll.SetChild(nil)
		scroll.SetChild(v)
		win.SetFocusedWidget(v)
		v.RevealSelection()
	})
	afterLayout("remounted selection", v, func() {
		check(v.Selection().Caret == mid, "remount lost selection")
		recordDisplay("remounted middle", func() { assertVisible(v, mid, markers[1]) })
	})
	var topBeforeWrap int
	add("preserve top anchor while wrapping", func() {
		topBeforeWrap = v.Snapshot().TextEditing.VisibleRange.Start
		v.SetWrapMode(source.WrapWordChar)
	})
	afterLayout("wrapped content", v, func() {
		check(v.Selection().Caret == mid, "wrap changed selection")
		recordDisplay("wrapped top anchor", func() {
			check(v.Snapshot().TextEditing.VisibleRange.Start == topBeforeWrap, "wrap lost top logical line anchor")
		})
		assertContent()
		app.SetStyleSheet(modern.Sheet(modern.Options{Dark: true}))
	})
	afterLayout("dark content and restore wrap", v, func() {
		recordDisplay("theme top anchor", func() {
			check(v.Snapshot().TextEditing.VisibleRange.Start == topBeforeWrap, "theme lost top logical line anchor")
		})
		assertContent()
		v.SetWrapMode(source.WrapNone)
	})
	afterLayout("final restored content", v, func() {
		recordDisplay("restored top anchor", func() {
			check(v.Snapshot().TextEditing.VisibleRange.Start == topBeforeWrap, "restoring wrap lost top logical line anchor")
		})
		assertContent()
	})
	index := 0
	started := time.Now()
	var lastWait time.Time
	timer := app.NewTimer()
	defer timer.Stop()
	complete := false
	timer.ConnectTimeout(func() {
		currentStep = steps[index].name
		if time.Since(started) > 120*time.Second {
			panic("step timeout")
		}
		before := len(failures)
		if !steps[index].run() {
			if time.Since(started) > 5*time.Second && time.Since(lastWait) > 5*time.Second {
				info := v.Snapshot()
				var visibleRange any
				if info.TextEditing != nil {
					visibleRange = info.TextEditing.VisibleRange
				}
				log.Printf("WAIT step=%q layout=%v range=%+v caret=%d native_markers=%d frames=%d source_timer_ticks=%d", currentStep, info.Attributes["goui.sourceview"], visibleRange, v.Selection().Caret, len(ctx.lines), frames, sourceTimerTicks)
				lastWait = time.Now()
			}
			return
		}
		if len(failures) == before {
			log.Printf("PASS step=%q elapsed=%s", currentStep, time.Since(started))
		}
		index++
		changedFrame = frames
		started = time.Now()
		if index == len(steps) {
			complete = true
			log.Printf("COMPLETE large-file language=%s mib=%d bytes=%d lines=%d steps=%d failures=%d", id, *mib, len(original), lines, len(steps), len(failures))
			win.Destroy()
		}
	})
	must(timer.Start(140 * time.Millisecond))
	app.Run()
	check(complete, "verification aborted")
	if len(failures) != 0 {
		return fmt.Errorf("%d display assertion(s) failed", len(failures))
	}
	return nil
}

type observedApplication struct {
	gui.Application
	context    typography.Context
	timerTicks *int
}

func (a observedApplication) Typography() typography.Context { return a.context }

func (a observedApplication) NewTimer() *gui.Timer {
	timer := a.Application.NewTimer()
	timer.ConnectTimeout(func() { *a.timerTicks++ })
	return timer
}

type observedLine struct {
	typography.TextLayout
	colors []color.NRGBA
}

func (l *observedLine) SetTextColor(start, length int, c color.Color) {
	for i := start; i < start+length; i++ {
		l.colors[i] = color.NRGBAModel.Convert(c).(color.NRGBA)
	}
	l.TextLayout.SetTextColor(start, length, c)
}

type observedTypography struct {
	typography.Context
	lines map[string]*observedLine
}

func (c *observedTypography) NewTextLayout(text string, f typography.TextFormat, w, h float32) (typography.TextLayout, error) {
	l, err := c.Context.NewTextLayout(text, f, w, h)
	if err != nil {
		return nil, err
	}
	tracked := false
	for _, marker := range markers {
		tracked = tracked || strings.Contains(text, marker)
	}
	if !tracked {
		return l, nil
	}
	line := &observedLine{TextLayout: l, colors: make([]color.NRGBA, len(text))}
	c.lines[text] = line
	l.ConnectDestroy(func() {
		if c.lines[text] == line {
			delete(c.lines, text)
		}
	})
	return line, nil
}

type paintProbe struct {
	gui.WidgetBase
	reported bool
}

func (p *paintProbe) Measure(c layout.Constraint) layout.Measurement {
	return layout.Measured(c.Clamp(geometry.Size{Width: 1, Height: 1}))
}
func (p *paintProbe) Paint(draw gui.Painter) {
	if p.reported {
		return
	}
	p.reported = true
	img, err := draw.NewImage(image.NewRGBA(image.Rect(0, 0, 1, 1)))
	must(err)
	log.Printf("actual painter=%T scale=%g", img, draw.PixelScale())
	img.Destroy()
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
