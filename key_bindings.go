package sourceview

import (
	"fmt"
	"runtime"
	"slices"

	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
)

// Command identifies an editor operation, not a global command registration.
// Zero and unknown values are invalid. Bindings do not change an operation's
// editing, focus, preedit or lifecycle requirements.
type Command uint16

const (
	CommandUndo Command = iota + 1
	CommandRedo
	CommandCut
	CommandCopy
	CommandPaste
	CommandDeleteSelection
	CommandMoveLinesUp
	CommandMoveLinesDown
	CommandDuplicateLinesUp
	CommandDuplicateLinesDown
	CommandDeleteLines
	CommandSelectAll
	CommandFold
	CommandUnfold
	CommandFoldAll
	CommandUnfoldAll
	CommandComplete
	CommandToggleLineComment
	CommandToggleBlockComment
	CommandIndent
	CommandOutdent
	CommandInsertLineAbove
	CommandInsertLineBelow
	CommandSelectLine
	CommandJumpToBracket
	CommandMoveWordLeft
	CommandMoveWordRight
	CommandSelectWordLeft
	CommandSelectWordRight
	CommandDeleteWordLeft
	CommandDeleteWordRight
	CommandMoveLineStart
	CommandMoveLineEnd
	CommandSelectLineStart
	CommandSelectLineEnd
	CommandMoveDocumentStart
	CommandMoveDocumentEnd
	CommandSelectDocumentStart
	CommandSelectDocumentEnd
	CommandFindNext
	CommandFindPrevious
	// Find/Replace/GoTo are handled only by an attached companion bar.
	CommandFind
	CommandReplace
	CommandGoTo
	commandLimit
)

// KeyBindings maps editor commands to alternative single-key gestures.
// It covers command shortcuts, not text commits or transient popup navigation.
type KeyBindings map[Command][]gui.KeyGesture

