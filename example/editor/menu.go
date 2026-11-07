package main

import (
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
