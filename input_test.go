package sourceview

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
	"github.com/golang-gui/goui/platform/typography"
)

// 指针命中只读取已排版的几何，不为侧栏或尚未显示的占位标记创建排版。
func TestPointerRegionsUseCachedFoldGeometry(t *testing.T) {
	v := foldingFixture(t)
	if !v.Fold(1) {
		t.Fatal("fold fixture")
	}
	v.LayoutVisible(v.viewport, v.offset)
	button := v.foldButtonRect(1)
	center := func(r geometry.Rectangle) geometry.Point {
		return geometry.Point{X: r.X + r.Width/2, Y: r.Y + r.Height/2}
	}
	if got := v.cursorAt(center(button)); got != gui.CursorPointing {
		t.Fatal("fold toggle cursor", got)
	}
	p := v.cachedParagraph(1)
	if p == nil || v.cursorAt(center(v.foldBadgeRect(1, p))) != gui.CursorPointing {
		t.Fatal("collapsed badge cursor")
	}
	if got := v.cursorAt(geometry.Point{X: 2, Y: button.Y + button.Height/2}); got != gui.CursorDefault {
		t.Fatal("line number cursor", got)
	}
	if got := v.cursorAt(geometry.Point{X: v.gutter - 2, Y: v.viewport.Height - 2}); got != gui.CursorDefault {
		t.Fatal("empty fold column cursor", got)
	}
	v.SetCursor(gui.CursorCrosshair)
	if got := v.cursorAt(geometry.Point{X: v.gutter + 200, Y: 10}); got != gui.CursorCrosshair {
		t.Fatal("body lost configured cursor", got)
	}
	v.dragging, v.dragGutter = true, true
	if v.Cursor() != gui.CursorDefault {
		t.Fatal("gutter drag cursor")
	}
	v.dragGutter = false
	if v.Cursor() != gui.CursorCrosshair {
		t.Fatal("body drag lost configured cursor")
	}
	v.dragging = false
	ctx := v.context.(*shapedContext)
	v.dropLayouts()
	created := ctx.created
	_ = v.cursorAt(geometry.Point{X: v.gutter + 200, Y: button.Y + button.Height/2})
	if ctx.created != created {
		t.Fatal("pointer hit test shaped an unmeasured paragraph")
	}
}

func TestWordPointerAndDragUnits(t *testing.T) {
	v, _ := colorFixture(t)
	v.Document().Model().SetText("alpha beta\nlast\n")
	v.LayoutVisible(geometry.Size{Width: 400, Height: 200}, geometry.Point{})
	point := func(x float32) geometry.Point { return geometry.Point{X: v.gutter + padding + x, Y: padding + 10} }
	// Last half of the final 'a' still selects alpha, not the following space.
	if got := v.wordSelection(point(48)); got != (textedit.Range{Start: 0, End: 5}) {
		t.Fatal(got)
	}
	v.dragging, v.dragUnit, v.dragRange = true, 1, textedit.Range{Start: 0, End: 5}
	v.setSelection(textedit.Selection{Caret: 5}, false, false)
	v.dragSelection(point(48))
	if v.Selection() != (textedit.Selection{Caret: 5}) {
		t.Fatal("release shrank word", v.Selection())
	}
	v.dragSelection(point(85))
	if v.Selection() != (textedit.Selection{Caret: 10}) {
		t.Fatal("word drag", v.Selection())
	}
	v.dragRange = textedit.Range{Start: 6, End: 10}
	v.dragSelection(point(18))
	if v.Selection() != (textedit.Selection{Anchor: 10, Caret: 0}) {
		t.Fatal("backward word drag", v.Selection())
	}
	v.dragUnit, v.dragRange = 2, v.lineSelection(2)
	v.dragSelection(point(10))
	if v.Selection().Range() != (textedit.Range{Start: 0, End: 11}) {
		t.Fatal("line release lost LF", v.Selection())
	}
	if got := v.lineSelection(v.Document().Model().Len()); got.Start != got.End {
		t.Fatal("last empty line", got)
	}
}

// Exercise the navigation calculation with pure mock typography, not simulated
// platform input. Window key dispatch is covered by tests/window/editor/shortcuts.
func TestNavigationCancelsProjectionBeforeReadingGeometry(t *testing.T) {
	v := NewSourceView()
	v.document.model.SetText("abcdef")
	observe(t, v)
	v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
	v.context = &shapedContext{}
	t.Cleanup(v.dropLayouts)
	v.projection = textedit.NewProjection(v.document.model, v.Selection())
	v.projection.Update("p\nq", 1)
	resets := 0
	v.im = &resetContext{reset: func() { resets++ }}
	v.navigate(events.KeyEnd, false)
	if v.Selection() != (textedit.Selection{Anchor: 6, Caret: 6}) || v.projection != nil || resets != 1 {
		t.Fatal("navigation used temporary paragraph geometry", v.Selection(), resets)
	}
}

