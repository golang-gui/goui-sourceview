package sourceview

import (
	"github.com/golang-gui/goui/gui/textedit"
	"github.com/golang-gui/goui/platform/events"
)

// runBoundCommand executes core commands after transient completion handling.
// Companion commands return false; only their mounted owner consumes them.
func (v *SourceView) runBoundCommand(command Command) bool {
	run := func(fn func() error) {
		if v.editable() == nil {
			v.report(fn())
		}
	}
	switch command {
	case CommandFind, CommandReplace, CommandGoTo:
		return false
	case CommandComplete:
		run(v.ShowCompletion)
	case CommandUndo:
		v.Undo()
	case CommandRedo:
		v.Redo()
	case CommandCopy:
		v.report(v.Copy())
	case CommandCut:
		run(v.Cut)
	case CommandPaste:
		run(v.Paste)
	case CommandDeleteSelection:
		run(func() error { return v.ReplaceSelection("") })
	case CommandSelectAll:
		state := v.operationState()
		v.cancelPreedit()
		if state.valid(v) {
			v.SetSelection(textedit.Selection{Caret: v.document.model.Len()})
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
		v.SelectLine()
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
