package ui

import (
	source "github.com/golang-gui/goui-sourceview"
	"github.com/golang-gui/goui/gui"
	base "github.com/golang-gui/goui/ui"
)

// Declarative configuration uses the same types as the imperative control.
// Aliases preserve identity, methods and ownership; they add no parallel state.
type (
	CaseMode           = source.CaseMode
	Folds              = source.Folds
	FoldRange          = source.FoldRange
	Highlights         = source.Highlights
	HighlightSpan      = source.HighlightSpan
	Document           = source.Document
	IndentOptions      = source.IndentOptions
	MinimapOptions     = source.MinimapOptions
	WrapMode           = source.WrapMode
	WhitespaceMode     = source.WhitespaceMode
	SearchQuery        = source.SearchQuery
	FoldRegion         = source.FoldRegion
	Completion         = source.Completion
	CompletionContext  = source.CompletionContext
	CompletionProvider = source.CompletionProvider
	Command            = source.Command
	KeyBindings        = source.KeyBindings
	LineEnding         = source.LineEnding
	LineMarkID         = source.LineMarkID
	LineMark           = source.LineMark
	LineMarkInfo       = source.LineMarkInfo
	LineMarkKind       = source.LineMarkKind
	LineMarkState      = source.LineMarkState
	LineMarkAction     = source.LineMarkAction
	LineMarkEvent      = source.LineMarkEvent
)

// Text values share GOUI's UTF-8 byte coordinates and model contracts.
type (
	TextModel     = base.TextModel
	TextRange     = base.TextRange
	TextEdit      = base.TextEdit
	TextChange    = base.TextChange
	TextSelection = base.TextSelection
)

// KeyGesture uses GOUI's key and modifier values, also available in goui/ui.
type KeyGesture = gui.KeyGesture

const (
	CaseUpper    = source.CaseUpper
	CaseLower    = source.CaseLower
	WrapNone     = source.WrapNone
	WrapChar     = source.WrapChar
	WrapWordChar = source.WrapWordChar

	WhitespaceNone      = source.WhitespaceNone
	WhitespaceSelection = source.WhitespaceSelection
	WhitespaceTrailing  = source.WhitespaceTrailing
	WhitespaceAll       = source.WhitespaceAll
	WhitespaceBoundary  = source.WhitespaceBoundary

	LineEndingLF              = source.LineEndingLF
	LineEndingCRLF            = source.LineEndingCRLF
	LineEndingCR              = source.LineEndingCR
	MarkBreakpoint            = source.MarkBreakpoint
	MarkConditionalBreakpoint = source.MarkConditionalBreakpoint
	MarkLogpoint              = source.MarkLogpoint
	MarkBookmark              = source.MarkBookmark
	MarkEnabled               = source.MarkEnabled
	MarkDisabled              = source.MarkDisabled
	MarkPending               = source.MarkPending
	MarkRejected              = source.MarkRejected
	LineMarkToggle            = source.LineMarkToggle
	LineMarkToggleEnabled     = source.LineMarkToggleEnabled
	LineMarkContextMenu       = source.LineMarkContextMenu
)