func TestNavigationKeepsStateChosenByResetCallback(t *testing.T) {
	v := NewSourceView()
	v.document.model.SetText("abcdef")
	observe(t, v)
	v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
	v.projection = textedit.NewProjection(v.document.model, v.Selection())
	v.projection.Update("p\nq", 1)
	d := NewDocument("keep")
	v.im = &resetContext{reset: func() { v.SetDocument(d); v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2}) }}
	v.navigate(events.KeyEnd, false)
	if v.Document() != d || v.Selection() != (textedit.Selection{Anchor: 2, Caret: 2}) {
		t.Fatal("reset callback state overwritten", v.Selection())
	}
}

func TestResetReentryCannotApplyOldEditRange(t *testing.T) {
	for _, switchDocument := range []bool{false, true} {
		t.Run(map[bool]string{false: "model edit", true: "document switch"}[switchDocument], func(t *testing.T) {
			v := NewSourceView()
			v.Document().Model().SetText("abc")
			observe(t, v)
			v.SetSelection(textedit.Selection{Caret: 2})
			old := v.Document()
			v.projection = textedit.NewProjection(old.model, v.Selection())
			v.projection.Update("preedit", 2)
			v.im = &resetContext{reset: func() {
				if switchDocument {
					v.SetDocument(NewDocument("new"))
				} else {
					_ = old.model.Replace(textedit.Range{Start: 3, End: 3}, "Z")
				}
			}}
			if err := v.ReplaceSelection("X"); !errors.Is(err, ErrOperationChanged) {
				t.Fatal("stale edit was accepted", err)
			}
			expected := "abcZ"
			if switchDocument {
				expected = "new"
			}
			if v.Document().model.Text() != expected {
				t.Fatal(v.Document().model.Text())
			}
			if switchDocument && len(old.views) != 0 {
				t.Fatal("old document retained view")
			}
		})
	}
}

func TestUnmountReleasesOriginalDocumentBeforeNativeReset(t *testing.T) {
	v := NewSourceView()
	observe(t, v)
	original := v.Document()
	v.projection = textedit.NewProjection(original.model, v.Selection())
	replacement := NewDocument("new")
	v.im = &resetContext{reset: func() { v.SetDocument(replacement) }}
	v.unmount()
	if original.connection != nil || original.engine != nil || len(original.views) != 0 || replacement.connection != nil {
		t.Fatal("unmount retained external subscription")
	}
}

func TestEditCallbackSelectionTakesPrecedence(t *testing.T) {
	v := NewSourceView()
	v.Document().model.SetText("abc")
	observe(t, v)
	v.SetSelection(textedit.Selection{Caret: 2})
	once := true
	v.ConnectChange(func(textedit.Change) {
		if once {
			once = false
			v.SetSelection(textedit.Selection{Caret: 1})
		}
	})
	if err := v.ReplaceSelection("X"); err != nil {
		t.Fatal(err)
	}
	if v.Selection() != (textedit.Selection{Caret: 1}) {
		t.Fatal("command overwrote callback selection", v.Selection())
	}
}

func TestProjectionOnlySuppressesAffectedLineHighlighting(t *testing.T) {
	v := NewSourceView()
	v.Document().model.SetText("zero\none\ntwo\nthree")
	v.projection = textedit.NewProjection(v.Document().model, textedit.Selection{Anchor: 5, Caret: 12})
	v.projection.Update("a\nb\nc", 2)
	// Two committed paragraphs become three display paragraphs.
	for row, expected := range []struct {
		row   int
		valid bool
	}{{0, true}, {0, false}, {0, false}, {0, false}, {3, true}} {
		actual, valid := v.highlightRow(row)
		if actual != expected.row || valid != expected.valid {
			t.Fatalf("display row %d maps to %d/%v", row, actual, valid)
		}
	}
}

