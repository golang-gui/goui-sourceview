package sourceview

import (
	"strings"

	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

func (v *SourceView) ReplaceSelection(text string) error {
	return v.replace(v.Selection().Range(), text, true)
}

func (v *SourceView) replace(r textedit.Range, text string, atomic bool) error {
	if err := v.editable(); err != nil {
		return err
	}
	text = textedit.Normalize(text)
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) {
		return ErrOperationChanged
	}
	if err := v.editable(); err != nil {
		return err
	}
	m, d := v.document.model, v.document
	revision := m.Revision()
	epoch := v.mountEpoch
	before := v.Selection()
	err := m.Apply(textedit.Edit{Range: r, Text: text}, textedit.EditOptions{Source: &v.source, Selection: &before, Atomic: atomic})
	if err == nil && v.document == d && v.mountEpoch == epoch && m.Revision() == revision+1 && v.selectionEpoch == state.selection {
		end := r.Start + len(text)
		v.setSelection(textedit.Selection{Anchor: end, Caret: end}, false, true)
	}
	return err
}

func (v *SourceView) Undo() bool { return v.history(true) }

func (v *SourceView) Redo() bool { return v.history(false) }

func (v *SourceView) history(undo bool) bool {
	if v.editable() != nil {
		return false
	}
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) || v.editable() != nil {
		return false
	}
	d := v.document
	epoch := v.mountEpoch
	var result textedit.HistoryResult
	if undo {
		result = d.model.UndoWithSelection()
	} else {
		result = d.model.RedoWithSelection()
	}
	if result.Applied && result.Selection != nil && v.document == d && v.mountEpoch == epoch && d.model.Revision() == result.Revision && v.selectionEpoch == state.selection {
		v.setSelection(*result.Selection, false, true)
	}
	return result.Applied
}

func (v *SourceView) applyPlan(p editor.Plan) error {
	if err := v.editable(); err != nil {
		return err
	}
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) {
		return ErrOperationChanged
	}
	if err := v.editable(); err != nil {
		return err
	}
	d := v.document
	revision := d.model.Revision()
	epoch := v.mountEpoch
	err := p.Apply(d.model, &v.source)
	if err == nil && len(p.Edits) > 0 && v.document == d && v.mountEpoch == epoch && d.model.Revision() == revision+1 && v.selectionEpoch == state.selection {
		v.setSelection(p.After, false, true)
	}
	return err
}

func (v *SourceView) Indent(outdent bool) error {
	p, err := editor.Indent(v.document.model, v.Selection(), editor.IndentOptions{TabWidth: v.indent.TabWidth, IndentWidth: v.indent.IndentWidth, InsertSpaces: v.indent.InsertSpaces}, outdent)
	if err != nil {
		return err
	}
	return v.applyPlan(p)
}

func (v *SourceView) tab(outdent bool) error {
	s := v.Selection()
	if outdent || s.Anchor != s.Caret {
		return v.Indent(outdent)
	}
	text := "\t"
	if v.indent.InsertSpaces {
		column := 0
		r, _ := v.document.model.LineRange(v.document.model.LineAt(s.Caret))
		r.End = s.Caret
		prefix, _ := v.document.model.Slice(r)
		for _, ch := range prefix {
			if ch == '\t' {
				column += v.indent.TabWidth - column%v.indent.TabWidth
			} else {
				column++
			}
		}
		text = strings.Repeat(" ", v.indent.TabWidth-column%v.indent.TabWidth)
	}
	return v.ReplaceSelection(text)
}

// ApplyEdits applies original-coordinate edits as one undo unit, with an
// explicit post-edit selection. Document identity and revision must match.
// All ranges and the selection are validated before mutation by textedit.
func (v *SourceView) ApplyEdits(document *Document, revision uint64, edits []textedit.Edit, after textedit.Selection) error {
	if document != v.document || revision != v.document.model.Revision() {
		return ErrOperationChanged
	}
	return v.applyPlan(editor.Plan{Edits: edits, Before: v.Selection(), After: after})
}