// Command values are shared with SourceView; this package does not allocate IDs.
const (
	CommandMoveWordPartLeft       = source.CommandMoveWordPartLeft
	CommandMoveWordPartRight      = source.CommandMoveWordPartRight
	CommandSelectWordPartLeft     = source.CommandSelectWordPartLeft
	CommandSelectWordPartRight    = source.CommandSelectWordPartRight
	CommandDeleteWordPartLeft     = source.CommandDeleteWordPartLeft
	CommandDeleteWordPartRight    = source.CommandDeleteWordPartRight
	CommandExpandSelection        = source.CommandExpandSelection
	CommandShrinkSelection        = source.CommandShrinkSelection
	CommandReindentSelection      = source.CommandReindentSelection
	CommandPasteReindented        = source.CommandPasteReindented
	CommandJoinLines              = source.CommandJoinLines
	CommandUpperCase              = source.CommandUpperCase
	CommandLowerCase              = source.CommandLowerCase
	CommandTrimTrailingWhitespace = source.CommandTrimTrailingWhitespace
	CommandUndo                   = source.CommandUndo
	CommandRedo                   = source.CommandRedo
	CommandCut                    = source.CommandCut
	CommandCopy                   = source.CommandCopy
	CommandPaste                  = source.CommandPaste
	CommandDeleteSelection        = source.CommandDeleteSelection
	CommandMoveLinesUp            = source.CommandMoveLinesUp
	CommandMoveLinesDown          = source.CommandMoveLinesDown
	CommandDuplicateLinesUp       = source.CommandDuplicateLinesUp
	CommandDuplicateLinesDown     = source.CommandDuplicateLinesDown
	CommandDeleteLines            = source.CommandDeleteLines
	CommandSelectAll              = source.CommandSelectAll
	CommandFold                   = source.CommandFold
	CommandUnfold                 = source.CommandUnfold
	CommandFoldAll                = source.CommandFoldAll
	CommandUnfoldAll              = source.CommandUnfoldAll
	CommandCollapseLine           = source.CommandCollapseLine
	CommandExpandLine             = source.CommandExpandLine
	CommandComplete               = source.CommandComplete
	CommandToggleLineComment      = source.CommandToggleLineComment
	CommandToggleBlockComment     = source.CommandToggleBlockComment
	CommandIndent                 = source.CommandIndent
	CommandOutdent                = source.CommandOutdent
	CommandInsertLineAbove        = source.CommandInsertLineAbove
	CommandInsertLineBelow        = source.CommandInsertLineBelow
	CommandSelectLine             = source.CommandSelectLine
	CommandJumpToBracket          = source.CommandJumpToBracket
	CommandMoveWordLeft           = source.CommandMoveWordLeft
	CommandMoveWordRight          = source.CommandMoveWordRight
	CommandSelectWordLeft         = source.CommandSelectWordLeft
	CommandSelectWordRight        = source.CommandSelectWordRight
	CommandDeleteWordLeft         = source.CommandDeleteWordLeft
	CommandDeleteWordRight        = source.CommandDeleteWordRight
	CommandMoveLineStart          = source.CommandMoveLineStart
	CommandMoveLineEnd            = source.CommandMoveLineEnd
	CommandSelectLineStart        = source.CommandSelectLineStart
	CommandSelectLineEnd          = source.CommandSelectLineEnd
	CommandMoveDocumentStart      = source.CommandMoveDocumentStart
	CommandMoveDocumentEnd        = source.CommandMoveDocumentEnd
	CommandSelectDocumentStart    = source.CommandSelectDocumentStart
	CommandSelectDocumentEnd      = source.CommandSelectDocumentEnd
	CommandFindNext               = source.CommandFindNext
	CommandFindPrevious           = source.CommandFindPrevious
	CommandFind                   = source.CommandFind
	CommandReplace                = source.CommandReplace
	CommandGoTo                   = source.CommandGoTo
	CommandToggleLineMark         = source.CommandToggleLineMark
)

// NewDocument creates a document using the control's normalization and language defaults.
func NewDocument(text string) *Document { return source.NewDocument(text) }

// NewDocumentWithModel shares an existing GOUI text model.
func NewDocumentWithModel(model *TextModel) *Document { return source.NewDocumentWithModel(model) }

// DetectLineEnding inspects raw text before its newlines are normalized.
func DetectLineEnding(text string) (ending LineEnding, mixed bool) {
	return source.DetectLineEnding(text)
}

// DefaultKeyBindings returns a fresh editable table for the current platform.
func DefaultKeyBindings() KeyBindings { return source.DefaultKeyBindings() }

// LanguageCompletions returns candidates supplied by the effective language.
func LanguageCompletions(c CompletionContext) []Completion { return source.LanguageCompletions(c) }

// DocumentCompletions returns candidates found in the bounded text snapshot.
func DocumentCompletions(c CompletionContext) []Completion { return source.DocumentCompletions(c) }

// DefaultCompletions combines the control's built-in completion sources.
func DefaultCompletions(c CompletionContext) []Completion { return source.DefaultCompletions(c) }

// NewHighlights builds a worker-safe foreground snapshot; see sourceview.NewHighlights.
func NewHighlights(text string, spans []HighlightSpan) (*Highlights, error) {
	return source.NewHighlights(text, spans)
}

// NewFolds creates an immutable external folding snapshot.
func NewFolds(text string, ranges []FoldRange) (*Folds, error) { return source.NewFolds(text, ranges) }