func TestKeyBindingsOwnAllMapsAndSlices(t *testing.T) {
	a, b := NewSourceView(), NewSourceView()
	defaults := DefaultKeyBindings()
	epoch := a.keyBindingsEpoch
	if err := a.SetKeyBindings(a.KeyBindings()); err != nil || a.keyBindingsEpoch != epoch {
		t.Fatal("equal bindings invalidated active input", err)
	}
	if !reflect.DeepEqual(a.KeyBindings(), defaults) || len(defaults[CommandComplete]) != 0 {
		t.Fatal("incorrect defaults or implicit completion gesture")
	}
	for _, key := range []events.KeyEvent{{Key: events.KeySpace, Modifiers: events.ModifierControl}, {Key: events.KeyI, Modifiers: events.ModifierControl}} {
		if _, bound := a.keyCommand(key); bound {
			t.Fatal("reserved completion alternative became a default", key)
		}
	}
	copy := a.KeyBindings()
	copy[CommandCopy][0].Key = gui.KeyQ
	delete(copy, CommandUndo)
	if !reflect.DeepEqual(a.KeyBindings(), defaults) || !reflect.DeepEqual(b.KeyBindings(), defaults) {
		t.Fatal("reading bindings exposed mutable state")
	}
	if err := a.SetKeyBindings(copy); err != nil {
		t.Fatal(err)
	}
	copy[CommandCopy][0].Key = gui.KeyW
	if a.KeyBindings()[CommandCopy][0].Key != gui.KeyQ || !reflect.DeepEqual(b.KeyBindings(), defaults) {
		t.Fatal("setting bindings retained caller storage or changed another view")
	}
	if err := a.SetKeyBindings(nil); err != nil || len(a.KeyBindings()) != 0 {
		t.Fatal("nil did not clear all commands", err)
	}
	// The empty getter is still directly editable.
	copy = a.KeyBindings()
	copy[CommandComplete] = []gui.KeyGesture{{Key: gui.KeyF2}}
	if err := a.SetKeyBindings(copy); err != nil {
		t.Fatal(err)
	}
	if command, ok := a.keyCommand(events.KeyEvent{Key: events.KeyF2}); !ok || command != CommandComplete {
		t.Fatal("explicit completion not matched", command, ok)
	}
	if err := a.SetKeyBindings(DefaultKeyBindings()); err != nil || !reflect.DeepEqual(a.KeyBindings(), defaults) {
		t.Fatal("explicit reset failed", err)
	}
}

func TestKeyBindingsValidateAtomicallyAndResolveAliases(t *testing.T) {
	v := NewSourceView()
	before := v.KeyBindings()
	for _, bindings := range []KeyBindings{
		{0: nil}, {commandLimit: nil}, {Command(65535): nil},
		{CommandCopy: {{Key: gui.KeyUnknown}}},
		{CommandCopy: {{Key: gui.KeyF2, Modifiers: gui.KeyModifiers(1 << 15)}}},
		{CommandCopy: {{Key: gui.KeyControl}}},
		{CommandCopy: {{Key: gui.KeyEnter}}},
		{CommandCopy: {{Key: gui.KeyTab, Modifiers: gui.ModShift}}},
		{CommandCopy: {{Key: gui.KeyQ}}},
		{CommandCopy: {{Key: gui.KeyF2}}, CommandPaste: {{Key: gui.KeyF2}}},
		{CommandCopy: {{Key: gui.KeyC, Modifiers: gui.ModPrimary}}, CommandPaste: {{Key: gui.KeyC, Modifiers: primaryAlias()}}},
	} {
		if err := v.SetKeyBindings(bindings); err == nil || !reflect.DeepEqual(v.KeyBindings(), before) {
			t.Fatalf("invalid table accepted or changed state: %v, %v", bindings, err)
		}
	}
	if err := v.SetKeyBindings(KeyBindings{CommandCopy: {
		{Key: gui.KeyC, Modifiers: gui.ModPrimary},
		{Key: gui.KeyC, Modifiers: primaryAlias()},
		{Key: gui.KeyF2},
	}}); err != nil {
		t.Fatal(err)
	}
	if len(v.KeyBindings()[CommandCopy]) != 2 {
		t.Fatal("same-command aliases were not deduplicated")
	}
	if _, ok := v.keyCommand(events.KeyEvent{Key: events.KeyF2, Modifiers: events.ModifierShift}); ok {
		t.Fatal("extra modifiers matched a command")
	}
	if _, ok := v.keyCommand(events.KeyEvent{Key: events.KeyZ, Modifiers: events.ModifierControl}); ok {
		t.Fatal("replacing a table retained an old binding")
	}
}

func primaryAlias() gui.KeyModifiers {
	if runtime.GOOS == "darwin" {
		return gui.ModCommand
	}
	return gui.ModControl
}

func TestClearedCommandBindingsPreserveDirectOperations(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText("a\nb")
	observe(t, v)
	if err := v.SetKeyBindings(nil); err != nil {
		t.Fatal(err)
	}
	if err := v.MoveLines(true); err != nil || v.Document().Model().Text() != "b\na" {
		t.Fatal("clearing shortcuts disabled direct methods", err)
	}
	if !v.Undo() || v.Document().Model().Text() != "a\nb" {
		t.Fatal("clearing shortcuts disabled history")
	}
}

