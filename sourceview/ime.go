package sourceview

import (
	"github.com/golang-gui/goui-sourceview/internal/editor"
	textcore "github.com/golang-gui/goui-sourceview/internal/text"
	"github.com/golang-gui/goui/gui"
)

func (v *SourceView) cancelPreedit() {
	v.compositionSeen = false
	if v.projection == nil {
		return
	}
	v.projection = nil
	v.resumeFolding()
	v.invalidateLayout()
	if !v.resettingIM {
		v.resettingIM = true
		v.im.Reset()
		v.resettingIM = false
	}
}
func (v *SourceView) preedit(text string, caret int) {
	if v.resettingIM || !v.mounted || !v.Focused() || v.editable() != nil {
		return
	}
	v.cancelViewRestore()
	if v.delayInput(deferredInput{apply: func() { v.applyPreedit(text, caret) }, geometry: true}) {
		return
	}
	v.applyPreedit(text, caret)
}
func (v *SourceView) applyPreedit(text string, caret int) {
	v.selectionTrail.steps = nil
	v.cancelEdit()
	if v.resettingIM || !v.mounted || !v.Focused() || v.editable() != nil {
		return
	}
	if text == "" {
		v.projection = nil
		v.resumeFolding()
		v.invalidateLayout()
		return
	}
	v.ClearInlineSuggestion()
	v.closeCompletion()
	v.compositionSeen = true
	m := v.document.model
	if v.projection == nil || v.projectionRevision != m.Revision() {
		v.cancelPaste()
		line, _ := m.LineRange(m.LineAt(v.Selection().Caret))
		replacement := v.Selection().Range()
		line.Start, line.End = min(line.Start, replacement.Start), max(line.End, replacement.End)
		state := v.operationState()
		if v.expandFoldRange(line) {
			v.foldingSignal.Emit()
			if !state.valid(v) || v.Destroyed() || !v.mounted || v.editable() != nil {
				return
			}
		}
		v.projection = textcore.NewProjection(m, v.Selection())
		v.projectionRevision = m.Revision()
		m.BreakUndoGroup()
	}
	v.projection.Update(textcore.Normalize(text), caret)
	v.invalidateLayout()
	v.resetBlink()
	v.RevealSelection()
}
func (v *SourceView) commit(c gui.IMCommit) {
	if v.resettingIM || !v.mounted || !v.Focused() || v.editable() != nil {
		return
	}
	v.cancelViewRestore()
	action := deferredInput{apply: func() { v.applyCommit(c) }, geometry: true, mutates: true}
	action.typed = &c
	action.plan = func(s cursorState) (editor.Plan, error) { p, err := v.typingPlan(s, c); return p.plan, err }
	if !c.Composed && !v.compositionSeen && v.projection == nil {
		action.merge = textcore.MergeTyping
	}
	if v.delayInput(action) {
		return
	}
	v.applyCommit(c)
}
func (v *SourceView) applyCommit(c gui.IMCommit) {
	if v.resettingIM || !v.mounted || !v.Focused() || v.editable() != nil {
		return
	}
	if c.Text == "" {
		v.cancelPreedit()
		return
	}
	r := v.Selection().Range()
	atomic := c.Composed || v.compositionSeen
	v.compositionSeen = false
	if v.projection != nil {
		if v.projectionRevision != v.document.model.Revision() {
			v.cancelPreedit()
			return
		}
		r = v.projection.Replacement()
		v.projection = nil
		v.invalidateLayout()
		atomic = true
	}
	if len(v.additional) > 0 {
		c.Composed = atomic
		state := v.operationState()
		results := make([]plannedPair, 0, len(v.additional)+1)
		for _, cursor := range v.cursors() {
			p, err := v.typingPlan(cursor, c)
			if err == nil && !state.editingValid(v) {
				err = ErrOperationChanged
			}
			if err != nil {
				v.report(err)
				return
			}
			results = append(results, p)
		}
		merge := textcore.MergeNone
		if !atomic {
			merge = textcore.MergeTyping
		}
		v.report(v.applyPairPlans(results, merge))
		return
	}
	if !atomic {
		if handled, err := v.linkedTag(r, c.Text); handled {
			v.report(err)
			return
		}
		if handled, err := v.closeTag(r, c.Text); handled {
			v.report(err)
			return
		}
		state := v.operationState()
		if handled, err := v.pairText(r, c.Text); handled {
			v.report(err)
			if err == nil && v.automaticCompletion && v.mounted && v.editable() == nil && v.document == state.document && v.mountEpoch == state.mount && v.document.model.Revision() == state.revision+1 && v.Selection().Caret == r.Start+1 {
				v.report(v.completeTriggered(false, c.Text))
			}
			return
		}
	}
	state := v.operationState()
	continued, err := v.completionInput(r, textcore.Normalize(c.Text), atomic)
	v.report(err)
	end := r.Start + len(textcore.Normalize(c.Text))
	if err == nil && !continued && !atomic && v.automaticCompletion && v.mounted && v.editable() == nil && v.document == state.document && v.mountEpoch == state.mount && v.document.model.Revision() == state.revision+1 && v.Selection() == (TextSelection{Anchor: end, Caret: end}) {
		v.report(v.completeTriggered(false, c.Text))
	}
}
func (v *SourceView) displayCount() int {
	if v.projection != nil {
		return v.projection.LineCount(v.document.model)
	}
	return v.document.model.LineCount()
}
func (v *SourceView) displayRange(row int) TextRange {
	if v.projection != nil {
		return v.projection.LineRange(v.document.model, row)
	}
	r, _ := v.document.model.LineRange(row)
	return r
}
func (v *SourceView) displayText(row int) string {
	if v.projection != nil {
		return v.projection.Paragraph(v.document.model, row)
	}
	r := v.displayRange(row)
	s, _ := v.document.model.Slice(r)
	return s
}
func (v *SourceView) displayLine(offset int) int {
	if v.projection != nil {
		return v.projection.LineAt(v.document.model, offset)
	}
	return v.document.model.LineAt(offset)
}
func (v *SourceView) displayCaret() int {
	if v.projection != nil {
		return v.projection.Replacement().Start + v.projection.Caret()
	}
	return v.Selection().Caret
}

// Unchanged paragraphs keep committed highlighting during composition. The
// projected paragraphs have no committed lexical coordinates and use Plain.
func (v *SourceView) highlightRow(row int) (int, bool) {
	if v.projection == nil {
		return row, true
	}
	first := v.projection.FirstLine()
	end := first + v.projection.ParagraphCount()
	if row < first {
		return row, true
	}
	if row < end {
		return 0, false
	}
	delta := v.projection.ParagraphCount() - (v.projection.LastLine() - first + 1)
	return row - delta, true
}
