package sourceview

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/events"
)

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