func TestLineKeyBindingsUseExactPlatformModifiers(t *testing.T) {
	v := NewSourceView()
	alt, _ := gui.ModAlt.Resolve()
	primary, _ := gui.ModPrimary.Resolve()
	duplicate := alt | events.ModifierShift
	if runtime.GOOS == "linux" {
		duplicate |= events.ModifierControl
	}
	for _, tt := range []struct {
		key  events.Key
		mods events.Modifiers
		want Command
	}{
		{events.KeyArrowUp, alt, CommandMoveLinesUp}, {events.KeyArrowDown, alt, CommandMoveLinesDown},
		{events.KeyArrowUp, duplicate, CommandDuplicateLinesUp}, {events.KeyArrowDown, duplicate, CommandDuplicateLinesDown},
		{events.KeyK, primary | events.ModifierShift, CommandDeleteLines},
	} {
		if got, ok := v.keyCommand(events.KeyEvent{Key: tt.key, Modifiers: tt.mods}); !ok || got != tt.want {
			t.Fatal("platform shortcut", tt, got, ok)
		}
		if _, ok := v.keyCommand(events.KeyEvent{Key: tt.key, Modifiers: tt.mods | events.ModifierAltGraph}); ok {
			t.Fatal("AltGraph was treated as an editor command")
		}
	}
	for _, event := range []events.KeyEvent{{Key: events.KeyArrowUp}, {Key: events.KeyK, Modifiers: primary}, {Key: events.KeyArrowLeft, Modifiers: alt}} {
		if command, ok := v.keyCommand(event); ok && command >= CommandMoveLinesUp && command <= CommandDeleteLines {
			t.Fatal("unrelated input consumed", event)
		}
	}
}

func TestMenuCommandsRespectStateAndHistory(t *testing.T) {
	v := NewSourceView()
	observe(t, v)
	v.Document().Model().SetText("abc")
	v.SetSelection(textedit.Selection{Anchor: 1, Caret: 2})
	if !v.canCommand(CommandDeleteSelection) || v.canCommand(CommandUndo) {
		t.Fatal("initial command state")
	}
	v.runCommand(CommandDeleteSelection)
	if v.Document().Model().Text() != "ac" || !v.canCommand(CommandUndo) {
		t.Fatal("delete command")
	}
	v.runCommand(CommandUndo)
	if v.Document().Model().Text() != "abc" || v.Selection() != (textedit.Selection{Anchor: 1, Caret: 2}) {
		t.Fatal("undo command")
	}
	v.SetReadOnly(true)
	for _, c := range []Command{CommandDeleteSelection, CommandCut, CommandPaste, CommandUndo, CommandRedo} {
		if v.canCommand(c) {
			t.Fatal("read-only command enabled", c)
		}
		v.runCommand(c)
	}
	v.runCommand(CommandSelectAll)
	if v.Selection() != (textedit.Selection{Caret: 3}) || v.Document().Model().Text() != "abc" {
		t.Fatal("read-only select all")
	}
}

func TestSelectionHitIncludesTrailingHalf(t *testing.T) {
	v, _ := colorFixture(t)
	v.SetSelection(textedit.Selection{Anchor: 1, Caret: 5})
	if !v.selectionContains(geometry.Point{X: v.gutter + padding + 48, Y: padding + 10}) {
		t.Fatal("selected last half missed")
	}
	if v.selectionContains(geometry.Point{X: v.gutter + padding + 51, Y: padding + 10}) {
		t.Fatal("unselected cluster hit")
	}
}

func TestLineMenuAvailabilityUsesLogicalSelectedRows(t *testing.T) {
	v := NewSourceView()
	observe(t, v)
	v.document.model.SetText("a\nb")
	v.SetSelection(textedit.Selection{Caret: 2}) // Ends at the next row's start.
	if v.canCommand(CommandMoveLinesUp) || !v.canCommand(CommandMoveLinesDown) || !v.canCommand(CommandDeleteLines) {
		t.Fatal("incorrect selected line boundary")
	}
	v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
	if !v.canCommand(CommandMoveLinesUp) || v.canCommand(CommandMoveLinesDown) {
		t.Fatal("last line availability")
	}
	v.SetReadOnly(true)
	for c := CommandMoveLinesUp; c <= CommandDeleteLines; c++ {
		if v.canCommand(c) {
			t.Fatal("read-only menu enabled", c)
		}
		v.runCommand(c)
	}
	if v.document.model.Text() != "a\nb" || v.document.model.CanUndo() {
		t.Fatal("read-only menu edited")
	}
	v.SetReadOnly(false)
	v.document.model.SetText("")
	if v.canCommand(CommandDeleteLines) || !v.canCommand(CommandDuplicateLinesUp) || !v.canCommand(CommandDuplicateLinesDown) {
		t.Fatal("empty document availability")
	}
}

