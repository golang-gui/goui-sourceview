package sourceview

import (
	"runtime"
	"slices"

	"github.com/golang-gui/goui-sourceview/internal/controls"
	"github.com/golang-gui/goui/gui"
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
	// Applications may bind F9; there is no default breakpoint binding.
	CommandToggleLineMark
	CommandMoveWordPartLeft
	CommandMoveWordPartRight
	CommandSelectWordPartLeft
	CommandSelectWordPartRight
	CommandDeleteWordPartLeft
	CommandDeleteWordPartRight
	CommandExpandSelection
	CommandShrinkSelection
	CommandReindentSelection
	CommandPasteReindented
	CommandJoinLines
	CommandUpperCase
	CommandLowerCase
	CommandTrimTrailingWhitespace
	// Long wrapped lines are a view operation, independent of language folding.
	CommandCollapseLine
	CommandExpandLine
	CommandAddCursorAbove
	CommandAddCursorBelow
	CommandSelectNextOccurrence
	CommandSelectAllOccurrences
	CommandSplitSelectionLines
	CommandColumnSelectLeft
	CommandColumnSelectRight
	CommandColumnSelectUp
	CommandColumnSelectDown
	CommandUndoSelection
	CommandSkipNextOccurrence
	CommandRemovePrimarySelection
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
		CommandUndoSelection:        {{Key: gui.KeyU, Modifiers: p}},
		CommandSkipNextOccurrence:   {{Key: gui.KeyD, Modifiers: p | gui.ModShift}},
		CommandAddCursorAbove:       {{Key: gui.KeyArrowUp, Modifiers: p | gui.ModAlt}},
		CommandAddCursorBelow:       {{Key: gui.KeyArrowDown, Modifiers: p | gui.ModAlt}},
		CommandSelectNextOccurrence: {{Key: gui.KeyD, Modifiers: p}},
		CommandSelectAllOccurrences: {{Key: gui.KeyL, Modifiers: p | gui.ModShift}},
		CommandSplitSelectionLines:  {{Key: gui.KeyI, Modifiers: gui.ModAlt | gui.ModShift}},
		CommandColumnSelectLeft:     {{Key: gui.KeyArrowLeft, Modifiers: p | gui.ModAlt | gui.ModShift}},
		CommandColumnSelectRight:    {{Key: gui.KeyArrowRight, Modifiers: p | gui.ModAlt | gui.ModShift}},
		CommandColumnSelectUp:       {{Key: gui.KeyArrowUp, Modifiers: gui.ModControl | gui.ModShift}},
		CommandColumnSelectDown:     {{Key: gui.KeyArrowDown, Modifiers: gui.ModControl | gui.ModShift}},

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

// SetKeyBindings validates and copies a complete replacement table. nil/empty
// removes all command shortcuts; absent commands have no fallback binding.
// Gestures require a non-Shift modifier or a function key. Unmodified/Shift-only
// text, navigation and editing keys belong to ordinary input and popup control.
// Invalid commands, unsupported gestures or conflicting commands return an
// error without changing the old table. Same-command aliases are deduplicated.
// Menu hints follow the configured bindings; ordinary input, IME, menu actions
// and direct method calls retain their existing behavior.
func (v *SourceView) SetKeyBindings(bindings KeyBindings) error {
	if v.Destroyed() {
		return nil
	}
	table, keys, err := controls.ValidateBindings[Command](bindings, commandLimit)
	if err != nil {
		return err
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
	v.keyBindings, v.boundKeys = KeyBindings(table), keys
	v.keyBindingsEpoch++
	v.updateMenuBindings()
	return nil
}

func (v *SourceView) keyCommand(event events.KeyEvent) (Command, bool) {
	for _, binding := range v.boundKeys {
		if binding.Gesture.Matches(event) {
			return binding.Command, true
		}
	}
	return 0, false
}

// runBoundCommand executes core commands after transient completion handling.
// Internal companion commands remain available without a key binding.
func (v *SourceView) runBoundCommand(command Command) bool {
	if len(v.additional) > 0 && !multipleCommandReady(command) {
		return true
	}
	run := func(fn func() error) {
		if v.editable() == nil {
			v.report(fn())
		}
	}
	switch command {
	case CommandUndoSelection:
		v.UndoSelection()
	case CommandRemovePrimarySelection:
		v.report(v.RemoveSelection(0))
	case CommandAddCursorAbove, CommandAddCursorBelow, CommandSelectNextOccurrence, CommandSkipNextOccurrence, CommandSelectAllOccurrences, CommandSplitSelectionLines, CommandColumnSelectLeft, CommandColumnSelectRight, CommandColumnSelectUp, CommandColumnSelectDown:
		v.report(v.queueSelectionCommand(command))
	case CommandMoveWordPartLeft, CommandMoveWordPartRight, CommandSelectWordPartLeft, CommandSelectWordPartRight:
		v.MoveWordPart(command == CommandMoveWordPartRight || command == CommandSelectWordPartRight, command == CommandSelectWordPartLeft || command == CommandSelectWordPartRight)
	case CommandDeleteWordPartLeft, CommandDeleteWordPartRight:
		run(func() error { return v.DeleteWordPart(command == CommandDeleteWordPartRight) })
	case CommandExpandSelection:
		v.ExpandSelection()
	case CommandShrinkSelection:
		v.ShrinkSelection()
	case CommandReindentSelection:
		run(v.ReindentSelection)
	case CommandPasteReindented:
		run(v.PasteReindented)
	case CommandJoinLines:
		run(v.JoinLines)
	case CommandUpperCase:
		run(func() error { return v.TransformCase(CaseUpper) })
	case CommandLowerCase:
		run(func() error { return v.TransformCase(CaseLower) })
	case CommandTrimTrailingWhitespace:
		run(v.TrimTrailingWhitespace)
	case CommandToggleLineMark:
		row := v.document.model.LineAt(v.Selection().Caret)
		v.requestLineMark(row, LineMarkToggle, v.lineMarkRect(row).Pos)
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
		run(func() error {
			if len(v.additional) > 0 {
				return v.replaceMultiple([]string{""})
			}
			return v.replace(v.Selection().Range(), "", true)
		})
	case CommandSelectAll:
		state := v.operationState()
		v.cancelPreedit()
		if state.valid(v) {
			v.document.model.BreakUndoGroup()
			v.setSelection(TextSelection{Caret: v.document.model.Len()}, false, true)
		}
	case CommandToggleLineComment:
		run(v.ToggleLineComment)
	case CommandToggleBlockComment:
		run(v.ToggleBlockComment)
	case CommandMoveLinesUp, CommandMoveLinesDown, CommandDuplicateLinesUp, CommandDuplicateLinesDown,
		CommandDeleteLines, CommandFold, CommandUnfold, CommandFoldAll, CommandUnfoldAll, CommandCollapseLine, CommandExpandLine:
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
