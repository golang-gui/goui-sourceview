package controls

import (
	"fmt"
	"slices"

	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/platform/events"
)

// BoundKey is a resolved ordering entry; it installs no event controller.
type BoundKey[C ~uint16] struct {
	Command C
	Gesture gui.KeyGesture
}

func ValidateBindings[C ~uint16](bindings map[C][]gui.KeyGesture, limit C) (map[C][]gui.KeyGesture, []BoundKey[C], error) {
	table := make(map[C][]gui.KeyGesture, len(bindings))
	type resolvedKey struct {
		key  events.Key
		mods events.Modifiers
	}
	seen := make(map[resolvedKey]C)
	// Stable command order makes errors and dispatch deterministic.
	commands := make([]int, 0, len(bindings))
	for command := range bindings {
		commands = append(commands, int(command))
	}
	slices.Sort(commands)
	var keys []BoundKey[C]
	for _, value := range commands {
		command := C(value)
		if command == 0 || command >= limit {
			return nil, nil, fmt.Errorf("sourceview: key bindings: unknown command %d", command)
		}
		for _, gesture := range bindings[command] {
			key, keyOK := gesture.Key.Resolve()
			mods, modsOK := gesture.Modifiers.Resolve()
			function := key >= events.KeyF1 && key <= events.KeyF24
			if !keyOK || !modsOK || modifierKey(key) || mods&^events.ModifierShift == 0 && !function {
				return nil, nil, fmt.Errorf("sourceview: key bindings: invalid gesture %+v for command %d", gesture, command)
			}
			resolved := resolvedKey{key, mods}
			if previous, exists := seen[resolved]; exists {
				if previous != command {
					return nil, nil, fmt.Errorf("sourceview: key bindings: gesture %+v conflicts between commands %d and %d", gesture, previous, command)
				}
				continue
			}
			seen[resolved] = command
			table[command] = append(table[command], gesture)
			keys = append(keys, BoundKey[C]{command, gesture})
		}
	}
	return table, keys, nil
}

func modifierKey(key events.Key) bool {
	switch key {
	case events.KeyShift, events.KeyControl, events.KeyAlt, events.KeyOption,
		events.KeyCommand, events.KeySuper, events.KeyWin, events.KeyAltGraph:
		return true
	}
	return false
}

// Text arrives separately via IMClient/CommitText. Do not dismiss on its
// preceding printable key, including Shift and layout-specific unknown keys.
func CompletionTextKey(e events.KeyEvent) bool {
	if e.Modifiers&^(events.ModifierShift|events.ModifierAltGraph) != 0 {
		return false
	}
	return e.Key == events.KeyUnknown || e.Key == events.KeyShift || e.Key == events.KeyAltGraph || e.Key == events.KeySpace || e.Key >= events.KeyA && e.Key <= events.KeyNumpadDecimal
}
