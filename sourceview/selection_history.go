package sourceview

import (
	"fmt"
	"slices"

	"github.com/golang-gui/goui/gui"
)

// Selection history belongs to a view and a single document revision. It is
// independent of text undo. Both snapshot count and total cursor count are bounded.
type cursorUndo struct {
	document *Document
	revision uint64
	steps    [][]cursorState
	count    int
	gesture  uint64
}

const maxCursorUndoStates = 16384

func equalCursorSelections(a, b []cursorState) bool {
	return slices.EqualFunc(a, b, func(a, b cursorState) bool {
		return a.selection == b.selection && a.upstream == b.upstream
	})
}

func (v *SourceView) rememberSelections(before, after []cursorState, gesture uint64) {
	h := &v.cursorUndo
	if h.document != v.document || h.revision != v.document.Revision() {
		*h = cursorUndo{document: v.document, revision: v.document.Revision()}
	}
	if v.selectionRevision != h.revision || v.projection != nil || equalCursorSelections(before, after) {
		return
	}
	if gesture != 0 && gesture == h.gesture {
		return
	}
	for len(h.steps) > 0 && (len(h.steps) >= 64 || h.count+len(before) > maxCursorUndoStates) {
		h.count -= len(h.steps[0])
		h.steps[0] = nil
		h.steps = h.steps[1:]
	}
	h.steps = append(h.steps, slices.Clone(before))
	h.count += len(before)
	h.gesture = gesture
}

// UndoSelection restores the previous complete selection set without changing
// text or text undo. One pointer drag is one step. Text changes, document changes
// and unmount discard selection history. Read-only views can restore selections.
// Pending input is cancelled; active preedit is left to the input method.
func (v *SourceView) UndoSelection() bool {
	if v.Destroyed() || !gui.IsEnabled(v) || v.projection != nil {
		return false
	}
	h := &v.cursorUndo
	if h.document != v.document || h.revision != v.document.Revision() {
		*h = cursorUndo{}
		return false
	}
	if len(h.steps) == 0 {
		return false
	}
	last := len(h.steps) - 1
	before := h.steps[last]
	h.steps[last] = nil
	h.steps = h.steps[:last]
	h.count -= len(before)
	h.gesture = 0
	v.pendingInput = nil
	v.document.model.BreakUndoGroup()
	v.setCursorStates(before, true, nil, 0, false)
	return true
}

// RemoveSelection removes one item from Selections. Removing the primary
// promotes the first remaining selection in document order. The final selection
// is retained. An invalid index returns an error without changing the set.
func (v *SourceView) RemoveSelection(index int) error {
	if v.Destroyed() || !gui.IsEnabled(v) {
		return ErrDisabled
	}
	before := v.cursors()
	if index < 0 || index >= len(before) {
		return fmt.Errorf("sourceview: invalid selection index %d", index)
	}
	if len(before) == 1 {
		return nil
	}
	state := v.operationState()
	v.cancelPreedit()
	if !state.valid(v) || v.Destroyed() {
		return ErrOperationChanged
	}
	v.pendingInput = nil
	v.document.model.BreakUndoGroup()
	after := append(before[:index:index], before[index+1:]...)
	v.setCursorStatesKeepingCompletion(after, true, nil)
	return nil
}