// DefaultKeyBindings returns a fresh, editable table for the current platform.
// Explicit completion has no default gesture; applications may bind it.
func DefaultKeyBindings() KeyBindings {
	p := gui.ModPrimary
	word := p
	duplicate := gui.ModAlt | gui.ModShift
	fold := p | gui.ModShift
	if runtime.GOOS == "darwin" {
		word = gui.ModAlt
		fold = p | gui.ModAlt
	}
	if runtime.GOOS == "linux" {
		duplicate |= gui.ModControl
	}
	b := KeyBindings{
		CommandUndo:                {{Key: gui.KeyZ, Modifiers: p}},
		CommandRedo:                {{Key: gui.KeyZ, Modifiers: p | gui.ModShift}},
		CommandCut:                 {{Key: gui.KeyX, Modifiers: p}},
		CommandCopy:                {{Key: gui.KeyC, Modifiers: p}},
		CommandPaste:               {{Key: gui.KeyV, Modifiers: p}},
		CommandSelectAll:           {{Key: gui.KeyA, Modifiers: p}},
		CommandMoveLinesUp:         {{Key: gui.KeyArrowUp, Modifiers: gui.ModAlt}},
		CommandMoveLinesDown:       {{Key: gui.KeyArrowDown, Modifiers: gui.ModAlt}},
		CommandDuplicateLinesUp:    {{Key: gui.KeyArrowUp, Modifiers: duplicate}},
		CommandDuplicateLinesDown:  {{Key: gui.KeyArrowDown, Modifiers: duplicate}},
		CommandDeleteLines:         {{Key: gui.KeyK, Modifiers: p | gui.ModShift}},
		CommandFold:                {{Key: gui.KeyBracketLeft, Modifiers: fold}},
		CommandUnfold:              {{Key: gui.KeyBracketRight, Modifiers: fold}},
		CommandToggleLineComment:   {{Key: gui.KeySlash, Modifiers: p}},
		CommandIndent:              {{Key: gui.KeyBracketRight, Modifiers: p}},
		CommandOutdent:             {{Key: gui.KeyBracketLeft, Modifiers: p}},
		CommandInsertLineAbove:     {{Key: gui.KeyEnter, Modifiers: p | gui.ModShift}},
		CommandInsertLineBelow:     {{Key: gui.KeyEnter, Modifiers: p}},
		CommandSelectLine:          {{Key: gui.KeyL, Modifiers: p}},
		CommandJumpToBracket:       {{Key: gui.KeyBackslash, Modifiers: p | gui.ModShift}},
		CommandMoveWordLeft:        {{Key: gui.KeyArrowLeft, Modifiers: word}},
		CommandMoveWordRight:       {{Key: gui.KeyArrowRight, Modifiers: word}},
		CommandSelectWordLeft:      {{Key: gui.KeyArrowLeft, Modifiers: word | gui.ModShift}},
		CommandSelectWordRight:     {{Key: gui.KeyArrowRight, Modifiers: word | gui.ModShift}},
		CommandDeleteWordLeft:      {{Key: gui.KeyBackspace, Modifiers: word}},
		CommandDeleteWordRight:     {{Key: gui.KeyDelete, Modifiers: word}},
		CommandMoveDocumentStart:   {{Key: gui.KeyHome, Modifiers: p}},
		CommandMoveDocumentEnd:     {{Key: gui.KeyEnd, Modifiers: p}},
		CommandSelectDocumentStart: {{Key: gui.KeyHome, Modifiers: p | gui.ModShift}},
		CommandSelectDocumentEnd:   {{Key: gui.KeyEnd, Modifiers: p | gui.ModShift}},
		CommandFindNext:            {{Key: gui.KeyF3}},
		CommandFindPrevious:        {{Key: gui.KeyF3, Modifiers: gui.ModShift}},
		CommandFind:                {{Key: gui.KeyF, Modifiers: p}},
		CommandReplace:             {{Key: gui.KeyH, Modifiers: p}},
		CommandGoTo:                {{Key: gui.KeyG, Modifiers: gui.ModControl}},
	}
	if runtime.GOOS == "darwin" {
		b[CommandReplace] = []gui.KeyGesture{{Key: gui.KeyF, Modifiers: p | gui.ModAlt}}
		b[CommandMoveLineStart] = []gui.KeyGesture{{Key: gui.KeyArrowLeft, Modifiers: p}}
		b[CommandMoveLineEnd] = []gui.KeyGesture{{Key: gui.KeyArrowRight, Modifiers: p}}
		b[CommandSelectLineStart] = []gui.KeyGesture{{Key: gui.KeyArrowLeft, Modifiers: p | gui.ModShift}}
		b[CommandSelectLineEnd] = []gui.KeyGesture{{Key: gui.KeyArrowRight, Modifiers: p | gui.ModShift}}
		b[CommandMoveDocumentStart] = append(b[CommandMoveDocumentStart], gui.KeyGesture{Key: gui.KeyArrowUp, Modifiers: p})
		b[CommandMoveDocumentEnd] = append(b[CommandMoveDocumentEnd], gui.KeyGesture{Key: gui.KeyArrowDown, Modifiers: p})
		b[CommandSelectDocumentStart] = append(b[CommandSelectDocumentStart], gui.KeyGesture{Key: gui.KeyArrowUp, Modifiers: p | gui.ModShift})
		b[CommandSelectDocumentEnd] = append(b[CommandSelectDocumentEnd], gui.KeyGesture{Key: gui.KeyArrowDown, Modifiers: p | gui.ModShift})
		b[CommandFindNext] = append(b[CommandFindNext], gui.KeyGesture{Key: gui.KeyG, Modifiers: p})
		b[CommandFindPrevious] = append(b[CommandFindPrevious], gui.KeyGesture{Key: gui.KeyG, Modifiers: p | gui.ModShift})
	} else {
		b[CommandRedo] = append(b[CommandRedo], gui.KeyGesture{Key: gui.KeyY, Modifiers: p})
	}
	return b
}

// KeyBindings returns a mutable deep copy, including when no keys are bound.
func (v *SourceView) KeyBindings() KeyBindings {
	copy := make(KeyBindings, len(v.keyBindings))
	for command, keys := range v.keyBindings {
		copy[command] = slices.Clone(keys)
	}
	return copy
}

type boundKey struct {
	command Command
	gesture gui.KeyGesture
}

// SetKeyBindings validates and copies a complete replacement table. nil/empty
// removes all command shortcuts; absent commands have no fallback binding.
// Gestures require a non-Shift modifier or a function key. Unmodified/Shift-only
// text, navigation and editing keys belong to ordinary input and popup control.
// Invalid commands, unsupported gestures or conflicting commands return an
// error without changing the old table. Same-command aliases are deduplicated.
// It does not affect ordinary input, IME, menus or direct method calls.
func (v *SourceView) SetKeyBindings(bindings KeyBindings) error {
	if v.Destroyed() {
		return nil
	}
	table := make(KeyBindings, len(bindings))
	type resolvedKey struct {
		key  events.Key
		mods events.Modifiers
	}
	seen := make(map[resolvedKey]Command)
	// Stable command order makes errors and dispatch deterministic.
	commands := make([]int, 0, len(bindings))
	for command := range bindings {
		commands = append(commands, int(command))
	}
	slices.Sort(commands)
	var keys []boundKey
	for _, value := range commands {
		command := Command(value)
		if command == 0 || command >= commandLimit {
			return fmt.Errorf("sourceview: key bindings: unknown command %d", command)
		}
		for _, gesture := range bindings[command] {
			key, keyOK := gesture.Key.Resolve()
			mods, modsOK := gesture.Modifiers.Resolve()
			function := key >= events.KeyF1 && key <= events.KeyF24
			if !keyOK || !modsOK || modifierKey(key) || mods&^events.ModifierShift == 0 && !function {
				return fmt.Errorf("sourceview: key bindings: invalid gesture %+v for command %d", gesture, command)
			}
			resolved := resolvedKey{key, mods}
			if previous, exists := seen[resolved]; exists {
				if previous != command {
					return fmt.Errorf("sourceview: key bindings: gesture %+v conflicts between commands %d and %d", gesture, previous, command)
				}
				continue
			}
			seen[resolved] = command
			table[command] = append(table[command], gesture)
			keys = append(keys, boundKey{command, gesture})
		}
	}
	unchanged := len(v.keyBindings) == len(table)
	if unchanged {
		for command, gestures := range table {
			if !slices.Equal(v.keyBindings[command], gestures) {
				unchanged = false
				break
			}
		}
	}
	if unchanged {
		return nil
	}
	v.keyBindings, v.boundKeys = table, keys
	v.keyBindingsEpoch++
	return nil
}

