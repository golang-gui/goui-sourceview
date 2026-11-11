package sourceview

import (
	"github.com/golang-gui/goui-sourceview/internal/editor"
	"github.com/golang-gui/goui/gui"
)

// MoveWordPart moves across camel-case, acronym, digit and underscore boundaries.
// Without extension, a nonempty selection collapses towards the requested side.
func (v *SourceView) MoveWordPart(forward, extend bool) {
	if len(v.additional) > 0 {
		return
	}
	if v.Destroyed() || !gui.IsEnabled(v) {
		return
	}
	s := v.Selection()
	at := s.Caret
	if !extend && s.Anchor != s.Caret {
		at = s.Range().Start
		if forward {
			at = s.Range().End
		}
	} else {
		at = editor.WordPartBoundary(v.document.model, at, forward, false, v.definitionAt(at).IsWord)
	}
	v.moveTo(v.foldNavigationTarget(at, forward), false, extend)
}

// DeleteWordPart deletes one component or the selection as one undo item.
func (v *SourceView) DeleteWordPart(forward bool) error {
	if len(v.additional) > 0 {
		return ErrMultipleSelections
	}
	if err := v.editable(); err != nil {
		return err
	}
	if v.unfoldDeleteBoundary(forward) {
		return nil
	}
	s := v.Selection()
	r := s.Range()
	if r.Start == r.End {
		at := editor.WordPartBoundary(v.document.model, s.Caret, forward, true, v.definitionAt(s.Caret).IsWord)
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
	return v.applyPlan(editor.Plan{Before: s, After: TextSelection{Anchor: r.Start, Caret: r.Start}, Edits: []TextEdit{{Range: r}}})
}

type selectionTrail struct {
	state operationState
	steps []TextSelection
}

// ExpandSelection selects the smallest confirmed enclosing word, literal,
// bracket/tag region, logical line or document. Analysis never changes a later selection.
func (v *SourceView) ExpandSelection() bool {
	if len(v.additional) > 0 {
		return false
	}
	if v.Destroyed() || !gui.IsEnabled(v) || v.projection != nil {
		return false
	}
	state := v.operationState()
	s := v.Selection()
	r := s.Range()
	m := v.document.model
	if !v.selectionTrail.state.valid(v) {
		v.selectionTrail.steps = nil
	}
	best := TextRange{Start: 0, End: m.Len()}
	found := best != r
	add := func(q TextRange) {
		if q.Start <= r.Start && q.End >= r.End && q != r && (!found || q.End-q.Start < best.End-best.Start) {
			best = q
			found = true
		}
	}
	add(editor.WordRange(m, r.Start, v.definitionAt(s.Caret).IsWord))
	a, _ := m.LineRange(m.LineAt(r.Start))
	b, _ := m.LineRange(m.LineAt(r.End))
	add(TextRange{Start: a.Start, End: b.End})
	add(TextRange{Start: a.Start, End: min(m.Len(), b.End+1)})
	v.structureCandidates(r, add)
	if !found || !state.valid(v) {
		return false
	}
	steps := append([]TextSelection(nil), v.selectionTrail.steps...)
	if len(steps) == 64 {
		steps = steps[1:]
	}
	steps = append(steps, s)
	next := TextSelection{Anchor: best.Start, Caret: best.End}
	if s.Anchor > s.Caret {
		next.Anchor, next.Caret = next.Caret, next.Anchor
	}
	v.document.model.BreakUndoGroup()
	v.setSelection(next, false, true)
	if v.document != state.document || v.mountEpoch != state.mount || v.document.model.Revision() != state.revision || v.selectionEpoch != state.selection+1 {
		return false
	}
	v.selectionTrail = selectionTrail{v.operationState(), steps}
	return true
}

// ShrinkSelection restores the preceding actual selection, including direction.
func (v *SourceView) ShrinkSelection() bool {
	if len(v.additional) > 0 {
		return false
	}
	t := v.selectionTrail
	if v.Destroyed() || !gui.IsEnabled(v) || v.projection != nil || !t.state.valid(v) || len(t.steps) == 0 {
		v.selectionTrail.steps = nil
		return false
	}
	s := t.steps[len(t.steps)-1]
	v.setSelection(s, false, true)
	if v.document != t.state.document || v.mountEpoch != t.state.mount || v.document.model.Revision() != t.state.revision || v.selectionEpoch != t.state.selection+1 {
		return false
	}
	v.selectionTrail = selectionTrail{v.operationState(), t.steps[:len(t.steps)-1]}
	return true
}

func (v *SourceView) structureCandidates(r TextRange, add func(TextRange)) {
	editor.StructureCandidates(v.document.model, r, v.definitionAt(r.Start).Editing(), func() editor.TokenReader {
		if e := v.document.engine; e != nil {
			return e.Tokens
		}
		return nil
	}, v.lexicalContext, add)
}
