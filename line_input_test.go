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
	if v.canCommand(commandMoveLinesUp) || !v.canCommand(commandMoveLinesDown) || !v.canCommand(commandDeleteLines) {
		t.Fatal("incorrect selected line boundary")
	}
	v.SetSelection(textedit.Selection{Anchor: 2, Caret: 2})
	if !v.canCommand(commandMoveLinesUp) || v.canCommand(commandMoveLinesDown) {
		t.Fatal("last line availability")
	}
	v.SetReadOnly(true)
	for c := commandMoveLinesUp; c <= commandDeleteLines; c++ {
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
	if v.canCommand(commandDeleteLines) || !v.canCommand(commandDuplicateLinesUp) || !v.canCommand(commandDuplicateLinesDown) {
		t.Fatal("empty document availability")
	}
}
func TestLineKeyBindingsUseExactPlatformModifiers(t *testing.T) {
	alt, _ := gui.ModAlt.Resolve()
	primary, _ := gui.ModPrimary.Resolve()
	duplicate := alt | events.ModifierShift
	if runtime.GOOS == "linux" {
		duplicate |= events.ModifierControl
	}
	for _, tt := range []struct {
		key  events.Key
		mods events.Modifiers
		want editCommand
	}{
		{events.KeyArrowUp, alt, commandMoveLinesUp}, {events.KeyArrowDown, alt, commandMoveLinesDown},
		{events.KeyArrowUp, duplicate, commandDuplicateLinesUp}, {events.KeyArrowDown, duplicate, commandDuplicateLinesDown},
		{events.KeyK, primary | events.ModifierShift, commandDeleteLines},
	} {
		if got, ok := lineKeyCommand(events.KeyEvent{Key: tt.key, Modifiers: tt.mods}); !ok || got != tt.want {
			t.Fatal("platform shortcut", tt, got, ok)
		}
		if _, ok := lineKeyCommand(events.KeyEvent{Key: tt.key, Modifiers: tt.mods | events.ModifierAltGraph}); ok {
			t.Fatal("AltGraph was treated as an editor command")
		}
	}
	for _, event := range []events.KeyEvent{{Key: events.KeyArrowUp}, {Key: events.KeyK, Modifiers: primary}, {Key: events.KeyArrowLeft, Modifiers: alt}} {
		if _, ok := lineKeyCommand(event); ok {
			t.Fatal("unrelated input consumed", event)
		}
	}
}
