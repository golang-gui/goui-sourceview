package main

import (
	"testing"

	"github.com/golang-gui/goui/gui"
)

func TestMenuActionsKeepHintsSeparateAndReleaseAssociations(t *testing.T) {
	var actions menuActions
	menu := gui.NewMenu()
	calls := 0
	gesture := gui.KeyGesture{Key: gui.KeyF4, Modifiers: gui.ModShift}
	actions.append(menu, "Shrink selection", []gui.KeyGesture{gesture}, true, func() { calls++ })
	item := menu.ItemAt(0)
	command := item.Command()
	key := command.Shortcuts()[0]
	if item.Label() != "Shrink selection" || item.Action() != nil || key.Gesture() != gesture {
		t.Fatal("example menu did not separate its label, operation and hint")
	}
	command.Activate()
	actions.clear()
	command.Activate()
	if calls != 1 || key.Command() != nil || len(command.Shortcuts()) != 0 {
		t.Fatal("example retained an obsolete menu activation or shortcut")
	}
}
