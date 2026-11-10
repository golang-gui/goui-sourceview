package controls

import (
	"github.com/golang-gui/goui/core/signal"
	"github.com/golang-gui/goui/gui"
)

// These shortcuts describe the editor's existing bindings for menu hints.
// They are never registered with a controller: input still follows applyKey.
type MenuCommand struct {
	Command  *gui.Command
	activate signal.Handle
	keys     []*gui.Shortcut
}

func NewMenuCommand(gestures []gui.KeyGesture, run func()) *MenuCommand {
	b := &MenuCommand{Command: gui.NewCommand()}
	b.activate = b.Command.ConnectActivate(run)
	b.SetGestures(gestures)
	return b
}

func (b *MenuCommand) SetGestures(gestures []gui.KeyGesture) {
	for i, gesture := range gestures {
		if i < len(b.keys) {
			b.keys[i].SetGesture(gesture)
		} else {
			key := gui.NewShortcut(gesture)
			key.SetCommand(b.Command)
			b.keys = append(b.keys, key)
		}
	}
	for _, key := range b.keys[len(gestures):] {
		key.SetCommand(nil)
	}
	clear(b.keys[len(gestures):])
	b.keys = b.keys[:len(gestures)]
}

func (b *MenuCommand) Release() {
	b.activate.Disconnect()
	b.Command.SetEnabled(false)
	b.SetGestures(nil)
	b.keys = nil
}
