package main

import (
	"runtime"
	"strings"

	source "github.com/golang-gui/goui-sourceview/sourceview"
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
)

// The example owns these presentation associations; the editor owns key input.
type menuActions struct {
	handles signal.Handles
	keys    []*gui.Shortcut
}

func (a *menuActions) append(menu *gui.Menu, label string, keys []gui.KeyGesture, enabled bool, run func()) {
	command := gui.NewCommand()
	command.SetEnabled(enabled)
	a.handles = append(a.handles, command.ConnectActivate(run))
	for _, gesture := range keys {
		key := gui.NewShortcut(gesture)
		key.SetCommand(command)
		a.keys = append(a.keys, key)
	}
	menu.Append(label, nil).SetCommand(command)
}

func (a *menuActions) clear() {
	a.handles.Disconnect()
	a.handles = nil
	for _, key := range a.keys {
		key.SetCommand(nil)
	}
	a.keys = nil
}

// Hints describe the editor's actual configuration; they do not register input.
func selectionHints(editor *source.SourceView) (string, string) {
	bindings := editor.KeyBindings()
	shortcut := func(command source.Command) string {
		if keys := bindings[command]; len(keys) > 0 {
			return keys[0].DisplayText()
		}
		return "unbound"
	}
	mods := editor.SelectionModifiers()
	var mouse []string
	if mods.Add != 0 {
		mouse = append(mouse, modifierHint(mods.Add)+"+click: add cursor")
	}
	if mods.Column != 0 {
		mouse = append(mouse, modifierHint(mods.Column)+"+drag: column")
	}
	mouse = append(mouse, shortcut(source.CommandSelectNextOccurrence)+": next", shortcut(source.CommandSelectAllOccurrences)+": all")
	keyboard := strings.Join([]string{
		"Undo selection: " + shortcut(source.CommandUndoSelection),
		"Skip current occurrence: " + shortcut(source.CommandSkipNextOccurrence),
		"Add-selection click on an existing cursor/range: remove it (last selection stays)",
		"Add cursor: " + shortcut(source.CommandAddCursorAbove) + " / " + shortcut(source.CommandAddCursorBelow),
		"Column horizontally: " + shortcut(source.CommandColumnSelectLeft) + " / " + shortcut(source.CommandColumnSelectRight),
		"Column vertically: " + shortcut(source.CommandColumnSelectUp) + " / " + shortcut(source.CommandColumnSelectDown),
		"Split into lines: " + shortcut(source.CommandSplitSelectionLines),
		"Esc: close transient UI, then keep the primary selection",
		shortcut(source.CommandComplete) + ": completion (single selection) · " + shortcut(source.CommandFindNext) + ": next find match",
	}, "\n")
	return strings.Join(mouse, "  ·  "), keyboard
}
func modifierHint(modifiers gui.KeyModifiers) string {
	var parts []string
	for _, entry := range []struct {
		modifier gui.KeyModifiers
		key      gui.Key
	}{
		{gui.ModPrimary, gui.KeyPrimary}, {gui.ModControl, gui.KeyControl}, {gui.ModAlt, gui.KeyAlt}, {gui.ModShift, gui.KeyShift},
	} {
		if modifiers&entry.modifier != 0 {
			parts = append(parts, (gui.KeyGesture{Key: entry.key}).DisplayText())
		}
	}
	separator := "+"
	if runtime.GOOS == "darwin" {
		separator = ""
	}
	return strings.Join(parts, separator)
}
