package sourceview

import (
	"slices"
	"testing"

	"github.com/golang-gui/goui/gui"
)

func TestMenuBindingsFollowValidatedEditorConfiguration(t *testing.T) {
	v, other := NewSourceView(), NewSourceView()
	controllers := len(v.EventControllers())
	calls := 0
	b := newMenuCommand(v.KeyBindings()[CommandUndo], func() { calls++ })
	v.menu = &editMenu{commands: map[Command]*menuCommand{CommandUndo: b}}
	t.Cleanup(v.menu.releaseCommands)
	first := b.command.Shortcuts()[0]
	old := first.Gesture()
	if err := v.SetKeyBindings(KeyBindings{CommandUndo: {{Key: gui.KeyF6}}, CommandRedo: {{Key: gui.KeyF6}}}); err == nil || first.Gesture() != old {
		t.Fatal("invalid configuration changed menu hints", err)
	}
	keys := []gui.KeyGesture{{Key: gui.KeyF6}, {Key: gui.KeyF7, Modifiers: gui.ModShift}}
	if err := v.SetKeyBindings(KeyBindings{CommandUndo: keys}); err != nil {
		t.Fatal(err)
	}
	shortcuts := b.command.Shortcuts()
	if len(shortcuts) != 2 || shortcuts[0] != first || shortcuts[0].Gesture() != keys[0] || shortcuts[1].Gesture() != keys[1] {
		t.Fatal("menu did not preserve shortcut association order", shortcuts)
	}
	if !slices.Equal(other.KeyBindings()[CommandUndo], []gui.KeyGesture{old}) || len(v.EventControllers()) != controllers {
		t.Fatal("presentation bindings affected another view or added an input controller")
	}
	if err := v.SetKeyBindings(nil); err != nil {
		t.Fatal(err)
	}
	if len(b.command.Shortcuts()) != 0 || shortcuts[0].Command() != nil || shortcuts[1].Command() != nil {
		t.Fatal("clearing bindings retained shortcut hints or reverse references")
	}
	b.command.Activate()
	if calls != 1 {
		t.Fatal("removing shortcuts disabled the independent menu operation")
	}
}

func TestMenuReleaseCancelsOldActionsAndDetachesShortcuts(t *testing.T) {
	calls := 0
	b := newMenuCommand([]gui.KeyGesture{{Key: gui.KeyF6}}, func() { calls++ })
	key := b.command.Shortcuts()[0]
	m := &editMenu{commands: map[Command]*menuCommand{CommandUndo: b}}
	m.releaseCommands()
	b.command.SetEnabled(true) // A retained old entry must have no action.
	b.command.Activate()
	if calls != 0 || key.Command() != nil || len(b.command.Shortcuts()) != 0 || m.commands != nil {
		t.Fatal("released menu retained an action or shortcut association")
	}
	m.releaseCommands()
}