// 每次前缀读取固定超过单步预算，可靠保留远处几何尚未准备好的状态。
// 只减慢纯内存 reader，不向生产代码加入测试开关，也不依赖机器速度。
func pendingInputFixture(t *testing.T, tail string) *SourceView {
	t.Helper()
	v := NewSourceView()
	v.Document().Model().SetText(strings.Repeat("x ", 1_000_000) + tail)
	v.SetFoldingEnabled(false)
	v.SetShowLineNumbers(false)
	v.SetAutomaticCompletion(false)
	observe(t, v)
	v.context = &boundedParagraphContext{t: t}
	v.LayoutVisible(geometry.Size{Width: 400, Height: 160}, geometry.Point{})
	p := v.paragraph(0)
	if p == nil || p.paged == nil {
		t.Fatal("missing long paragraph")
	}
	p.paged.Destroy()
	model := v.document.model
	format := typography.TextFormat{Font: v.font, WrapMode: typography.WrapNone, TextColor: v.colors.Foreground, TabWidth: 4 * v.spaceWidth}
	p.paged = gui.NewParagraphLayout(v.context, format, v.paragraphWidth(), model.Len(), func(r textedit.Range) (string, error) { time.Sleep(3 * time.Millisecond); return model.Slice(r) })
	p.styled = false
	t.Cleanup(v.dropLayouts)
	focusBracketFixture(t, v)
	v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyEnd})
	if v.Selection().Caret != model.Len() {
		t.Fatal("End not at logical end")
	}
	return v
}
func drainInput(t *testing.T, v *SourceView) {
	t.Helper()
	for i := 0; i < 256 && v.pendingInput != nil; i++ {
		v.inputStep(time.Now().Add(2 * time.Millisecond))
	}
	if v.pendingInput != nil {
		t.Fatal("deferred input never completed")
	}
}
func tailText(t *testing.T, v *SourceView, n int) string {
	t.Helper()
	m := v.Document().Model()
	text, err := m.Slice(textedit.Range{Start: m.Len() - n, End: m.Len()})
	if err != nil {
		t.Fatal(err)
	}
	return text
}

func TestDeferredNavigationTextAndHistoryStayOrdered(t *testing.T) {
	v := pendingInputFixture(t, "abc")
	m := v.Document().Model()
	originalLength, revision := m.Len(), m.Revision()
	handled := false
	v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyArrowLeft, Handled: &handled})
	if v.pendingInput == nil || v.Selection().Caret != originalLength {
		t.Fatal("unready navigation used another viewport", v.Selection())
	}
	v.commit(gui.IMCommit{Text: "Z"})
	v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyArrowLeft, Modifiers: events.ModifierShift})
	v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyBackspace})
	if m.Revision() != revision {
		t.Fatal("text committed before queued cursor movement")
	}
	drainInput(t, v)
	if tailText(t, v, 3) != "abc" || v.Selection().Caret != originalLength-1 || handled {
		t.Fatal("input order or native response lifetime", v.Selection(), tailText(t, v, 3), handled)
	}
	primary, _ := gui.ModPrimary.Resolve()
	v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyZ, Modifiers: primary})
	drainInput(t, v)
	if tailText(t, v, 4) != "abZc" || v.Selection().Range() != (textedit.Range{Start: originalLength - 1, End: originalLength}) {
		t.Fatal("undo selection/text", v.Selection(), tailText(t, v, 4))
	}
	v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyZ, Modifiers: primary | events.ModifierShift})
	drainInput(t, v)
	if tailText(t, v, 3) != "abc" || v.Selection().Caret != originalLength-1 {
		t.Fatal("redo after pending input")
	}
}

func TestDeferredDeleteUsesMeasuredCluster(t *testing.T) {
	v := pendingInputFixture(t, "e\u0301")
	length := v.Document().Model().Len()
	v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyBackspace})
	if v.pendingInput == nil || v.Document().Model().Len() != length {
		t.Fatal("unready deletion used stale geometry")
	}
	drainInput(t, v)
	if v.Document().Model().Len() != length-3 || tailText(t, v, 2) != "x " {
		t.Fatal("combining cluster split", tailText(t, v, 6))
	}
	if !v.Undo() || tailText(t, v, 3) != "e\u0301" {
		t.Fatal("cluster delete undo")
	}
}

