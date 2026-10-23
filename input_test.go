package sourceview

import (
	"errors"
	"reflect"
	"runtime"
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
)

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
