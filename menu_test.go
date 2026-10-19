package sourceview

import (
	"github.com/golang-gui/goui/core/geometry"
	"github.com/golang-gui/goui/gui/textedit"
	"testing"
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