func TestDeferredInputContextChangesCancelPendingText(t *testing.T) {
	for _, kind := range []string{"selection", "document", "edit", "readonly", "disabled", "bindings", "focus", "unmount", "escape", "hidden", "language"} {
		t.Run(kind, func(t *testing.T) {
			v := pendingInputFixture(t, "abc")
			model := v.Document().Model()
			v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyArrowLeft})
			v.commit(gui.IMCommit{Text: "Z"})
			if v.pendingInput == nil {
				t.Fatal("fixture not pending")
			}
			switch kind {
			case "selection":
				v.SetSelection(textedit.Selection{})
			case "document":
				v.SetDocument(NewDocument("replacement"))
			case "edit":
				_ = model.Replace(textedit.Range{}, "!")
			case "readonly":
				v.SetReadOnly(true)
			case "disabled":
				v.SetEnabled(false)
			case "bindings":
				_ = v.SetKeyBindings(nil)
			case "focus":
				var dispatcher gui.EventDispatcher
				host := &bracketHost{widget: v, focused: v}
				for _, focused := range []bool{true, false} {
					if !focused {
						host.SetFocusedWidget(nil)
					}
					if err := dispatcher.DispatchEvent(host, events.FocusEvent{Focused: focused}); err != nil {
						t.Fatal(err)
					}
				}
			case "unmount":
				v.unmount()
			case "escape":
				v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyEscape})
			case "hidden":
				v.SetVisible(false)
			case "language":
				if err := v.Document().SetLanguage("json"); err != nil {
					t.Fatal(err)
				}
			}
			expected := v.Document().Model().Text()
			v.inputStep(time.Time{})
			if v.pendingInput != nil {
				t.Fatal("obsolete input retained", kind)
			}
			if !v.Document().Model().EqualText(expected) {
				t.Fatal("stale text applied", kind)
			}
		})
	}
}

func TestDeferredInputReentrantSelectionCancelsRemainder(t *testing.T) {
	v := pendingInputFixture(t, "abc")
	v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyArrowLeft})
	v.commit(gui.IMCommit{Text: "Z"})
	once := false
	handle := v.ConnectSelection(func(textedit.Selection) {
		if !once {
			once = true
			v.SetSelection(textedit.Selection{})
		}
	})
	defer handle.Disconnect()
	drainInput(t, v)
	if !once || v.Selection().Caret != 0 || tailText(t, v, 3) != "abc" {
		t.Fatal("callback replacement lost", v.Selection(), tailText(t, v, 3))
	}
}

func TestDeferredCompositionFollowsNavigation(t *testing.T) {
	v := pendingInputFixture(t, "abc")
	originalLength := v.Document().Model().Len()
	v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyArrowLeft})
	v.preedit("中", 3)
	v.commit(gui.IMCommit{Text: "中文", Composed: true})
	drainInput(t, v)
	if v.projection != nil || tailText(t, v, 9) != "ab中文c" || v.Selection().Caret != originalLength+5 {
		t.Fatal("composition at stale position", v.Selection(), tailText(t, v, 9))
	}
	if !v.Undo() || tailText(t, v, 3) != "abc" {
		t.Fatal("composition was not one undo")
	}
}

// 覆盖层移除或首次进入时没有 PointerMove，仍应使用 crossing 提供的正文坐标。
func TestCursorUsesCrossingPositionBeforeFirstMotion(t *testing.T) {
	v := NewSourceView()
	v.Arrange(geometry.Rect(0, 0, 400, 160))
	host := &bracketHost{widget: v, focused: v}
	var dispatcher gui.EventDispatcher
	for _, event := range []events.PointerEvent{
		{EventType: events.PointerEnter, Position: geometry.Point{X: 150, Y: 20}},
		{EventType: events.PointerMove, Position: geometry.Point{X: 5, Y: 20}},
		{EventType: events.PointerLeave, Position: geometry.Point{X: 150, Y: 20}},
		{EventType: events.PointerEnter, Position: geometry.Point{X: 150, Y: 20}},
	} {
		if err := dispatcher.DispatchEvent(host, event); err != nil {
			t.Fatal(err)
		}
		if event.EventType == events.PointerLeave {
			continue
		}
		expected := gui.CursorText
		if event.Position.X < 10 {
			expected = gui.CursorDefault
		}
		if v.Cursor() != expected {
			t.Fatal("stale hover coordinates", event, v.Cursor(), expected)
		}
	}
}

func TestDeferredNewlineTabAndTypingStayOrdered(t *testing.T) {
	v := pendingInputFixture(t, "abc")
	v.SetAcceptsTab(true)
	indent := v.IndentOptions()
	indent.InsertSpaces = false
	if err := v.SetIndentOptions(indent); err != nil {
		t.Fatal(err)
	}
	v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyArrowLeft})
	v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyEnter})
	v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyTab})
	v.commit(gui.IMCommit{Text: "Z"})
	drainInput(t, v)
	if tailText(t, v, 6) != "ab\n\tZc" {
		t.Fatal("newline/tab discarded later input", tailText(t, v, 8))
	}
	for range 3 {
		if !v.Undo() {
			t.Fatal("missing undo")
		}
	}
	if tailText(t, v, 3) != "abc" {
		t.Fatal("newline/tab history", tailText(t, v, 8))
	}
}

