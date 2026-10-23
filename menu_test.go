package sourceview

import (
	"testing"

	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
)

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
