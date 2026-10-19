package sourceview

import (
	"runtime"
	"testing"

	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
)

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