// 模拟 RequestText 返回后、后续按键前才到达的异步回复。
type frameDelayedClipboard struct{ *fakeClipboard }

func (c *frameDelayedClipboard) RequestText(fn func(string, error)) {
	c.pending = fn
	time.Sleep(3 * time.Millisecond)
}

func TestDeferredPasteAndTypingStayOrdered(t *testing.T) {
	for _, immediate := range []bool{true, false} {
		t.Run(fmt.Sprint(immediate), func(t *testing.T) {
			v := pendingInputFixture(t, "abc")
			clip := &fakeClipboard{text: "P", immediate: immediate}
			var clipboard gui.Clipboard = clip
			if !immediate {
				clipboard = &frameDelayedClipboard{clip}
			}
			previous := gui.App
			gui.App = clipboardApplication{clipboard: clipboard}
			defer func() { gui.App = previous }()
			v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyArrowLeft})
			primary, _ := gui.ModPrimary.Resolve()
			v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyV, Modifiers: primary})
			v.commit(gui.IMCommit{Text: "Q"})
			if !immediate {
				for i := 0; i < 256 && clip.pending == nil; i++ {
					v.inputStep(time.Now().Add(2 * time.Millisecond))
				}
				if clip.pending == nil || v.pendingInput == nil || tailText(t, v, 3) != "abc" {
					t.Fatal("missing deterministic paste/typing interval")
				}
				clip.pending("P", nil)
			}
			drainInput(t, v)
			if tailText(t, v, 5) != "abPQc" {
				t.Fatal("paste lost later typing", tailText(t, v, 5))
			}
		})
	}
}

// 几何未就绪时设置在组合 cluster 内的合法 rune 偏移，编辑前仍须校正。
func TestDeferredInputNormalizesPreviouslyUnmeasuredSelection(t *testing.T) {
	v := pendingInputFixture(t, "e\u0301")
	length := v.Document().Model().Len()
	v.SetSelection(textedit.Selection{Anchor: length - 2, Caret: length - 2})
	v.commit(gui.IMCommit{Text: "Z"})
	drainInput(t, v)
	if tailText(t, v, 4) != "e\u0301Z" || v.Selection().Caret != length+1 {
		t.Fatal("insert split a shaping cluster", v.Selection(), tailText(t, v, 6))
	}
	if !v.Undo() || tailText(t, v, 3) != "e\u0301" {
		t.Fatal("normalized insertion undo")
	}
}

// 在原生窗口的起止附近重复相同编辑，位置预期直接来自原始模型 byte 范围。
func TestDeferredInputAcrossShapingWindows(t *testing.T) {
	for _, at := range []int{4095, 4096, 4097, 8192, 65536, 1_000_000} {
		t.Run(fmt.Sprint(at), func(t *testing.T) {
			v := pendingInputFixture(t, "abc")
			m := v.Document().Model()
			original := m.Text()
			v.SetSelection(textedit.Selection{Anchor: at, Caret: at})
			v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyArrowLeft})
			v.commit(gui.IMCommit{Text: "Z"})
			v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyArrowLeft, Modifiers: events.ModifierShift})
			v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyDelete})
			drainInput(t, v)
			if !m.EqualText(original) || v.Selection() != (textedit.Selection{Anchor: at - 1, Caret: at - 1}) {
				t.Fatal("window boundary changed edit position", at, v.Selection())
			}
			if !v.Undo() || !m.EqualText(original[:at-1]+"Z"+original[at-1:]) || v.Selection().Range() != (textedit.Range{Start: at - 1, End: at}) {
				t.Fatal("window boundary history", at, v.Selection())
			}
			if !v.Redo() || !m.EqualText(original) {
				t.Fatal("window boundary redo", at)
			}
		})
	}
}

func TestDeferredInputReentrantModelEditCancelsRemainder(t *testing.T) {
	v := pendingInputFixture(t, "abc")
	v.inputKey(events.KeyEvent{EventType: events.KeyDown, Key: events.KeyArrowLeft})
	v.commit(gui.IMCommit{Text: "Z"})
	v.commit(gui.IMCommit{Text: "Q"})
	changed := false
	h := v.ConnectChange(func(textedit.Change) {
		if !changed {
			changed = true
			m := v.Document().Model()
			if err := m.Replace(textedit.Range{Start: m.Len(), End: m.Len()}, "!"); err != nil {
				t.Fatal(err)
			}
		}
	})
	defer h.Disconnect()
	drainInput(t, v)
	if !changed || tailText(t, v, 5) != "abZc!" {
		t.Fatal("callback edit was overwritten by obsolete input", tailText(t, v, 8))
	}
}

