package sourceview

import (
	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui/gui"
	"github.com/golang-gui/goui/gui/textedit"
)

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
	at := editor.WordBoundary(v.document.model, v.Selection().Caret, forward, false)
	v.moveTo(at, false, extend)
}

// DeleteWord deletes the selection, a whitespace run or one code word.
// Each invocation has its own undo item. At a document edge it is a no-op.
func (v *SourceView) DeleteWord(forward bool) error {
	if err := v.editable(); err != nil {
		return err
	}
	if !forward {
		if handled, err := v.deletePair(); handled {
			return err
		}
	}
	s := v.Selection()
	r := s.Range()
	if r.Start == r.End {
		at := editor.WordBoundary(v.document.model, s.Caret, forward, true)
		if forward {
			r.End = v.snapOffset(at, true)
		} else {
			r.Start = v.snapOffset(at, false)
		}
	}
	if r.Start == r.End {
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