func modifierKey(key events.Key) bool {
	switch key {
	case events.KeyShift, events.KeyControl, events.KeyAlt, events.KeyOption,
		events.KeyCommand, events.KeySuper, events.KeyWin, events.KeyAltGraph:
		return true
	}
	return false
}

func (v *SourceView) keyCommand(event events.KeyEvent) (Command, bool) {
	for _, binding := range v.boundKeys {
		if binding.gesture.Matches(event) {
			return binding.command, true
		}
	}
	return 0, false
}

// runBoundCommand executes core commands after transient completion handling.
// Internal companion commands remain available without a key binding.
func (v *SourceView) runBoundCommand(command Command) bool {
	run := func(fn func() error) {
		if v.editable() == nil {
			v.report(fn())
		}
	}
	switch command {
	case CommandFind:
		v.report(v.ShowFind())
	case CommandReplace:
		v.report(v.ShowReplace())
	case CommandGoTo:
		v.report(v.ShowGoTo())
	case CommandComplete:
		run(v.ShowCompletion)
	case CommandUndo:
		v.history(true)
	case CommandRedo:
		v.history(false)
	case CommandCopy:
		v.report(v.Copy())
	case CommandCut:
		run(v.Cut)
	case CommandPaste:
		run(v.Paste)
	case CommandDeleteSelection:
		run(func() error { return v.replace(v.Selection().Range(), "", true) })
	case CommandSelectAll:
		state := v.operationState()
		v.cancelPreedit()
		if state.valid(v) {
			v.document.model.BreakUndoGroup()
			v.setSelection(textedit.Selection{Caret: v.document.model.Len()}, false, true)
		}
	case CommandToggleLineComment:
		run(v.ToggleLineComment)
	case CommandToggleBlockComment:
		run(v.ToggleBlockComment)
	case CommandMoveLinesUp, CommandMoveLinesDown, CommandDuplicateLinesUp, CommandDuplicateLinesDown,
		CommandDeleteLines, CommandFold, CommandUnfold, CommandFoldAll, CommandUnfoldAll:
		v.runCommand(command)
	case CommandInsertLineAbove, CommandInsertLineBelow:
		run(func() error { return v.InsertLine(command == CommandInsertLineBelow) })
	case CommandIndent, CommandOutdent:
		run(func() error { return v.Indent(command == CommandOutdent) })
	case CommandSelectLine:
		v.selectLine()
	case CommandJumpToBracket:
		v.JumpToBracket()
	case CommandMoveWordLeft, CommandMoveWordRight, CommandSelectWordLeft, CommandSelectWordRight:
		v.MoveWord(command == CommandMoveWordRight || command == CommandSelectWordRight,
			command == CommandSelectWordLeft || command == CommandSelectWordRight)
	case CommandDeleteWordLeft, CommandDeleteWordRight:
		run(func() error { return v.DeleteWord(command == CommandDeleteWordRight) })
	case CommandMoveLineStart, CommandMoveLineEnd, CommandSelectLineStart, CommandSelectLineEnd:
		key := events.KeyHome
		if command == CommandMoveLineEnd || command == CommandSelectLineEnd {
			key = events.KeyEnd
		}
		v.navigate(key, command == CommandSelectLineStart || command == CommandSelectLineEnd)
	case CommandMoveDocumentStart, CommandMoveDocumentEnd, CommandSelectDocumentStart, CommandSelectDocumentEnd:
		at := 0
		if command == CommandMoveDocumentEnd || command == CommandSelectDocumentEnd {
			at = v.document.model.Len()
		}
		v.moveTo(at, false, command == CommandSelectDocumentStart || command == CommandSelectDocumentEnd)
	case CommandFindNext, CommandFindPrevious:
		v.FindNext(command == CommandFindPrevious)
	default:
		return false
	}
	return true
}