// 第一行的宽度不能让第二行已经到尾部的拖选继续向右滚动。
func TestDragOutsideShortLineDoesNotScrollHorizontally(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText(strings.Repeat("x", 120) + "\nshort\n")
	v.SetFoldingEnabled(false)
	v.SetShowLineNumbers(false)
	observe(t, v)
	v.context = &shapedContext{}
	v.LayoutVisible(geometry.Size{Width: 400, Height: 100}, geometry.Point{})
	v.dragging = true
	v.setSelection(textedit.Selection{Anchor: 122, Caret: 122}, false, false)
	var targets []geometry.Rectangle
	h := v.ConnectScrollIntoView(func(r geometry.Rectangle) { targets = append(targets, r) })
	defer h.Disconnect()
	v.dragSelection(geometry.Point{X: 450, Y: padding + v.lineHeight + 5})
	if v.Selection().Range() != (textedit.Range{Start: 122, End: 126}) {
		t.Fatal("short line end", v.Selection())
	}
	for _, r := range targets {
		if float64(r.X)+float64(r.Width) > float64(v.viewport.Width) {
			t.Fatal("short line drag requested horizontal scrolling", r)
		}
	}
}

func TestDragAutoscrollKeepsAxesIndependent(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		x, y, offset                       float32
		row                                int
		wrap, gutter, horizontal, vertical bool
	}{
		{name: "long line right", x: 450, y: 10, horizontal: true},
		{name: "short line left", x: -30, y: 30, row: 1},
		{name: "short line hidden left", x: -30, y: 30, row: 1, offset: 300, horizontal: true},
		{name: "gutter drag right", x: 450, y: 10, gutter: true},
		{name: "wrapped right", x: 450, y: 10, wrap: true},
		{name: "vertical only", x: 450, y: 150, vertical: true},
		{name: "vertical at large x", x: 450, y: 150, offset: 1 << 28, vertical: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := NewSourceView()
			v.Document().Model().SetText(strings.Repeat("x", 120) + "\nshort\n")
			v.SetFoldingEnabled(false)
			v.SetShowLineNumbers(false)
			if tc.wrap {
				v.SetWrapMode(WrapWordChar)
			}
			observe(t, v)
			v.context = &shapedContext{}
			v.LayoutVisible(geometry.Size{Width: 400, Height: 100}, geometry.Point{X: tc.offset})
			v.dragging, v.dragGutter = true, tc.gutter
			r, _ := v.Document().Model().LineRange(tc.row)
			v.setSelection(textedit.Selection{Anchor: r.Start, Caret: r.Start}, false, false)
			var targets []geometry.Rectangle
			h := v.ConnectScrollIntoView(func(r geometry.Rectangle) { targets = append(targets, r) })
			defer h.Disconnect()
			v.dragSelection(geometry.Point{X: tc.x, Y: tc.y})
			if len(targets) == 0 {
				if tc.horizontal || tc.vertical {
					t.Fatal("missing scroll request")
				}
				return
			}
			if len(targets) != 1 {
				t.Fatal("duplicate scroll requests", targets)
			}
			got := targets[0]
			if !tc.horizontal && (got.X != tc.offset || got.Width != 0) {
				t.Fatal("unchanged x axis moved", got)
			}
			if tc.horizontal && (got.X == tc.offset && got.Width == 0) {
				t.Fatal("horizontal drag stopped", got)
			}
			if !tc.vertical && (got.Y != 0 || got.Height != 0) {
				t.Fatal("horizontal drag moved y axis", got)
			}
			if tc.vertical && got.Y+got.Height <= 100 {
				t.Fatal("vertical drag stopped", got)
			}
		})
	}
}

func TestDragScrollCallbackCannotApplyOldSelection(t *testing.T) {
	v := NewSourceView()
	v.Document().Model().SetText(strings.Repeat("x", 120))
	v.SetShowLineNumbers(false)
	v.SetFoldingEnabled(false)
	observe(t, v)
	v.context = &shapedContext{}
	v.LayoutVisible(geometry.Size{Width: 400, Height: 100}, geometry.Point{})
	v.dragging = true
	replacement := NewDocument("replacement")
	h := v.ConnectScrollIntoView(func(geometry.Rectangle) { v.SetDocument(replacement) })
	defer h.Disconnect()
	v.dragSelection(geometry.Point{X: 450, Y: 10})
	if v.Document() != replacement || v.Selection() != (textedit.Selection{}) {
		t.Fatal("applied selection after reentrant document replacement", v.Selection())
	}
}