// InsertLine inserts an indented blank line above/below the caret's logical line.
// Existing text is preserved, including when there is a selection.
func (v *SourceView) InsertLine(below bool) error {
	if err := v.editable(); err != nil {
		return err
	}
	p, err := editor.InsertLine(v.document.model, v.Selection(), below)
	if err != nil {
		return err
	}
	return v.applyPlan(p)
}

// MoveWord moves to the previous code-word start or next code-word end.
func (v *SourceView) MoveWord(forward, extend bool) {
	if v.Destroyed() || !gui.IsEnabled(v) {
		return
	}
	at := editor.WordBoundaryWith(v.document.model, v.Selection().Caret, forward, false, v.definitionAt(v.Selection().Caret).IsWord)
	v.moveTo(v.foldNavigationTarget(at, forward), false, extend)
}

// DeleteWord deletes the selection, a whitespace run or one code word.
// Each invocation has its own undo item. At a document edge it is a no-op.
func (v *SourceView) DeleteWord(forward bool) error {
	if err := v.editable(); err != nil {
		return err
	}
	if v.unfoldDeleteBoundary(forward) {
		return nil
	}
	if !forward {
		if handled, err := v.deletePair(); handled {
			return err
		}
	}
	s := v.Selection()
	r := s.Range()
	if r.Start == r.End {
		at := editor.WordBoundaryWith(v.document.model, s.Caret, forward, true, v.definitionAt(v.Selection().Caret).IsWord)
		if forward {
			r.End = v.snapOffset(at, true)
		} else {
			r.Start = v.snapOffset(at, false)
		}
	}
	if r.Start == r.End {
		return nil
	}
	if s.Anchor == s.Caret && v.expandFoldRange(r) {
		v.foldingSignal.Emit()
		return nil
	}
	return v.applyPlan(editor.Plan{Before: s, After: textedit.Selection{Anchor: r.Start, Caret: r.Start}, Edits: []textedit.Edit{{Range: r}}})
}

// SelectLine expands to whole logical lines, including the following LF.
// Repeating the command extends the selection one more line downward.
func (v *SourceView) SelectLine() {
	if v.Destroyed() || !gui.IsEnabled(v) {
		return
	}
	r := v.Selection().Range()
	start, _ := v.document.model.LineRange(v.document.model.LineAt(r.Start))
	end := v.lineSelection(r.End)
	v.SetSelection(textedit.Selection{Anchor: start.Start, Caret: end.End})
}

// JumpToBracket jumps between a confirmed pair adjacent to the caret. Unknown,
// unmatched or out-of-budget pairs leave the selection unchanged.
func (v *SourceView) JumpToBracket() bool {
	if v.Destroyed() || !gui.IsEnabled(v) {
		return false
	}
	ranges := v.matchingBrackets()
	if len(ranges) != 2 {
		return false
	}
	v.moveTo(ranges[1].Start, false, false)
	return true
}

// MoveLines moves the current logical line or selected lines one row up/down.
// It preserves indentation and selection direction. A document edge is a no-op.
func (v *SourceView) MoveLines(down bool) error {
	if err := v.editable(); err != nil {
		return err
	}
	p, err := editor.MoveLines(v.document.model, v.Selection(), down)
	if err != nil {
		return err
	}
	return v.applyPlan(p)
}

// DuplicateLines copies the current logical line or selected lines above/below,
// retaining the selection in the new copy. The clipboard is not involved.
func (v *SourceView) DuplicateLines(down bool) error {
	if err := v.editable(); err != nil {
		return err
	}
	p, err := editor.DuplicateLines(v.document.model, v.Selection(), down)
	if err != nil {
		return err
	}
	return v.applyPlan(p)
}

// DeleteLines deletes the current logical line or selected lines, placing the
// caret at the deletion boundary. Deleting an empty document is a no-op.
func (v *SourceView) DeleteLines() error {
	if err := v.editable(); err != nil {
		return err
	}
	p, err := editor.DeleteLines(v.document.model, v.Selection())
	if err != nil {
		return err
	}
	return v.applyPlan(p)
}
